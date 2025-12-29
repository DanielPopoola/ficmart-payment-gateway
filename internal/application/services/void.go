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

type VoidService struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	coordinator     *postgres.TransactionCoordinator
	bankClient      application.BankClient
}

func NewVoidService(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	coordinator *postgres.TransactionCoordinator,
	bankClient application.BankClient,
) *VoidService {
	return &VoidService{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		coordinator:     coordinator,
		bankClient:      bankClient,
	}
}

func (s *VoidService) Void(ctx context.Context, cmd VoidCommand, idempotencyKey string) (*domain.Payment, error) {
	requestHash := s.computeRequestHash(cmd)

	existingKey, err := s.idempotencyRepo.FindByKey(ctx, idempotencyKey)
	if err == nil {
		if existingKey.ResponsePayload != nil {
			payment, _ := s.paymentRepo.FindByID(ctx, existingKey.PaymentID)
			return payment, nil
		}
		return s.waitForCompletion(ctx, idempotencyKey, existingKey.PaymentID)
	}

	existingHash, err := s.idempotencyRepo.FindByRequestHash(ctx, requestHash)
	if err == nil && existingHash.Key != idempotencyKey {
		payment, _ := s.paymentRepo.FindByID(ctx, existingHash.PaymentID)
		if payment != nil && payment.Status() != domain.StatusAuthorized {
			return nil, application.NewDuplicateBusinessRequestError(existingHash.PaymentID, existingHash.Key)
		}
	}

	var payment *domain.Payment
	err = s.coordinator.WithTransaction(ctx, func(ctx context.Context, txPaymentRepo *postgres.PaymentRepository, txIdempotencyRepo *postgres.IdempotencyRepository) error {
		if err := txIdempotencyRepo.AcquireLock(ctx, idempotencyKey, cmd.PaymentID, requestHash); err != nil {
			if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
				return err
			}
			return err
		}

		var err error
		payment, err = txPaymentRepo.FindByIDForUpdate(ctx, cmd.PaymentID)
		if err != nil {
			return err
		}

		if err := payment.MarkVoiding(); err != nil {
			return err
		}

		if err := txPaymentRepo.Update(ctx, payment); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			return s.waitForCompletion(ctx, idempotencyKey, cmd.PaymentID)
		}
		return nil, application.NewInternalError(err)
	}

	bankReq := application.BankVoidRequest{
		AuthorizationID: *payment.BankAuthID(),
	}

	bankResp, err := s.bankClient.Void(ctx, bankReq, idempotencyKey)

	if err != nil {
		return payment, err
	}

	err = s.coordinator.WithTransaction(ctx, func(ctx context.Context, txPaymentRepo *postgres.PaymentRepository, txIdempotencyRepo *postgres.IdempotencyRepository) error {
		if err := payment.Void(bankResp.VoidID, bankResp.VoidedAt); err != nil {
			return err
		}

		if err := txPaymentRepo.Update(ctx, payment); err != nil {
			return err
		}

		responsePayload, _ := json.Marshal(bankResp)
		if err := txIdempotencyRepo.StoreResponse(ctx, idempotencyKey, responsePayload); err != nil {
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

func (s *VoidService) waitForCompletion(ctx context.Context, idempotencyKey string, paymentID string) (*domain.Payment, error) {
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
				return nil, application.NewRequestProcessingError()
			}
		}
	}
}

func (s *VoidService) computeRequestHash(cmd VoidCommand) string {
	data := fmt.Sprintf("%+v", cmd)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
