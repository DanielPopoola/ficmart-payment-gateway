package service

import (
	"context"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
	"github.com/google/uuid"
)

// MockPaymentRepository
type MockPaymentRepository struct {
	payments        map[string]*domain.Payment
	idempotencyKeys map[string]*domain.IdempotencyKey

	CreatePaymentFn          func(ctx context.Context, payment *domain.Payment) error
	UpdatePaymentFn          func(ctx context.Context, payment *domain.Payment) error
	FindByIDFn               func(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	FindByIDForUpdateFn      func(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	FindByIdempotencyKeyFn   func(ctx context.Context, key string) (*domain.Payment, error)
	CreateIdempotencyKeyFn   func(ctx context.Context, key *domain.IdempotencyKey) error
	FindIdempotencyKeyFn     func(ctx context.Context, key string) (*domain.IdempotencyKey, error)
	WithTxFn                 func(ctx context.Context, fn func(repo ports.PaymentRepository) error) error
}

func NewMockPaymentRepository() *MockPaymentRepository {
	return &MockPaymentRepository{
		payments:        make(map[string]*domain.Payment),
		idempotencyKeys: make(map[string]*domain.IdempotencyKey),
	}
}

func (m *MockPaymentRepository) CreatePayment(ctx context.Context, payment *domain.Payment) error {
	if m.CreatePaymentFn != nil {
		return m.CreatePaymentFn(ctx, payment)
	}
	m.payments[payment.ID.String()] = payment
	return nil
}

func (m *MockPaymentRepository) UpdatePayment(ctx context.Context, payment *domain.Payment) error {
	if m.UpdatePaymentFn != nil {
		return m.UpdatePaymentFn(ctx, payment)
	}
	m.payments[payment.ID.String()] = payment
	return nil
}

func (m *MockPaymentRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	if m.FindByIDFn != nil {
		return m.FindByIDFn(ctx, id)
	}
	if p, ok := m.payments[id.String()]; ok {
		return p, nil
	}
	return nil, domain.NewPaymentNotFoundError(id.String())
}

func (m *MockPaymentRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	if m.FindByIDForUpdateFn != nil {
		return m.FindByIDForUpdateFn(ctx, id)
	}
	return m.FindByID(ctx, id)
}

func (m *MockPaymentRepository) FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	if m.FindByIdempotencyKeyFn != nil {
		return m.FindByIdempotencyKeyFn(ctx, key)
	}
	for _, p := range m.payments {
		if p.IdempotencyKey == key {
			return p, nil
		}
	}
	return nil, nil
}

func (m *MockPaymentRepository) CreateIdempotencyKey(ctx context.Context, key *domain.IdempotencyKey) error {
	if m.CreateIdempotencyKeyFn != nil {
		return m.CreateIdempotencyKeyFn(ctx, key)
	}
	if _, ok := m.idempotencyKeys[key.Key]; ok {
		return domain.NewDuplicateKeyError(key.Key)
	}
	m.idempotencyKeys[key.Key] = key
	return nil
}

func (m *MockPaymentRepository) FindIdempotencyKeyRecord(ctx context.Context, key string) (*domain.IdempotencyKey, error) {
	if m.FindIdempotencyKeyFn != nil {
		return m.FindIdempotencyKeyFn(ctx, key)
	}
	if k, ok := m.idempotencyKeys[key]; ok {
		return k, nil
	}
	return nil, nil
}

func (m *MockPaymentRepository) FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	return nil, nil // Not used in current tests
}

func (m *MockPaymentRepository) FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
	return nil, nil // Not used in current tests
}

func (m *MockPaymentRepository) FindPendingPayments(ctx context.Context, olderThan time.Duration, limit int) ([]*domain.PendingPaymentCheck, error) {
	return nil, nil // Not used in current tests
}

func (m *MockPaymentRepository) WithTx(ctx context.Context, fn func(repo ports.PaymentRepository) error) error {
	if m.WithTxFn != nil {
		return m.WithTxFn(ctx, fn)
	}
	return fn(m)
}

// MockBankPort
type MockBankPort struct {
	AuthorizeFn func(ctx context.Context, req domain.BankAuthorizationRequest, idempotencyKey string) (*domain.BankAuthorizationResponse, error)
	CaptureFn   func(ctx context.Context, req domain.BankCaptureRequest, idempotencyKey string) (*domain.BankCaptureResponse, error)
	VoidFn      func(ctx context.Context, req domain.BankVoidRequest, idempotencyKey string) (*domain.BankVoidResponse, error)
	RefundFn    func(ctx context.Context, req domain.BankRefundRequest, idempotencyKey string) (*domain.BankRefundResponse, error)
}

func (m *MockBankPort) Authorize(ctx context.Context, req domain.BankAuthorizationRequest, idempotencyKey string) (*domain.BankAuthorizationResponse, error) {
	if m.AuthorizeFn != nil {
		return m.AuthorizeFn(ctx, req, idempotencyKey)
	}
	return &domain.BankAuthorizationResponse{
		AuthorizationID: "auth-123",
		Status:          "AUTHORIZED",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}, nil
}

func (m *MockBankPort) Capture(ctx context.Context, req domain.BankCaptureRequest, idempotencyKey string) (*domain.BankCaptureResponse, error) {
	if m.CaptureFn != nil {
		return m.CaptureFn(ctx, req, idempotencyKey)
	}
	return &domain.BankCaptureResponse{
		CaptureID:       "cap-123",
		AuthorizationID: req.AuthorizationID,
		Status:          "CAPTURED",
		CapturedAt:      time.Now(),
	}, nil
}

func (m *MockBankPort) Void(ctx context.Context, req domain.BankVoidRequest, idempotencyKey string) (*domain.BankVoidResponse, error) {
	if m.VoidFn != nil {
		return m.VoidFn(ctx, req, idempotencyKey)
	}
	return &domain.BankVoidResponse{
		VoidID:          "void-123",
		AuthorizationID: req.AuthorizationID,
		Status:          "VOIDED",
		VoidedAt:        time.Now(),
	}, nil
}

func (m *MockBankPort) Refund(ctx context.Context, req domain.BankRefundRequest, idempotencyKey string) (*domain.BankRefundResponse, error) {
	if m.RefundFn != nil {
		return m.RefundFn(ctx, req, idempotencyKey)
	}
	return &domain.BankRefundResponse{
		RefundID:   "ref-123",
		CaptureID:  req.CaptureID,
		Status:     "REFUNDED",
		RefundedAt: time.Now(),
	}, nil
}
