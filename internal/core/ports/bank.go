package ports

import (
	"context"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
)

// BankPort defines the behavior of the external banking system.
type BankPort interface {
	Authorize(ctx context.Context, req domain.BankAuthorizationRequest, idempotencyKey string) (*domain.BankAuthorizationResponse, error)
	Capture(ctx context.Context, req domain.BankCaptureRequest, idempotencyKey string) (*domain.BankCaptureResponse, error)
	Void(ctx context.Context, req domain.BankVoidRequest, idempotencyKey string) (*domain.BankVoidResponse, error)
	Refund(ctx context.Context, req domain.BankRefundRequest, idempotencyKey string) (*domain.BankRefundResponse, error)
}
