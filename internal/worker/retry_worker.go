package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
)

type RetryWorker struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	bankClient      bank.BankClient
	interval        time.Duration
	batchSize       int
	maxRetries      int32
	maxBackoff      int32
	db              *postgres.DB
	logger          *slog.Logger
}

func NewRetryWorker(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	bankClient bank.BankClient,
	db *postgres.DB,
	interval time.Duration,
	batchSize int,
	maxRetries int32,
	maxBackoff int32,
	logger *slog.Logger,
) *RetryWorker {
	return &RetryWorker{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
		interval:        interval,
		batchSize:       batchSize,
		maxRetries:      maxRetries,
		maxBackoff:      maxBackoff,
		db:              db,
		logger:          logger,
	}
}

func (w *RetryWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.ProcessRetries(ctx); err != nil {
				w.logger.Error("retry processing failed", "error", err)
			}

			if err := w.timeoutUnauthorizedPayments(ctx); err != nil {
				w.logger.Error("timeout failed", "error", err)
			}
		}
	}
}

type stuckPayment struct {
	id             string
	status         string
	idempotencyKey string
}

func (w *RetryWorker) ProcessRetries(ctx context.Context) error {
	query := `
		SELECT p.id, p.status, i.key
		FROM payments p
		JOIN idempotency_keys i on p.id = i.payment_id
		WHERE
			p.status IN ('CAPTURING', 'VOIDING', 'REFUNDING')
			AND (
				p.next_retry_at IS NULL OR p.next_retry_at <= NOW()
			)
			AND p.attempt_count < $1
			AND i.locked_at < NOW() - $2::interval
		ORDER BY p.created_at ASC
		LIMIT $3
	`

	rows, err := w.db.Query(ctx, query, w.maxRetries, w.interval, w.batchSize)
	if err != nil {
		return fmt.Errorf("query stuck payments: %w", err)
	}
	defer rows.Close()

	var processed int
	for rows.Next() {
		var sp stuckPayment
		if err := rows.Scan(&sp.id, &sp.status, &sp.idempotencyKey); err != nil {
			w.logger.Error("scan failed", "error", err)
			continue
		}

		if err := w.retryPayment(ctx, sp); err != nil {
			w.logger.Error("retry failed",
				"payment_id", sp.id,
				"status", sp.status,
				"error", err)
		} else {
			processed++
		}
	}

	if processed > 0 {
		w.logger.Info("processed stuck payments", "count", processed)
	}

	return rows.Err()
}

func (w *RetryWorker) timeoutUnauthorizedPayments(ctx context.Context) error {
	query := `
        SELECT p.id, p.order_id, i.key, p.created_at
        FROM payments p
        JOIN idempotency_keys i ON p.id = i.payment_id
        WHERE 
            p.status = 'PENDING'
            AND p.created_at < NOW() - INTERVAL '10 minutes'
            AND i.locked_at IS NOT NULL
    `

	rows, err := w.db.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, orderID, key string
		var createdAt time.Time
		if err := rows.Scan(&id, &orderID, &key, &createdAt); err != nil {
			w.logger.Error("scan failed", "error", err)
			continue
		}

		payment, err := w.paymentRepo.FindByID(ctx, id)
		if err != nil {
			continue
		}

		if err := payment.Fail(); err != nil {
			w.logger.Error("failed to mark payment as failed", "error", err)
		}
		if err := w.paymentRepo.Update(ctx, nil, payment); err != nil {
			return err
		}

		w.logger.Error("ORPHANED_AUTHORIZATION_RISK",
			"payment_id", id,
			"order_id", orderID,
			"age_minutes", time.Since(createdAt).Minutes(),
			"action", "MANUAL_RECONCILIATION_REQUIRED")

	}

	return nil
}

func (w *RetryWorker) retryPayment(ctx context.Context, sp stuckPayment) error {
	payment, err := w.paymentRepo.FindByID(ctx, sp.id)
	if err != nil {
		return err
	}

	switch domain.PaymentStatus(sp.status) {
	case domain.StatusCapturing:
		return w.resumeCapture(ctx, payment, sp.idempotencyKey)
	case domain.StatusVoiding:
		return w.resumeVoid(ctx, payment, sp.idempotencyKey)
	case domain.StatusRefunding:
		return w.resumeRefund(ctx, payment, sp.idempotencyKey)
	case domain.StatusPending, domain.StatusAuthorized, domain.StatusCaptured, domain.StatusFailed, domain.StatusRefunded, domain.StatusVoided, domain.StatusExpired:
		return fmt.Errorf("unexpected status %s for retry: %w", sp.status, domain.ErrInvalidState)
	default:
		return fmt.Errorf("unexpected status %s: %w", sp.status, domain.ErrInvalidState)
	}
}

func (w *RetryWorker) resumeCapture(ctx context.Context, payment *domain.Payment, idempotencyKey string) error {
	return w.resumeOperation(
		ctx,
		payment,
		idempotencyKey,
		func(ctx context.Context, key string) (any, error) {
			req := bank.CaptureRequest{
				Amount:          payment.AmountCents,
				AuthorizationID: *payment.BankAuthID,
			}
			return w.bankClient.Capture(ctx, req, key)
		},
		func(p *domain.Payment, resp any) error {
			r, ok := resp.(*bank.CaptureResponse)
			if !ok {
				return fmt.Errorf("expected *bank.CaptureResponse, got %T", resp)
			}
			return p.Capture(r.Status, r.CaptureID, r.CapturedAt)
		},
	)
}

func (w *RetryWorker) resumeVoid(ctx context.Context, payment *domain.Payment, idempotencyKey string) error {
	return w.resumeOperation(
		ctx,
		payment,
		idempotencyKey,
		func(ctx context.Context, key string) (any, error) {
			req := bank.VoidRequest{
				AuthorizationID: *payment.BankAuthID,
			}
			return w.bankClient.Void(ctx, req, key)
		},
		func(p *domain.Payment, resp any) error {
			r, ok := resp.(*bank.VoidResponse)
			if !ok {
				return fmt.Errorf("expected *bank.VoidResponse, got %T", resp)
			}
			return p.Void(r.Status, r.VoidID, r.VoidedAt)
		},
	)

}

func (w *RetryWorker) resumeRefund(ctx context.Context, payment *domain.Payment, idempotencyKey string) error {
	return w.resumeOperation(
		ctx,
		payment,
		idempotencyKey,
		func(ctx context.Context, key string) (any, error) {
			req := bank.RefundRequest{
				Amount:    payment.AmountCents,
				CaptureID: *payment.BankCaptureID,
			}
			return w.bankClient.Refund(ctx, req, key)
		},
		func(p *domain.Payment, resp any) error {
			r, ok := resp.(*bank.RefundResponse)
			if !ok {
				return fmt.Errorf("expected *bank.RefundResponse, got %T", resp)
			}
			return p.Refund(r.RefundID, r.RefundedAt)
		},
	)
}
