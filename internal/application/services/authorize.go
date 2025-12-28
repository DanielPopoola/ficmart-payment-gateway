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
	"github.com/google/uuid"
)

type AuthorizeService struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	coordinator     *postgres.TransactionCoordinator
	bankClient      application.BankClient
}

func NewAuthorizeService(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	coordinator *postgres.TransactionCoordinator,
	bankClient application.BankClient,
) *AuthorizeService {
	return &AuthorizeService{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		coordinator:     coordinator,
		bankClient:      bankClient,
	}
}

func (s *AuthorizeService) Authorize(ctx context.Context, cmd AuthorizeCommand, idempotencyKey string) (*domain.Payment, error) {
	requestHash := s.computeRequestHash(cmd)

	existingKey, err := s.idempotencyRepo.FindByKey(ctx, idempotencyKey)
	if err == nil {
		if existingKey.RequestHash != requestHash {
			return nil, application.NewIdempotencyMismatchError()
		}

		if existingKey.ResponsePayload != nil {
			payment, _ := s.paymentRepo.FindByID(ctx, existingKey.PaymentID)
			return payment, nil
		}
		return s.waitForCompletion(ctx, idempotencyKey, cmd)
	}

	existingHash, err := s.idempotencyRepo.FindByRequestHash(ctx, requestHash)
	if err == nil && existingHash.Key != idempotencyKey {
		payment, _ := s.paymentRepo.FindByID(ctx, existingHash.PaymentID)
		if payment != nil && payment.Status() != domain.StatusPending {
			return nil, application.NewDuplicateBusinessRequestError(existingHash.PaymentID, existingHash.Key)
		}
	}

	paymentID := uuid.New().String()
	var payment *domain.Payment

	err = s.coordinator.WithTransaction(ctx, func(ctx context.Context, txPaymentRepo *postgres.PaymentRepository, txIdempotencyRepo *postgres.IdempotencyRepository) error {
		money, err := domain.NewMoney(cmd.Amount, cmd.Currency)
		if err != nil {
			return err
		}

		payment, err = domain.NewPayment(paymentID, cmd.OrderID, cmd.CustomerID, money)
		if err != nil {
			return err
		}

		if err := txPaymentRepo.Create(ctx, payment); err != nil {
			return err
		}

		if err := txIdempotencyRepo.AcquireLock(ctx, idempotencyKey, paymentID, requestHash); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, postgres.ErrIdempotencyMismatch) {
			return nil, application.NewIdempotencyMismatchError()
		}
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			return s.waitForCompletion(ctx, idempotencyKey, cmd)
		}
		return nil, application.NewInternalError(err)
	}

	bankReq := application.BankAuthorizationRequest{
		Amount:      cmd.Amount,
		CardNumber:  cmd.CardNumber,
		Cvv:         cmd.CVV,
		ExpiryMonth: cmd.ExpiryMonth,
		ExpiryYear:  cmd.ExpiryYear,
	}

	s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "CALLING_BANK")
	bankResp, err := s.bankClient.Authorize(ctx, bankReq, idempotencyKey)
	if err != nil {
		s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "BANK_FAILED")

		if err := s.idempotencyRepo.ReleaseLock(ctx, idempotencyKey); err != nil {
			return payment, err
		}
		return payment, err
	}

	s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "BANK_RESPONDED")

	err = s.coordinator.WithTransaction(ctx, func(ctx context.Context, txPaymentRepo *postgres.PaymentRepository, txIdempotencyRepo *postgres.IdempotencyRepository) error {
		if err := payment.Authorize(bankResp.AuthorizationID, bankResp.CreatedAt, bankResp.ExpiresAt); err != nil {
			return err
		}

		if err := txPaymentRepo.Update(ctx, payment); err != nil {
			return err
		}

		responsePayload, _ := json.Marshal(bankResp)
		if err = txIdempotencyRepo.StoreResponse(ctx, idempotencyKey, responsePayload); err != nil {
			return err
		}

		if err := txIdempotencyRepo.ReleaseLock(ctx, idempotencyKey); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, application.NewInternalError(err)
	}

	return payment, nil
}

func (s *AuthorizeService) waitForCompletion(ctx context.Context, idempotencyKey string, cmd AuthorizeCommand) (*domain.Payment, error) {
	requestHash := s.computeRequestHash(cmd)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, application.NewTimeoutError("")
		case <-ticker.C:
			key, err := s.idempotencyRepo.FindByKey(ctx, idempotencyKey)
			if err != nil {
				return nil, application.NewInternalError(err)
			}

			if key.RequestHash != requestHash {
				return nil, application.NewIdempotencyMismatchError()
			}

			if key.LockedAt == nil {
				payment, err := s.paymentRepo.FindByID(ctx, key.PaymentID)
				if err != nil {
					return nil, err
				}
				return payment, nil
			}

			if time.Since(*key.LockedAt) > 5*time.Minute {
				return nil, application.NewRequestProcessingError()
			}
		}
	}
}

func (s *AuthorizeService) computeRequestHash(cmd AuthorizeCommand) string {
	data := fmt.Sprintf("%+v", cmd)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
