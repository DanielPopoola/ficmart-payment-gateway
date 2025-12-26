package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/jackc/pgx/v5"
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

func (r *idempotencyRepository) FindByKey(ctx context.Context, key string) (*application.IdempotencyKeyInfo, error) {
	query := `
        SELECT key, payment_id, request_hash, locked_at, response_payload, status_code, recovery_point
        FROM idempotency_keys
        WHERE key = $1
    `
	var i application.IdempotencyKeyInfo

	err := r.db.Pool.QueryRow(ctx, query, key).Scan(
		&i.Key,
		&i.PaymentID,
		&i.RequestHash,
		&i.LockedAt,
		&i.ResponsePayload,
		&i.StatusCode,
		&i.RecoveryPoint,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no key found: %w", err)
		}
	}

	return &i, nil
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

func (r *idempotencyRepository) UpdateRecoveryPoint(ctx context.Context, key string, point string) error {
	query := `UPDATE idempotency_keys SET recovery_point = $1 WHERE key = $2`
	_, err := r.db.Pool.Exec(ctx, query, point, key)
	return err
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
