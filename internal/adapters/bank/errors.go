package bank

import "fmt"

type BankError struct {
	Code       string
	Err        error
	Message    string
	StatusCode int
}

func (e *BankError) Error() string {
	return fmt.Sprintf("bank error: %s (status: %d)", e.Message, e.StatusCode)
}

type BankErrorResponse struct {
	Err     string `json:"error"`
	Message string `json:"message"`
}
