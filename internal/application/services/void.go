package services

import (
	"context"
	"errors"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
)

type VoidService struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	bankClient      bank.BankClient
	db              *postgres.DB
}

func NewVoidService(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	bankClient bank.BankClient,
	db *postgres.DB,
) *VoidService {
	return &VoidService{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
		db:              db,
	}
}

func (s *VoidService) Void(ctx context.Context, cmd VoidCommand, idempotencyKey string) (*domain.Payment, error) {
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

	payment, err := markPaymentTransitioning(
		ctx,
		s.db,
		s.paymentRepo,
		s.idempotencyRepo,
		cmd.PaymentID,
		idempotencyKey,
		requestHash,
		func(p *domain.Payment) error {
			return p.MarkVoiding()
		},
	)
	if err != nil {
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			return waitForCompletion(ctx, s.idempotencyRepo, s.paymentRepo, idempotencyKey)
		}
		return nil, err
	}

	bankReq := bank.VoidRequest{
		AuthorizationID: *payment.BankAuthID,
	}

	bankResp, err := s.bankClient.Void(ctx, bankReq, idempotencyKey)
	if err != nil {
		return payment, HandleBankFailure(
			ctx,
			s.db,
			s.paymentRepo,
			s.idempotencyRepo,
			payment,
			idempotencyKey,
			err,
		)
	}
	if err := payment.Void(bankResp.Status, bankResp.VoidID, bankResp.VoidedAt); err != nil {
		return nil, application.NewInvalidStateError(err)
	}

	if err := FinalizePayment(ctx, s.db, s.paymentRepo, s.idempotencyRepo, payment, idempotencyKey, bankResp); err != nil {
		return payment, err
	}

	return payment, nil
}
