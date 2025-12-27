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
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
)

// withIdempotency handles the boilerplate for idempotent operations
func (s *PaymentService) withIdempotency(
	ctx context.Context,
	idempotencyKey string,
	paymentID string,
	cmd any,
	fn func() (*domain.Payment, any, error),
) (*domain.Payment, error) {
	requestHash := s.computeRequestHash(cmd)

	// Check 1: Same idempotency key
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

	// Check 2: Same business request, different key (client bug detection)
	existingHash, err := s.idempotencyRepo.FindByRequestHash(ctx, requestHash)
	if err == nil && existingHash.Key != idempotencyKey {
		payment, _ := s.paymentRepo.FindByID(ctx, existingHash.PaymentID)
		if payment.Status() != domain.StatusPending {
			return nil, application.NewDuplicateBusinessRequestError(
				existingHash.PaymentID, existingHash.Key,
			)
		}
		s.logger.Warn("duplicate_business_request_after_failure",
			"original_key", existingHash.Key,
			"new_key", idempotencyKey)
	}

	if err := s.idempotencyRepo.AcquireLock(ctx, idempotencyKey, paymentID, requestHash); err != nil {
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			return s.waitForCompletion(ctx, idempotencyKey, paymentID)
		}
		return nil, application.NewInternalError(err)
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

	category := application.CategorizeError(err)

	if category == application.CategoryTransient || category == application.CategoryInfrastructure {
		// Retryable error - keep lock
		s.logger.Info("retryable error, keeping lock",
			"payment_id", paymentID,
			"category", category,
			"error", err)
		return payment, err
	}

	if payment != nil && !payment.IsTerminal() {
		payment.Fail()
		if updateErr := s.paymentRepo.Update(ctx, payment); updateErr != nil {
			s.logger.Error("failed to mark payment as failed", "error", updateErr)
		}
	}

	errorPayload, _ := json.Marshal(map[string]string{
		"error":    application.ToErrorCode(err),
		"message":  err.Error(),
		"category": string(category),
	})
	s.idempotencyRepo.StoreResponse(ctx, idempotencyKey, errorPayload, 400)
	s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "completed")
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
			return nil, application.NewTimeoutError(paymentID)
		case <-ticker.C:
			key, err := s.idempotencyRepo.FindByKey(ctx, idempotencyKey)
			if err != nil {
				return nil, application.NewInternalError(err)
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
