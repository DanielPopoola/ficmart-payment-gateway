package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
	"github.com/google/uuid"
)

type CaptureService struct {
	repo       ports.PaymentRepository
	bankClient ports.BankPort
}

func NewCaptureService(repo ports.PaymentRepository, bankClient ports.BankPort) *CaptureService {
	return &CaptureService{
		repo:       repo,
		bankClient: bankClient,
	}
}

func (c *CaptureService) Capture(ctx context.Context, paymentID uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error) {
	if err := c.validate(amount); err != nil {
		return nil, err
	}

	hashInput := fmt.Sprintf("%s|%d", paymentID.String(), amount)
	hashBytes := sha256.Sum256([]byte(hashInput))
	requestHash := hex.EncodeToString(hashBytes[:])

	var payment *domain.Payment
	err := c.repo.WithTx(ctx, func(txRepo ports.PaymentRepository) error {
		idemKey := &domain.IdempotencyKey{
			Key:         idempotencyKey,
			RequestHash: requestHash,
			LockedAt:    time.Now(),
		}

		err := txRepo.CreateIdempotencyKey(ctx, idemKey)
		if err != nil {
			if domain.IsErrorCode(err, domain.ErrCodeDuplicateIdempotencyKey) {
				return domain.NewRequestProcessingError()
			}
			return err
		}

		p, err := txRepo.FindByIDForUpdate(ctx, paymentID)
		if err != nil {
			return err
		}
		if p.AmountCents != amount {
			return domain.NewAmountMismatchError(p.AmountCents, amount)
		}

		if p.Status != domain.StatusAuthorized {
			return domain.NewInvalidStateError(string(p.Status), string(domain.StatusAuthorized))
		}
		if p.BankAuthID == nil {
			return domain.NewMissingDependencyError("BankAuthID")
		}

		p.Status = domain.StatusCapturing
		if err := txRepo.UpdatePayment(ctx, p); err != nil {
			return err
		}

		payment = p
		return nil
	})

	if err != nil {
		if domain.IsErrorCode(err, domain.ErrRequestProcessing) {
			existingKey, fetchErr := c.repo.FindIdempotencyKeyRecord(ctx, idempotencyKey)
			if fetchErr != nil {
				return nil, fmt.Errorf("failed to check existing idempotency key: %w", fetchErr)
			}

			if existingKey == nil {
				return nil, fmt.Errorf("unexpected state: duplicate key error but key not found")
			}

			if existingKey.RequestHash != requestHash {
				return nil, domain.NewIdempotencyMismatchError()
			}

			if existingKey.CompletedAt != nil && existingKey.ResponsePayload != nil {
				return c.repo.FindByIdempotencyKey(ctx, idempotencyKey)
			}

			return c.pollForPayment(ctx, idempotencyKey)
		}
		return nil, err
	}

	bankReq := domain.BankCaptureRequest{
		Amount:          amount,
		AuthorizationID: *payment.BankAuthID,
	}

	bankResp, bankErr := c.bankClient.Capture(ctx, bankReq, idempotencyKey)

	updateErr := c.repo.WithTx(ctx, func(txRepo ports.PaymentRepository) error {
		p, err := txRepo.FindByIDForUpdate(ctx, paymentID)
		if err != nil {
			return err
		}

		if bankErr != nil {
			errMsg := bankErr.Error()
			isRetryable := false
			var retryableErr domain.Retryable
			if errors.As(bankErr, &retryableErr) {
				isRetryable = retryableErr.IsRetryable()
			} else if errors.Is(bankErr, context.DeadlineExceeded) {
				isRetryable = true
			}

			if isRetryable {
				baseDelay := math.Pow(2, float64(p.AttemptCount+1)) * float64(time.Minute)
				maxDelay := float64(24 * time.Hour)
				if baseDelay > maxDelay {
					baseDelay = maxDelay
				}

				jitter := rand.Int63n(1000)
				nextRetry := time.Now().Add(time.Duration(baseDelay) + time.Duration(jitter)*time.Millisecond)
				p.ScheduleRetry(errMsg, nextRetry)
			} else {
				if err := p.Fail(errMsg); err != nil {
					return err
				}
			}
		} else {
			if err := p.Capture(bankResp.CaptureID, bankResp.CapturedAt); err != nil {
				return err
			}

			// Cache the response
			respJSON, _ := json.Marshal(bankResp)
			idemKey := &domain.IdempotencyKey{
				Key:             idempotencyKey,
				ResponsePayload: respJSON,
				StatusCode:      200,
				CompletedAt:     &bankResp.CapturedAt,
			}
			if err := txRepo.UpdateIdempotencyKey(ctx, idemKey); err != nil {
				return err
			}
		}
		return txRepo.UpdatePayment(ctx, p)
	})

	if updateErr != nil {
		payment, fetchErr := c.repo.FindByID(ctx, paymentID)
		if fetchErr != nil {
			return &domain.Payment{
				ID:     paymentID,
				Status: domain.StatusCapturing,
			}, nil
		}
		return payment, nil
	}

	return c.repo.FindByID(ctx, paymentID)

}

func (c *CaptureService) pollForPayment(ctx context.Context, key string) (*domain.Payment, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(5 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, domain.NewTimeoutError("payment processing")
		case <-ticker.C:
			p, err := c.repo.FindByIdempotencyKey(ctx, key)
			if err != nil {
				if domain.IsErrorCode(err, domain.ErrCodePaymentNotFound) {
					continue
				}
				return nil, fmt.Errorf("error checking payment status: %w", err)
			}
			if p != nil && p.Status != domain.StatusCapturing {
				return p, nil
			}
		}
	}
}

func (c *CaptureService) validate(amount int64) error {
	if amount < 0 {
		return domain.NewInvalidAmountError(amount)
	}
	return nil
}

func (c *CaptureService) Reconcile(ctx context.Context, p *domain.Payment) error {
	if p.BankCaptureID == nil {
		return nil
	}

	bankResp, err := c.bankClient.GetCapture(ctx, *p.BankCaptureID)
	if err != nil {
		return err
	}

	return c.repo.WithTx(ctx, func(txRepo ports.PaymentRepository) error {
		payment, err := txRepo.FindByIDForUpdate(ctx, p.ID)
		if err != nil {
			return err
		}

		if payment.Status != domain.StatusCapturing {
			return nil
		}

		if err := payment.Capture(bankResp.CaptureID, bankResp.CapturedAt); err != nil {
			return err
		}

		return txRepo.UpdatePayment(ctx, payment)
	})
}
