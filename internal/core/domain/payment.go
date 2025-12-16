// Package models defines the domain models for the payment gateway.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// PaymentStatus represents the current state of a payment in its lifecycle
type PaymentStatus string

const (
	StatusPending    PaymentStatus = "PENDING"
	StatusAuthorized PaymentStatus = "AUTHORIZED"
	StatusCaptured   PaymentStatus = "CAPTURED"
	StatusFailed     PaymentStatus = "FAILED"
	StatusRefunded   PaymentStatus = "REFUNDED"
	StatusVoided     PaymentStatus = "VOIDED"
	StatusExpired    PaymentStatus = "EXPIRED"
)

// Payment represents a payment transaction in the system
type Payment struct {
	ID          uuid.UUID
	OrderID     string
	CustomerID  string
	AmountCents int64
	Currency    string

	Status         PaymentStatus
	IdempotencyKey string
	BankAuthID     *string
	BankCaptureID  *string
	BankVoidID     *string
	BankRefundID   *string

	CreatedAt    time.Time
	UpdatedAt    time.Time
	AuthorizedAt *time.Time
	CapturedAt   *time.Time
	VoidedAt     *time.Time
	RefundedAt   *time.Time
	ExpiresAt    *time.Time // When authorization expires (7 days from auth)

	AttemptCount      int
	NextRetryAt       *time.Time
	LastErrorCategory *string
}

// CanTransitionTo validates whether a payment can transition from its current status to the target status.
// It returns nil if the transition is allowed, otherwise returns an error describing why the transition is invalid.
//
// Terminal states (Voided, Refunded, Expired, Failed) do not allow any further transitions.
//
// Valid transitions are:
//   - Pending → Authorized, Failed
//   - Authorized → Captured, Voided, Expired, Failed
//   - Captured → Refunded
//
// Any other transition will return an error.
func (p *Payment) CanTransitionTo(target PaymentStatus) error {
	switch p.Status {
	case StatusVoided, StatusRefunded, StatusExpired, StatusFailed:
		return NewInvalidTransitionError(p.Status, target)

	case StatusPending:
		if target == StatusAuthorized || target == StatusFailed {
			return nil
		}

	case StatusAuthorized:
		if target == StatusCaptured || target == StatusVoided || target == StatusExpired || target == StatusFailed {
			return nil
		}

	case StatusCaptured:
		if target == StatusRefunded {
			return nil
		}
	}
	return NewInvalidTransitionError(p.Status, target)
}

func (p *Payment) IsTerminal() bool {
	switch p.Status {
	case StatusVoided, StatusRefunded, StatusExpired, StatusFailed:
		return true
	default:
		return false
	}
}
