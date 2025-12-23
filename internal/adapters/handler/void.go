package handler

import (
	"encoding/json"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

type VoidRequest struct {
	PaymentID      string `json:"payment_id" validate:"required,uuid"`
	IdempotencyKey string `json:"idempotency_key" validate:"required"`
}

func (h *PaymentHandler) HandleVoid(w http.ResponseWriter, r *http.Request) {
	var req VoidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, err)
		return
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

	payment, err := h.voidService.Void(
		r.Context(),
		paymentID,
		req.IdempotencyKey,
	)
	if err != nil {
		respondWithError(w, err)
		return
	}

	respondWithJSON(w, http.StatusOK, payment)
}
