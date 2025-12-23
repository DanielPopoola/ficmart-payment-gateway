package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PaymentRepository struct {
	pool *pgxpool.Pool
	q    Executor
}

func NewPaymentRepository(db *DB) ports.PaymentRepository {
	return &PaymentRepository{
		pool: db.Pool,
		q:    db.Pool,
	}
}

// Create saves a new payment to the database
func (r *PaymentRepository) CreatePayment(ctx context.Context, p *domain.Payment) error {
	query := `INSERT INTO payments (
				id, order_id, customer_id, amount_cents, currency, status, idempotency_key,
				bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
				created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
				attempt_count)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`

	_, err := r.q.Exec(ctx, query,
		p.ID,
		p.OrderID,
		p.CustomerID,
		p.AmountCents,
		p.Currency,
		p.Status,
		p.IdempotencyKey,
		p.BankAuthID,
		p.BankCaptureID,
		p.BankVoidID,
		p.BankRefundID,
		p.CreatedAt,
		p.UpdatedAt,
		p.AuthorizedAt,
		p.CapturedAt,
		p.VoidedAt,
		p.RefundedAt,
		p.ExpiresAt,
		p.AttemptCount,
	)
	if err != nil {
		if IsUniqueViolation(err) {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				if pgErr.ConstraintName == "payments_idempotency_key_key" {
					return domain.NewDuplicateKeyError(p.IdempotencyKey)
				}
			}
		}
		return fmt.Errorf("failed to create payment: %w", err)
	}
	return nil
}

// FindByID retrieves a payment by its unique system ID
func (r *PaymentRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	query := `
			SELECT id, order_id, customer_id, amount_cents, currency, status, idempotency_key,
				bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
				created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
				attempt_count, next_retry_at, last_error_category
			FROM payments
			WHERE id = $1
			`

	row := r.q.QueryRow(ctx, query, id)
	return scanPayment(row)
}

// FindByIDForUpdate retrieves a payment by its unique system ID and locks the row
func (r *PaymentRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	query := `
			SELECT id, order_id, customer_id, amount_cents, currency, status, idempotency_key,
				bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
				created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
				attempt_count, next_retry_at, last_error_category
			FROM payments
			WHERE id = $1
			FOR UPDATE
			`

	row := r.q.QueryRow(ctx, query, id)
	return scanPayment(row)
}

// FindByIdempotencyKey retrieves a payment by the client's idempotency key.
func (r *PaymentRepository) FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	query := `
			SELECT id, order_id, customer_id, amount_cents, currency, status, idempotency_key,
				bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
				created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
				attempt_count, next_retry_at, last_error_category
			FROM payments
			WHERE idempotency_key = $1
			`

	row := r.q.QueryRow(ctx, query, key)
	return scanPayment(row)
}

// FindByOrderID retrieves a payment by FicMart's order ID.
func (r *PaymentRepository) FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	query := `
			SELECT id, order_id, customer_id, amount_cents, currency, status, idempotency_key,
				bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
				created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
				attempt_count, next_retry_at, last_error_category
			FROM payments
			WHERE order_id = $1
			`

	row := r.q.QueryRow(ctx, query, orderID)
	return scanPayment(row)
}

func (r *PaymentRepository) FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
	query := `
			SELECT id, order_id, customer_id, amount_cents, currency, status, idempotency_key,
				bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
				created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
				attempt_count, next_retry_at, last_error_category
			FROM payments
			WHERE customer_id = $1
			LIMIT $2 OFFSET $3
			`

	rows, err := r.q.Query(ctx, query, customerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query payments by customer_id: %w", err)
	}
	results, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*domain.Payment, error) {
		var p domain.Payment
		err := row.Scan(
			&p.ID,
			&p.OrderID,
			&p.CustomerID,
			&p.AmountCents,
			&p.Currency,
			&p.Status,
			&p.IdempotencyKey,
			&p.BankAuthID,
			&p.BankCaptureID,
			&p.BankVoidID,
			&p.BankRefundID,
			&p.CreatedAt,
			&p.UpdatedAt,
			&p.AuthorizedAt,
			&p.CapturedAt,
			&p.VoidedAt,
			&p.RefundedAt,
			&p.ExpiresAt,
			&p.AttemptCount,
			&p.NextRetryAt,
			&p.LastErrorCategory,
		)
		return &p, err
	})

	if err != nil {
		return nil, fmt.Errorf("error occcured while scanning rows: %w", err)
	}
	return results, nil

}

func (r *PaymentRepository) FindPendingPayments(ctx context.Context, olderThan time.Duration, limit int) ([]*domain.PendingPaymentCheck, error) {
	cutoff := time.Now().Add(-olderThan)

	query := `
        SELECT id, idempotency_key, status, bank_auth_id, bank_capture_id, attempt_count
        FROM payments
        WHERE status IN ('PENDING', 'AUTHORIZED', 'CAPTURING', 'VOIDING', 'REFUNDING')
            AND created_at < $1
            AND (next_retry_at IS NULL OR next_retry_at <= NOW())
        ORDER BY next_retry_at ASC NULLS FIRST
        LIMIT $2
    `

	rows, err := r.q.Query(ctx, query, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending payments: %w", err)
	}

	pending, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*domain.PendingPaymentCheck, error) {
		var p domain.PendingPaymentCheck
		err := row.Scan(
			&p.ID,
			&p.IdempotencyKey,
			&p.Status,
			&p.BankAuthID,
			&p.BankCaptureID,
			&p.AttemptCount,
		)
		return &p, err
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan pending payments: %w", err)
	}

	return pending, nil
}

func (r *PaymentRepository) UpdatePayment(ctx context.Context, p *domain.Payment) error {
	query := `
			UPDATE payments SET status = $1,
				bank_auth_id = $2, bank_capture_id = $3, bank_void_id = $4, bank_refund_id = $5,
				authorized_at = $6, captured_at = $7, voided_at = $8, refunded_at = $9, expires_at = $10, 
				attempt_count = $11, next_retry_at = $12, last_error_category = $13, updated_at = NOW() 
			WHERE id = $14
	`

	cmdTag, err := r.q.Exec(ctx, query,
		p.Status,
		p.BankAuthID,
		p.BankCaptureID,
		p.BankVoidID,
		p.BankRefundID,
		p.AuthorizedAt,
		p.CapturedAt,
		p.VoidedAt,
		p.RefundedAt,
		p.ExpiresAt,
		p.AttemptCount,
		p.NextRetryAt,
		p.LastErrorCategory,
		p.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update payment record: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return domain.NewPaymentNotFoundError(p.ID.String())
	}
	return nil
}

func (r *PaymentRepository) CreateIdempotencyKey(ctx context.Context, k *domain.IdempotencyKey) error {
	query := `INSERT INTO idempotency_keys (key, request_hash, locked_at)
			  VALUES ($1, $2, $3)`

	_, err := r.q.Exec(ctx, query,
		k.Key,
		k.RequestHash,
		k.LockedAt,
	)
	if err != nil {
		if IsUniqueViolation(err) {
			return domain.NewDuplicateKeyError(k.Key)
		}
		return fmt.Errorf("failed to create idempotency key: %w", err)
	}
	return nil
}

func (r *PaymentRepository) FindIdempotencyKeyRecord(ctx context.Context, key string) (*domain.IdempotencyKey, error) {
	query := `SELECT key, request_hash, locked_at, response_payload, status_code, completed_at
			  FROM idempotency_keys
			  WHERE key = $1`

	row := r.q.QueryRow(ctx, query, key)
	var k domain.IdempotencyKey
	err := row.Scan(
		&k.Key,
		&k.RequestHash,
		&k.LockedAt,
		&k.ResponsePayload,
		&k.StatusCode,
		&k.CompletedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Return nil if not found
		}
		return nil, fmt.Errorf("failed to scan idempotency key: %w", err)
	}

	return &k, nil
}

func (r *PaymentRepository) UpdateIdempotencyKey(ctx context.Context, k *domain.IdempotencyKey) error {
	query := `UPDATE idempotency_keys SET response_payload = $1, status_code = $2, completed_at = $3
			  WHERE key = $4`

	_, err := r.q.Exec(ctx, query,
		k.ResponsePayload,
		k.StatusCode,
		k.CompletedAt,
		k.Key,
	)
	if err != nil {
		return fmt.Errorf("failed to update idempotency key: %w", err)
	}
	return nil
}

// WithTx executes a function within a database transaction
func (r *PaymentRepository) WithTx(ctx context.Context, fn func(ports.PaymentRepository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Defer rollback in case of panic or error (if commit isn't reached)
	defer tx.Rollback(ctx)

	repoWithTx := &PaymentRepository{
		pool: r.pool,
		q:    tx, // Switch the executor to the transaction
	}

	if err := fn(repoWithTx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// scanPayment scans a pgx.Row into a domain.Payment and returns a pointer to the populated Payment.
func scanPayment(row pgx.Row) (*domain.Payment, error) {
	var p domain.Payment
	err := row.Scan(
		&p.ID,
		&p.OrderID,
		&p.CustomerID,
		&p.AmountCents,
		&p.Currency,
		&p.Status,
		&p.IdempotencyKey,
		&p.BankAuthID,
		&p.BankCaptureID,
		&p.BankVoidID,
		&p.BankRefundID,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.AuthorizedAt,
		&p.CapturedAt,
		&p.VoidedAt,
		&p.RefundedAt,
		&p.ExpiresAt,
		&p.AttemptCount,
		&p.NextRetryAt,
		&p.LastErrorCategory,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewPaymentNotFoundError(p.ID.String())
		}
		return nil, fmt.Errorf("failed to scan payment: %w", err)
	}
	return &p, nil
}
