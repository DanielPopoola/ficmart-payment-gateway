package handler

import (
	"encoding/json"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
)

type AuthorizeRequest struct {
	OrderID        string `json:"order_id" validate:"required"`
	CustomerID     string `json:"customer_id" validate:"required"`
	Amount         int64  `json:"amount" validate:"required,gt=0"`
	CardNumber     string `json:"card_number" validate:"required,numeric,min=13,max=19"`
	CVV            string `json:"cvv" validate:"required,numeric,len=3"`
	ExpiryMonth    int    `json:"expiry_month" validate:"required,min=1,max=12"`
	ExpiryYear     int    `json:"expiry_year" validate:"required,min=2024"`
	IdempotencyKey string `json:"idempotency_key" validate:"required"`
}

func (h *PaymentHandler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	var req AuthorizeRequest
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

	payment, err := h.authService.Authorize(
		r.Context(),
		req.OrderID,
		req.CustomerID,
		req.IdempotencyKey,
		req.Amount,
		req.CardNumber,
		req.CVV,
		req.ExpiryMonth,
		req.ExpiryYear,
	)
	if err != nil {
		respondWithError(w, err)
		return
	}

	respondWithJSON(w, http.StatusCreated, payment)
}
