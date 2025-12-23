package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func respondWithJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := APIResponse{
		Success: status >= 200 && status < 300,
	}

	if response.Success {
		response.Data = data
	} else {
		if apiErr, ok := data.(*APIError); ok {
			response.Error = apiErr
		}
	}

	_ = json.NewEncoder(w).Encode(response)
}

func respondWithError(w http.ResponseWriter, err error) {
	var domainErr *domain.DomainError
	code := "INTERNAL_ERROR"
	message := err.Error()
	status := http.StatusInternalServerError

	if errors.As(err, &domainErr) {
		code = domainErr.Code
		message = domainErr.Message

		switch domainErr.Code {
		case domain.ErrCodeInvalidAmount, domain.ErrCodeMissingRequiredField, domain.ErrCodeIdempotencyMismatch:
			status = http.StatusBadRequest
		case domain.ErrCodePaymentNotFound:
			status = http.StatusNotFound
		case domain.ErrCodeDuplicateIdempotencyKey, domain.ErrCodeInvalidState, domain.ErrCodeAmountMismatch, domain.ErrCodeInvalidTransition:
			status = http.StatusConflict
		case domain.ErrRequestProcessing:
			status = http.StatusAccepted
		case domain.ErrCodeTimeout:
			status = http.StatusConflict
			code = domain.ErrRequestProcessing
			message = "Request is being processed"
		case domain.ErrCodeMissingDependency:
			status = http.StatusPreconditionFailed
		default:
			status = http.StatusBadRequest
		}
	}

	respondWithJSON(w, status, &APIError{
		Code:    code,
		Message: message,
	})
}
