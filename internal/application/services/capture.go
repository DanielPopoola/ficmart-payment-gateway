package services

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
	"github.com/jackc/pgx/v5"
)

type CaptureService struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	bankClient      bank.BankClient
	db              *postgres.DB
}

func NewCaptureService(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	bankClient bank.BankClient,
	db *postgres.DB,
) *CaptureService {
	return &CaptureService{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
		db:              db,
	}
}

func (s *CaptureService) Capture(ctx context.Context, cmd CaptureCommand, idempotencyKey string) (*domain.Payment, error) {
	requestHash := ComputeHash(cmd)

	existingKey, err := s.idempotencyRepo.FindByKey(ctx, idempotencyKey)
	if err == nil {
		if existingKey.RequestHash != requestHash {
			return nil, application.NewIdempotencyMismatchError()
		}

		if existingKey.LockedAt != nil {
			payment, err := s.paymentRepo.FindByID(ctx, existingKey.PaymentID)
			if err != nil {
				return nil, application.NewInternalError(err)
			}
			return payment, nil
		}

		return s.waitForCompletion(ctx, idempotencyKey)
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})

	if err != nil {
		return nil, application.NewInternalError(err)
	}
	defer tx.Rollback(ctx)

	payment, err := s.paymentRepo.FindByIDForUpdate(ctx, tx, cmd.PaymentID)
	if err != nil {
		if errors.Is(err, domain.ErrAmountMismatch) {
			return nil, application.NewInvalidInputError(err)
		}
		return nil, application.NewInternalError(err)
	}

	if err := s.idempotencyRepo.AcquireLock(ctx, tx, idempotencyKey, cmd.PaymentID, requestHash); err != nil {
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			tx.Rollback(ctx)
			return s.waitForCompletion(ctx, idempotencyKey)
		}
		return nil, application.NewInternalError(err)
	}

	if err := payment.MarkCapturing(); err != nil {
		return nil, application.NewInvalidTransitionError(err)
	}

	if err := s.paymentRepo.Update(ctx, tx, payment); err != nil {
		return nil, application.NewInternalError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, application.NewInternalError(err)
	}

	bankReq := bank.CaptureRequest{
		Amount:          cmd.Amount,
		AuthorizationID: *payment.BankAuthID,
	}

	bankResp, err := s.bankClient.Capture(ctx, bankReq, idempotencyKey)
	if err != nil {
		category := application.CategorizeError(err)
		if category == application.CategoryPermanent {
			if failErr := payment.Fail(); failErr != nil {
				return nil, application.NewInvalidTransitionError(err)
			}

			tx, err := s.db.BeginTx(ctx, pgx.TxOptions{
				IsoLevel: pgx.Serializable,
			})

			if err != nil {
				return nil, application.NewInternalError(err)
			}
			defer tx.Rollback(ctx)

			if updateErr := s.paymentRepo.Update(ctx, tx, payment); updateErr != nil {
				return nil, application.NewInternalError(updateErr)
			}
			responsePayload, _ := json.Marshal(err)
			if storeErr := s.idempotencyRepo.StoreResponse(ctx, tx, idempotencyKey, responsePayload); storeErr != nil {
				return nil, application.NewInternalError(storeErr)
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, application.NewInternalError(err)
			}
		}
		return payment, err // Return payment object in current state
	}

	tx, err = s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return payment, application.NewInternalError(err)
	}
	defer tx.Rollback(ctx)

	if err := payment.Capture(bankResp.Status, bankResp.CaptureID, bankResp.CapturedAt); err != nil {
		return nil, application.NewInvalidStateError(err)
	}

	if err := s.paymentRepo.Update(ctx, tx, payment); err != nil {
		return nil, application.NewInternalError(err)
	}

	responsePayload, _ := json.Marshal(bankResp)
	if err := s.idempotencyRepo.StoreResponse(ctx, tx, idempotencyKey, responsePayload); err != nil {
		return nil, application.NewInternalError(err)
	}

	if err := s.idempotencyRepo.ReleaseLock(ctx, tx, idempotencyKey); err != nil {
		return nil, application.NewInternalError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, application.NewInternalError(err)
	}

	return payment, nil
}

func (s *CaptureService) waitForCompletion(ctx context.Context, idempotencyKey string) (*domain.Payment, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, application.NewTimeoutError("")
		case <-timeout:
			return nil, application.NewTimeoutError("")
		case <-ticker.C:
			key, err := s.idempotencyRepo.FindByKey(ctx, idempotencyKey)
			if err != nil {
				return nil, application.NewInternalError(err)
			}

			if key.LockedAt == nil {
				payment, err := s.paymentRepo.FindByID(ctx, key.PaymentID)
				if err != nil {
					return nil, application.NewInternalError(err)
				}
				return payment, nil
			}

			if time.Since(*key.LockedAt) > 5*time.Minute {
				return nil, application.NewRequestProcessingError()
			}
		}
	}
}
