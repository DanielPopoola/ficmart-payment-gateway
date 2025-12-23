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

	handler := NewPaymentHandler(mockAuth, nil, nil, nil)

	reqBody, _ := json.Marshal(AuthorizeRequest{
		OrderID:        "order-123",
		CustomerID:     "cust-456",
		Amount:         1000,
		CardNumber:     "1234567890123456",
		CVV:            "123",
		ExpiryMonth:    12,
		ExpiryYear:     2026,
		IdempotencyKey: "idem-key",
	})

	req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewBuffer(reqBody))
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

	handler := NewPaymentHandler(mockAuth, nil, nil, nil)

	reqBody, _ := json.Marshal(AuthorizeRequest{
		OrderID:        "order-123",
		CustomerID:     "cust-456",
		Amount:         -10,
		CardNumber:     "1234567890123456",
		CVV:            "123",
		ExpiryMonth:    12,
		ExpiryYear:     2026,
		IdempotencyKey: "idem-key",
	})

	req := httptest.NewRequest(http.MethodPost, "/authorize", bytes.NewBuffer(reqBody))
	rr := httptest.NewRecorder()

	handler.HandleAuthorize(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Success {
		t.Errorf("expected success false, got true")
	}
	if resp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("expected error code %s, got %s", "VALIDATION_ERROR", resp.Error.Code)
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

	handler := NewPaymentHandler(nil, mockCapture, nil, nil)

	reqBody, _ := json.Marshal(CaptureRequest{
		PaymentID:      paymentID.String(),
		Amount:         1000,
		IdempotencyKey: "idem-key",
	})

	req := httptest.NewRequest(http.MethodPost, "/capture", bytes.NewBuffer(reqBody))
	rr := httptest.NewRecorder()

	handler.HandleCapture(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestHandleVoid_Success(t *testing.T) {
	paymentID := uuid.New()
	mockVoid := &mockVoidService{
		voidFn: func(ctx context.Context, pid uuid.UUID, idempotencyKey string) (*domain.Payment, error) {
			return &domain.Payment{
				ID:     pid,
				Status: domain.StatusVoided,
			}, nil
		},
	}

	handler := NewPaymentHandler(nil, nil, nil, mockVoid)

	reqBody, _ := json.Marshal(VoidRequest{
		PaymentID:      paymentID.String(),
		IdempotencyKey: "idem-key",
	})

	req := httptest.NewRequest(http.MethodPost, "/void", bytes.NewBuffer(reqBody))
	rr := httptest.NewRecorder()

	handler.HandleVoid(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestHandleRefund_Success(t *testing.T) {
	paymentID := uuid.New()
	mockRefund := &mockRefundService{
		refundFn: func(ctx context.Context, pid uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error) {
			return &domain.Payment{
				ID:          pid,
				AmountCents: amount,
				Status:      domain.StatusRefunded,
			}, nil
		},
	}

	handler := NewPaymentHandler(nil, nil, mockRefund, nil)

	reqBody, _ := json.Marshal(RefundRequest{
		PaymentID:      paymentID.String(),
		Amount:         1000,
		IdempotencyKey: "idem-key",
	})

	req := httptest.NewRequest(http.MethodPost, "/refund", bytes.NewBuffer(reqBody))
	rr := httptest.NewRecorder()

	handler.HandleRefund(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestHandleRefund_ValidationError(t *testing.T) {
	handler := NewPaymentHandler(nil, nil, nil, nil)

	reqBody, _ := json.Marshal(RefundRequest{
		PaymentID:      "invalid-uuid",
		Amount:         -500,
		IdempotencyKey: "",
	})

	req := httptest.NewRequest(http.MethodPost, "/refund", bytes.NewBuffer(reqBody))
	rr := httptest.NewRecorder()

	handler.HandleRefund(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %s", resp.Error.Code)
	}
}
