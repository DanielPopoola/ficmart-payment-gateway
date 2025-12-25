package domain

import "context"

type Repository interface {
	// Save persists a payment
	SaveWithIdempotency(ctx context.Context, payment *Payment, idempotencyKey string) error

	// FindbyID retrieves a payment
	FindByID(ctx context.Context, id PaymentID) (*Payment, error)

	// FindByOrderID retrieves a payment by order
	FindByOrderID(ctx context.Context, orderID OrderID) (*Payment, error)

	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (*Payment, error)

	// FindByCustomerID retrieves a payment for a customer
	FindByCustomerID(ctx context.Context, customerID CustomerID, limit, offset int) ([]*Payment, error)

	UpdatePayment(ctx context.Context, payment *Payment, idempotencyKey string) error
}
