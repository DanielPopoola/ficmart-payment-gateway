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

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/repository"
	"github.com/google/uuid"
)

type AuthorizationService struct {
	repo       repository.PaymentRepository
	bankClient bank.BankPort
}

func NewAuthorizationService(repo repository.PaymentRepository, bankClient bank.BankPort) *AuthorizationService {
	return &AuthorizationService{
		repo:       repo,
		bankClient: bankClient,
	}
}

// Authorize processes a payment authorization request for a given order and customer.
func (s *AuthorizationService) Authorize(
	ctx context.Context,
	orderID, customerID, idempotencyKey string,
	amount int64,
	cardNumber, cvv string,
	expiryMonth, expiryYear int,
) (*domain.Payment, error) {
	if err := s.validate(orderID, customerID, amount); err != nil {
		return nil, err
	}

	hashInput := fmt.Sprintf("%s|%d|%s", orderID, amount, customerID)
	hashBytes := sha256.Sum256([]byte(hashInput))
	requestHash := hex.EncodeToString(hashBytes[:])

	paymentID := uuid.New()

	err := s.repo.WithTx(ctx, func(txRepo repository.PaymentRepository) error {
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

		payment := &domain.Payment{
			ID:             paymentID,
			OrderID:        orderID,
			CustomerID:     customerID,
			AmountCents:    amount,
			Currency:       "USD",
			Status:         domain.StatusPending,
			IdempotencyKey: idempotencyKey,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		return txRepo.CreatePayment(ctx, payment)
	})

	if err != nil {
		if domain.IsErrorCode(err, domain.ErrRequestProcessing) {
			existingKey, fetchErr := s.repo.FindIdempotencyKeyRecord(ctx, idempotencyKey)
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
				return s.repo.FindByIdempotencyKey(ctx, idempotencyKey)
			}

			return s.pollForPayment(ctx, idempotencyKey)
		}
		return nil, err
	}

	bankReq := bank.AuthorizationRequest{
		Amount:      amount,
		CardNumber:  cardNumber,
		Cvv:         cvv,
		ExpiryMonth: expiryMonth,
		ExpiryYear:  expiryYear,
	}

	bankResp, bankErr := s.bankClient.Authorize(ctx, bankReq, idempotencyKey)

	updateErr := s.repo.WithTx(ctx, func(txRepo repository.PaymentRepository) error {
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
			if err := p.Authorize(bankResp.AuthorizationID, bankResp.CreatedAt, bankResp.ExpiresAt); err != nil {
				return err
			}

			// Cache the response
			respJSON, _ := json.Marshal(bankResp)
			statusCode := 201
			idemKey := &domain.IdempotencyKey{
				Key:             idempotencyKey,
				ResponsePayload: respJSON,
				StatusCode:      &statusCode,
				CompletedAt:     &bankResp.CreatedAt,
			}
			if err := txRepo.UpdateIdempotencyKey(ctx, idemKey); err != nil {
				return err
			}
		}
		return txRepo.UpdatePayment(ctx, p)
	})

	if updateErr != nil {
		payment, fetchErr := s.repo.FindByID(ctx, paymentID)
		if fetchErr != nil {
			return &domain.Payment{
				ID:     paymentID,
				Status: domain.StatusPending,
			}, nil
		}
		return payment, nil
	}

	return s.repo.FindByID(ctx, paymentID)
}

// pollForPayment polls the repository for a payment status update until the payment is no longer pending,
// the context is cancelled, or a 5-second timeout is reached.
func (s *AuthorizationService) pollForPayment(ctx context.Context, key string) (*domain.Payment, error) {
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
			p, err := s.repo.FindByIdempotencyKey(ctx, key)
			if err != nil {
				if errors.Is(err, domain.ErrPaymentNotFound) {
					continue
				}
				return nil, fmt.Errorf("error checking payment status: %w", err)
			}
			if p != nil && p.Status != domain.StatusPending {
				return p, nil
			}
		}
	}
}

func (s *AuthorizationService) validate(
	orderID, customerID string,
	amount int64,
) error {
	if orderID == "" {
		return domain.NewMissingRequiredFieldError("order_id")
	}
	if customerID == "" {
		return domain.NewMissingRequiredFieldError("customer_id")
	}
	if amount <= 0 {
		return domain.NewInvalidAmountError(amount)
	}
	return nil
}

func (s *AuthorizationService) Reconcile(ctx context.Context, p *domain.Payment) error {
	if p.BankAuthID == nil {
		return nil
	}

	bankResp, err := s.bankClient.GetAuthorization(ctx, *p.BankAuthID)
	if err != nil {
		return err
	}

	return s.repo.WithTx(ctx, func(txRepo repository.PaymentRepository) error {
		payment, err := txRepo.FindByIDForUpdate(ctx, p.ID)
		if err != nil {
			return err
		}

		if payment.Status != domain.StatusPending {
			return nil
		}

		if err := payment.Authorize(bankResp.AuthorizationID, bankResp.CreatedAt, bankResp.ExpiresAt); err != nil {
			return err
		}

		return txRepo.UpdatePayment(ctx, payment)
	})
}
