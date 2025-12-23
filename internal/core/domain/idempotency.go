package domain

import (
	"time"
)

// IdempotencyKey represents the lock and validation for a request.
type IdempotencyKey struct {
	Key             string
	RequestHash     string
	LockedAt        time.Time
	ResponsePayload []byte
	StatusCode      int
	CompletedAt     *time.Time
}