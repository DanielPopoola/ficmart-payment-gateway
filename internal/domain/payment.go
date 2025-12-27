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
	id         string
	orderID    string
	customerID string
	amount     Money
	status     PaymentStatus

	bankAuthID    *string
	bankCaptureID *string
	bankVoidID    *string
	bankRefundID  *string

	createdAt    time.Time
	authorizedAt *time.Time
	capturedAt   *time.Time
	voidedAt     *time.Time
	refundedAt   *time.Time
	expiresAt    *time.Time

	attemptCount      int
	nextRetryAt       *time.Time
	lastErrorCategory *string
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
		id:         id,
		orderID:    orderID,
		customerID: customerID,
		amount:     amount,
		status:     StatusPending,
		createdAt:  time.Now(),
	}, nil
}

// Getters
func (p *Payment) ID() string                 { return p.id }
func (p *Payment) OrderID() string            { return p.orderID }
func (p *Payment) CustomerID() string         { return p.customerID }
func (p *Payment) Amount() Money              { return p.amount }
func (p *Payment) Status() PaymentStatus      { return p.status }
func (p *Payment) BankAuthID() *string        { return p.bankAuthID }
func (p *Payment) BankCaptureID() *string     { return p.bankCaptureID }
func (p *Payment) BankVoidID() *string        { return p.bankVoidID }
func (p *Payment) BankRefundID() *string      { return p.bankRefundID }
func (p *Payment) CreatedAt() time.Time       { return p.createdAt }
func (p *Payment) AuthorizedAt() *time.Time   { return p.authorizedAt }
func (p *Payment) CapturedAt() *time.Time     { return p.capturedAt }
func (p *Payment) VoidedAt() *time.Time       { return p.voidedAt }
func (p *Payment) RefundedAt() *time.Time     { return p.refundedAt }
func (p *Payment) ExpiresAt() *time.Time      { return p.expiresAt }
func (p *Payment) AttemptCount() int          { return p.attemptCount }
func (p *Payment) NextRetryAt() *time.Time    { return p.nextRetryAt }
func (p *Payment) LastErrorCategory() *string { return p.lastErrorCategory }

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

func (p *Payment) transition(target PaymentStatus) error {
	if err := p.canTransitionTo(target); err != nil {
		return err
	}
	p.status = target
	return nil
}

// defines various payment statuses that can be transitioned to
func (p *Payment) canTransitionTo(target PaymentStatus) error {
	switch p.status {
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
	p.bankAuthID = &bankAuthID
	p.authorizedAt = &authorizedAt
	p.expiresAt = &expiresAt
	return nil
}

// Capture transitions the payment to captured status and records the bank capture details.
func (p *Payment) Capture(bankCaptureID string, capturedAt time.Time) error {
	if err := p.transition(StatusCaptured); err != nil {
		return err
	}
	p.bankCaptureID = &bankCaptureID
	p.capturedAt = &capturedAt
	return nil
}

// Void transitions the payment to voided status an records the bank void details.
func (p *Payment) Void(bankVoidID string, voidedAt time.Time) error {
	if err := p.transition(StatusVoided); err != nil {
		return err
	}
	p.bankVoidID = &bankVoidID
	p.voidedAt = &voidedAt
	return nil
}

// Refund transitions the payment to refunded status and records the bank refund details
func (p *Payment) Refund(bankRefundID string, refundedAt time.Time) error {
	if err := p.transition(StatusRefunded); err != nil {
		return err
	}
	p.bankRefundID = &bankRefundID
	p.refundedAt = &refundedAt
	return nil
}

// helper to identify payment statuses that are terminal
func (p *Payment) IsTerminal() bool {
	switch p.status {
	case StatusVoided, StatusRefunded, StatusExpired, StatusFailed:
		return true
	default:
		return false
	}
}

func (p *Payment) ScheduleRetry(backoff time.Duration, errorCategory string) {
	p.attemptCount++
	next := time.Now().Add(backoff)
	p.nextRetryAt = &next
	p.lastErrorCategory = &errorCategory
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
		id:                id,
		orderID:           orderID,
		customerID:        customerID,
		amount:            Money{Amount: amount, Currency: currency},
		status:            status,
		bankAuthID:        bankAuthID,
		bankCaptureID:     bankCaptureID,
		bankVoidID:        bankVoidID,
		bankRefundID:      bankRefundID,
		createdAt:         createdAt,
		authorizedAt:      authorizedAt,
		capturedAt:        capturedAt,
		voidedAt:          voidedAt,
		refundedAt:        refundedAt,
		expiresAt:         expiresAt,
		attemptCount:      attempCount,
		nextRetryAt:       nextRetryAt,
		lastErrorCategory: lastErrorCategory,
	}
}
