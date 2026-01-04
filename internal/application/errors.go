package application

import (
	"errors"
	"fmt"
	"net/http"
)

// APPLICATION-LEVEL ERRORS (Orchestration)

type ServiceError struct {
	Code       string
	Message    string
	HTTPStatus int
	Err        error
}

func (e *ServiceError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *ServiceError) Unwrap() error {
	return e.Err
}

const (
	ErrCodeIdempotencyMismatch = "IDEMPOTENCY_MISMATCH"
	ErrCodeRequestProcessing   = "REQUEST_PROCESSING"
	ErrCodeTimeout             = "TIMEOUT"
	ErrCodeInternal            = "INTERNAL_ERROR"
	ErrCodeInvalidInput        = "INVALID_INPUT"
	ErrCodeInvalidState        = "INVALID_STATE"
)

func NewIdempotencyMismatchError() *ServiceError {
	return &ServiceError{
		Code:       ErrCodeIdempotencyMismatch,
		Message:    "Idempotency key reused with different request parameters",
		HTTPStatus: http.StatusBadRequest,
	}
}

func NewRequestProcessingError() *ServiceError {
	return &ServiceError{
		Code:       ErrCodeRequestProcessing,
		Message:    "Request is being processed. Please retry in a moment.",
		HTTPStatus: http.StatusAccepted,
	}
}

func NewTimeoutError() *ServiceError {
	return &ServiceError{
		Code:       ErrCodeTimeout,
		Message:    "Request timed out waiting for completion",
		HTTPStatus: http.StatusRequestTimeout,
	}
}

func NewInternalError(err error) *ServiceError {
	return &ServiceError{
		Code:       ErrCodeInternal,
		Message:    "An internal error occurred",
		HTTPStatus: http.StatusInternalServerError,
		Err:        err,
	}
}

func NewInvalidInputError(err error) *ServiceError {
	return &ServiceError{
		Code:       ErrCodeInvalidInput,
		Message:    "Invalid input",
		HTTPStatus: http.StatusBadRequest,
	}
}

func NewInvalidStateError(err error) *ServiceError {
	return &ServiceError{
		Code:       ErrCodeInvalidState,
		Message:    "Invalid state",
		HTTPStatus: http.StatusConflict,
		Err:        err,
	}
}

func NewInvalidTransitionError(err error) *ServiceError {
	return &ServiceError{
		Code:       ErrCodeInvalidState,
		Message:    "Invalid transition",
		HTTPStatus: http.StatusConflict,
		Err:        err,
	}
}

func NewPaymentExpiredError(err error) *ServiceError {
	return &ServiceError{
		Code:       ErrCodeInvalidState,
		Message:    "payment has expired",
		HTTPStatus: http.StatusConflict,
		Err:        err,
	}
}

func IsServiceError(err error) (*ServiceError, bool) {
	var svcErr *ServiceError
	ok := errors.As(err, &svcErr)
	return svcErr, ok
}
