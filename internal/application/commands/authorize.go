// Package commands provides command handlers for the payment gateway application.
// It contains the business logic for processing payment requests.
package commands

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/google/uuid"
)

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

type AuthorizeHandler struct {
	paymentRepo     application.PaymentRepository
	idempotencyRepo application.IdempotencyRepository
	bankClient      application.BankClient
}

func NewAuthorizeHandler(
	paymentRepo application.PaymentRepository,
	idempotencyRepo application.IdempotencyRepository,
	bankClient application.BankClient,
) *AuthorizeHandler {
	return &AuthorizeHandler{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
	}
}

func (h *AuthorizeHandler) Handle(ctx context.Context, cmd AuthorizeCommand) (*domain.Payment, error) {
	existingPayment, err := h.idempotencyRepo.FindByKey(ctx, cmd.IdempotencyKey)
	if err == nil {
		return existingPayment, nil
	}

	money, err := domain.NewMoney(cmd.Amount, cmd.Currency)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	paymentID := uuid.New().String()
	payment, err := domain.NewPayment(paymentID, cmd.OrderID, cmd.CustomerID, money)
	if err != nil {
		return nil, fmt.Errorf("invalid payment: %w", err)
	}

	requestHash := h.computeRequestHash(cmd)
	err = h.idempotencyRepo.AcquireLock(ctx, cmd.IdempotencyKey, paymentID, requestHash)
	if err != nil {
		return nil, err
	}

	err = h.paymentRepo.Create(ctx, payment)
	if err != nil {
		return nil, fmt.Errorf("failed to save payment: %w", err)
	}

	bankReq := application.AuthorizationRequest{
		Amount:      cmd.Amount,
		CardNumber:  cmd.CardNumber,
		Cvv:         cmd.CVV,
		ExpiryMonth: cmd.ExpiryMonth,
		ExpiryYear:  cmd.ExpiryYear,
	}

	bankResp, err := h.bankClient.Authorize(ctx, bankReq, cmd.IdempotencyKey)
	if err != nil {
		payment.Fail()
		h.paymentRepo.Update(ctx, payment)
		return nil, fmt.Errorf("bank authorization failed: %w", err)
	}

	err = payment.Authorize(bankResp.AuthorizationID, bankResp.CreatedAt, bankResp.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("invalid state transition: %w", err)
	}

	err = h.paymentRepo.Update(ctx, payment)
	if err != nil {
		return nil, fmt.Errorf("failed to save authorized payment: %w", err)
	}

	responsePayload, _ := json.Marshal(bankResp)
	err = h.idempotencyRepo.StoreResponse(ctx, cmd.IdempotencyKey, responsePayload, 200)
	if err != nil {
		fmt.Printf("Warning: failed to store idempotency response: %v\n", err)
	}

	err = h.idempotencyRepo.ReleaseLock(ctx, cmd.IdempotencyKey)
	if err != nil {
		fmt.Printf("Warning: failed to release idempotency lock: %v\n", err)
	}

	return payment, nil
}

func (h *AuthorizeHandler) computeRequestHash(cmd AuthorizeCommand) string {
	data := fmt.Sprintf("%s:%s:%d:%s:%s",
		cmd.OrderID, cmd.CustomerID, cmd.Amount, cmd.Currency, cmd.CardNumber)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
