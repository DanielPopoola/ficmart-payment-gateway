package application

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
)

// ErrorCategory represents the nature of an error for retry logic
type ErrorCategory string

const (
	CategoryTransient      ErrorCategory = "TRANSIENT"
	CategoryPermanent      ErrorCategory = "PERMANENT"
	CategoryBusinessRule   ErrorCategory = "BUSINESS_RULE"
	CategoryClientError    ErrorCategory = "CLIENT_ERROR"
	CategoryInfrastructure ErrorCategory = "INFRASTRUCTURE"
)

// CategorizeError determines error category for retry and logging purposes
func CategorizeError(err error) ErrorCategory {
	if err == nil {
		return ""
	}

	// Context Errors (Transient - network/timeout issues)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return CategoryTransient
	}

	// Domain Errors (Business Rules)
	if errors.Is(err, domain.ErrPaymentExpired) {
		return CategoryBusinessRule
	}

	if errors.Is(err, domain.ErrInvalidAmount) ||
		errors.Is(err, domain.ErrAmountMismatch) {
		return CategoryBusinessRule
	}

	// State/Transition errors are conflicts
	if errors.Is(err, domain.ErrInvalidTransition) ||
		errors.Is(err, domain.ErrInvalidState) {
		return CategoryBusinessRule
	}

	// Persistence Errors
	if errors.Is(err, postgres.ErrPaymentNotFound) ||
		errors.Is(err, domain.ErrMissingRequiredField) {
		return CategoryClientError
	}

	// Service/Application Errors
	if svcErr, ok := IsServiceError(err); ok {
		switch svcErr.Code {
		case ErrCodeIdempotencyMismatch, ErrCodeInvalidInput:
			return CategoryClientError
		case ErrCodeInternal:
			return CategoryInfrastructure
		case ErrCodeRequestProcessing, ErrCodeTimeout:
			return CategoryTransient
		}
	}

	// Bank Errors (External API)
	if bankErr, ok := IsBankError(err); ok {
		if bankErr.StatusCode >= 500 {
			return CategoryTransient
		}

		switch bankErr.Code {
		// PERMANENT: Card/Payment Issues (Customer must fix)
		case "invalid_card":
			return CategoryPermanent
		case "invalid_cvv":
			return CategoryPermanent
		case "card_expired":
			return CategoryPermanent
		case "insufficient_funds":
			return CategoryPermanent
		case "invalid_amount":
			return CategoryPermanent
		case "amount_mismatch":
			return CategoryPermanent
		case "authorization_already_used":
			return CategoryPermanent
		case "already_captured":
			return CategoryPermanent
		case "already_voided":
			return CategoryPermanent
		case "already_refunded":
			return CategoryPermanent
		case "authorization_expired":
			return CategoryPermanent

		// CLIENT_ERROR: Not Found / Missing Data
		case "authorization_not_found":
			return CategoryClientError
		case "capture_not_found":
			return CategoryClientError
		case "refund_not_found":
			return CategoryClientError
		case "not_found":
			return CategoryClientError
		case "missing_idempotency_key":
			return CategoryClientError

		// TRANSIENT: Infrastructure Issues (Retry safe)
		case "internal_error":
			return CategoryTransient

		default:
			return CategoryPermanent
		}
	}

	// Default: Transient (safe fallback)
	return CategoryTransient
}

// IsRetryable returns true if the error category suggests retry
func IsRetryable(err error) bool {
	category := CategorizeError(err)
	return category == CategoryTransient || category == CategoryInfrastructure
}

// ToHTTPStatus maps error to appropriate HTTP status code
func ToHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}

	if svcErr, ok := IsServiceError(err); ok {
		return svcErr.HTTPStatus
	}

	switch {
	case errors.Is(err, domain.ErrInvalidAmount),
		errors.Is(err, domain.ErrAmountMismatch),
		errors.Is(err, domain.ErrMissingRequiredField):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrInvalidTransition),
		errors.Is(err, domain.ErrInvalidState),
		errors.Is(err, postgres.ErrDuplicateIdempotencyKey),
		errors.Is(err, domain.ErrPaymentExpired):
		return http.StatusConflict

	case errors.Is(err, postgres.ErrPaymentNotFound):
		return http.StatusNotFound

	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusRequestTimeout

	case errors.Is(err, context.Canceled):
		return http.StatusRequestTimeout
	}

	if bankErr, ok := IsBankError(err); ok {
		return bankErr.StatusCode
	}

	// Default to 500
	return http.StatusInternalServerError
}

// ToErrorCode clear error code for API responses
func ToErrorCode(err error) string {
	if svcErr, ok := IsServiceError(err); ok {
		return svcErr.Code
	}

	if errors.Is(err, domain.ErrInvalidTransition) {
		return "INVALID_TRANSITION"
	}
	if errors.Is(err, domain.ErrPaymentExpired) {
		return "PAYMENT_EXPIRED"
	}
	if errors.Is(err, domain.ErrInvalidAmount) {
		return "INVALID_AMOUNT"
	}
	if errors.Is(err, domain.ErrMissingRequiredField) {
		return "MISSING_REQUIRED_FIELD"
	}
	if errors.Is(err, domain.ErrInvalidState) {
		return "INVALID_STATE"
	}
	if errors.Is(err, domain.ErrAmountMismatch) {
		return "AMOUNT_MISMATCH"
	}
	if errors.Is(err, postgres.ErrDuplicateIdempotencyKey) {
		return "DUPLICATE_IDEMPOTENCY_KEY"
	}
	if errors.Is(err, postgres.ErrPaymentNotFound) {
		return "PAYMENT_NOT_FOUND"
	}

	if bankErr, ok := IsBankError(err); ok {
		return strings.ToUpper(bankErr.Code)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}

	if svcErr, ok := IsServiceError(err); ok {
		return svcErr.Code
	}

	return "INTERNAL_ERROR"
}
