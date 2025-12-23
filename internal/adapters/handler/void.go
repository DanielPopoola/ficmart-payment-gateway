package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

type VoidRequest struct {
	PaymentID string `json:"payment_id" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// HandleVoid processes a payment void request
// @Summary      Void a payment
// @Description  Cancel an authorized payment before it has been captured. Releases the hold on funds.
// @Tags         payments
// @Accept       json
// @Produce      json
// @Param        Idempotency-Key  header    string        true  "Unique key to prevent duplicate requests"  example:"750e8400-e29b-41d4-a716-446655440002"
// @Param        request          body      VoidRequest   true  "Payment void details"
// @Success      200              {object}  APIResponse   "Payment voided successfully"
// @Failure      400              {object}  APIResponse   "Invalid request parameters"
// @Failure      404              {object}  APIResponse   "Payment not found"
// @Failure      409              {object}  APIResponse   "Payment not in voidable state"
// @Failure      412              {object}  APIResponse   "Missing required bank authorization ID"
// @Failure      500              {object}  APIResponse   "Internal server error"
// @Router       /void [post]
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
