package commands

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

type VoidCommand struct {
	PaymentID      string
	IdempotencyKey string
}

type VoidHandler struct {
	paymentRepo     application.PaymentRepository
	idempotencyRepo application.IdempotencyRepository
	bankClient      application.BankClient
}

func NewVoidHandler(
	paymentRepo application.PaymentRepository,
	idempotencyRepo application.IdempotencyRepository,
	bankClient application.BankClient,
) *VoidHandler {
	return &VoidHandler{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
	}
}

func (h *VoidHandler) Handle(ctx context.Context, cmd VoidCommand) (*domain.Payment, error) {
	existingPayment, err := h.idempotencyRepo.FindByKey(ctx, cmd.IdempotencyKey)
	if err == nil {
		return existingPayment, nil
	}

	requestHash := h.computeRequestHash(cmd)
	err = h.idempotencyRepo.AcquireLock(ctx, cmd.IdempotencyKey, cmd.PaymentID, requestHash)
	if err != nil {
		return nil, err
	}

	payment, err := h.paymentRepo.FindByIDForUpdate(ctx, cmd.PaymentID)
	if err != nil {
		return nil, err
	}

	err = payment.MarkVoiding()
	if err != nil {
		return nil, err
	}

	err = h.paymentRepo.Update(ctx, payment)
	if err != nil {
		return nil, err
	}

	bankReq := application.VoidRequest{
		AuthorizationID: *payment.BankAuthID(),
	}

	bankResp, err := h.bankClient.Void(ctx, bankReq, cmd.IdempotencyKey)
	if err != nil {
		payment.Fail()
		h.paymentRepo.Update(ctx, payment)
		return nil, fmt.Errorf("bank void failed: %w", err)
	}

	err = payment.Void(bankResp.VoidID, bankResp.VoidedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid state transition: %w", err)
	}

	err = h.paymentRepo.Update(ctx, payment)
	if err != nil {
		return nil, fmt.Errorf("failed to save voided payment: %w", err)
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

func (h *VoidHandler) computeRequestHash(cmd VoidCommand) string {
	data := fmt.Sprintf("%s", cmd.PaymentID)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
