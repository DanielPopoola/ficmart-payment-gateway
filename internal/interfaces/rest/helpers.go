package rest

import (
	"fmt"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/google/uuid"
)

func ToAPIPayment(p *domain.Payment) (api.Payment, error) {
	parsedID, err := uuid.Parse(p.ID)
	if err != nil {
		return api.Payment{}, fmt.Errorf("failed to parse payment ID '%s' as UUID: %w", p.ID, err)
	}

	apiPayment := api.Payment{
		AmountCents:  p.AmountCents,
		CreatedAt:    p.CreatedAt,
		Currency:     p.Currency,
		CustomerId:   p.CustomerID,
		Id:           parsedID,
		OrderId:      p.OrderID,
		Status:       api.PaymentStatus(p.Status),
		AttemptCount: p.AttemptCount,
	}

	if p.AuthorizedAt != nil {
		apiPayment.AuthorizedAt = *p.AuthorizedAt
	}
	if p.CapturedAt != nil {
		apiPayment.CapturedAt = *p.CapturedAt
	}
	if p.VoidedAt != nil {
		apiPayment.VoidedAt = *p.VoidedAt
	}
	if p.RefundedAt != nil {
		apiPayment.RefundedAt = *p.RefundedAt
	}
	if p.ExpiresAt != nil {
		apiPayment.ExpiresAt = *p.ExpiresAt
	}
	if p.BankAuthID != nil {
		apiPayment.BankAuthId = *p.BankAuthID
	}
	if p.BankCaptureID != nil {
		apiPayment.BankCaptureId = *p.BankCaptureID
	}
	if p.BankVoidID != nil {
		apiPayment.BankVoidId = *p.BankVoidID
	}
	if p.BankRefundID != nil {
		apiPayment.BankRefundId = *p.BankRefundID
	}
	if p.LastErrorCategory != nil {
		apiPayment.LastErrorCategory = *p.LastErrorCategory
	}
	if p.NextRetryAt != nil {
		apiPayment.NextRetryAt = *p.NextRetryAt
	}

	return apiPayment, nil
}
