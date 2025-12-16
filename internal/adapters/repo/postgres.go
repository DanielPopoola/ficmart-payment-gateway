package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresPaymentRepository struct {
	db *pgxpool.Pool
}

func NewPaymentRepository(db *pgxpool.Pool) ports.PaymentRepository {
	return &PostgresPaymentRepository{db: db}
}

func (r *PostgresPaymentRepository) Create(ctx context.Context, p *domain.Payment) error {
	query := `
		INSERT INTO payments(
			id, order_id, customer_id, amount_cents, currency, status,
			idempotency_key, created_at, updated_at, attempt_count
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.Exec(ctx, query,
		p.ID,
		p.OrderID,
		p.CustomerID,
		p.AmountCents,
		p.Currency,
		p.Status,
		p.IdempotencyKey,
		p.CreatedAt,
		p.UpdatedAt,
		p.AttemptCount,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "payments_idempotency_key_key" {
				return domain.NewDuplicateKeyError(p.IdempotencyKey)
			}
		}
		return fmt.Errorf("failed to create payment: %w", err)
	}
	return nil
}

func (r *PostgresPaymentRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
			idempotency_key, bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
			created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
			attempt_count, next_retry_at, last_error_category
		FROM payments
		WHERE id = $1
	`

	row := r.db.QueryRow(ctx, query, id)
	return r.scanPayment(row, id.String())
}

func (r *PostgresPaymentRepository) FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
			idempotency_key, bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
			created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
			attempt_count, next_retry_at, last_error_category
		FROM payments
		WHERE idempotency_key = $1
	`

	row := r.db.QueryRow(ctx, query, key)
	return r.scanPayment(row, key)
}

func (r *PostgresPaymentRepository) FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
			idempotency_key, bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
			created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
			attempt_count, next_retry_at, last_error_category
		FROM payments
		WHERE order_id = $1
	`

	row := r.db.QueryRow(ctx, query, orderID)
	return r.scanPayment(row, orderID)
}

func (r *PostgresPaymentRepository) FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
			idempotency_key, bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
			created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
			attempt_count, next_retry_at, last_error_category
		FROM payments
		WHERE customer_id = $1
		LIMIT $2
		OFFSET $3
	`

	rows, err := r.db.Query(ctx, query, customerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch customer data: %w", err)
	}
	defer rows.Close()
	return r.scanPayments(rows)
}

func (r *PostgresPaymentRepository) Update(ctx context.Context, p *domain.Payment) error {
	query := `
		UPDATE payments
		SET status = $1,
			bank_auth_id = $2, bank_capture_id = $3, bank_void_id = $4, bank_refund_id = $5,
			authorized_at = $6, captured_at = $7, voided_at = $8, refunded_at = $9, expires_at = $10,
			attempt_count = $11, next_retry_at = $12, last_error_category = $13,
			updated_at = NOW()
		WHERE id = $14
	`

	cmdTag, err := r.db.Exec(ctx, query,
		p.Status,
		p.BankAuthID, p.BankCaptureID, p.BankVoidID, p.BankRefundID,
		p.AuthorizedAt, p.CapturedAt, p.VoidedAt, p.RefundedAt, p.ExpiresAt,
		p.AttemptCount, p.NextRetryAt, p.LastErrorCategory,
		p.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update payment: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return domain.NewPaymentNotFoundError(p.ID.String())
	}

	return nil
}

func (r *PostgresPaymentRepository) FindPendingPayments(ctx context.Context, olderThan time.Duration, limit int) ([]*domain.Payment, error) {
	// Cutoff time: e.g., NOW - 1 minute
	cutoff := time.Now().Add(-olderThan)

	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status, idempotency_key,
			bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
			created_at, updated_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
			attempt_count, next_retry_at, last_error_category
		FROM payments
		WHERE status = 'PENDING'
			AND (next_retry_at <= NOW() OR next_retry_at IS NULL)
			AND created_at < $1
		ORDER BY next_retry_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`

	rows, err := r.db.Query(ctx, query, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending payments: %w", err)
	}
	defer rows.Close()
	return r.scanPayments(rows)
}

// Helper function to scan row into Payment struct
func (r *PostgresPaymentRepository) scanPayment(row pgx.Row, idStr string) (*domain.Payment, error) {
	var p domain.Payment
	err := row.Scan(
		&p.ID, &p.OrderID, &p.CustomerID, &p.AmountCents, &p.Currency, &p.Status, &p.IdempotencyKey,
		&p.BankAuthID, &p.BankCaptureID, &p.BankVoidID, &p.BankRefundID,
		&p.CreatedAt, &p.UpdatedAt, &p.AuthorizedAt, &p.CapturedAt, &p.VoidedAt, &p.RefundedAt, &p.ExpiresAt,
		&p.AttemptCount, &p.NextRetryAt, &p.LastErrorCategory,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NewPaymentNotFoundError(idStr)
		}
		return nil, fmt.Errorf("failed to scan payment: %w", err)
	}
	return &p, nil
}

// Helper function to scan rows into Payment struct
func (r *PostgresPaymentRepository) scanPayments(rows pgx.Rows) ([]*domain.Payment, error) {
	var payments []*domain.Payment
	for rows.Next() {
		var p domain.Payment
		err := rows.Scan(
			&p.ID, &p.OrderID, &p.CustomerID, &p.AmountCents, &p.Currency, &p.Status, &p.IdempotencyKey,
			&p.BankAuthID, &p.BankCaptureID, &p.BankVoidID, &p.BankRefundID,
			&p.CreatedAt, &p.UpdatedAt, &p.AuthorizedAt, &p.CapturedAt, &p.VoidedAt, &p.RefundedAt, &p.ExpiresAt,
			&p.AttemptCount, &p.NextRetryAt, &p.LastErrorCategory,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		payments = append(payments, &p)
	}
	return payments, nil
}
