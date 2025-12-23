package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
	"github.com/google/uuid"
)

type VoidService struct {
	repo       ports.PaymentRepository
	bankClient ports.BankPort
}

func NewVoidService(repo ports.PaymentRepository, bankClient ports.BankPort) *VoidService {
	return &VoidService{
		repo:       repo,
		bankClient: bankClient,
	}
}

func (v *VoidService) Void(ctx context.Context, paymentID uuid.UUID, idempotencyKey string) (*domain.Payment, error) {

	hashInput := fmt.Sprintf("%s", paymentID.String())
	hashBytes := sha256.Sum256([]byte(hashInput))
	requestHash := hex.EncodeToString(hashBytes[:])

	var payment *domain.Payment
	err := v.repo.WithTx(ctx, func(txRepo ports.PaymentRepository) error {
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
		if p.Status != domain.StatusAuthorized {
			return fmt.Errorf("invalid state: payment is %s, expected AUTHORIZED", p.Status)
		}
		if p.BankAuthID == nil {
			return &domain.DomainError{
				Code:    "PAYMENT_PROCESSING",
				Message: "payment authorization still in progress, try again",
			}
		}

		p.Status = domain.StatusVoiding
		p.AttemptCount = 0
		if err := txRepo.UpdatePayment(ctx, p); err != nil {
			return err
		}
		payment = p
		return nil
	})

	if err != nil {
		if domain.IsErrorCode(err, domain.ErrRequestProcessing) {
			existingKey, fetchErr := v.repo.FindIdempotencyKeyRecord(ctx, idempotencyKey)
			if fetchErr != nil {
				return nil, fmt.Errorf("failed to check existing idempotency key: %w", fetchErr)
			}

			if existingKey == nil {
				return nil, fmt.Errorf("unexpected state: duplicate key error but key not found")
			}

			if existingKey.RequestHash != requestHash {
				return nil, fmt.Errorf("idempotency key reused with different parameters")
			}

			return v.pollForPayment(ctx, idempotencyKey)
		}
		return nil, err
	}

	bankReq := &domain.BankVoidRequest{
		AuthorizationID: *payment.BankAuthID,
	}

	bankResp, bankErr := v.bankClient.Void(ctx, *bankReq, idempotencyKey)

	updateErr := v.repo.WithTx(ctx, func(txRepo ports.PaymentRepository) error {
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
				if p.AttemptCount >= 3 {
					if err := p.Fail(errMsg); err != nil {
						return err
					}
				} else {
					baseDelay := math.Pow(2, float64(p.AttemptCount+1)) * float64(time.Minute)
					maxDelay := float64(4 * time.Minute)
					if baseDelay > maxDelay {
						baseDelay = maxDelay
					}

					jitter := rand.Int63n(1000)
					nextRetry := time.Now().Add(time.Duration(baseDelay) + time.Duration(jitter)*time.Millisecond)
					p.ScheduleRetry(errMsg, nextRetry)
				}
			} else {
				if err := p.Fail(errMsg); err != nil {
					return err
				}
			}
		} else {
			if err := p.Void(bankResp.VoidID, bankResp.VoidedAt); err != nil {
				return err
			}
		}
		return txRepo.UpdatePayment(ctx, p)
	})

	if updateErr != nil {
		payment, fetchErr := v.repo.FindByID(ctx, paymentID)
		if fetchErr != nil {
			return &domain.Payment{
				ID:     paymentID,
				Status: domain.StatusVoiding,
			}, nil
		}
		return payment, nil
	}

	return v.repo.FindByID(ctx, paymentID)
}

func (v *VoidService) pollForPayment(ctx context.Context, key string) (*domain.Payment, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(5 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, errors.New("timeout waiting for payment processing")
		case <-ticker.C:
			p, err := v.repo.FindByIdempotencyKey(ctx, key)
			if err != nil {
				if domain.IsErrorCode(err, domain.ErrCodePaymentNotFound) {
					continue
				}
				return nil, fmt.Errorf("error checking payment status: %w", err)
			}
			if p != nil && p.Status != domain.StatusVoiding {
				return p, nil
			}
		}
	}
}
