package commands

import (
	"context"
	"fmt"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain/payment"
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

// AuthorizePaymentHandler orchestrates the use case
type AuthorizeHandler struct {
	paymentRepo payment.Repository
	bankClient  application.BankClient
	idempotency application.IdempotencyStore
}

func NewAuthorizePaymentHandler(
	paymentRepo payment.Repository,
	bankClient application.BankClient,
	idempotency application.IdempotencyStore,
) *AuthorizeHandler {
	return &AuthorizeHandler{
		paymentRepo: paymentRepo,
		bankClient:  bankClient,
		idempotency: idempotency,
	}
}

func (h *AuthorizeHandler) Execute(ctx context.Context, cmd AuthorizeCommand) (*domain.Payment, error) {
	requestHash := h.computeHash(cmd.OrderID, cmd.Amount, cmd.CustomerID)

	if cached, err := h.idempotency.GetCachedResponse(ctx, cmd.IdempotencyKey); err == nil && cached != nil {
		return cached, nil
	}

	payment, err := domain.NewPayment(
		uuid.New().String(),
		cmd.OrderID,
		cmd.CustomerID,
		cmd.Amount,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid payment: %w", err)
	}

	err = h.paymentRepo.SaveWithIdempotency(
		ctx,
		payment,
		cmd.IdempotencyKey,
		requestHash,
	)

	if err != nil {
		if isDuplicateKeyError(err) {
			return h.handleDuplicateKey(ctx, cmd.IdempotencyKey, requestHash)
		}
		return nil, err
	}

	bankResp, err := h.bankClient.Authorize(ctx, application.AuthorizationRequest{
		Amount:      cmd.Amount,
		CardNumber:  cmd.CardNumber,
		Cvv:         cmd.CVV,
		ExpiryMonth: cmd.ExpiryMonth,
		ExpiryYear:  cmd.ExpiryYear,
	})

	if err == nil {
		if err := payment.Authorize(bankResp.AuthorizationID, bankResp.CreatedAt, bankResp.ExpiresAt); err != nil {
			return nil, err
		}
	} else {
		// Bank failed - payment stays PENDING (your reconciler will handle it)
		// We still return the payment so caller knows the ID
	}

	// Step 7: Save updated state
	if err := h.paymentRepo.UpdatePayment(ctx, payment); err != nil {
		// Log error but don't fail - reconciler will fix it
	}

	// Step 8: Cache for idempotency
	_ = h.idempotency.CacheResponse(ctx, cmd.IdempotencyKey, payment)

	return payment, nil

}
