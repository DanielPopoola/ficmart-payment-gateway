package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

type idempotencyRepository struct {
	db *DB
}

func NewIdempotencyRepository(db *DB) application.IdempotencyRepository {
	return &idempotencyRepository{db: db}
}

func (r *idempotencyRepository) AcquireLock(ctx context.Context, key string, paymentID string, requestHash string) error {
	query := `
		INSERT INTO idempotency_keys (key, payment_id, request_hash, locked_at)
		VALUES ($1, $2, $3, $4)
	`

	_, err := r.db.Pool.Exec(ctx, query, key, paymentID, requestHash, time.Now())
	if err != nil {
		if IsUniqueViolation(err) {
			// Key exists - check if request hash matches
			var existingHash string
			checkQuery := `SELECT request_hash FROM idempotency_keys WHERE key = $1`
			err = r.db.Pool.QueryRow(ctx, checkQuery, key).Scan(&existingHash)
			if err != nil {
				return fmt.Errorf("failed to check idempotency key: %w", err)
			}

			if existingHash != requestHash {
				return domain.ErrDuplicateIdempotencyKey
			}

			return nil
		}
		return fmt.Errorf("failed to acquire idempotency lock: %w", err)
	}

	return nil
}

func (r *idempotencyRepository) FindByKey(ctx context.Context, key string) (*domain.Payment, error) {
	query := `
        SELECT p.id, p.order_id, p.customer_id, p.amount_cents, p.currency, p.status,
               p.bank_auth_id, p.bank_capture_id, p.bank_void_id, p.bank_refund_id,
               p.created_at, p.authorized_at, p.captured_at, p.voided_at, p.refunded_at, p.expires_at
        FROM payments p
        JOIN idempotency_keys i ON p.id = i.payment_id
        WHERE i.key = $1
    `

	row := r.db.Pool.QueryRow(ctx, query, key)
	return scanPayment(row)
}

func (r *idempotencyRepository) StoreResponse(ctx context.Context, key string, responsePayload []byte, statusCode int) error {
	query := `
		UPDATE idempotency_keys
		SET response_payload = $1, status_code = $2
		WHERE key = $3
	`

	_, err := r.db.Pool.Exec(ctx, query, responsePayload, statusCode, key)
	if err != nil {
		return fmt.Errorf("failed to store idempotency response: %w", err)
	}

	return nil
}

func (r *idempotencyRepository) ReleaseLock(ctx context.Context, key string) error {
	query := `
        UPDATE idempotency_keys
        SET locked_at = NULL, recovery_point = 'completed'
        WHERE key = $1
    `

	_, err := r.db.Pool.Exec(ctx, query, key)
	if err != nil {
		return fmt.Errorf("failed to release idempotency lock: %w", err)
	}

	return nil
}
