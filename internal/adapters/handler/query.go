package handler

import (
	"net/http"
	"strconv"
)

func (h *PaymentHandler) HandleGetPaymentByOrder(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("orderID")
	if orderID == "" {
		respondWithJSON(w, http.StatusBadRequest, &APIError{
			Code:    "MISSING_PARAMETER",
			Message: "orderID is required",
		})
		return
	}

	payment, err := h.queryService.GetPaymentByOrderID(r.Context(), orderID)
	if err != nil {
		respondWithError(w, err)
		return
	}

	respondWithJSON(w, http.StatusOK, payment)
}

func (h *PaymentHandler) HandleGetPaymentsByCustomer(w http.ResponseWriter, r *http.Request) {
	customerID := r.PathValue("customerID")
	if customerID == "" {
		respondWithJSON(w, http.StatusBadRequest, &APIError{
			Code:    "MISSING_PARAMETER",
			Message: "customerID is required",
		})
		return
	}

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 10
	offset := 0

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	payments, err := h.queryService.GetPaymentsByCustomerID(r.Context(), customerID, limit, offset)
	if err != nil {
		respondWithError(w, err)
		return
	}

	respondWithJSON(w, http.StatusOK, payments)
}
