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
	ErrCodeMissingDependency   = "MISSING_DEPENDENCY"
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

func NewTimeoutError(paymentID string) *ServiceError {
	return &ServiceError{
		Code:       ErrCodeTimeout,
		Message:    fmt.Sprintf("Request timed out waiting for completion: %s", paymentID),
		HTTPStatus: http.StatusRequestTimeout,
	}
}

func NewMissingDependencyError(dependency string) *ServiceError {
	return &ServiceError{
		Code:       ErrCodeMissingDependency,
		Message:    fmt.Sprintf("Missing required dependency: %s", dependency),
		HTTPStatus: http.StatusBadRequest,
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
		Message:    "Invalid state transition or unallowed state",
		HTTPStatus: http.StatusConflict,
	}
}

func IsServiceError(err error) (*ServiceError, bool) {
	var svcErr *ServiceError
	ok := errors.As(err, &svcErr)
	return svcErr, ok
}

// INFRASTRUCTURE-LEVEL ERRORS (External APIs)

type BankError struct {
	Code       string
	Message    string
	StatusCode int
}

type BankErrorResponse struct {
	Err     string `json:"error"`
	Message string `json:"message"`
}

func (e *BankError) Error() string {
	return fmt.Sprintf("bank error [%s]: %s (status: %d)", e.Code, e.Message, e.StatusCode)
}

// IsRetryable checks if the bank error is transient (5xx) or permanent (4xx)
func (e *BankError) IsRetryable() bool {
	return e.StatusCode >= 500
}

// Helper to check if error is a BankError
func IsBankError(err error) (*BankError, bool) {
	var bankErr *BankError
	ok := errors.As(err, &bankErr)
	return bankErr, ok
}
