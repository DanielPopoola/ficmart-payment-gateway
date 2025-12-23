package worker

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/service"
	"github.com/google/uuid"
)

func TestReconciler_StuckPending(t *testing.T) {
	// Setup
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := service.NewMockPaymentRepository()
	mockBank := &service.MockBankPort{}
	
	// Mock Services
	authSvc := service.NewAuthorizationService(mockRepo, mockBank)
	capSvc := service.NewCaptureService(mockRepo, mockBank)
	voidSvc := service.NewVoidService(mockRepo, mockBank)
	refSvc := service.NewRefundService(mockRepo, mockBank)

	paymentID := uuid.New()
	idemKey := "idem-stuck"
	authID := "bank-auth-123"
	
	// Seed stuck PENDING payment that ALREADY has a bank ID (simulating crash after bank call but before DB update)
	mockRepo.CreatePayment(context.Background(), &domain.Payment{
		ID:             paymentID,
		Status:         domain.StatusPending,
		IdempotencyKey: idemKey,
		BankAuthID:     &authID,
		CreatedAt:      time.Now().Add(-2 * time.Minute),
	})
	mockRepo.CreateIdempotencyKey(context.Background(), &domain.IdempotencyKey{
		Key:         idemKey,
		RequestHash: "some-hash",
		LockedAt:    time.Now().Add(-2 * time.Minute),
	})

	// Mock repository FindPendingPayments
	mockRepo.FindPendingPaymentsFn = func(ctx context.Context, olderThan time.Duration, limit int) ([]*domain.PendingPaymentCheck, error) {
		return []*domain.PendingPaymentCheck{
			{ID: paymentID, Status: domain.StatusPending, IdempotencyKey: idemKey},
		}, nil
	}

	reconciler := NewReconciler(mockRepo, mockBank, authSvc, capSvc, voidSvc, refSvc, time.Second, 10, logger)

	// Action
	reconciler.reconcileStuckPayments(context.Background())

	// Assert
	p, _ := mockRepo.FindByID(context.Background(), paymentID)
	if p.Status != domain.StatusAuthorized {
		t.Errorf("expected status AUTHORIZED, got %s", p.Status)
	}
}

func TestReconciler_LazyExpiration(t *testing.T) {
	// Setup
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockRepo := service.NewMockPaymentRepository()
	mockBank := &service.MockBankPort{}
	
	authSvc := service.NewAuthorizationService(mockRepo, mockBank)

	paymentID := uuid.New()
	authID := "auth-expiring"
	
	// Seed AUTHORIZED payment that is past its local expiry
	expiry := time.Now().Add(-10 * time.Minute)
	mockRepo.CreatePayment(context.Background(), &domain.Payment{
		ID:           paymentID,
		Status:       domain.StatusAuthorized,
		BankAuthID:   &authID,
		ExpiresAt:    &expiry,
		CreatedAt:    time.Now().Add(-8 * 24 * time.Hour),
	})

	// Mock FindPendingPayments to return this payment for check
	mockRepo.FindPendingPaymentsFn = func(ctx context.Context, olderThan time.Duration, limit int) ([]*domain.PendingPaymentCheck, error) {
		return []*domain.PendingPaymentCheck{
			{ID: paymentID, Status: domain.StatusAuthorized, BankAuthID: &authID},
		}, nil
	}

	// Correctly mock GetAuthorization to return expired response
	mockBank.GetAuthorizationFn = func(ctx context.Context, id string) (*domain.BankAuthorizationResponse, error) {
		return &domain.BankAuthorizationResponse{
			AuthorizationID: id,
			ExpiresAt:       time.Now().Add(-1 * time.Hour), // Expired
		}, nil
	}

	reconciler := NewReconciler(mockRepo, mockBank, authSvc, nil, nil, nil, time.Second, 10, logger)

	// Action
	reconciler.reconcileStuckPayments(context.Background())

	// Assert
	p, _ := mockRepo.FindByID(context.Background(), paymentID)
	if p.Status != domain.StatusExpired {
		t.Errorf("expected status EXPIRED, got %s", p.Status)
	}
}
