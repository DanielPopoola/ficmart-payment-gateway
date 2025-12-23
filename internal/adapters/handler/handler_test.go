package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

// Mock services
type mockAuthService struct {
	authorizeFn func(ctx context.Context, orderID, customerID, idempotencyKey string, amount int64, cardNumber, cvv string, expiryMonth, expiryYear int) (*domain.Payment, error)
}

func (m *mockAuthService) Authorize(ctx context.Context, orderID, customerID, idempotencyKey string, amount int64, cardNumber, cvv string, expiryMonth, expiryYear int) (*domain.Payment, error) {
	return m.authorizeFn(ctx, orderID, customerID, idempotencyKey, amount, cardNumber, cvv, expiryMonth, expiryYear)
}

type mockCaptureService struct {
	captureFn func(ctx context.Context, paymentID uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error)
}

func (m *mockCaptureService) Capture(ctx context.Context, paymentID uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error) {
	return m.captureFn(ctx, paymentID, amount, idempotencyKey)
}

type mockRefundService struct {
	refundFn func(ctx context.Context, paymentID uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error)
}

func (m *mockRefundService) Refund(ctx context.Context, paymentID uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error) {
	return m.refundFn(ctx, paymentID, amount, idempotencyKey)
}

type mockVoidService struct {
	voidFn func(ctx context.Context, paymentID uuid.UUID, idempotencyKey string) (*domain.Payment, error)
}

func (m *mockVoidService) Void(ctx context.Context, paymentID uuid.UUID, idempotencyKey string) (*domain.Payment, error) {
	return m.voidFn(ctx, paymentID, idempotencyKey)
}

type mockQueryService struct {
	getPaymentByOrderFn      func(ctx context.Context, orderID string) (*domain.Payment, error)
	getPaymentsByCustomerFn func(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error)
}

func (m *mockQueryService) GetPaymentByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	return m.getPaymentByOrderFn(ctx, orderID)
}

func (m *mockQueryService) GetPaymentsByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
	return m.getPaymentsByCustomerFn(ctx, customerID, limit, offset)
}

func TestHandleAuthorize_Success(t *testing.T) {
	paymentID := uuid.New()
	mockAuth := &mockAuthService{
		authorizeFn: func(ctx context.Context, orderID, customerID, idempotencyKey string, amount int64, cardNumber, cvv string, expiryMonth, expiryYear int) (*domain.Payment, error) {
			return &domain.Payment{
				ID:          paymentID,
				OrderID:     orderID,
				CustomerID:  customerID,
				AmountCents: amount,
				Status:      domain.StatusAuthorized,
				CreatedAt:   time.Now(),
			}, nil
		},
	}

	handler := NewPaymentHandler(mockAuth, nil, nil, nil, nil)

	reqBody, _ := json.Marshal(AuthorizeRequest{
		OrderID:     "order-123",
		CustomerID:  "cust-456",
		Amount:      1000,
		CardNumber:  "1234567890123456",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2026,
	})

	req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewBuffer(reqBody))
	req.Header.Set("Idempotency-Key", "idem-key")
	rr := httptest.NewRecorder()

	handler.HandleAuthorize(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if !resp.Success {
		t.Errorf("expected success true, got false")
	}
}

func TestHandleAuthorize_Error(t *testing.T) {
	mockAuth := &mockAuthService{
		authorizeFn: func(ctx context.Context, orderID, customerID, idempotencyKey string, amount int64, cardNumber, cvv string, expiryMonth, expiryYear int) (*domain.Payment, error) {
			return nil, domain.NewInvalidAmountError(amount)
		},
	}

	handler := NewPaymentHandler(mockAuth, nil, nil, nil, nil)

	reqBody, _ := json.Marshal(AuthorizeRequest{
		OrderID:     "order-123",
		CustomerID:  "cust-456",
		Amount:      -10,
		CardNumber:  "1234567890123456",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2026,
	})

	req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewBuffer(reqBody))
	req.Header.Set("Idempotency-Key", "idem-key")
	rr := httptest.NewRecorder()

	handler.HandleAuthorize(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestHandleAuthorize_IdempotencyHeader(t *testing.T) {
	paymentID := uuid.New()
	headerIdemKey := "header-idem-key"

	mockAuth := &mockAuthService{
		authorizeFn: func(ctx context.Context, orderID, customerID, idempotencyKey string, amount int64, cardNumber, cvv string, expiryMonth, expiryYear int) (*domain.Payment, error) {
			if idempotencyKey != headerIdemKey {
				t.Errorf("expected idempotency key %s, got %s", headerIdemKey, idempotencyKey)
			}
			return &domain.Payment{
				ID:          paymentID,
				OrderID:     orderID,
				IdempotencyKey: idempotencyKey,
			}, nil
		},
	}

	handler := NewPaymentHandler(mockAuth, nil, nil, nil, nil)

	reqBody, _ := json.Marshal(AuthorizeRequest{
		OrderID:     "order-123",
		CustomerID:  "cust-456",
		Amount:      1000,
		CardNumber:  "1234567890123456",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2026,
	})

	req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewBuffer(reqBody))
	req.Header.Set("Idempotency-Key", headerIdemKey)
	rr := httptest.NewRecorder()

	handler.HandleAuthorize(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rr.Code)
	}
}

func TestHandleAuthorize_StillProcessing(t *testing.T) {
	mockAuth := &mockAuthService{
		authorizeFn: func(ctx context.Context, orderID, customerID, idempotencyKey string, amount int64, cardNumber, cvv string, expiryMonth, expiryYear int) (*domain.Payment, error) {
			return nil, domain.NewRequestProcessingError()
		},
	}

	handler := NewPaymentHandler(mockAuth, nil, nil, nil, nil)

	reqBody, _ := json.Marshal(AuthorizeRequest{
		OrderID:     "order-123",
		CustomerID:  "cust-456",
		Amount:      1000,
		CardNumber:  "1234567890123456",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2026,
	})

	req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewBuffer(reqBody))
	req.Header.Set("Idempotency-Key", "idem-key")
	rr := httptest.NewRecorder()

	handler.HandleAuthorize(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected status 202, got %d", rr.Code)
	}
}

func TestHandleAuthorize_Timeout(t *testing.T) {
	mockAuth := &mockAuthService{
		authorizeFn: func(ctx context.Context, orderID, customerID, idempotencyKey string, amount int64, cardNumber, cvv string, expiryMonth, expiryYear int) (*domain.Payment, error) {
			return nil, domain.NewTimeoutError("payment processing")
		},
	}

	handler := NewPaymentHandler(mockAuth, nil, nil, nil, nil)

	reqBody, _ := json.Marshal(AuthorizeRequest{
		OrderID:     "order-123",
		CustomerID:  "cust-456",
		Amount:      1000,
		CardNumber:  "1234567890123456",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2026,
	})

	req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewBuffer(reqBody))
	req.Header.Set("Idempotency-Key", "idem-key")
	rr := httptest.NewRecorder()

	handler.HandleAuthorize(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Error.Code != domain.ErrRequestProcessing {
		t.Errorf("expected code %s, got %s", domain.ErrRequestProcessing, resp.Error.Code)
	}
}

func TestHandleAuthorize_IdempotencyMismatch(t *testing.T) {
	mockAuth := &mockAuthService{
		authorizeFn: func(ctx context.Context, orderID, customerID, idempotencyKey string, amount int64, cardNumber, cvv string, expiryMonth, expiryYear int) (*domain.Payment, error) {
			return nil, domain.NewIdempotencyMismatchError()
		},
	}

	handler := NewPaymentHandler(mockAuth, nil, nil, nil, nil)

	reqBody, _ := json.Marshal(AuthorizeRequest{
		OrderID:     "order-123",
		CustomerID:  "cust-456",
		Amount:      1000,
		CardNumber:  "1234567890123456",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2026,
	})

	req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewBuffer(reqBody))
	req.Header.Set("Idempotency-Key", "idem-key")
	rr := httptest.NewRecorder()

	handler.HandleAuthorize(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestHandleAuthorize_MissingIdempotencyHeader(t *testing.T) {
	handler := NewPaymentHandler(nil, nil, nil, nil, nil)

	reqBody, _ := json.Marshal(AuthorizeRequest{
		OrderID:     "order-123",
		CustomerID:  "cust-456",
		Amount:      1000,
		CardNumber:  "1234567890123456",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2026,
	})

	req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewBuffer(reqBody))
	rr := httptest.NewRecorder()

	handler.HandleAuthorize(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %s", resp.Error.Code)
	}
}

func TestHandleCapture_Success(t *testing.T) {
	paymentID := uuid.New()
	mockCapture := &mockCaptureService{
		captureFn: func(ctx context.Context, pid uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error) {
			return &domain.Payment{
				ID:          pid,
				AmountCents: amount,
				Status:      domain.StatusCaptured,
			}, nil
		},
	}

	handler := NewPaymentHandler(nil, mockCapture, nil, nil, nil)

	reqBody, _ := json.Marshal(CaptureRequest{
		PaymentID: paymentID.String(),
		Amount:    1000,
	})

	req := httptest.NewRequest(http.MethodPost, "/capture", bytes.NewBuffer(reqBody))
	req.Header.Set("Idempotency-Key", "idem-key")
	rr := httptest.NewRecorder()

	handler.HandleCapture(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestHandleGetPaymentByOrder_Success(t *testing.T) {
	paymentID := uuid.New()
	orderID := "order-123"
	mockQuery := &mockQueryService{
		getPaymentByOrderFn: func(ctx context.Context, id string) (*domain.Payment, error) {
			if id != orderID {
				t.Errorf("expected orderID %s, got %s", orderID, id)
			}
			return &domain.Payment{
				ID:      paymentID,
				OrderID: orderID,
			}, nil
		},
	}

	handler := NewPaymentHandler(nil, nil, nil, nil, mockQuery)

	req := httptest.NewRequest(http.MethodGet, "/payments/order/"+orderID, nil)
	// Mocking PathValue for tests
	req.SetPathValue("orderID", orderID)
	rr := httptest.NewRecorder()

	handler.HandleGetPaymentByOrder(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}