package services

import (
	"context"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
)

type QueryService struct {
	paymentRepo *postgres.PaymentRepository
}

func NewQueryService(
	paymentRepo *postgres.PaymentRepository,
) *QueryService {
	return &QueryService{
		paymentRepo: paymentRepo,
	}
}

func (s *QueryService) FindByID(ctx context.Context, id string) (*domain.Payment, error) {
	return s.paymentRepo.FindByID(ctx, id)
}

func (s *QueryService) FindByOrderID(ctx context.Context, orderID string) (*domain.Payment, error) {
	return s.paymentRepo.FindByOrderID(ctx, orderID)
}

func (s *QueryService) FindByCustomerID(ctx context.Context, customerID string, limit, offset int) ([]*domain.Payment, error) {
	return s.paymentRepo.FindByCustomerID(ctx, customerID, limit, offset)
}
