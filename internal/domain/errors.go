package domain

import (
	"errors"
)

var (
	ErrInvalidTransition       = errors.New("invalid transition")
	ErrPaymentExpired          = errors.New("payment expired")
	ErrPaymentNotFound         = errors.New("payment not found")
	ErrInvalidAmount           = errors.New("invalid amount")
	ErrDuplicateIdempotencyKey = errors.New("duplicate transaction")
	ErrMissingRequiredField    = errors.New("missing required fields")
	ErrInvalidState            = errors.New("invalid state")
	ErrAmountMismatch          = errors.New("amounts mismatch")
)
