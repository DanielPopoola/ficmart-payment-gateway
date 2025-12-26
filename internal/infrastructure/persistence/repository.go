package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/jackc/pgx/v5"
)

type postgresRepository struct {
	db *DB
}

func NewPaymentRepository(db *DB) application.PaymentRepository {
	return &postgresRepository{db: db}
}

// Create persists a payment with idempotency
func (r *postgresRepository) Create(ctx context.Context, payment *domain.Payment, idempotencyKey string, requestHash string) error {
	p := toDBModel(payment)

	tx, err := r.db.Pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO payments (
			id, order_id, customer_id, amount_cents, currency, status,
			bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
			created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
			attempt_count, next_retry_at, last_error_category
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`

	_, err = tx.Exec(ctx, query,
		p.ID,
		p.OrderID,
		p.CustomerID,
		p.AmountCents,
		p.Currency,
		p.Status,
		p.BankAuthID,
		p.BankCaptureID,
		p.BankVoidID,
		p.BankRefundID,
		p.CreatedAt,
		p.AuthorizedAt,
		p.CapturedAt,
		p.VoidedAt,
		p.RefundedAt,
		p.ExpiresAt,
		p.AttemptCount,
		p.NextRetryAt,
		p.LastErrorCategory,
	)

	if err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}

	idemquery := `
			INSERT INTO  idempotency_keys (
				key, payment_id, request_hash, locked_at, response_payload, status_code, recovery_point
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = tx.Exec(ctx, idemquery,
		idempotencyKey, p.ID, requestHash, time.Now(), "CREATED",
	)
	if err != nil {
		if IsUniqueViolation(err) {
			return domain.ErrDuplicateIdempotencyKey
		}
		return fmt.Errorf("failed to insert idempotency key: %w", err)
	}

	return tx.Commit(ctx)
}

// FindbyID retrieves a payment
func (r *postgresRepository) FindByID(ctx context.Context, id string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at
		FROM payments WHERE id = $1
	`

	row := r.db.Pool.QueryRow(ctx, query, id)
	return scanPayment(row)
}

// FindByOrderID retrieves a payment by order
func (r *postgresRepository) FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at
		FROM payments WHERE order_id = $1
	`

	row := r.db.Pool.QueryRow(ctx, query, orderID)
	return scanPayment(row)

}

// FindByIdempotencyKey retrieve payment info using an idempotency key
func (r *postgresRepository) FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
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

// FindByCustomerID retrieves a payment for a customer
func (r *postgresRepository) FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at
		FROM payments WHERE customer_id = $1
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Pool.Query(ctx, query, customerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query payments by customer_id: %w", err)
	}
	results, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*domain.Payment, error) {
		var m PaymentModel
		err := row.Scan(
			&m.ID, &m.OrderID, &m.CustomerID, &m.AmountCents, &m.Currency, &m.Status,
			&m.BankAuthID, &m.BankCaptureID, &m.BankVoidID, &m.BankRefundID,
			&m.CreatedAt, &m.AuthorizedAt, &m.CapturedAt, &m.VoidedAt, &m.RefundedAt, &m.ExpiresAt,
		)
		return toDomainModel(m), err
	})

	if err != nil {
		return nil, fmt.Errorf("error occcured while scanning rows: %w", err)
	}
	return results, nil
}

// Update payment with idempotency
func (r *postgresRepository) Update(ctx context.Context, payment *domain.Payment, idempotencyKey string, responsePayload []byte, statusCode int, recoveryPoint string) error {
	p := toDBModel(payment)

	tx, err := r.db.Pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		UPDATE payments
		SET status = $1,
			bank_auth_id = $2, bank_capture_id = $3, bank_void_id = $4, bank_refund_id = $5,
			authorized_at = $6, captured_at = $7, voided_at = $8, refunded_at = $9, expires_at = $10
		WHERE id = $11
	`

	_, err = tx.Exec(ctx, query,
		p.Status, p.BankAuthID, p.BankCaptureID, p.BankVoidID, p.BankRefundID,
		p.AuthorizedAt, p.CapturedAt, p.VoidedAt, p.RefundedAt, p.ExpiresAt, p.ID,
	)

	if err != nil {
		return err
	}

	idemQuery := `
        UPDATE idempotency_keys 
        SET response_payload = $1, 
            status_code = $2, 
            recovery_point = $3,
            locked_at = NULL
        WHERE key = $4
	`
	_, err = tx.Exec(ctx, idemQuery, responsePayload, statusCode, recoveryPoint, idempotencyKey)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// helper to scan payments and return mapped db model to domain entity
func scanPayment(row pgx.Row) (*domain.Payment, error) {
	var m PaymentModel
	err := row.Scan(
		&m.ID, &m.OrderID, &m.CustomerID, &m.AmountCents, &m.Currency, &m.Status,
		&m.BankAuthID, &m.BankCaptureID, &m.BankVoidID, &m.BankRefundID,
		&m.CreatedAt, &m.AuthorizedAt, &m.CapturedAt, &m.VoidedAt, &m.RefundedAt, &m.ExpiresAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPaymentNotFound
		}
		return nil, fmt.Errorf("failed to scan payment: %w", err)
	}
	return toDomainModel(m), nil
}
