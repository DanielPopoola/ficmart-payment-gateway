package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/google/uuid"
)

type PaymentService struct {
	paymentRepo     application.PaymentRepository
	idempotencyRepo application.IdempotencyRepository
	bankClient      application.BankClient
	logger          *slog.Logger
}

func NewPaymentService(
	paymentRepo application.PaymentRepository,
	idempotencyRepo application.IdempotencyRepository,
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

type AuthorizeCommand struct {
	OrderID        string
	CustomerID     string
	Amount         int64
	Currency       string
	CardNumber     string
	CVV            string
	ExpiryMonth    int
	ExpiryYear     int
	IdempotencyKey string
}

type CaptureCommand struct {
	PaymentID      string
	Amount         int64
	IdempotencyKey string
}

type VoidCommand struct {
	PaymentID      string
	IdempotencyKey string
}

type RefundCommand struct {
	PaymentID      string
	Amount         int64
	IdempotencyKey string
}

// Authorize creates a new payment and reserves funds
func (s *PaymentService) Authorize(ctx context.Context, cmd AuthorizeCommand) (*domain.Payment, error) {
	paymentID := uuid.New().String()
	return s.withIdempotency(ctx, cmd.IdempotencyKey, paymentID, cmd, func() (*domain.Payment, interface{}, error) {
		money, err := domain.NewMoney(cmd.Amount, cmd.Currency)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid amount: %w", err)
		}

		payment, err := domain.NewPayment(paymentID, cmd.OrderID, cmd.CustomerID, money)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid payment: %w", err)
		}

		if err := s.paymentRepo.Create(ctx, payment); err != nil {
			return nil, nil, fmt.Errorf("failed to save payment: %w", err)
		}

		bankReq := application.AuthorizationRequest{
			Amount:      cmd.Amount,
			CardNumber:  cmd.CardNumber,
			Cvv:         cmd.CVV,
			ExpiryMonth: cmd.ExpiryMonth,
			ExpiryYear:  cmd.ExpiryYear,
		}

		bankResp, err := s.bankClient.Authorize(ctx, bankReq, cmd.IdempotencyKey)
		if err != nil {
			payment.Fail()
			s.paymentRepo.Update(ctx, payment)
			return payment, nil, fmt.Errorf("bank authorization failed: %w", err)
		}

		if err := payment.Authorize(bankResp.AuthorizationID, bankResp.CreatedAt, bankResp.ExpiresAt); err != nil {
			return nil, nil, fmt.Errorf("invalid state transition: %w", err)
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, fmt.Errorf("failed to save authorized payment: %w", err)
		}

		return payment, bankResp, nil
	})
}

// Capture charges a previously authorized payment
func (s *PaymentService) Capture(ctx context.Context, cmd CaptureCommand) (*domain.Payment, error) {
	return s.withIdempotency(ctx, cmd.IdempotencyKey, cmd.PaymentID, cmd, func() (*domain.Payment, interface{}, error) {
		payment, err := s.paymentRepo.FindByIDForUpdate(ctx, cmd.PaymentID)
		if err != nil {
			return nil, nil, err
		}

		if err := payment.MarkCapturing(); err != nil {
			return nil, nil, err
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, err
		}

		bankReq := application.CaptureRequest{
			Amount:          payment.Amount().Amount,
			AuthorizationID: *payment.BankAuthID(),
		}

		bankResp, err := s.bankClient.Capture(ctx, bankReq, cmd.IdempotencyKey)
		if err != nil {
			payment.Fail()
			s.paymentRepo.Update(ctx, payment)
			return payment, nil, fmt.Errorf("bank capture failed: %w", err)
		}

		if err := payment.Capture(bankResp.CaptureID, bankResp.CapturedAt); err != nil {
			return nil, nil, fmt.Errorf("invalid state transition: %w", err)
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, fmt.Errorf("failed to save captured payment: %w", err)
		}

		return payment, bankResp, nil
	})
}

// Void cancels an authorization
func (s *PaymentService) Void(ctx context.Context, cmd VoidCommand) (*domain.Payment, error) {
	return s.withIdempotency(ctx, cmd.IdempotencyKey, cmd.PaymentID, cmd, func() (*domain.Payment, interface{}, error) {
		payment, err := s.paymentRepo.FindByIDForUpdate(ctx, cmd.PaymentID)
		if err != nil {
			return nil, nil, err
		}

		if err := payment.MarkVoiding(); err != nil {
			return nil, nil, err
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, err
		}

		bankReq := application.VoidRequest{
			AuthorizationID: *payment.BankAuthID(),
		}

		bankResp, err := s.bankClient.Void(ctx, bankReq, cmd.IdempotencyKey)
		if err != nil {
			payment.Fail()
			s.paymentRepo.Update(ctx, payment)
			return payment, nil, fmt.Errorf("bank void failed: %w", err)
		}

		if err := payment.Void(bankResp.VoidID, bankResp.VoidedAt); err != nil {
			return nil, nil, fmt.Errorf("invalid state transition: %w", err)
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, fmt.Errorf("failed to save voided payment: %w", err)
		}

		return payment, bankResp, nil
	})
}

// Refund returns funds after capture
func (s *PaymentService) Refund(ctx context.Context, cmd RefundCommand) (*domain.Payment, error) {
	return s.withIdempotency(ctx, cmd.IdempotencyKey, cmd.PaymentID, cmd, func() (*domain.Payment, interface{}, error) {
		payment, err := s.paymentRepo.FindByIDForUpdate(ctx, cmd.PaymentID)
		if err != nil {
			return nil, nil, err
		}

		if err := payment.MarkRefunding(); err != nil {
			return nil, nil, err
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, err
		}

		bankReq := application.RefundRequest{
			Amount:    cmd.Amount,
			CaptureID: *payment.BankCaptureID(),
		}

		bankResp, err := s.bankClient.Refund(ctx, bankReq, cmd.IdempotencyKey)
		if err != nil {
			payment.Fail()
			s.paymentRepo.Update(ctx, payment)
			return payment, nil, fmt.Errorf("bank refund failed: %w", err)
		}

		if err := payment.Refund(bankResp.RefundID, bankResp.RefundedAt); err != nil {
			return nil, nil, fmt.Errorf("invalid state transition: %w", err)
		}

		if err := s.paymentRepo.Update(ctx, payment); err != nil {
			return nil, nil, fmt.Errorf("failed to save refunded payment: %w", err)
		}

		return payment, bankResp, nil
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

// withIdempotency handles the boilerplate for idempotent operations
func (s *PaymentService) withIdempotency(
	ctx context.Context,
	idempotencyKey string,
	paymentID string,
	cmd interface{},
	fn func() (*domain.Payment, interface{}, error),
) (*domain.Payment, error) {
	existingPayment, err := s.idempotencyRepo.FindByKey(ctx, idempotencyKey)
	if err == nil {
		return existingPayment, nil
	}

	requestHash := s.computeRequestHash(cmd)
	if err := s.idempotencyRepo.AcquireLock(ctx, idempotencyKey, paymentID, requestHash); err != nil {
		return nil, err
	}

	defer func() {
		if err := s.idempotencyRepo.ReleaseLock(ctx, idempotencyKey); err != nil {
			s.logger.Warn("failed to release idempotency lock",
				"payment_id", paymentID,
				"error", err)
		}
	}()

	payment, bankResp, err := fn()
	if err != nil {
		return payment, err
	}

	responsePayload, _ := json.Marshal(bankResp)
	if err := s.idempotencyRepo.StoreResponse(ctx, idempotencyKey, responsePayload, 200); err != nil {
		s.logger.Warn("failed to store idempotency response",
			"payment_id", paymentID,
			"error", err)
	}

	return payment, nil
}

// Helper: compute request hash for idempotency
func (s *PaymentService) computeRequestHash(cmd interface{}) string {
	data := fmt.Sprintf("%+v", cmd)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
