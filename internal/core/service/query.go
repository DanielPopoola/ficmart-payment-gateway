package service

import (
	"context"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
)

type PaymentQueryService struct {
	repo ports.PaymentRepository
}

func NewPaymentQueryService(repo ports.PaymentRepository) *PaymentQueryService {
	return &PaymentQueryService{
		repo: repo,
	}
}

func (s *PaymentQueryService) GetPaymentByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	return s.repo.FindByOrderID(ctx, orderID)
}

func (s *PaymentQueryService) GetPaymentsByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
	return s.repo.FindByCustomerID(ctx, customerID, limit, offset)
}
