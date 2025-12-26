package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

// withIdempotency handles the boilerplate for idempotent operations
func (s *PaymentService) withIdempotency(
	ctx context.Context,
	idempotencyKey string,
	paymentID string,
	cmd any,
	fn func() (*domain.Payment, any, error),
) (*domain.Payment, error) {
	existingKey, err := s.idempotencyRepo.FindByKey(ctx, idempotencyKey)
	if err == nil {
		if existingKey.ResponsePayload != nil {
			payment, err := s.paymentRepo.FindByID(ctx, existingKey.PaymentID)
			if err != nil {
				return nil, fmt.Errorf("payment not found: %w", err)
			}
			return payment, nil
		}

		return s.waitForCompletion(ctx, idempotencyKey, existingKey.PaymentID)
	}

	requestHash := s.computeRequestHash(cmd)
	if err := s.idempotencyRepo.AcquireLock(ctx, idempotencyKey, paymentID, requestHash); err != nil {
		if errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
			return s.waitForCompletion(ctx, idempotencyKey, paymentID)
		}
		return nil, err
	}

	payment, bankResp, err := fn()

	if err == nil {
		responsePayload, _ := json.Marshal(bankResp)
		if storeErr := s.idempotencyRepo.StoreResponse(ctx, idempotencyKey, responsePayload, 200); storeErr != nil {
			s.logger.Warn("failed to store response", "error", storeErr)
		}

		if releaseErr := s.idempotencyRepo.ReleaseLock(ctx, idempotencyKey); releaseErr != nil {
			s.logger.Error("failed to release lock", "error", releaseErr)
		}

		return payment, nil
	}

	if isRetryableError(err) {
		s.logger.Info("retryable error, keeping lock",
			"payment_id", paymentID,
			"error", err)
		return payment, err
	}

	if payment != nil && !payment.IsTerminal() {
		payment.Fail()
		if updateErr := s.paymentRepo.Update(ctx, payment); updateErr != nil {
			s.logger.Error("failed to mark payment as failed", "error", updateErr)
		}
	}

	errorPayload, _ := json.Marshal(map[string]string{"error": err.Error()})
	s.idempotencyRepo.StoreResponse(ctx, idempotencyKey, errorPayload, 400)
	s.idempotencyRepo.ReleaseLock(ctx, idempotencyKey)

	return payment, err
}

func (s *PaymentService) waitForCompletion(ctx context.Context, idempotencyKey string, paymentID string) (*domain.Payment, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for idempotent request to complete")
		case <-ticker.C:
			key, err := s.idempotencyRepo.FindByKey(ctx, idempotencyKey)
			if err != nil {
				return nil, err
			}
			if key.LockedAt == nil {
				payment, err := s.paymentRepo.FindByID(ctx, paymentID)
				if err != nil {
					return nil, err
				}
				return payment, nil
			}

			if time.Since(*key.LockedAt) > 5*time.Minute {
				return nil, fmt.Errorf("idempotent request appears stuck")
			}
		}
	}
}

func isRetryableError(err error) bool {
	var bankErr *application.BankError
	if errors.As(err, &bankErr) {
		return bankErr.IsRetryable()
	}
	// Network errors, timeouts, etc.
	return true
}

func (s *PaymentService) computeRequestHash(cmd interface{}) string {
	data := fmt.Sprintf("%+v", cmd)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
