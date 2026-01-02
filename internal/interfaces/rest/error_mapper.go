package rest

import (
	"encoding/json"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
)

type ErrorResponse struct {
	Success bool        `json:"success"`
	Error   ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// WriteError maps application errors to HTTP responses
func WriteError(w http.ResponseWriter, err error) {
	statusCode := application.ToHTTPStatus(err)
	errorCode := application.ToErrorCode(err)

	response := ErrorResponse{
		Success: false,
		Error: ErrorDetail{
			Code:    errorCode,
			Message: err.Error(),
		},
	}

	// Add extra details for specific error types
	if svcErr, ok := application.IsServiceError(err); ok {
		if svcErr.Code == application.ErrCodeDuplicateBusinessRequest {
			response.Error.Details = map[string]string{
				"hint": "Query existing payment using GET /payments/order/{order_id}",
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// ErrorResponseFactory contains the response constructors for each status code
type ErrorResponseFactory struct {
	BadRequest    func(api.ErrorResponse) interface{}
	Timeout       func(api.ErrorResponse) interface{}
	Conflict      func(api.ErrorResponse) interface{}
	NotFound      func(api.ErrorResponse) interface{}
	InternalError func(api.ErrorResponse) interface{}
}
