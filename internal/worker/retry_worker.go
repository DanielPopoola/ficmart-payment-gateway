package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
)

type RetryWorker struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	bankClient      application.BankClient
	interval        time.Duration
	batchSize       int
	db              *postgres.DB
	logger          *slog.Logger
}

func NewRetryWorker(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	bankClient application.BankClient,
	db *postgres.DB,
	interval time.Duration,
	batchSize int,
	logger *slog.Logger,
) *RetryWorker {
	return &RetryWorker{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
		interval:        interval,
		batchSize:       batchSize,
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
			AND p.attempt_count < 5
			AND i.locked_at < NOW() - $1::interval
		ORDER BY p.created_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`

	rows, err := w.db.Query(ctx, query, w.interval, w.batchSize)
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
		FOR UPDATE
    `

	rows, err := w.db.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, orderID, key string
		var createdAt time.Time
		rows.Scan(&id, &orderID, &key, &createdAt)

		payment, err := w.paymentRepo.FindByID(ctx, id)
		if err != nil {
			continue
		}

		payment.Fail()
		w.paymentRepo.Update(ctx, nil, payment)

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
	default:
		return fmt.Errorf("unexpected status %s: %w", sp.status, domain.ErrInvalidState)
	}
}

func (w *RetryWorker) resumeCapture(ctx context.Context, payment *domain.Payment, idempotencyKey string) error {
	captureReq := application.BankCaptureRequest{
		Amount:          payment.AmountCents,
		AuthorizationID: *payment.BankAuthID,
	}

	resp, err := w.bankClient.Capture(ctx, captureReq, idempotencyKey)
	if err != nil {
		category := application.CategorizeError(err)
		w.logger.Error("capture retry failed",
			"payment_id", payment.ID,
			"category", category,
			"error", err)

		if category == application.CategoryPermanent {
			if failErr := payment.Fail(); failErr != nil {
				return failErr
			}

			tx, err := w.db.Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx)

			if updateErr := w.paymentRepo.Update(ctx, tx, payment); updateErr != nil {
				return updateErr
			}

			responsePayload, _ := json.Marshal(err)
			if storeErr := w.idempotencyRepo.StoreResponse(ctx, tx, idempotencyKey, responsePayload); storeErr != nil {
				return storeErr
			}

			if err := tx.Commit(ctx); err != nil {
				return err
			}

			return err
		}
		return w.scheduleRetry(ctx, payment, err)
	}

	tx, err := w.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err = payment.Capture(resp.CaptureID, resp.CapturedAt); err != nil {
		return err
	}

	if updateErr := w.paymentRepo.Update(ctx, tx, payment); updateErr != nil {
		return updateErr
	}

	responsePayload, _ := json.Marshal(resp)
	if err := w.idempotencyRepo.StoreResponse(ctx, tx, idempotencyKey, responsePayload); err != nil {
		return err
	}

	if err := w.idempotencyRepo.ReleaseLock(ctx, tx, idempotencyKey); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (w *RetryWorker) resumeVoid(ctx context.Context, payment *domain.Payment, idempotencyKey string) error {
	voidReq := application.BankVoidRequest{
		AuthorizationID: *payment.BankAuthID,
	}

	resp, err := w.bankClient.Void(ctx, voidReq, idempotencyKey)
	if err != nil {
		category := application.CategorizeError(err)
		w.logger.Error("void retry failed",
			"payment_id", payment.ID,
			"category", category,
			"error", err)

		if category == application.CategoryPermanent {
			if failErr := payment.Fail(); failErr != nil {
				return failErr
			}

			tx, err := w.db.Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx)

			if updateErr := w.paymentRepo.Update(ctx, tx, payment); updateErr != nil {
				return updateErr
			}

			responsePayload, _ := json.Marshal(err)
			if storeErr := w.idempotencyRepo.StoreResponse(ctx, tx, idempotencyKey, responsePayload); storeErr != nil {
				return storeErr
			}

			if err := tx.Commit(ctx); err != nil {
				return err
			}

			return err
		}
		return w.scheduleRetry(ctx, payment, err)
	}
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err = payment.Void(resp.VoidID, resp.VoidedAt); err != nil {
		return err
	}

	if updateErr := w.paymentRepo.Update(ctx, tx, payment); updateErr != nil {
		return updateErr
	}

	responsePayload, _ := json.Marshal(resp)
	if err := w.idempotencyRepo.StoreResponse(ctx, tx, idempotencyKey, responsePayload); err != nil {
		return err
	}

	if err := w.idempotencyRepo.ReleaseLock(ctx, tx, idempotencyKey); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (w *RetryWorker) resumeRefund(ctx context.Context, payment *domain.Payment, idempotencyKey string) error {
	refundReq := application.BankRefundRequest{
		Amount:    payment.AmountCents,
		CaptureID: *payment.BankCaptureID,
	}

	resp, err := w.bankClient.Refund(ctx, refundReq, idempotencyKey)
	if err != nil {
		category := application.CategorizeError(err)
		w.logger.Error("refund retry failed",
			"payment_id", payment.ID,
			"category", category,
			"error", err)

		if category == application.CategoryPermanent {
			if failErr := payment.Fail(); failErr != nil {
				return failErr
			}

			tx, err := w.db.Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx)

			if updateErr := w.paymentRepo.Update(ctx, tx, payment); updateErr != nil {
				return updateErr
			}

			responsePayload, _ := json.Marshal(err)
			if storeErr := w.idempotencyRepo.StoreResponse(ctx, tx, idempotencyKey, responsePayload); storeErr != nil {
				return storeErr
			}

			if err := tx.Commit(ctx); err != nil {
				return err
			}

			return err
		}
		return w.scheduleRetry(ctx, payment, err)
	}

	tx, err := w.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err = payment.Refund(resp.RefundID, resp.RefundedAt); err != nil {
		return err
	}

	if updateErr := w.paymentRepo.Update(ctx, tx, payment); updateErr != nil {
		return updateErr
	}

	responsePayload, _ := json.Marshal(resp)
	if err := w.idempotencyRepo.StoreResponse(ctx, tx, idempotencyKey, responsePayload); err != nil {
		return err
	}

	if err := w.idempotencyRepo.ReleaseLock(ctx, tx, idempotencyKey); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (w *RetryWorker) scheduleRetry(ctx context.Context, payment *domain.Payment, lastErr error) error {
	category := application.CategorizeError(lastErr)

	payment.ScheduleRetry(
		time.Duration(1<<payment.AttemptCount)*time.Minute,
		string(category),
	)
	return w.paymentRepo.Update(ctx, nil, payment)
}
