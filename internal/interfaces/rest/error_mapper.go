package rest

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
)

// WriteError maps application errors to HTTP responses using OpenAPI-generated types
func WriteError(w http.ResponseWriter, err error, logger *slog.Logger) {
	statusCode := application.ToHTTPStatus(err)
	errorCode := application.ToErrorCode(err)

	response := api.ErrorResponse{
		Success: false,
		Error: struct {
			Code    api.ErrorResponseErrorCode `json:"code"`
			Message string                     `json:"message"`
		}{
			Code:    api.ErrorResponseErrorCode(errorCode),
			Message: err.Error(),
		},
	}

	body, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		logger.Error("failed to marshal error response",
			"error", marshalErr,
			"original_error", err,
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body) //nolint:errcheck // Write failure is non-actionable at this point (headers already sent)
}
