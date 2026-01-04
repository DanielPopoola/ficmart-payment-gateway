package services

import (
	"context"
	"errors"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
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
			return p.MarkCapturing()
		},
	)
	if err != nil {
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			return waitForCompletion(ctx, s.idempotencyRepo, s.paymentRepo, idempotencyKey)
		}
		return nil, err
	}

	bankReq := bank.CaptureRequest{
		Amount:          cmd.Amount,
		AuthorizationID: *payment.BankAuthID,
	}

	bankResp, err := s.bankClient.Capture(ctx, bankReq, idempotencyKey)
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

	if err := payment.Capture(bankResp.Status, bankResp.CaptureID, bankResp.CapturedAt); err != nil {
		return nil, application.NewInvalidStateError(err)
	}

	if err := FinalizePaymentSuccess(ctx, s.db, s.paymentRepo, s.idempotencyRepo, payment, idempotencyKey, bankResp); err != nil {
		return payment, err
	}

	return payment, nil
}
