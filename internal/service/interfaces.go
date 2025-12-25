package service

import (
	"context"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/google/uuid"
)

// Authorizer handles payment authorization operations
type Authorizer interface {
	Authorize(ctx context.Context, orderID, customerID, idempotencyKey string,
		amount int64,
		cardNumber, cvv string,
		expiryMonth, expiryYear int,
	) (*domain.Payment, error)
	Reconcile(ctx context.Context, p *domain.Payment) error
}

// Capturer handles payment capture operations
type Capturer interface {
	Capture(ctx context.Context, paymentID uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error)
	Reconcile(ctx context.Context, p *domain.Payment) error
}

// Voider handles authorization void operations
type Voider interface {
	Void(ctx context.Context, paymentID uuid.UUID, idempotencyKey string) (*domain.Payment, error)
	Reconcile(ctx context.Context, p *domain.Payment) error
}

// Refunder handles refund operations
type Refunder interface {
	Refund(ctx context.Context, paymentID uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error)
	Reconcile(ctx context.Context, p *domain.Payment) error
}

type PaymentQuery interface {
	GetPaymentByOrderID(ctx context.Context, orderID string) (*domain.Payment, error)
	GetPaymentsByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error)
}

// Ensure concrete types implement interfaces
var (
	_ Authorizer   = (*AuthorizationService)(nil)
	_ Capturer     = (*CaptureService)(nil)
	_ Voider       = (*VoidService)(nil)
	_ Refunder     = (*RefundService)(nil)
	_ PaymentQuery = (*PaymentQueryService)(nil)
)
