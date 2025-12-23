package service

import (
	"context"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

func TestRefundService_Refund_Success(t *testing.T) {
	// Setup
	mockRepo := NewMockPaymentRepository()
	mockBank := &MockBankPort{}
	service := NewRefundService(mockRepo, mockBank)

	paymentID := uuid.New()
	capID := "cap-123"
	mockRepo.payments[paymentID.String()] = &domain.Payment{
		ID:            paymentID,
		Status:        domain.StatusCaptured,
		AmountCents:   1000,
		BankCaptureID: &capID,
		CapturedAt:    &time.Time{},
	}

	// Action
	payment, err := service.Refund(
		context.Background(),
		paymentID,
		1000,
		"idem-refund-1",
	)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if payment.Status != domain.StatusRefunded {
		t.Errorf("expected status REFUNDED, got %s", payment.Status)
	}
	if payment.BankRefundID == nil || *payment.BankRefundID != "ref-123" {
		t.Error("expected bank refund ID set")
	}
}

func TestRefundService_Refund_NotCaptured(t *testing.T) {
	// Setup
	mockRepo := NewMockPaymentRepository()
	service := NewRefundService(mockRepo, &MockBankPort{})

	paymentID := uuid.New()
	mockRepo.payments[paymentID.String()] = &domain.Payment{
		ID:          paymentID,
		Status:      domain.StatusAuthorized, // Not Captured
		AmountCents: 1000,
	}

	// Action
	_, err := service.Refund(
		context.Background(),
		paymentID,
		1000,
		"idem-refund-2",
	)

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
