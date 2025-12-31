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
	"github.com/jackc/pgx/v5/pgxpool"
)

type RefundService struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	bankClient      application.BankClient
	db              *pgxpool.Pool
}

func NewRefundService(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	bankClient application.BankClient,
	db *pgxpool.Pool,
) *RefundService {
	return &RefundService{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
		db:              db,
	}
}

func (s *RefundService) Refund(ctx context.Context, cmd RefundCommand, idempotencyKey string) (*domain.Payment, error) {
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
		if payment != nil && payment.Status != domain.StatusCaptured && payment.Status != domain.StatusRefunding {
			return nil, application.NewDuplicateBusinessRequestError(existingHash.PaymentID, existingHash.Key)
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, application.NewInternalError(err)
	}
	defer tx.Rollback(ctx)

	payment, err := s.paymentRepo.FindByIDForUpdate(ctx, tx, cmd.PaymentID)
	if err != nil {
		return nil, application.NewInternalError(err)
	}

	if err := s.idempotencyRepo.AcquireLock(ctx, tx, idempotencyKey, cmd.PaymentID, requestHash); err != nil {
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			tx.Rollback(ctx)
			return s.waitForCompletion(ctx, idempotencyKey, cmd)
		}
		return nil, application.NewInternalError(err)
	}

	if err := payment.MarkRefunding(); err != nil {
		return nil, application.NewInternalError(err)
	}

	if err := s.paymentRepo.Update(ctx, tx, payment); err != nil {
		return nil, application.NewInternalError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, application.NewInternalError(err)
	}

	bankReq := application.BankRefundRequest{
		Amount:    cmd.Amount,
		CaptureID: *payment.BankCaptureID,
	}

	bankResp, err := s.bankClient.Refund(ctx, bankReq, idempotencyKey)
	if err != nil {
		category := application.CategorizeError(err)
		if category == application.CategoryPermanent {
			if failErr := payment.FailWithCategory(string(category)); failErr != nil {
				return nil, application.NewInternalError(failErr)
			}

			tx, err := s.db.Begin(ctx)
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
		return payment, err
	}

	tx, err = s.db.Begin(ctx)
	if err != nil {
		return payment, application.NewInternalError(err)
	}
	defer tx.Rollback(ctx)

	if err := payment.Refund(bankResp.RefundID, bankResp.RefundedAt); err != nil {
		return nil, application.NewInternalError(err)
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
		return payment, application.NewInternalError(err)
	}
	return payment, nil
}

func (s *RefundService) waitForCompletion(ctx context.Context, idempotencyKey string, cmd RefundCommand) (*domain.Payment, error) {
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

func (s *RefundService) computeRequestHash(cmd RefundCommand) string {
	data := fmt.Sprintf("%+v", cmd)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
