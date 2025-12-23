package service

import (
	"context"
	"sync"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
	"github.com/google/uuid"
)

// MockPaymentRepository
type MockPaymentRepository struct {
	mu              sync.RWMutex
	payments        map[string]*domain.Payment
	idempotencyKeys map[string]*domain.IdempotencyKey

	CreatePaymentFn          func(ctx context.Context, payment *domain.Payment) error
	UpdatePaymentFn          func(ctx context.Context, payment *domain.Payment) error
	FindByIDFn               func(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	FindByIDForUpdateFn      func(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	FindByIdempotencyKeyFn   func(ctx context.Context, key string) (*domain.Payment, error)
	CreateIdempotencyKeyFn   func(ctx context.Context, key *domain.IdempotencyKey) error
	FindIdempotencyKeyFn     func(ctx context.Context, key string) (*domain.IdempotencyKey, error)
	FindPendingPaymentsFn    func(ctx context.Context, olderThan time.Duration, limit int) ([]*domain.PendingPaymentCheck, error)
	WithTxFn                 func(ctx context.Context, fn func(repo ports.PaymentRepository) error) error
}

func NewMockPaymentRepository() *MockPaymentRepository {
	return &MockPaymentRepository{
		payments:        make(map[string]*domain.Payment),
		idempotencyKeys: make(map[string]*domain.IdempotencyKey),
	}
}

func (m *MockPaymentRepository) CreatePayment(ctx context.Context, payment *domain.Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CreatePaymentFn != nil {
		return m.CreatePaymentFn(ctx, payment)
	}
	m.payments[payment.ID.String()] = payment
	return nil
}

func (m *MockPaymentRepository) UpdatePayment(ctx context.Context, payment *domain.Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.UpdatePaymentFn != nil {
		return m.UpdatePaymentFn(ctx, payment)
	}
	m.payments[payment.ID.String()] = payment
	return nil
}

func (m *MockPaymentRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
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
	m.mu.RLock()
	defer m.mu.RUnlock()
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
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.FindIdempotencyKeyFn != nil {
		return m.FindIdempotencyKeyFn(ctx, key)
	}
	if k, ok := m.idempotencyKeys[key]; ok {
		return k, nil
	}
	return nil, nil
}

func (m *MockPaymentRepository) UpdateIdempotencyKey(ctx context.Context, key *domain.IdempotencyKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.idempotencyKeys[key.Key]
	if !ok {
		return nil // Or error? Real DB would just do nothing if row missing
	}
	existing.ResponsePayload = key.ResponsePayload
	existing.StatusCode = key.StatusCode
	existing.CompletedAt = key.CompletedAt
	return nil
}

func (m *MockPaymentRepository) FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	return nil, nil // Not used in current tests
}

func (m *MockPaymentRepository) FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
	return nil, nil // Not used in current tests
}

func (m *MockPaymentRepository) FindPendingPayments(ctx context.Context, olderThan time.Duration, limit int) ([]*domain.PendingPaymentCheck, error) {
	if m.FindPendingPaymentsFn != nil {
		return m.FindPendingPaymentsFn(ctx, olderThan, limit)
	}
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
	mu         sync.Mutex
	calls      map[string]int
	Delay      time.Duration
	AuthorizeFn func(ctx context.Context, req domain.BankAuthorizationRequest, idempotencyKey string) (*domain.BankAuthorizationResponse, error)
	CaptureFn   func(ctx context.Context, req domain.BankCaptureRequest, idempotencyKey string) (*domain.BankCaptureResponse, error)
	VoidFn      func(ctx context.Context, req domain.BankVoidRequest, idempotencyKey string) (*domain.BankVoidResponse, error)
	RefundFn    func(ctx context.Context, req domain.BankRefundRequest, idempotencyKey string) (*domain.BankRefundResponse, error)
	GetAuthorizationFn func(ctx context.Context, authID string) (*domain.BankAuthorizationResponse, error)
}

func (m *MockBankPort) inc(method string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls == nil {
		m.calls = make(map[string]int)
	}
	m.calls[method]++
}

func (m *MockBankPort) GetCalls(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[method]
}

func (m *MockBankPort) Authorize(ctx context.Context, req domain.BankAuthorizationRequest, idempotencyKey string) (*domain.BankAuthorizationResponse, error) {
	m.inc("Authorize")
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
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
	m.inc("Capture")
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
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
	m.inc("Void")
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
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
	m.inc("Refund")
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
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

func (m *MockBankPort) GetAuthorization(ctx context.Context, authID string) (*domain.BankAuthorizationResponse, error) {
	if m.GetAuthorizationFn != nil {
		return m.GetAuthorizationFn(ctx, authID)
	}
	return &domain.BankAuthorizationResponse{
		AuthorizationID: authID,
		Status:          "AUTHORIZED",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}, nil
}

func (m *MockBankPort) GetCapture(ctx context.Context, captureID string) (*domain.BankCaptureResponse, error) {
	return &domain.BankCaptureResponse{
		CaptureID:  captureID,
		Status:     "CAPTURED",
		CapturedAt: time.Now(),
	}, nil
}

func (m *MockBankPort) GetRefund(ctx context.Context, refundID string) (*domain.BankRefundResponse, error) {
	return &domain.BankRefundResponse{
		RefundID:   refundID,
		Status:     "REFUNDED",
		RefundedAt: time.Now(),
	}, nil
}
