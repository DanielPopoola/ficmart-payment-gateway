package persistence

import (
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

// PaymentModel - Database representation
type PaymentModel struct {
	ID           string
	OrderID      string
	CustomerID   string
	AmountCents  int64
	Currency     string
	Status       string
	BankAuthID   *string
	CreatedAt    time.Time
	AuthorizedAt *time.Time
	ExpiresAt    *time.Time
}

// toDomainModel - Database → Domain
func toDomainModel(m PaymentModel) *domain.Payment {
	// Use Reconstitute for loading from DB
	return domain.Reconstitute(
		m.ID,
		m.OrderID,
		m.CustomerID,
		m.AmountCents,
		domain.PaymentStatus(m.Status),
		m.BankAuthID,
		m.CreatedAt,
		m.AuthorizedAt,
		m.ExpiresAt,
	)
}

// toDBModel - Domain → Database
func toDBModel(p *domain.Payment) PaymentModel {
	return PaymentModel{
		ID:          p.ID(),
		OrderID:     p.OrderID(),
		CustomerID:  p.CustomerID(),
		AmountCents: p.AmountCents(),
		Currency:    "USD",
		Status:      string(p.Status()),
		BankAuthID:  p.BankAuthID(), // Already a pointer
		// ... map all fields
	}
}
