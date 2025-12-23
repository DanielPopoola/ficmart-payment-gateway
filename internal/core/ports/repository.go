package ports

import (
	"context"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

// PaymentRepository defines the interface for payment data and idempotency management
type PaymentRepository interface {
	CreatePayment(ctx context.Context, payment *domain.Payment) error
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error)
	FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error)
	FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error)
	UpdatePayment(ctx context.Context, payment *domain.Payment) error
	FindPendingPayments(ctx context.Context, olderThan time.Duration, limit int) ([]*domain.PendingPaymentCheck, error)

	// Idempotency Key Management
	CreateIdempotencyKey(ctx context.Context, key *domain.IdempotencyKey) error
	FindIdempotencyKeyRecord(ctx context.Context, key string) (*domain.IdempotencyKey, error)

	// WithTx executes a function within a database transaction.
	WithTx(ctx context.Context, fn func(PaymentRepository) error) error
}
