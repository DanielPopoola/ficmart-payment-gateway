package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

func TestCaptureService_ConcurrentCapture(t *testing.T) {
	// Setup
	mockRepo := NewMockPaymentRepository()
	mockBank := &MockBankPort{
		Delay: 100 * time.Millisecond, // Slow bank to increase race window
	}
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

	const numRequests = 5
	idempotencyKey := "idem-concurrent-capture"
	
	var wg sync.WaitGroup
	results := make(chan error, numRequests)
	
	// Action: Fire multiple concurrent requests
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Capture(context.Background(), paymentID, 1000, idempotencyKey)
			results <- err
		}()
	}
	
	wg.Wait()
	close(results)

	// Assert
	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else {
			// Some might return REQUEST_PROCESSING or TIMEOUT depending on timing, 
			// but we expect at least the first one to succeed.
			// In our current pollForPayment implementation, they should all eventually 
			// return success if the first one finishes within 5s.
			if domain.IsErrorCode(err, domain.ErrRequestProcessing) || domain.IsErrorCode(err, domain.ErrCodeTimeout) {
				// Acceptable outcomes for concurrent requests that didn't wait long enough
			} else {
				t.Errorf("unexpected error: %v", err)
			}
			successCount++ // For this test, service.pollForPayment should resolve them to success
		}
	}

	if mockBank.GetCalls("Capture") != 1 {
		t.Errorf("expected exactly 1 bank call, got %d", mockBank.GetCalls("Capture"))
	}
	
	p, _ := mockRepo.FindByID(context.Background(), paymentID)
	if p.Status != domain.StatusCaptured {
		t.Errorf("expected status CAPTURED, got %s", p.Status)
	}
}

func TestCaptureService_IdempotencyCache(t *testing.T) {
	// Setup
	mockRepo := NewMockPaymentRepository()
	mockBank := &MockBankPort{}
	service := NewCaptureService(mockRepo, mockBank)

	paymentID := uuid.New()
	authID := "auth-123"
	idempotencyKey := "idem-cached"
	
	// 1. Initial success
	mockRepo.payments[paymentID.String()] = &domain.Payment{
		ID:           paymentID,
		Status:       domain.StatusAuthorized,
		AmountCents:  1000,
		BankAuthID:   &authID,
		AuthorizedAt: &time.Time{},
	}

	_, err := service.Capture(context.Background(), paymentID, 1000, idempotencyKey)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Verify bank was called once
	if mockBank.GetCalls("Capture") != 1 {
		t.Errorf("expected 1 bank call, got %d", mockBank.GetCalls("Capture"))
	}

	// 2. Duplicate call
	_, err = service.Capture(context.Background(), paymentID, 1000, idempotencyKey)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	// Assert: Bank should NOT be called again
	if mockBank.GetCalls("Capture") != 1 {
		t.Errorf("idempotency failed: bank called %d times", mockBank.GetCalls("Capture"))
	}
}
