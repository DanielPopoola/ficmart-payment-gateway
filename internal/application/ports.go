package application

import (
	"context"
)

// BankClient is the port for the external bank infrastructure.
type BankClient interface {
	Authorize(ctx context.Context, req BankAuthorizationRequest, idempotencyKey string) (*BankAuthorizationResponse, error)
	Capture(ctx context.Context, req BankCaptureRequest, idempotencyKey string) (*BankCaptureResponse, error)
	Void(ctx context.Context, req BankVoidRequest, idempotencyKey string) (*BankVoidResponse, error)
	Refund(ctx context.Context, req BankRefundRequest, idempotencyKey string) (*BankRefundResponse, error)

	GetAuthorization(ctx context.Context, authID string) (*BankAuthorizationResponse, error)
	GetCapture(ctx context.Context, captureID string) (*BankCaptureResponse, error)
	GetRefund(ctx context.Context, refundID string) (*BankRefundResponse, error)
}
