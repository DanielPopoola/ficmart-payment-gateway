package service

import (
	"context"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/repository"
)

type PaymentQueryService struct {
	repo repository.PaymentRepository
}

func NewPaymentQueryService(repo repository.PaymentRepository) *PaymentQueryService {
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
