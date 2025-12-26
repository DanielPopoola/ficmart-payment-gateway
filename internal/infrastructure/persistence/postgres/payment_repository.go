package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/jackc/pgx/v5"
)

type paymentRepository struct {
	db *DB
}

func NewPaymentRepository(db *DB) application.PaymentRepository {
	return &paymentRepository{db: db}
}

func (r *paymentRepository) Create(ctx context.Context, payment *domain.Payment) error {
	query := `
		INSERT INTO payments (
            id, order_id, customer_id, amount_cents, currency, status,
            bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
            created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
            attempt_count, next_retry_at, last_error_category
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`

	p := toDBModel(payment)
	_, err := r.db.Pool.Exec(ctx, query,
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

	return nil
}

// FindbyID retrieves a payment
func (r *paymentRepository) FindByID(ctx context.Context, id string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at
		FROM payments WHERE id = $1
	`

	row := r.db.Pool.QueryRow(ctx, query, id)
	return scanPayment(row)
}

// FindbyIDByForUpdate retrieves a payment with row-level lock
func (r *paymentRepository) FindByIDForUpdate(ctx context.Context, id string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at
		FROM payments WHERE id = $1
		FOR UPDATE
	`

	row := r.db.Pool.QueryRow(ctx, query, id)
	return scanPayment(row)
}

// FindByOrderID retrieves a payment by order
func (r *paymentRepository) FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at
		FROM payments WHERE order_id = $1
	`

	row := r.db.Pool.QueryRow(ctx, query, orderID)
	return scanPayment(row)

}

// FindByCustomerID retrieves a payment for a customer
func (r *paymentRepository) FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
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

func (r *paymentRepository) Update(ctx context.Context, payment *domain.Payment) error {
	query := `
		UPDATE payments
		SET status = $1,
			bank_auth_id = $2, bank_capture_id = $3, bank_void_id = $4, bank_refund_id = $5,
			authorized_at = $6, captured_at = $7, voided_at = $8, refunded_at = $9, expires_at = $10
		WHERE id = $11
	`

	p := toDBModel(payment)
	results, err := r.db.Pool.Exec(ctx, query,
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
		p.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	rowsAffected := results.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("payment not found")
	}

	return nil
}

// scanPayment converts a database row into a domain Payment.
// Returns ErrPaymentNotFound if the row doesn't exist.
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
