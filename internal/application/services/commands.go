package services

type AuthorizeCommand struct {
	OrderID     string
	CustomerID  string
	Amount      int64
	Currency    string
	CardNumber  string
	CVV         string
	ExpiryMonth int
	ExpiryYear  int
}

type CaptureCommand struct {
	PaymentID string
	Amount    int64
}

type VoidCommand struct {
	PaymentID string
}

type RefundCommand struct {
	PaymentID string
	Amount    int64
}
