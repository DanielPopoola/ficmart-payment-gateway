package handler

import (
	"net/http"
	"strconv"
)

// HandleGetPaymentByOrder retrieves a payment by order ID
// @Summary      Get payment by order ID
// @Description  Retrieve the payment record associated with a specific FicMart order
// @Tags         queries
// @Produce      json
// @Param        orderID  path      string       true  "FicMart Order ID"  example:"order-12345"
// @Success      200      {object}  APIResponse  "Payment found"
// @Failure      400      {object}  APIResponse  "Missing or invalid order ID"
// @Failure      404      {object}  APIResponse  "Payment not found for this order"
// @Failure      500      {object}  APIResponse  "Internal server error"
// @Router       /payments/order/{orderID} [get]
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

// HandleGetPaymentsByCustomer retrieves all payments for a customer
// @Summary      Get payments by customer ID
// @Description  Retrieve all payment records for a specific customer with pagination support
// @Tags         queries
// @Produce      json
// @Param        customerID  path      string       true   "FicMart Customer ID"  example:"cust-67890"
// @Param        limit       query     int          false  "Number of results to return"  default:"10"  minimum:"1"  maximum:"100"
// @Param        offset      query     int          false  "Number of results to skip"    default:"0"   minimum:"0"
// @Success      200         {object}  APIResponse  "List of payments"
// @Failure      400         {object}  APIResponse  "Missing or invalid customer ID"
// @Failure      500         {object}  APIResponse  "Internal server error"
// @Router       /payments/customer/{customerID} [get]
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
