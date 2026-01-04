package services

import (
	"context"
	"errors"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
)

type RefundService struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	bankClient      bank.BankClient
	db              *postgres.DB
}

func NewRefundService(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	bankClient bank.BankClient,
	db *postgres.DB,
) *RefundService {
	return &RefundService{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
		db:              db,
	}
}

func (s *RefundService) Refund(ctx context.Context, cmd RefundCommand, idempotencyKey string) (*domain.Payment, error) {
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
			return p.MarkRefunding()
		},
	)
	if err != nil {
		if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
			return waitForCompletion(ctx, s.idempotencyRepo, s.paymentRepo, idempotencyKey)
		}
		return nil, err
	}

	bankReq := bank.RefundRequest{
		Amount:    payment.AmountCents,
		CaptureID: *payment.BankCaptureID,
	}

	bankResp, err := s.bankClient.Refund(ctx, bankReq, idempotencyKey)
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
	if err := payment.Refund(bankResp.Status, bankResp.RefundID, bankResp.RefundedAt); err != nil {
		return nil, application.NewInvalidStateError(err)
	}

	if err := FinalizePaymentSuccess(ctx, s.db, s.paymentRepo, s.idempotencyRepo, payment, idempotencyKey, bankResp); err != nil {
		return payment, err
	}

	return payment, nil
}
