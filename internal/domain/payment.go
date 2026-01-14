// Package domain encodes a payment entity and it's attributes
package domain

import (
	"errors"
	"slices"
	"time"
)

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
	CreatedAt     time.Time
	ID            string
	OrderID       string
	CustomerID    string
	Currency      string
	BankAuthID    *string
	BankCaptureID *string
	BankVoidID    *string
	BankRefundID  *string
	AuthorizedAt  *time.Time
	CapturedAt    *time.Time
	VoidedAt      *time.Time
	RefundedAt    *time.Time
	ExpiresAt     *time.Time
	NextRetryAt   *time.Time
	AmountCents   int64
	Status        PaymentStatus
	AttemptCount  int
}

func NewPayment(
	id string,
	orderID string,
	customerID string,
	amount int64, currency string,
) (*Payment, error) {
	if id == "" {
		return nil, errors.New("payment ID is required")
	}
	if amount < 0 {
		return nil, ErrInvalidAmount
	}
	if currency == "" {
		return nil, errors.New("invalid currency type")
	}

	return &Payment{
		ID:          id,
		OrderID:     orderID,
		CustomerID:  customerID,
		AmountCents: amount,
		Currency:    currency,
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
	case StatusFailed, StatusRefunded, StatusVoided, StatusExpired:
		return ErrInvalidTransition
	}
	return ErrInvalidTransition
}

func (p *Payment) allow(target PaymentStatus, allowed ...PaymentStatus) error {
	if slices.Contains(allowed, target) {
		return nil
	}
	return ErrInvalidTransition
}

func (p *Payment) Authorize(bankAuthID string, authorizedAt, expiresAt time.Time) error {
	if err := p.transition(StatusAuthorized); err != nil {
		return err
	}
	p.BankAuthID = &bankAuthID
	p.AuthorizedAt = &authorizedAt
	p.ExpiresAt = &expiresAt
	return nil
}

func (p *Payment) Capture(status, bankCaptureID string, capturedAt time.Time) error {
	if status != "captured" {
		return ErrPaymentExpired
	}
	if err := p.transition(StatusCaptured); err != nil {
		return err
	}
	p.BankCaptureID = &bankCaptureID
	p.CapturedAt = &capturedAt
	return nil
}

func (p *Payment) Void(status, bankVoidID string, voidedAt time.Time) error {
	if status != "voided" {
		return ErrPaymentExpired
	}
	if err := p.transition(StatusVoided); err != nil {
		return err
	}
	p.BankVoidID = &bankVoidID
	p.VoidedAt = &voidedAt
	return nil
}

func (p *Payment) Refund(status, bankRefundID string, refundedAt time.Time) error {
	if status != "refunded" {
		return ErrPaymentExpired
	}
	if err := p.transition(StatusRefunded); err != nil {
		return err
	}
	p.BankRefundID = &bankRefundID
	p.RefundedAt = &refundedAt
	return nil
}

func (p *Payment) IsTerminal() bool {
	switch p.Status {
	case StatusVoided, StatusRefunded, StatusExpired, StatusFailed:
		return true
	case StatusPending, StatusAuthorized, StatusCapturing, StatusCaptured, StatusRefunding, StatusVoiding:
		return false
	}
	return false
}

func (p *Payment) ScheduleRetry(backoff time.Duration) {
	p.AttemptCount++
	next := time.Now().Add(backoff)
	p.NextRetryAt = &next
}
