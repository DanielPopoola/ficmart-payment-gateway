package testdata

// Test cards from bank API documentation
type TestCard struct {
	CardNumber  string
	CVV         string
	ExpiryMonth int
	ExpiryYear  int
	Description string
}

var (
	ValidCard = TestCard{
		CardNumber:  "4111111111111111",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
		Description: "Happy path card",
	}

	InsufficientFundsCard = TestCard{
		CardNumber:  "5555555555554444",
		CVV:         "789",
		ExpiryMonth: 9,
		ExpiryYear:  2030,
		Description: "Card with $0 balance",
	}

	ExpiredCard = TestCard{
		CardNumber:  "5105105105105100",
		CVV:         "321",
		ExpiryMonth: 3,
		ExpiryYear:  2020,
		Description: "Expired card",
	}
)
