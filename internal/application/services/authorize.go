package services

import (
	"context"
	"errors"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
	"github.com/google/uuid"
)

type AuthorizeService struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	bankClient      bank.BankClient
	db              *postgres.DB
}

func NewAuthorizeService(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	bankClient bank.BankClient,
	db *postgres.DB,
) *AuthorizeService {
	return &AuthorizeService{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
		db:              db,
	}
}

func (s *AuthorizeService) Authorize(ctx context.Context, cmd AuthorizeCommand, idempotencyKey string) (*domain.Payment, error) {
	requestHash := ComputeHash(cmd)

	cachedPayment, isCached, err := checkIdempotency(
		ctx,
		s.idempotencyRepo,
		s.paymentRepo,
		idempotencyKey,
		requestHash,
	)
	if err != nil {
		return nil, err
	}

	if isCached {
		return cachedPayment, nil
	}

	paymentID := uuid.New().String()
	payment, err := domain.NewPayment(paymentID, cmd.OrderID, cmd.CustomerID, cmd.Amount, cmd.Currency)
	if err != nil {
		return nil, application.NewInvalidInputError(err)
	}

	err = acquireIdempotencyLock(
		ctx,
		s.db,
		s.paymentRepo,
		s.idempotencyRepo,
		payment,
		idempotencyKey,
		requestHash,
	)
	if err != nil {
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			return waitForCompletion(ctx, s.idempotencyRepo, s.paymentRepo, idempotencyKey)
		}
		return nil, application.NewInternalError(err)
	}

	bankReq := bank.AuthorizationRequest{
		Amount:      cmd.Amount,
		CardNumber:  cmd.CardNumber,
		Cvv:         cmd.CVV,
		ExpiryMonth: cmd.ExpiryMonth,
		ExpiryYear:  cmd.ExpiryYear,
	}

	bankResp, err := s.bankClient.Authorize(ctx, bankReq, idempotencyKey)
	if err != nil {
		return payment, handleBankFailure(
			ctx,
			s.db,
			s.paymentRepo,
			s.idempotencyRepo,
			payment,
			idempotencyKey,
			err,
		)
	}

	if err := payment.Authorize(bankResp.AuthorizationID, bankResp.CreatedAt, bankResp.ExpiresAt); err != nil {
		return nil, application.NewInvalidStateError(err)
	}
	if err := finalizePaymentSuccess(ctx, s.db, s.paymentRepo, s.idempotencyRepo, payment, idempotencyKey, bankResp); err != nil {
		return payment, err
	}

	return payment, nil
}
