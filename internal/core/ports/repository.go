package ports

import (
	"context"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

// PaymentRepository defines the interface for payment data access
type PaymentRepository interface {
	// Create saves a new payment.
	// Returns an error if Idempotency Key already exists (Unique Constraint).
	Create(ctx context.Context, payment *domain.Payment) error

	// FindByID retrieves a payment by its unique system ID.
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)

	// FindByIdempotencyKey retrieves a payment by the client's idempotency key.
	// CRITICAL: Used to return the cached response for duplicate requests.
	FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error)

	// FindByOrderID retrieves a payment by FicMart's order ID.
	FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error)

	// FindByCustomerID retrieves payments for a customer with pagination.
	// We return a slice of pointers to avoid copying large structs.
	FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error)

	// Update updates the mutable fields of a payment (Status, BankIDs, Timestamps).
	Update(ctx context.Context, payment *domain.Payment) error

	// FindPendingPayments retrieves payments that are eligible for reconciliation/retry.
	FindPendingPayments(ctx context.Context, olderThan time.Duration, limit int) ([]*domain.Payment, error)
}
