package postgres

import (
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

// toDomainModel: maps db model to domain entity
func toDomainModel(m PaymentModel) *domain.Payment {
	return domain.Reconstitute(
		m.ID,
		m.OrderID,
		m.CustomerID,
		m.AmountCents,
		m.Currency,
		domain.PaymentStatus(m.Status),
		m.BankAuthID,
		m.BankCaptureID,
		m.BankVoidID,
		m.BankRefundID,
		m.CreatedAt,
		m.AuthorizedAt,
		m.CapturedAt,
		m.VoidedAt,
		m.RefundedAt,
		m.ExpiresAt,
		m.AttemptCount,
		m.NextRetryAt,
		m.LastErrorCategory,
	)
}

// toDBModel: maps domain entity to db model
func toDBModel(p *domain.Payment) *PaymentModel {
	return &PaymentModel{
		ID:                p.ID(),
		OrderID:           p.OrderID(),
		CustomerID:        p.CustomerID(),
		AmountCents:       p.Amount().Amount,
		Currency:          p.Amount().Currency,
		Status:            string(p.Status()),
		BankAuthID:        p.BankAuthID(),
		BankCaptureID:     p.BankCaptureID(),
		BankVoidID:        p.BankVoidID(),
		BankRefundID:      p.BankRefundID(),
		CreatedAt:         p.CreatedAt(),
		AuthorizedAt:      p.AuthorizedAt(),
		CapturedAt:        p.CapturedAt(),
		VoidedAt:          p.VoidedAt(),
		RefundedAt:        p.RefundedAt(),
		ExpiresAt:         p.ExpiresAt(),
		AttemptCount:      p.AttemptCount(),
		NextRetryAt:       p.NextRetryAt(),
		LastErrorCategory: p.LastErrorCategory(),
	}
}
