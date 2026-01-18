package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
	"github.com/jackc/pgx/v5"
)

func ComputeHash(v any) string {
	data := fmt.Sprintf("%+v", v)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// checkIdempotency checks if request was already processed and returns cached result if available
func checkIdempotency(
	ctx context.Context,
	idempotencyRepo *postgres.IdempotencyRepository,
	paymentRepo *postgres.PaymentRepository,
	idempotencyKey string,
	requestHash string,
) (*domain.Payment, bool, error) {
	existingKey, err := idempotencyRepo.FindByKey(ctx, idempotencyKey)
	if err != nil {
		return nil, false, application.NewInternalError(err)
	}

	if existingKey == nil {
		return nil, false, nil
	}

	if existingKey.RequestHash != requestHash {
		return nil, false, application.NewIdempotencyMismatchError()
	}

	if existingKey.LockedAt != nil {
		payment, err := paymentRepo.FindByID(ctx, existingKey.PaymentID)
		if err != nil {
			return nil, false, application.NewInternalError(err)
		}
		return payment, true, nil
	}

	return nil, false, nil
}

// waitForCompletion polls for operation completion when another request is processing the same idempotency key
func waitForCompletion(
	ctx context.Context,
	idempotencyRepo *postgres.IdempotencyRepository,
	paymentRepo *postgres.PaymentRepository,
	idempotencyKey string,
) (*domain.Payment, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, application.NewTimeoutError()
		case <-timeout:
			return nil, application.NewTimeoutError()
		case <-ticker.C:
			key, err := idempotencyRepo.FindByKey(ctx, idempotencyKey)
			if err != nil {
				continue
			}

			if key.LockedAt == nil {
				payment, err := paymentRepo.FindByID(ctx, key.PaymentID)
				if err != nil {
					return nil, application.NewInternalError(err)
				}
				return payment, nil
			}

			if time.Since(*key.LockedAt) > 5*time.Minute {
				return nil, application.NewRequestProcessingError()
			}
		}
	}
}

// acquireIdempotencyLock creates payment and locks idempotency key in a single transaction
func acquireIdempotencyLock(
	ctx context.Context,
	db *postgres.DB,
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	payment *domain.Payment,
	idempotencyKey string,
	requestHash string,
) error {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return application.NewInternalError(err)
	}
	defer func() {
		_ = tx.Rollback(ctx) //nolint:errcheck // rollback error is not critical in defer
	}()

	if err := paymentRepo.Create(ctx, tx, payment); err != nil {
		return application.NewInternalError(err)
	}

	if err := idempotencyRepo.AcquireLock(ctx, tx, idempotencyKey, payment.ID, requestHash); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return application.NewInternalError(err)
	}

	return nil
}

// markPaymentTransitioning updates payment to intermediate state (CAPTURING, VOIDING, etc.)
func markPaymentTransitioning(
	ctx context.Context,
	db *postgres.DB,
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	paymentID string,
	idempotencyKey string,
	requestHash string,
	transitionFn func(*domain.Payment) error,
) (*domain.Payment, error) {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, application.NewInternalError(err)
	}
	defer func() {
		_ = tx.Rollback(ctx) //nolint:errcheck // rollback error is not critical in defer
	}()

	if err = idempotencyRepo.AcquireLock(ctx, tx, idempotencyKey, paymentID, requestHash); err != nil {
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			return nil, err
		}
		return nil, application.NewInternalError(err)
	}

	payment, err := paymentRepo.FindByIDForUpdate(ctx, tx, paymentID)
	if err != nil {
		return nil, application.NewInternalError(err)
	}

	if err = transitionFn(payment); err != nil {
		return nil, application.NewInvalidStateError(err)
	}

	if err = paymentRepo.Update(ctx, tx, payment); err != nil {
		return nil, application.NewInternalError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, application.NewInternalError(err)
	}

	return payment, nil
}

// HandleBankFailure handles permanent bank errors by marking payment as failed
func HandleBankFailure(
	ctx context.Context,
	db *postgres.DB,
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	payment *domain.Payment,
	idempotencyKey string,
	bankErr error,
) error {
	category := application.CategorizeError(bankErr)
	if category != application.CategoryPermanent {
		return bankErr
	}

	if err := payment.Fail(); err != nil {
		return application.NewInvalidStateError(err)
	}

	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return application.NewInternalError(err)
	}
	defer func() {
		_ = tx.Rollback(ctx) //nolint:errcheck // rollback error is not critical in defer
	}()

	if err = paymentRepo.Update(ctx, tx, payment); err != nil {
		return application.NewInternalError(err)
	}

	responsePayload, err := json.Marshal(bankErr)
	if err != nil {
		return application.NewInternalError(err)
	}

	if err = idempotencyRepo.StoreResponse(ctx, tx, idempotencyKey, responsePayload); err != nil {
		return application.NewInternalError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return application.NewInternalError(err)
	}

	return bankErr
}

// FinalizePayment stores successful bank response and releases lock
func FinalizePayment(
	ctx context.Context,
	db *postgres.DB,
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	payment *domain.Payment,
	idempotencyKey string,
	bankResponse any,
) error {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return application.NewInternalError(err)
	}
	defer func() {
		_ = tx.Rollback(ctx) //nolint:errcheck // rollback error is not critical in defer
	}()

	if err = paymentRepo.Update(ctx, tx, payment); err != nil {
		return application.NewInternalError(err)
	}

	responsePayload, err := json.Marshal(bankResponse)
	if err != nil {
		return application.NewInternalError(err)
	}

	if err = idempotencyRepo.StoreResponse(ctx, tx, idempotencyKey, responsePayload); err != nil {
		return application.NewInternalError(err)
	}

	if err = idempotencyRepo.ReleaseLock(ctx, tx, idempotencyKey); err != nil {
		return application.NewInternalError(err)
	}

	if err = tx.Commit(ctx); err != nil {
		return application.NewInternalError(err)
	}

	return nil
}
