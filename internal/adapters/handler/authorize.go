package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
)

type AuthorizeRequest struct {
	OrderID     string `json:"order_id" validate:"required"`
	CustomerID  string `json:"customer_id" validate:"required"`
	Amount      int64  `json:"amount" validate:"required,gt=0"`
	CardNumber  string `json:"card_number" validate:"required,numeric,min=13,max=19"`
	CVV         string `json:"cvv" validate:"required,numeric,len=3"`
	ExpiryMonth int    `json:"expiry_month" validate:"required,min=1,max=12"`
	ExpiryYear  int    `json:"expiry_year" validate:"required,min=2024"`
}

func (h *PaymentHandler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, err)
		return
	}

	var req AuthorizeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondWithError(w, err)
		return
	}

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

	payment, err := h.authService.Authorize(
		r.Context(),
		req.OrderID,
		req.CustomerID,
		idemKey,
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
