package handler

import (
	"encoding/json"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

type RefundRequest struct {
	PaymentID      string `json:"payment_id" validate:"required,uuid"`
	Amount         int64  `json:"amount" validate:"required,gt=0"`
	IdempotencyKey string `json:"idempotency_key" validate:"required"`
}

func (h *PaymentHandler) HandleRefund(w http.ResponseWriter, r *http.Request) {
	var req RefundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, err)
		return
	}

	// Extract Idempotency-Key from header, taking precedence over body
	if idemKey := r.Header.Get("Idempotency-Key"); idemKey != "" {
		req.IdempotencyKey = idemKey
	}

	if err := h.validate.Struct(req); err != nil {
		respondWithError(w, &domain.DomainError{
			Code:    "VALIDATION_ERROR",
			Message: err.Error(),
		})
		return
	}

	paymentID, err := uuid.Parse(req.PaymentID)
	if err != nil {
		respondWithError(w, err)
		return
	}

	payment, err := h.refundService.Refund(
		r.Context(),
		paymentID,
		req.Amount,
		req.IdempotencyKey,
	)
	if err != nil {
		respondWithError(w, err)
		return
	}

	respondWithJSON(w, http.StatusOK, payment)
}
