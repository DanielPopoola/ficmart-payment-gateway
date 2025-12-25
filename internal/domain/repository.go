package domain

import "context"

type Repository interface {
	// Create persists a payment with idempotency
	Create(ctx context.Context, payment *Payment, idempotencyKey string, requestHash string) error

	// FindbyID retrieves a payment
	FindByID(ctx context.Context, id string) (*Payment, error)

	// FindByOrderID retrieves a payment by order
	FindByOrderID(ctx context.Context, orderID string) (*Payment, error)

	// FindByIdempotencyKey retrieve payment info using an idempotency key
	FindByIdempotencyKey(ctx context.Context, key string) (*Payment, error)

	// FindByCustomerID retrieves a payment for a customer
	FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*Payment, error)

	// Update payment with idempotency
	Update(ctx context.Context, payment *Payment, idempotencyKey string, ResponsePayload []byte, statusCode int, recoveryPoint string) error
}
