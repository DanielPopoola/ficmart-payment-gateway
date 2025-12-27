package services

import (
	"context"
	"log/slog"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
	"github.com/google/uuid"
)

type PaymentService struct {
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	bankClient      application.BankClient
	logger          *slog.Logger
}

func NewPaymentService(
	paymentRepo *postgres.PaymentRepository,
	idempotencyRepo *postgres.IdempotencyRepository,
	bankClient application.BankClient,
	logger *slog.Logger,
) *PaymentService {
	return &PaymentService{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
		logger:          logger,
	}
}

// Authorize creates a new payment and reserves funds
func (s *PaymentService) Authorize(ctx context.Context, cmd AuthorizeCommand, idempotencyKey string) (*domain.Payment, error) {
	paymentID := uuid.New().String()
	return s.withIdempotency(ctx, idempotencyKey, paymentID, cmd, func() (*domain.Payment, any, error) {
		money, err := domain.NewMoney(cmd.Amount, cmd.Currency)
		if err != nil {
			return nil, nil, err
		}

		payment, err := domain.NewPayment(paymentID, cmd.OrderID, cmd.CustomerID, money)
		if err != nil {
			return nil, nil, err
		}

		if err := s.paymentRepo.Create(ctx, payment); err != nil {
			return nil, nil, application.NewInternalError(err)
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
			return payment, nil, err
		}

		s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "BANK_RESPONDED")
		if err := payment.Authorize(bankResp.AuthorizationID, bankResp.CreatedAt, bankResp.ExpiresAt); err != nil {
			return nil, nil, err
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, application.NewInternalError(err)
		}

		return payment, bankResp, nil
	})
}

// Capture charges a previously authorized payment
func (s *PaymentService) Capture(ctx context.Context, cmd CaptureCommand, idempotencyKey string) (*domain.Payment, error) {
	return s.withIdempotency(ctx, idempotencyKey, cmd.PaymentID, cmd, func() (*domain.Payment, any, error) {
		var payment *domain.Payment
		err := s.paymentRepo.WithTx(ctx, func(txRepo postgres.PaymentRepository) error {
			var txErr error
			payment, txErr = txRepo.FindByIDForUpdate(ctx, cmd.PaymentID)
			if txErr != nil {
				return txErr
			}

			if err := payment.MarkCapturing(); err != nil {
				return err
			}
			return txRepo.Update(ctx, payment)
		})
		if err != nil {
			return nil, nil, err
		}

		bankReq := application.BankCaptureRequest{
			Amount:          cmd.Amount,
			AuthorizationID: *payment.BankAuthID(),
		}

		s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "CALLING_BANK")
		bankResp, err := s.bankClient.Capture(ctx, bankReq, idempotencyKey)
		if err != nil {
			return payment, nil, err
		}

		s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "BANK_RESPONDED")
		if err := payment.Capture(bankResp.CaptureID, bankResp.CapturedAt); err != nil {
			return nil, nil, err
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, application.NewInternalError(err)
		}

		return payment, bankResp, err
	})
}

// Void cancels an authorization
func (s *PaymentService) Void(ctx context.Context, cmd VoidCommand, idempotencyKey string) (*domain.Payment, error) {
	return s.withIdempotency(ctx, idempotencyKey, cmd.PaymentID, cmd, func() (*domain.Payment, any, error) {
		var payment *domain.Payment
		err := s.paymentRepo.WithTx(ctx, func(txRepo postgres.PaymentRepository) error {
			var txErr error
			payment, txErr = txRepo.FindByIDForUpdate(ctx, cmd.PaymentID)
			if txErr != nil {
				return txErr
			}

			if err := payment.MarkVoiding(); err != nil {
				return err
			}
			return txRepo.Update(ctx, payment)
		})
		if err != nil {
			return nil, nil, err
		}

		bankReq := application.BankVoidRequest{
			AuthorizationID: *payment.BankAuthID(),
		}

		s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "CALLING_BANK")
		bankResp, err := s.bankClient.Void(ctx, bankReq, idempotencyKey)
		if err != nil {
			return payment, nil, err
		}

		s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "BANK_RESPONDED")
		if err := payment.Void(bankResp.VoidID, bankResp.VoidedAt); err != nil {
			return nil, nil, err
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, application.NewInternalError(err)
		}
		return payment, bankResp, err
	})
}

// Refund returns funds after capture
func (s *PaymentService) Refund(ctx context.Context, cmd RefundCommand, idempotencyKey string) (*domain.Payment, error) {
	return s.withIdempotency(ctx, idempotencyKey, cmd.PaymentID, cmd, func() (*domain.Payment, any, error) {
		var payment *domain.Payment
		err := s.paymentRepo.WithTx(ctx, func(txRepo postgres.PaymentRepository) error {
			var txErr error
			payment, txErr := s.paymentRepo.FindByIDForUpdate(ctx, cmd.PaymentID)
			if txErr != nil {
				return txErr
			}

			if err := payment.MarkRefunding(); err != nil {
				return err
			}
			return txRepo.Update(ctx, payment)
		})
		if err != nil {
			return nil, nil, err
		}

		bankReq := application.BankRefundRequest{
			Amount:    cmd.Amount,
			CaptureID: *payment.BankCaptureID(),
		}

		s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "CALLING_BANK")
		bankResp, err := s.bankClient.Refund(ctx, bankReq, idempotencyKey)
		if err != nil {
			return payment, nil, err
		}

		s.idempotencyRepo.UpdateRecoveryPoint(ctx, idempotencyKey, "BANK_RESPONDED")
		if err := payment.Refund(bankResp.RefundID, bankResp.RefundedAt); err != nil {
			return nil, nil, err
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, application.NewInternalError(err)
		}
		return payment, bankResp, err
	})
}

// GetPaymentByOrder retrieves a payment by order ID
func (s *PaymentService) GetPaymentByOrder(ctx context.Context, orderID string) (*domain.Payment, error) {
	return s.paymentRepo.FindByOrderID(ctx, orderID)
}

// GetPaymentsByCustomer retrieves all payments for a customer
func (s *PaymentService) GetPaymentsByCustomer(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
	return s.paymentRepo.FindByCustomerID(ctx, customerID, limit, offset)
}
