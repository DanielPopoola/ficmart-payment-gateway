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
	ResponsePayload *[]byte
	StatusCode      *int
}
