package domain

import (
	"errors"
	"fmt"
)

// DomainError represents a business logic error
type DomainError struct {
	Code    string
	Message string
	Err     error
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error for errors.Is/As support
func (e *DomainError) Unwrap() error {
	return e.Err
}

// Retryable interface for errors that can be retried
type Retryable interface {
	IsRetryable() bool
}

// Domain validation errors
const (
	ErrCodeInvalidTransition       = "INVALID_TRANSITION"
	ErrCodePaymentExpired          = "PAYMENT_EXPIRED"
	ErrCodePaymentNotFound         = "PAYMENT_NOT_FOUND"
	ErrCodeInvalidAmount           = "INVALID_AMOUNT"
	ErrCodeDuplicateIdempotencyKey = "DUPLICATE_IDEMPOTENCY_KEY"
	ErrRequestProcessing           = "REQUEST_PROCESSING"
)

func NewInvalidAmountError(amount int64) *DomainError {
	return &DomainError{
		Code:    ErrCodeInvalidAmount,
		Message: fmt.Sprintf("invalid amount %d", amount),
	}
}

func NewInvalidTransitionError(from, to PaymentStatus) *DomainError {
	return &DomainError{
		Code:    ErrCodeInvalidTransition,
		Message: fmt.Sprintf("cannot transition from %s to %s", from, to),
	}
}

func NewPaymentNotFoundError(id string) *DomainError {
	return &DomainError{
		Code:    ErrCodePaymentNotFound,
		Message: fmt.Sprintf("payment with ID %s not found", id),
	}
}

func NewDuplicateKeyError(key string) *DomainError {
	return &DomainError{
		Code:    ErrCodeDuplicateIdempotencyKey,
		Message: fmt.Sprintf("idempotency key %s already exists", key),
	}
}

func NewPaymentExpiredError(id string) *DomainError {
	return &DomainError{
		Code:    ErrCodePaymentExpired,
		Message: fmt.Sprintf("payment %s has expired", id),
	}
}

func NewRequestProcessingError() *DomainError {
	return &DomainError{
		Code:    ErrRequestProcessing,
		Message: "request is being processsed",
	}
}

// IsErrorCode checks if an error is a DomainError with a specific code
func IsErrorCode(err error, code string) bool {
	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		return domainErr.Code == code
	}
	return false
}
