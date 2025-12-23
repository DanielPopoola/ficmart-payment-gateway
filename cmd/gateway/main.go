package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/adapters/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/adapters/handler"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/adapters/postgres"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/config"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/service"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/worker"
)

func main() {
	// 1. Setup Logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// 2. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// 3. Connect to Database
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Connect(ctx, &cfg.Database, logger)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// 4. Initialize Repository
	repo := postgres.NewPaymentRepository(db)

	// 5. Initialize Bank Client
	baseBankClient := bank.NewBankClient(cfg.BankClient)
	bankClient := bank.NewRetryBankClient(baseBankClient, cfg.Retry)

	// 6. Initialize Services
	authService := service.NewAuthorizationService(repo, bankClient)
	capService := service.NewCaptureService(repo, bankClient)
	voidService := service.NewVoidService(repo, bankClient)
	refService := service.NewRefundService(repo, bankClient)
	queryService := service.NewPaymentQueryService(repo)

	// 7. Initialize Reconciler Worker
	reconciler := worker.NewReconciler(
		repo,
		bankClient,
		authService,
		capService,
		voidService,
		refService,
		cfg.Worker.Interval,
		cfg.Worker.BatchSize,
		logger,
	)

	// 8. Start Reconciler
	go reconciler.Start(ctx)

	// 9. Initialize HTTP Handler
	h := handler.NewPaymentHandler(
		authService,
		capService,
		refService,
		voidService,
		queryService,
	)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// 10. Start Server
	go func() {
		logger.Info("starting server", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 11. Wait for Shutdown
	<-ctx.Done()
	logger.Info("shutting down gracefully")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server forced shutdown", "error", err)
	}

	logger.Info("exit")
}
