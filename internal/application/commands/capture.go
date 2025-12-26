package commands

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

type CaptureCommand struct {
	PaymentID      string
	Amount         int64
	IdempotencyKey string
}

type CaptureHandler struct {
	paymentRepo     application.PaymentRepository
	idempotencyRepo application.IdempotencyRepository
	bankClient      application.BankClient
}

func NewCaptureHandler(
	paymentRepo application.PaymentRepository,
	idempotencyRepo application.IdempotencyRepository,
	bankClient application.BankClient,
) *CaptureHandler {
	return &CaptureHandler{
		paymentRepo:     paymentRepo,
		idempotencyRepo: idempotencyRepo,
		bankClient:      bankClient,
	}
}

func (h *CaptureHandler) Handle(ctx context.Context, cmd CaptureCommand) (*domain.Payment, error) {
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

	err = payment.MarkCapturing()
	if err != nil {
		return nil, err
	}

	err = h.paymentRepo.Update(ctx, payment)
	if err != nil {
		return nil, err
	}

	bankReq := application.CaptureRequest{
		Amount:          payment.Amount().Amount,
		AuthorizationID: *payment.BankAuthID(),
	}

	bankResp, err := h.bankClient.Capture(ctx, bankReq, cmd.IdempotencyKey)
	if err != nil {
		payment.Fail()
		h.paymentRepo.Update(ctx, payment)
		return nil, fmt.Errorf("bank capture failed: %w", err)
	}

	err = payment.Capture(bankResp.CaptureID, bankResp.CapturedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid state transition: %w", err)
	}

	err = h.paymentRepo.Update(ctx, payment)
	if err != nil {
		return nil, fmt.Errorf("failed to save captured payment: %w", err)
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

func (h *CaptureHandler) computeRequestHash(cmd CaptureCommand) string {
	data := fmt.Sprintf("%s:%d", cmd.PaymentID, cmd.Amount)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
