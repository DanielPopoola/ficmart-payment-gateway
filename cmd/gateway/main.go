package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/config"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/interfaces/rest/handlers"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/interfaces/rest/middleware"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/worker"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger := cfg.Logger.NewLogger()
	slog.SetDefault(logger)

	logger.Info("starting gateway service",
		"port", cfg.Server.Port,
		"log_level", cfg.Logger.Level,
	)

	ctx := context.Background()
	db, err := postgres.Connect(ctx, &cfg.Database, logger)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	paymentRepo := postgres.NewPaymentRepository(db)
	idempotencyRepo := postgres.NewIdempotencyRepository(db)

	bankClient := bank.NewBankClient(cfg.BankClient)
	retryBankClient := bank.NewRetryBankClient(bankClient, cfg.Retry)

	authService := services.NewAuthorizeService(paymentRepo, idempotencyRepo, retryBankClient, db)
	captureService := services.NewCaptureService(paymentRepo, idempotencyRepo, retryBankClient, db)
	voidService := services.NewVoidService(paymentRepo, idempotencyRepo, retryBankClient, db)
	refundService := services.NewRefundService(paymentRepo, idempotencyRepo, retryBankClient, db)
	queryService := services.NewQueryService(paymentRepo)

	h := handlers.NewHandlers(
		authService,
		captureService,
		voidService,
		refundService,
		queryService,
		logger,
	)

	strictHandler := api.NewStrictHandler(h, nil)

	mux := http.NewServeMux()
	api.RegisterDocsRoutes(mux)
	api.HandlerFromMux(strictHandler, mux)

	router := http.Handler(mux)

	handler := middleware.Recovery(logger)(router)
	handler = middleware.Logging(logger)(handler)
	handler = middleware.Timeout(cfg.Server.ReadTimeout)(handler)

	server := &http.Server{
		Addr:         "0.0.0.0:" + cfg.Server.Port,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	retryWorker := worker.NewRetryWorker(
		paymentRepo,
		idempotencyRepo,
		retryBankClient,
		db,
		cfg.Worker.Interval,
		cfg.Worker.BatchSize,
		logger,
	)

	expirationWorker := worker.NewExpirationWorker(
		paymentRepo,
		retryBankClient,
		cfg.Worker.Interval,
		logger,
	)

	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()

	go retryWorker.Start(workerCtx)
	go expirationWorker.Start(workerCtx)

	go func() {
		logger.Info("server starting", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	cancelWorkers()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}

	logger.Info("server exited")
}
