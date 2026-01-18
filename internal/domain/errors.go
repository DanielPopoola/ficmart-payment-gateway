package domain

import (
	"errors"
)

var (
	ErrInvalidTransition    = errors.New("invalid transition")
	ErrPaymentExpired       = errors.New("payment expired")
	ErrInvalidAmount        = errors.New("invalid amount")
	ErrMissingRequiredField = errors.New("missing required fields")
	ErrInvalidState         = errors.New("invalid state")
)
