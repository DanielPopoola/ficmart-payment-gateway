package entities

import (
	"time"

	"github.com/google/uuid"
)

// PaymentStatus represents the current state of a payment in its lifecycle
type PaymentStatus string

const (
	StatusPending    PaymentStatus = "PENDING"
	StatusAuthorized PaymentStatus = "AUTHORIZED"
	StatusCapturing  PaymentStatus = "CAPTURING"
	StatusCaptured   PaymentStatus = "CAPTURED"
	StatusFailed     PaymentStatus = "FAILED"
	StatusRefunded   PaymentStatus = "REFUNDED"
	StatusRefunding  PaymentStatus = "REFUNDING"
	StatusVoiding    PaymentStatus = "VOIDING"
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

type PendingPaymentCheck struct {
	ID             uuid.UUID
	IdempotencyKey string
	BankAuthID     *string
	BankCaptureID  *string
	Status         PaymentStatus
	AttemptCount   int
}

// CanTransitionTo validates whether a payment can transition from its current status to the target status.
func (p *Payment) CanTransitionTo(target PaymentStatus) error {
	switch p.Status {
	case StatusVoided, StatusRefunded, StatusExpired, StatusFailed:
		return NewInvalidTransitionError(p.Status, target)

	case StatusPending:
		if target == StatusAuthorized || target == StatusFailed {
			return nil
		}

	case StatusAuthorized:
		if target == StatusCaptured || target == StatusVoided || target == StatusExpired || target == StatusFailed || target == StatusCapturing || target == StatusVoiding {
			return nil
		}

	case StatusCapturing:
		if target == StatusCaptured || target == StatusFailed {
			return nil
		}

	case StatusVoiding:
		if target == StatusVoided || target == StatusFailed {
			return nil
		}

	case StatusCaptured:
		if target == StatusRefunded || target == StatusRefunding {
			return nil
		}

	case StatusRefunding:
		if target == StatusRefunded || target == StatusFailed {
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

func (p *Payment) Authorize(authID string, authorizedAt, expiresAt time.Time) error {
	if err := p.CanTransitionTo(StatusAuthorized); err != nil {
		return err
	}
	p.Status = StatusAuthorized
	p.BankAuthID = &authID
	p.AuthorizedAt = &authorizedAt
	p.ExpiresAt = &expiresAt
	return nil
}

func (p *Payment) Capture(captureID string, capturedAt time.Time) error {
	if err := p.CanTransitionTo(StatusCaptured); err != nil {
		return err
	}
	p.Status = StatusCaptured
	p.BankCaptureID = &captureID
	p.CapturedAt = &capturedAt
	return nil
}

func (p *Payment) Void(voidID string, voidedAt time.Time) error {
	if err := p.CanTransitionTo(StatusVoided); err != nil {
		return err
	}
	p.Status = StatusVoided
	p.BankVoidID = &voidID
	p.VoidedAt = &voidedAt
	return nil
}

func (p *Payment) Refund(refundID string, refundedAt time.Time) error {
	if err := p.CanTransitionTo(StatusRefunded); err != nil {
		return err
	}
	p.Status = StatusRefunded
	p.BankRefundID = &refundID
	p.RefundedAt = &refundedAt
	return nil
}

func (p *Payment) Fail(reason string) error {
	if err := p.CanTransitionTo(StatusFailed); err != nil {
		return err
	}
	p.Status = StatusFailed
	p.LastErrorCategory = &reason
	return nil
}

func (p *Payment) ScheduleRetry(reason string, nextRetry time.Time) {
	p.AttemptCount++
	p.LastErrorCategory = &reason
	p.NextRetryAt = &nextRetry
}
