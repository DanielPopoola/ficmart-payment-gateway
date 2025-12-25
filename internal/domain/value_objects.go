package domain

import (
	"errors"
)

type PaymentID string

type Money struct {
	Amount   int64
	Currency string
}

func NewMoney(amount int64, currency string) (Money, error) {
	if amount < 0 {
		return Money{}, errors.New("amount cannot be negative")
	}
	if currency == "" {
		return Money{}, errors.New("currency is required")
	}
	return Money{Amount: amount, Currency: currency}, nil
}

// OrderID is the external identifier from FicMart
type OrderID string

// CustomerID is the external identifier for customers
type CustomerID string

// BankAuthorizationID is what the bank gives us
type BankAuthorizationID string
