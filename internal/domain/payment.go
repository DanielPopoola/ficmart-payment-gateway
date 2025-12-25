package domain

import (
	"errors"
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
	id         PaymentID
	orderID    OrderID
	customerID CustomerID
	amount     Money
	status     PaymentStatus

	bankAuthID *string

	createdAt    time.Time
	authorizedAt *time.Time
	expiresAt    *time.Time
}

func NewPayment(
	id PaymentID,
	orderID OrderID,
	customerID CustomerID,
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

func (p *Payment) ID() PaymentID          { return p.id }
func (p *Payment) OrderID() OrderID       { return p.orderID }
func (p *Payment) CustomerID() CustomerID { return p.customerID }
func (p *Payment) Amount() Money          { return p.amount }
func (p *Payment) Status() PaymentStatus  { return p.status }
func (p *Payment) BankAuthID() *string    { return p.bankAuthID }
func (p *Payment) ExpiresAt() *time.Time  { return p.expiresAt }

func (p *Payment) Authorize(bankAuthID string, authorizedAt, expiresAt time.Time) error {
	if p.status != StatusPending {
		return ErrInvalidState
	}

	// State change
	p.status = StatusAuthorized
	p.bankAuthID = &bankAuthID
	p.authorizedAt = &authorizedAt
	p.expiresAt = &expiresAt

	return nil
}

func (p *Payment) CanAuthorize() bool {
	return p.status == StatusPending
}

// Reconstitute - Special constructor for loading from DB
func Reconstitute(
	id, orderID, customerID string,
	amount int64, currency string,
	status PaymentStatus,
	bankAuthID *string,
	createdAt time.Time, authorizedAt, expiresAt *time.Time,
) *Payment {
	return &Payment{
		id:           PaymentID(id),
		orderID:      OrderID(orderID),
		customerID:   CustomerID(customerID),
		amount:       Money{Amount: amount, Currency: currency},
		status:       status,
		bankAuthID:   (*BankAuthorizationID)(bankAuthID),
		createdAt:    createdAt,
		authorizedAt: authorizedAt,
		expiresAt:    expiresAt,
	}
}
