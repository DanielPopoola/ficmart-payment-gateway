package postgres

import (
	"time"
)

// IdempotencyKey enforces at-most-once semantics via unique constraint on key.
// LockedAt prevents polling clients from blocking on uncommitted rows.
type IdempotencyKey struct {
	Key             string
	PaymentID       string
	RequestHash     string
	LockedAt        *time.Time
	ResponsePayload *[]byte
	StatusCode      *int
}
