package service

import (
	"context"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

func TestVoidService_Void_Success(t *testing.T) {
	// Setup
	mockRepo := NewMockPaymentRepository()
	mockBank := &MockBankPort{}
	service := NewVoidService(mockRepo, mockBank)

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
	payment, err := service.Void(
		context.Background(),
		paymentID,
		"idem-void-1",
	)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if payment.Status != domain.StatusVoided {
		t.Errorf("expected status VOIDED, got %s", payment.Status)
	}
	if payment.BankVoidID == nil || *payment.BankVoidID != "void-123" {
		t.Error("expected bank void ID set")
	}
}

func TestVoidService_Void_NotAuthorized(t *testing.T) {
	// Setup
	mockRepo := NewMockPaymentRepository()
	service := NewVoidService(mockRepo, &MockBankPort{})

	paymentID := uuid.New()
	mockRepo.payments[paymentID.String()] = &domain.Payment{
		ID:          paymentID,
		Status:      domain.StatusPending, // Not Authorized
		AmountCents: 1000,
	}

	// Action
	_, err := service.Void(
		context.Background(),
		paymentID,
		"idem-void-2",
	)

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
