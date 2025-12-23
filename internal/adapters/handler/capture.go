package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

type CaptureRequest struct {
	PaymentID string `json:"payment_id" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	Amount    int64  `json:"amount" validate:"required,gt=0" example:"5000"`
}

// HandleCapture processes a payment capture request
// @Summary      Capture a payment
// @Description  Charge the funds that were previously authorized. The payment must be in AUTHORIZED status.
// @Tags         payments
// @Accept       json
// @Produce      json
// @Param        Idempotency-Key  header    string           true  "Unique key to prevent duplicate requests"  example:"650e8400-e29b-41d4-a716-446655440001"
// @Param        request          body      CaptureRequest   true  "Payment capture details"
// @Success      200              {object}  APIResponse      "Payment captured successfully"
// @Failure      400              {object}  APIResponse      "Invalid request or amount mismatch"
// @Failure      404              {object}  APIResponse      "Payment not found"
// @Failure      409              {object}  APIResponse      "Payment not in capturable state"
// @Failure      500              {object}  APIResponse      "Internal server error"
// @Router       /capture [post]
func (h *PaymentHandler) HandleCapture(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, err)
		return
	}

	var req CaptureRequest
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

	paymentID, err := uuid.Parse(req.PaymentID)
	if err != nil {
		respondWithError(w, err)
		return
	}

	payment, err := h.captureService.Capture(
		r.Context(),
		paymentID,
		req.Amount,
		idemKey,
	)
	if err != nil {
		respondWithError(w, err)
		return
	}

	respondWithJSON(w, http.StatusOK, payment)
}
