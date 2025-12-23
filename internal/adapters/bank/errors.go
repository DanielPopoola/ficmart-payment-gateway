package bank

import "fmt"

type BankError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *BankError) Error() string {
	return fmt.Sprintf("bank error: %s (status: %d)", e.Message, e.StatusCode)
}

// IsRetryable checks if the error is transient (5xx) or permanent (4xx)
func (e *BankError) IsRetryable() bool {
	if e.StatusCode >= 500 {
		return true
	}
	return false
}

type BankErrorResponse struct {
	Err     string `json:"error"`
	Message string `json:"message"`
}