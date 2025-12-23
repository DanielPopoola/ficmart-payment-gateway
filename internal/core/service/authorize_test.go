package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
)

func TestAuthorizeService_Authorize_Success(t *testing.T) {
	// Setup
	mockRepo := NewMockPaymentRepository()
	mockBank := &MockBankPort{}
	service := NewAuthorizationService(mockRepo, mockBank)

	// Action
	payment, err := service.Authorize(
		context.Background(),
		"order-1",
		"cust-1",
		"idem-1",
		1000,
		"1234567812345678",
		"123",
		12,
		2025,
	)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if payment.Status != domain.StatusAuthorized {
		t.Errorf("expected status AUTHORIZED, got %s", payment.Status)
	}
	if payment.BankAuthID == nil || *payment.BankAuthID != "auth-123" {
		t.Error("expected bank auth ID set")
	}
}

func TestAuthorizeService_Authorize_InvalidAmount(t *testing.T) {
	// Setup
	service := NewAuthorizationService(NewMockPaymentRepository(), &MockBankPort{})

	// Action
	_, err := service.Authorize(
		context.Background(),
		"order-1",
		"cust-1",
		"idem-1",
		-100, // Invalid
		"1234567812345678",
		"123",
		12,
		2025,
	)

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !domain.IsErrorCode(err, domain.ErrCodeInvalidAmount) {
		t.Errorf("expected error code %s, got %v", domain.ErrCodeInvalidAmount, err)
	}
}

func TestAuthorizeService_Authorize_BankDecline(t *testing.T) {
	// Setup
	mockRepo := NewMockPaymentRepository()
	mockBank := &MockBankPort{
		AuthorizeFn: func(ctx context.Context, req domain.BankAuthorizationRequest, idempotencyKey string) (*domain.BankAuthorizationResponse, error) {
			return nil, errors.New("insufficient funds")
		},
	}
	service := NewAuthorizationService(mockRepo, mockBank)

	// Action
	payment, err := service.Authorize(
		context.Background(),
		"order-1",
		"cust-1",
		"idem-1",
		1000,
		"1234567812345678",
		"123",
		12,
		2025,
	)

	// Assert
	if err != nil {
		t.Fatalf("expected no error (payment should remain failed), got %v", err)
	}
	if payment.Status != domain.StatusFailed {
		t.Errorf("expected status FAILED, got %s", payment.Status)
	}
}
