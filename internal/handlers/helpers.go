package handlers

import (
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

func mapDomainPaymentToAPI(p *domain.Payment) api.Payment {
	apiP := api.Payment{
		Id:             p.ID,
		OrderId:        p.OrderID,
		AmountCents:    p.AmountCents,
		Currency:       p.Currency,
		Status:         api.PaymentStatus(p.Status),
		IdempotencyKey: p.IdempotencyKey,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
		AttemptCount:   p.AttemptCount,
	}

	if p.BankAuthID != nil {
		apiP.BankAuthId = *p.BankAuthID
	}
	if p.BankCaptureID != nil {
		apiP.BankCaptureId = *p.BankCaptureID
	}
	if p.BankVoidID != nil {
		apiP.BankVoidId = *p.BankVoidID
	}
	if p.BankRefundID != nil {
		apiP.BankRefundId = *p.BankRefundID
	}
	if p.AuthorizedAt != nil {
		apiP.AuthorizedAt = *p.AuthorizedAt
	}
	if p.CapturedAt != nil {
		apiP.CapturedAt = *p.CapturedAt
	}
	if p.VoidedAt != nil {
		apiP.VoidedAt = *p.VoidedAt
	}
	if p.RefundedAt != nil {
		apiP.RefundedAt = *p.RefundedAt
	}
	if p.ExpiresAt != nil {
		apiP.ExpiresAt = *p.ExpiresAt
	}
	if p.NextRetryAt != nil {
		apiP.NextRetryAt = *p.NextRetryAt
	}
	if p.LastErrorCategory != nil {
		apiP.LastErrorCategory = *p.LastErrorCategory
	}

	return apiP
}
