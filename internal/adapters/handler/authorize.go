package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
)

type AuthorizeRequest struct {
	OrderID     string `json:"order_id" validate:"required" example:"order-12345"`
	CustomerID  string `json:"customer_id" validate:"required" example:"cust-67890"`
	Amount      int64  `json:"amount" validate:"required,gt=0" example:"5000"`
	CardNumber  string `json:"card_number" validate:"required,numeric,min=13,max=19" example:"4111111111111111"`
	CVV         string `json:"cvv" validate:"required,numeric,len=3" example:"123"`
	ExpiryMonth int    `json:"expiry_month" validate:"required,min=1,max=12" example:"12"`
	ExpiryYear  int    `json:"expiry_year" validate:"required,min=2024" example:"2030"`
}

// HandleAuthorize processes a payment authorization request
// @Summary      Authorize a payment
// @Description  Reserve funds on a customer's card for a specific order. Returns immediately with payment status.
// @Tags         payments
// @Accept       json
// @Produce      json
// @Param        Idempotency-Key  header    string              true  "Unique key to prevent duplicate requests"  example:"550e8400-e29b-41d4-a716-446655440000"
// @Param        request          body      AuthorizeRequest    true  "Payment authorization details"
// @Success      201              {object}  APIResponse         "Payment authorized successfully"
// @Failure      400              {object}  APIResponse         "Invalid request parameters"
// @Failure      202              {object}  APIResponse         "Request is being processed (duplicate idempotency key)"
// @Failure      409              {object}  APIResponse         "Conflict - duplicate or invalid state"
// @Failure      500              {object}  APIResponse         "Internal server error"
// @Router       /authorize [post]
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
