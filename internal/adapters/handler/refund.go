package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/google/uuid"
)

type RefundRequest struct {
	PaymentID string `json:"payment_id" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	Amount    int64  `json:"amount" validate:"required,gt=0" example:"5000"`
}

// HandleRefund processes a payment refund request
// @Summary      Refund a payment
// @Description  Return money to the customer for a previously captured payment. The payment must be in CAPTURED status.
// @Tags         payments
// @Accept       json
// @Produce      json
// @Param        Idempotency-Key  header    string          true  "Unique key to prevent duplicate requests"  example:"850e8400-e29b-41d4-a716-446655440003"
// @Param        request          body      RefundRequest   true  "Payment refund details"
// @Success      200              {object}  APIResponse     "Payment refunded successfully"
// @Failure      400              {object}  APIResponse     "Invalid request or amount"
// @Failure      404              {object}  APIResponse     "Payment not found"
// @Failure      409              {object}  APIResponse     "Payment not in refundable state"
// @Failure      412              {object}  APIResponse     "Missing required bank capture ID"
// @Failure      500              {object}  APIResponse     "Internal server error"
// @Router       /refund [post]
func (h *PaymentHandler) HandleRefund(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, err)
		return
	}

	var req RefundRequest
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

	payment, err := h.refundService.Refund(
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
