package service

import (
	"context"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

func TestCaptureService_Capture_Success(t *testing.T) {
	// Setup
	mockRepo := NewMockPaymentRepository()
	mockBank := &MockBankPort{}
	service := NewCaptureService(mockRepo, mockBank)

	paymentID := uuid.New()
	authID := "auth-123"
	mockRepo.payments[paymentID.String()] = &domain.Payment{
		ID:           paymentID,
		Status:       domain.StatusAuthorized,
		AmountCents:  1000,
		BankAuthID:   &authID,
		AuthorizedAt: &time.Time{},
	}

	// Action
	payment, err := service.Capture(
		context.Background(),
		paymentID,
		1000,
		"idem-capture-1",
	)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if payment.Status != domain.StatusCaptured {
		t.Errorf("expected status CAPTURED, got %s", payment.Status)
	}
	if payment.BankCaptureID == nil || *payment.BankCaptureID != "cap-123" {
		t.Error("expected bank capture ID set")
	}
}

func TestCaptureService_Capture_InvalidState(t *testing.T) {
	// Setup
	mockRepo := NewMockPaymentRepository()
	service := NewCaptureService(mockRepo, &MockBankPort{})

	paymentID := uuid.New()
	mockRepo.payments[paymentID.String()] = &domain.Payment{
		ID:          paymentID,
		Status:      domain.StatusPending, // Not Authorized
		AmountCents: 1000,
	}

	// Action
	_, err := service.Capture(
		context.Background(),
		paymentID,
		1000,
		"idem-capture-2",
	)

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The service returns a generic fmt.Errorf for invalid state, 
	// ideally it should return a DomainError, but for now we check error existence.
}
