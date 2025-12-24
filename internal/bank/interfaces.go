package bank

import "context"

type BankPort interface {
	//POST endpoints
	Authorize(ctx context.Context, req AuthorizationRequest, idempotencyKey string) (*AuthorizationResponse, error)
	Capture(ctx context.Context, req CaptureRequest, idempotencyKey string) (*CaptureResponse, error)
	Void(ctx context.Context, req VoidRequest, idempotencyKey string) (*VoidResponse, error)
	Refund(ctx context.Context, req RefundRequest, idempotencyKey string) (*RefundResponse, error)

	// GET endpoints
	GetAuthorization(ctx context.Context, authID string) (*AuthorizationResponse, error)
	GetCapture(ctx context.Context, captureID string) (*CaptureResponse, error)
	GetRefund(ctx context.Context, refundID string) (*RefundResponse, error)
}
