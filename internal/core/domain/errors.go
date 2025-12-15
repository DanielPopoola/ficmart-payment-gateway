package domain

import "fmt"

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

// Domain validation errors
const (
	ErrCodeInvalidTransition = "INVALID_TRANSITION"
	ErrCodePaymentExpired    = "PAYMENT_EXPIRED"
	ErrCodePaymentNotFound   = "PAYMENT_NOT_FOUND"
	ErrCodeInvalidAmount     = "INVALID_AMOUNT"
)

func NewInvalidTransitionError(from, to PaymentStatus) *DomainError {
	return &DomainError{
		Code:    ErrCodeInvalidTransition,
		Message: fmt.Sprintf("cannot transition from %s to %s", from, to),
	}
}
