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
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthorizeService struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	bankClient      application.BankClient
	db              *pgxpool.Pool
}

func NewAuthorizeService(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	bankClient application.BankClient,
	db *pgxpool.Pool,
) *AuthorizeService {
	return &AuthorizeService{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
		db:              db,
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

	// Begin transaction
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Save intent
	paymentID := uuid.New().String()
	var payment *domain.Payment

	money, err := domain.NewMoney(cmd.Amount, cmd.Currency)
	if err != nil {
		return nil, err
	}

	payment, err = domain.NewPayment(paymentID, cmd.OrderID, cmd.CustomerID, money)
	if err != nil {
		return nil, err
	}

	if err := s.paymentRepo.Create(ctx, tx, payment); err != nil {
		return nil, err
	}

	if err := s.idempotencyRepo.AcquireLock(ctx, tx, idempotencyKey, paymentID, requestHash); err != nil {
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			tx.Rollback(ctx)
			return s.waitForCompletion(ctx, idempotencyKey, cmd)
		}
		return nil, application.NewInternalError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Call bank
	bankReq := application.BankAuthorizationRequest{
		Amount:      cmd.Amount,
		CardNumber:  cmd.CardNumber,
		Cvv:         cmd.CVV,
		ExpiryMonth: cmd.ExpiryMonth,
		ExpiryYear:  cmd.ExpiryYear,
	}

	bankResp, err := s.bankClient.Authorize(ctx, bankReq, idempotencyKey)
	if err != nil {
		category := application.CategorizeError(err)
		if category == application.CategoryPermanent {
			payment.Fail()
			s.paymentRepo.Update(ctx, nil, payment)
		}
		responsePayload, _ := json.Marshal(err)
		if storeErr := s.idempotencyRepo.StoreResponse(ctx, nil, idempotencyKey, responsePayload); storeErr != nil {
		}
		return payment, err
	}

	// Save response in transaction
	tx2, err := s.db.Begin(ctx)
	if err != nil {
		return payment, err
	}
	defer tx2.Rollback(ctx)

	if err := payment.Authorize(bankResp.AuthorizationID, bankResp.CreatedAt, bankResp.ExpiresAt); err != nil {
		return payment, err
	}

	if err := s.paymentRepo.Update(ctx, tx2, payment); err != nil {
		return payment, err
	}

	responsePayload, _ := json.Marshal(bankResp)
	if err := s.idempotencyRepo.StoreResponse(ctx, tx2, idempotencyKey, responsePayload); err != nil {
		return payment, err
	}

	if err := s.idempotencyRepo.ReleaseLock(ctx, tx2, idempotencyKey); err != nil {
		return payment, err
	}

	if err := tx2.Commit(ctx); err != nil {
		return payment, err
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
