package commands

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

type RefundCommand struct {
	PaymentID      string
	Amount         int64
	IdempotencyKey string
}

type RefundHandler struct {
	paymentRepo     application.PaymentRepository
	idempotencyRepo application.IdempotencyRepository
	bankClient      application.BankClient
}

func NewRefundHandler(
	paymentRepo application.PaymentRepository,
	idempotencyRepo application.IdempotencyRepository,
	bankClient application.BankClient,
) *RefundHandler {
	return &RefundHandler{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
	}
}

func (h *RefundHandler) Handle(ctx context.Context, cmd RefundCommand) (*domain.Payment, error) {
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

	err = payment.MarkRefunding()
	if err != nil {
		return nil, err
	}

	err = h.paymentRepo.Update(ctx, payment)
	if err != nil {
		return nil, err
	}

	bankReq := application.RefundRequest{
		Amount:    cmd.Amount,
		CaptureID: *payment.BankCaptureID(),
	}

	bankResp, err := h.bankClient.Refund(ctx, bankReq, cmd.IdempotencyKey)
	if err != nil {
		payment.Fail()
		h.paymentRepo.Update(ctx, payment)
		return nil, fmt.Errorf("bank refund failed: %w", err)
	}

	err = payment.Refund(bankResp.RefundID, bankResp.RefundedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid state transition: %w", err)
	}

	err = h.paymentRepo.Update(ctx, payment)
	if err != nil {
		return nil, fmt.Errorf("failed to save refunded payment: %w", err)
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

func (h *RefundHandler) computeRequestHash(cmd RefundCommand) string {
	data := fmt.Sprintf("%s:%d", cmd.PaymentID, cmd.Amount)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
