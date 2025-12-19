package domain

import "time"

type BankAuthorizationRequest struct {
	Amount      int64  `json:"amount"`
	CardNumber  string `json:"card_number"`
	Cvv         string `json:"cvv"`
	ExpiryMonth int    `json:"expiry_month"`
	ExpiryYear  int    `json:"expiry_year"`
}

type BankAuthorizationResponse struct {
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`
	Status          string    `json:"status"`
	AuthorizationID string    `json:"authorization_id"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type BankCaptureRequest struct {
	Amount          int64  `json:"amount"`
	AuthorizationID string `json:"authorization_id"`
}

type BankCaptureResponse struct {
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`
	AuthorizationID string    `json:"authorization_id"`
	CaptureID       string    `json:"capture_id"`
	Status          string    `json:"status"`
	CapturedAt      time.Time `json:"captured_at"`
}

type BankVoidRequest struct {
	AuthorizationID string `json:"authorization_id"`
}

type BankVoidResponse struct {
	AuthorizationID string    `json:"authorization_id"`
	Status          string    `json:"status"`
	VoidID          string    `json:"void_id"`
	VoidedAt        time.Time `json:"voided_at"`
}

type BankRefundRequest struct {
	Amount    int64  `json:"amount"`
	CaptureID string `json:"capture_id"`
}

type BankRefundResponse struct {
	Amount     int64     `json:"amount"`
	Currency   string    `json:"currency"`
	Status     string    `json:"status"`
	CaptureID  string    `json:"capture_id"`
	RefundID   string    `json:"refund_id"`
	RefundedAt time.Time `json:"refunded_at"`
}
