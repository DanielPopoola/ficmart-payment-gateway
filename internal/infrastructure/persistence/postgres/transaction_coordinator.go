package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TransactionCoordinator manages transactions across multiple repositories
type TransactionCoordinator struct {
	pool *pgxpool.Pool
}

func NewTransactionCoordinator(db *DB) *TransactionCoordinator {
	return &TransactionCoordinator{
		pool: db.Pool,
	}
}

// WithTransaction executes a function within a database transaction
// The function receives repository instances that use the transaction
func (tc *TransactionCoordinator) WithTransaction(
	ctx context.Context,
	fn func(ctx context.Context, paymentRepo *PaymentRepository, idempotencyRepo *IdempotencyRepository) error,
) error {
	tx, err := tc.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	txPaymentRepo := &PaymentRepository{
		db: tc.pool,
		q:  tx,
	}

	txIdempotencyRepo := &IdempotencyRepository{
		q: tx,
	}

	if err := fn(ctx, txPaymentRepo, txIdempotencyRepo); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
