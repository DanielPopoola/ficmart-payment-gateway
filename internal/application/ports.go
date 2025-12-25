package application

import (
	"context"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

// BankClient is the port for the external bank infrastructure.
type BankClient interface {
	Authorize(ctx context.Context, req AuthorizationRequest) (*AuthorizationResponse, error)
	Capture(ctx context.Context, req CaptureRequest) (*CaptureResponse, error)
	Void(ctx context.Context, req VoidRequest) (*VoidResponse, error)
	Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error)
}

// PaymentRepository is the port for persistence.
type PaymentRepository interface {
	SaveWithIdempotency(ctx context.Context, payment *domain.Payment, idempotencyKey string) error
	FindByID(ctx context.Context, id domain.PaymentID) (*domain.Payment, error)
	FindByOrderID(ctx context.Context, orderID domain.OrderID) (*domain.Payment, error)
	FindByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.Payment, error)
	FindByCustomerID(ctx context.Context, customerID domain.CustomerID, limit, offset int) ([]*domain.Payment, error)
	UpdatePayment(ctx context.Context, payment *domain.Payment, idempotencyKey string) error
}

type IdempotencyStore interface {
	CheckAndLock(ctx context.Context, key, requestHash string) (bool, error)
	GetCachedResponse(ctx context.Context, key string) (*domain.Payment, error)
	CacheResponse(ctx context.Context, key string, payment *domain.Payment) error
}
