package services

type AuthorizeCommand struct {
	OrderID        string
	CustomerID     string
	Amount         int64
	Currency       string
	CardNumber     string
	CVV            string
	ExpiryMonth    int
	ExpiryYear     int
	IdempotencyKey string
}

type CaptureCommand struct {
	PaymentID      string
	Amount         int64
	IdempotencyKey string
}

type VoidCommand struct {
	PaymentID      string
	IdempotencyKey string
}

type RefundCommand struct {
	PaymentID      string
	Amount         int64
	IdempotencyKey string
}
