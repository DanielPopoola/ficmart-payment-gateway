package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrPaymentNotFound = errors.New("payment not found")

type PaymentRepository struct {
	db *pgxpool.Pool
}

func NewPaymentRepository(db *pgxpool.Pool) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) Create(ctx context.Context, tx pgx.Tx, payment *domain.Payment) error {
	query := `
		INSERT INTO payments (
            id, order_id, customer_id, amount_cents, currency, status,
            bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
            created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`

	p := toDBModel(payment)
	var q interface {
		Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	} = r.db
	if tx != nil {
		q = tx
	}
	_, err := q.Exec(ctx, query,
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
	)

	if err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}

	return nil
}

// FindbyID retrieves a payment
func (r *PaymentRepository) FindByID(ctx context.Context, id string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
			   attempt_count, next_retry_at, last_error_category
		FROM payments WHERE id = $1
	`

	row := r.db.QueryRow(ctx, query, id)
	return scanPayment(row)
}

// FindbyIDByForUpdate retrieves a payment with row-level lock
func (r *PaymentRepository) FindByIDForUpdate(ctx context.Context, tx pgx.Tx, id string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
			   attempt_count, next_retry_at, last_error_category
		FROM payments WHERE id = $1
		FOR UPDATE
	`
	var q interface {
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	} = r.db
	if tx != nil {
		q = tx
	}

	row := q.QueryRow(ctx, query, id)
	return scanPayment(row)
}

// FindByOrderID retrieves a payment by order
func (r *PaymentRepository) FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
			   attempt_count, next_retry_at, last_error_category
		FROM payments WHERE order_id = $1
	`

	row := r.db.QueryRow(ctx, query, orderID)
	return scanPayment(row)

}

// FindByCustomerID retrieves a payment for a customer
func (r *PaymentRepository) FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
			   attempt_count, next_retry_at, last_error_category
		FROM payments WHERE customer_id = $1
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(ctx, query, customerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query payments by customer_id: %w", err)
	}
	results, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*domain.Payment, error) {
		var m PaymentModel
		err := row.Scan(
			&m.ID, &m.OrderID, &m.CustomerID, &m.AmountCents, &m.Currency, &m.Status,
			&m.BankAuthID, &m.BankCaptureID, &m.BankVoidID, &m.BankRefundID,
			&m.CreatedAt, &m.AuthorizedAt, &m.CapturedAt, &m.VoidedAt, &m.RefundedAt, &m.ExpiresAt,
			&m.AttemptCount, &m.NextRetryAt, &m.LastErrorCategory,
		)
		return toDomainModel(m), err
	})

	if err != nil {
		return nil, fmt.Errorf("error occcured while scanning rows: %w", err)
	}
	return results, nil
}

// FindExpiredAuthorizations finds AUTHORIZED payments older than the cutoff time
func (r *PaymentRepository) FindExpiredAuthorizations(ctx context.Context, cutoffTime time.Time, limit int) ([]*domain.Payment, error) {
	query := `
		SELECT id, order_id, customer_id, amount_cents, currency, status,
		       bank_auth_id, bank_capture_id, bank_void_id, bank_refund_id,
		       created_at, authorized_at, captured_at, voided_at, refunded_at, expires_at,
		       attempt_count, next_retry_at, last_error_category
		FROM payments
		WHERE status = 'AUTHORIZED'
		  AND authorized_at < $1
		ORDER BY authorized_at ASC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, cutoffTime, limit)
	if err != nil {
		return nil, fmt.Errorf("query expired authorizations: %w", err)
	}

	results, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*domain.Payment, error) {
		var m PaymentModel
		err := row.Scan(
			&m.ID, &m.OrderID, &m.CustomerID, &m.AmountCents, &m.Currency, &m.Status,
			&m.BankAuthID, &m.BankCaptureID, &m.BankVoidID, &m.BankRefundID,
			&m.CreatedAt, &m.AuthorizedAt, &m.CapturedAt, &m.VoidedAt, &m.RefundedAt, &m.ExpiresAt,
			&m.AttemptCount, &m.NextRetryAt, &m.LastErrorCategory,
		)
		return toDomainModel(m), err
	})

	if err != nil {
		return nil, fmt.Errorf("scan expired authorizations: %w", err)
	}

	return results, nil
}

func (r *PaymentRepository) Update(ctx context.Context, tx pgx.Tx, payment *domain.Payment) error {
	query := `
		UPDATE payments
		SET status = $1,
			bank_auth_id = $2, bank_capture_id = $3, bank_void_id = $4, bank_refund_id = $5,
			authorized_at = $6, captured_at = $7, voided_at = $8, refunded_at = $9, expires_at = $10,
			attempt_count = $11, next_retry_at = $12, last_error_category = $13
		WHERE id = $14
	`

	p := toDBModel(payment)
	var q interface {
		Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	} = r.db
	if tx != nil {
		q = tx
	}
	results, err := q.Exec(ctx, query,
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
		&m.AttemptCount, &m.NextRetryAt, &m.LastErrorCategory,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("failed to scan payment: %w", err)
	}
	return toDomainModel(m), nil
}
