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
	Create(ctx context.Context, payment *domain.Payment, idempotencyKey string, requestHash string) error
	FindByID(ctx context.Context, id string) (*domain.Payment, error)
	FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error)
	FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error)
	FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error)
	Update(ctx context.Context, payment *domain.Payment, idempotencyKey string, ResponsePayload []byte, statusCode int, recoveryPoint string) error
}
