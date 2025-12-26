package postgres

import (
	"time"
)

// IdempotencyKey represents the lock and validation for a request.
type IdempotencyKey struct {
	Key             string
	PaymentID       string
	RequestHash     string
	LockedAt        *time.Time
	ResponsePayload []byte
	StatusCode      *int
	RecoveryPoint   string
}

// Payment represents db model for domain entity
type PaymentModel struct {
	ID          string
	OrderID     string
	CustomerID  string
	AmountCents int64
	Currency    string
	Status      string

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
