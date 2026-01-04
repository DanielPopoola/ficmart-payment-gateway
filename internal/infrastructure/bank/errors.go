package bank

import (
	"errors"
	"fmt"
)

type BankError struct {
	Code       string
	Message    string
	StatusCode int
}

type BankErrorResponse struct {
	Err     string `json:"error"`
	Message string `json:"message"`
}

func (e *BankError) Error() string {
	return fmt.Sprintf("bank error [%s]: %s (status: %d)", e.Code, e.Message, e.StatusCode)
}

func (e *BankError) IsRetryable() bool {
	return e.StatusCode >= 500
}

func IsBankError(err error) (*BankError, bool) {
	var bankErr *BankError
	ok := errors.As(err, &bankErr)
	return bankErr, ok
}
