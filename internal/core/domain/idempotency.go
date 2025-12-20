package domain

import (
	"encoding/json"
	"time"
)

// IdempotencyKey represents the state of a request for idempotency purposes.
type IdempotencyKey struct {
	Key             string
	RequestPayload  json.RawMessage
	ResponsePayload json.RawMessage
	StatusCode      *int
	LockedAt        time.Time
	CompletedAt     *time.Time
}

// IsComplete checks if the request associated with this key has been processed.
func (i *IdempotencyKey) IsComplete() bool {
	return i.ResponsePayload != nil
}
