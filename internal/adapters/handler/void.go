package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

type VoidRequest struct {
	PaymentID string `json:"payment_id" validate:"required,uuid"`
}

func (h *PaymentHandler) HandleVoid(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, err)
		return
	}

	var req VoidRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondWithError(w, err)
		return
	}

	// Extract Idempotency-Key strictly from header
	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		respondWithError(w, &domain.DomainError{
			Code:    "VALIDATION_ERROR",
			Message: "Idempotency-Key header is required",
		})
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
		idemKey,
	)
	if err != nil {
		respondWithError(w, err)
		return
	}

	respondWithJSON(w, http.StatusOK, payment)
}
