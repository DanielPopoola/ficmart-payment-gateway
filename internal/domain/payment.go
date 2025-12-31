// Package domain encodes a payment entity and it's attributes
package domain

import (
	"errors"
	"slices"
	"time"
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

type Payment struct {
	ID          string
	OrderID     string
	CustomerID  string
	AmountCents int64
	Currency    string
	Status      PaymentStatus

	BankAuthID    *string
	BankCaptureID *string
	BankVoidID    *string
	BankRefundID  *string

	CreatedAt    time.Time
	AuthorizedAt *time.Time
	CapturedAt   *time.Time
	VoidedAt     *time.Time
	RefundedAt   *time.Time
	ExpiresAt    *time.Time

	AttemptCount      int
	NextRetryAt       *time.Time
	LastErrorCategory *string
}

func NewPayment(
	id string,
	orderID string,
	customerID string,
	amount Money,
) (*Payment, error) {
	if id == "" {
		return nil, errors.New("payment ID is required")
	}
	if orderID == "" {
		return nil, errors.New("order ID is required")
	}
	if customerID == "" {
		return nil, errors.New("customer ID is required")
	}

	return &Payment{
		ID:          id,
		OrderID:     orderID,
		CustomerID:  customerID,
		AmountCents: amount.Amount,
		Currency:    amount.Currency,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
	}, nil
}

func (p *Payment) MarkCapturing() error {
	return p.transition(StatusCapturing)
}

func (p *Payment) MarkVoiding() error {
	return p.transition(StatusVoiding)
}

func (p *Payment) MarkRefunding() error {
	return p.transition(StatusRefunding)
}

func (p *Payment) Fail() error {
	return p.transition(StatusFailed)
}

func (p *Payment) MarkExpired() error {
	return p.transition(StatusExpired)
}

func (p *Payment) transition(target PaymentStatus) error {
	if err := p.canTransitionTo(target); err != nil {
		return err
	}
	p.Status = target
	return nil
}

// defines various payment statuses that can be transitioned to
func (p *Payment) canTransitionTo(target PaymentStatus) error {
	switch p.Status {
	case StatusPending:
		return p.allow(target, StatusAuthorized, StatusFailed)
	case StatusAuthorized:
		return p.allow(target, StatusCapturing, StatusVoiding, StatusExpired, StatusFailed)
	case StatusCapturing:
		return p.allow(target, StatusCaptured, StatusFailed)
	case StatusCaptured:
		return p.allow(target, StatusRefunding, StatusFailed)
	case StatusRefunding:
		return p.allow(target, StatusRefunded, StatusFailed)
	case StatusVoiding:
		return p.allow(target, StatusVoided, StatusFailed)
	}
	return ErrInvalidTransition
}

// Helper to check allowed state transitions
func (p *Payment) allow(target PaymentStatus, allowed ...PaymentStatus) error {
	if slices.Contains(allowed, target) {
		return nil
	}
	return ErrInvalidTransition
}

// Authorize sets the payment status to authorized and records the bank authorization details.
func (p *Payment) Authorize(bankAuthID string, authorizedAt, expiresAt time.Time) error {
	if err := p.transition(StatusAuthorized); err != nil {
		return err
	}
	p.BankAuthID = &bankAuthID
	p.AuthorizedAt = &authorizedAt
	p.ExpiresAt = &expiresAt
	return nil
}

// Capture transitions the payment to captured status and records the bank capture details.
func (p *Payment) Capture(bankCaptureID string, capturedAt time.Time) error {
	if err := p.transition(StatusCaptured); err != nil {
		return err
	}
	p.BankCaptureID = &bankCaptureID
	p.CapturedAt = &capturedAt
	return nil
}

// Void transitions the payment to voided status an records the bank void details.
func (p *Payment) Void(bankVoidID string, voidedAt time.Time) error {
	if err := p.transition(StatusVoided); err != nil {
		return err
	}
	p.BankVoidID = &bankVoidID
	p.VoidedAt = &voidedAt
	return nil
}

// Refund transitions the payment to refunded status and records the bank refund details
func (p *Payment) Refund(bankRefundID string, refundedAt time.Time) error {
	if err := p.transition(StatusRefunded); err != nil {
		return err
	}
	p.BankRefundID = &bankRefundID
	p.RefundedAt = &refundedAt
	return nil
}

// helper to identify payment statuses that are terminal
func (p *Payment) IsTerminal() bool {
	switch p.Status {
	case StatusVoided, StatusRefunded, StatusExpired, StatusFailed:
		return true
	default:
		return false
	}
}

func (p *Payment) ScheduleRetry(backoff time.Duration, errorCategory string) {
	p.AttemptCount++
	next := time.Now().Add(backoff)
	p.NextRetryAt = &next
	p.LastErrorCategory = &errorCategory
}

func (p *Payment) FailWithCategory(errorCategory string) error {
	if err := p.canTransitionTo(StatusFailed); err != nil {
		return err
	}
	p.LastErrorCategory = &errorCategory
	return nil
}

// Reconstitute - Special constructor for loading from DB
func Reconstitute(
	id string, orderID string, customerID string,
	amount int64, currency string,
	status PaymentStatus,
	bankAuthID, bankCaptureID, bankVoidID, bankRefundID *string,
	createdAt time.Time,
	authorizedAt, capturedAt, voidedAt, refundedAt, expiresAt *time.Time,
	attempCount int, nextRetryAt *time.Time, lastErrorCategory *string,
) *Payment {
	return &Payment{
		ID:                id,
		OrderID:           orderID,
		CustomerID:        customerID,
		AmountCents:       amount,
		Currency:          currency,
		Status:            status,
		BankAuthID:        bankAuthID,
		BankCaptureID:     bankCaptureID,
		BankVoidID:        bankVoidID,
		BankRefundID:      bankRefundID,
		CreatedAt:         createdAt,
		AuthorizedAt:      authorizedAt,
		CapturedAt:        capturedAt,
		VoidedAt:          voidedAt,
		RefundedAt:        refundedAt,
		ExpiresAt:         expiresAt,
		AttemptCount:      attempCount,
		NextRetryAt:       nextRetryAt,
		LastErrorCategory: lastErrorCategory,
	}
}
