package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
)

type ExpirationWorker struct {
	paymentRepo *postgres.PaymentRepository
	bankClient  application.BankClient
	interval    time.Duration
	logger      *slog.Logger
}

func NewExpirationWorker(
	paymentRepo *postgres.PaymentRepository,
	bankClient application.BankClient,
	interval time.Duration,
	logger *slog.Logger,
) *ExpirationWorker {
	return &ExpirationWorker{
		paymentRepo: paymentRepo,
		bankClient:  bankClient,
		interval:    interval,
		logger:      logger,
	}
}

func (w *ExpirationWorker) Start(ctx context.Context) {
	w.logger.Info("expiration worker started", "interval", w.interval)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.processExpirations(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("expiration worker stopping")
			return
		case <-ticker.C:
			if err := w.processExpirations(ctx); err != nil {
				w.logger.Error("expiration processing failed", "error", err)
			}
		}
	}
}

func (w *ExpirationWorker) processExpirations(ctx context.Context) error {
	cutoffTime := time.Now().Add(-8 * 24 * time.Hour)

	expiredPayments, err := w.paymentRepo.FindExpiredAuthorizations(ctx, cutoffTime, 100)
	if err != nil {
		return err
	}

	if len(expiredPayments) == 0 {
		return nil
	}

	var processed, expired int

	for _, payment := range expiredPayments {
		if err := w.checkAndMarkExpired(ctx, payment); err != nil {
			w.logger.Error("failed to process expiration",
				"payment_id", payment.ID(),
				"error", err)
		} else {
			expired++
		}
		processed++
	}

	w.logger.Info("processed expiration check",
		"processed", processed,
		"marked_expired", expired)

	return nil
}

func (w *ExpirationWorker) checkAndMarkExpired(ctx context.Context, payment *domain.Payment) error {
	bankAuth, err := w.bankClient.GetAuthorization(ctx, *payment.BankAuthID())

	if err != nil {
		if bankErr, ok := application.IsBankError(err); ok {
			if bankErr.Code == "authorization_expired" {
				return w.markAsExpired(ctx, payment)
			}
		}

		return err
	}

	if bankAuth.Status == "AUTHORIZED" {
		w.logger.Warn("payment still active at bank despite age",
			"payment_id", payment.ID(),
			"bank_auth_id", *payment.BankAuthID(),
			"authorized_at", payment.AuthorizedAt())
		return nil
	}

	return w.markAsExpired(ctx, payment)
}

func (w *ExpirationWorker) markAsExpired(ctx context.Context, payment *domain.Payment) error {
	if err := payment.MarkExpired(); err != nil {
		return err
	}

	return w.paymentRepo.Update(ctx, nil, payment)
}
