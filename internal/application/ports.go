package application

import (
	"context"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

// BankClient is the port for the external bank infrastructure.
type BankClient interface {
	Authorize(ctx context.Context, req AuthorizationRequest, idempotencyKey string) (*AuthorizationResponse, error)
	Capture(ctx context.Context, req CaptureRequest, idempotencyKey string) (*CaptureResponse, error)
	Void(ctx context.Context, req VoidRequest, idempotencyKey string) (*VoidResponse, error)
	Refund(ctx context.Context, req RefundRequest, idempotencyKey string) (*RefundResponse, error)

	GetAuthorization(ctx context.Context, authID string) (*AuthorizationResponse, error)
	GetCapture(ctx context.Context, captureID string) (*CaptureResponse, error)
	GetRefund(ctx context.Context, refundID string) (*RefundResponse, error)
}

// PaymentRepository - Pure domain operations
type PaymentRepository interface {
	Create(ctx context.Context, payment *domain.Payment) error
	FindByID(ctx context.Context, id string) (*domain.Payment, error)
	FindByIDForUpdate(ctx context.Context, id string) (*domain.Payment, error)
	FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error)
	FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error)
	Update(ctx context.Context, payment *domain.Payment) error
	WithTx(ctx context.Context, fn func(PaymentRepository) error) error
}

type IdempotencyKeyInfo struct {
	Key             string
	PaymentID       string
	RequestHash     string
	LockedAt        *time.Time
	ResponsePayload []byte
	StatusCode      int
	RecoveryPoint   string
}

// IdempotencyRepository - Infrastructure concern
type IdempotencyRepository interface {
	AcquireLock(ctx context.Context, key string, paymentID string, requestHash string) error
	FindByKey(ctx context.Context, key string) (*IdempotencyKeyInfo, error)
	StoreResponse(ctx context.Context, key string, responsePayload []byte, statusCode int) error
	UpdateRecoveryPoint(ctx context.Context, key string, recovery_point string) error
	ReleaseLock(ctx context.Context, key string) error
}
