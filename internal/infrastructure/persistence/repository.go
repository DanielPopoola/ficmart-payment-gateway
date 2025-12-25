package persistence

import (
	"context"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) SavePaymentWithIdempotency(
	ctx context.Context,
	payment *domain.Payment,
	idempotencyKey string,
	requestHash string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
        INSERT INTO idempotency_keys (key, request_hash, locked_at)
        VALUES ($1, $2, $3)
    `, idempotencyKey, requestHash, time.Now())
	if err != nil {
		return err // Duplicate key or other error
	}

	// Insert payment
	model := toDBModel(payment)
	_, err = tx.Exec(ctx, `
        INSERT INTO payments (id, order_id, customer_id, amount_cents, currency, status, idempotency_key, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `, model.ID, model.OrderID, model.CustomerID, model.AmountCents, model.Currency, model.Status, idempotencyKey, model.CreatedAt, model.UpdatedAt)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (*domain.Payment, error) {
	var model PaymentModel

	err := r.pool.QueryRow(ctx, `
        SELECT id, order_id, customer_id, amount_cents, currency, status,
               bank_auth_id, created_at, authorized_at, expires_at
        FROM payments WHERE id = $1
    `, id).Scan(
		&model.ID, &model.OrderID, &model.CustomerID, &model.AmountCents,
		&model.Currency, &model.Status, &model.BankAuthID,
		&model.CreatedAt, &model.AuthorizedAt, &model.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}

	return toDomainModel(model), nil
}

func (r *PostgresRepository) UpdatePayment(ctx context.Context, payment *domain.Payment) error {
	model := toDBModel(payment)

	_, err := r.pool.Exec(ctx, `
        UPDATE payments
        SET status = $1, bank_auth_id = $2, authorized_at = $3, expires_at = $4, updated_at = NOW()
        WHERE id = $5
    `, model.Status, model.BankAuthID, model.AuthorizedAt, model.ExpiresAt, model.ID)

	return err
}
