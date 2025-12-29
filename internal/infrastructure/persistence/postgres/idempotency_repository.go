package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrDuplicateIdempotencyKey = errors.New("duplicate transaction")
var ErrIdempotencyMismatch = errors.New("idempotency key mismatch")

type IdempotencyRepository struct {
	q Executor
}

func NewIdempotencyRepository(db *DB) *IdempotencyRepository {
	return &IdempotencyRepository{
		q: db.Pool,
	}
}

func (r *IdempotencyRepository) AcquireLock(ctx context.Context, key string, paymentID string, requestHash string) error {
	query := `
		INSERT INTO idempotency_keys (key, payment_id, request_hash, locked_at)
		VALUES ($1, $2, $3, $4)
	`

	_, err := r.q.Exec(ctx, query, key, paymentID, requestHash, time.Now())
	if err != nil {
		if IsUniqueViolation(err) {
			return ErrDuplicateIdempotencyKey
		}
		return fmt.Errorf("failed to acquire idempotency lock: %w", err)
	}

	return nil
}

func (r *IdempotencyRepository) FindByKey(ctx context.Context, key string) (*IdempotencyKey, error) {
	query := `
        SELECT key, payment_id, request_hash, locked_at, response_payload
        FROM idempotency_keys
        WHERE key = $1
    `
	var i IdempotencyKey

	err := r.q.QueryRow(ctx, query, key).Scan(
		&i.Key,
		&i.PaymentID,
		&i.RequestHash,
		&i.LockedAt,
		&i.ResponsePayload,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no key found: %w", err)
		}
		return nil, err
	}

	return &i, nil
}

func (r *IdempotencyRepository) FindByRequestHash(ctx context.Context, requestHash string) (*IdempotencyKey, error) {
	query := `
        SELECT key, payment_id, request_hash, locked_at, response_payload
        FROM idempotency_keys
        WHERE request_hash = $1
        LIMIT 1
    `

	var i IdempotencyKey
	err := r.q.QueryRow(ctx, query, requestHash).Scan(
		&i.Key,
		&i.PaymentID,
		&i.RequestHash,
		&i.LockedAt,
		&i.ResponsePayload,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no completed request found for hash: %w", err)
		}
		return nil, err
	}

	return &i, nil
}

func (r *IdempotencyRepository) StoreResponse(ctx context.Context, key string, responsePayload []byte) error {
	query := `
		UPDATE idempotency_keys
		SET response_payload = $1
		WHERE key = $2
	`

	_, err := r.q.Exec(ctx, query, responsePayload, key)
	if err != nil {
		return fmt.Errorf("failed to store idempotency response: %w", err)
	}

	return nil
}

func (r *IdempotencyRepository) ReleaseLock(ctx context.Context, key string) error {
	query := `
        UPDATE idempotency_keys
        SET locked_at = NULL
        WHERE key = $1
    `

	_, err := r.q.Exec(ctx, query, key)
	if err != nil {
		return fmt.Errorf("failed to release idempotency lock: %w", err)
	}

	return nil
}
