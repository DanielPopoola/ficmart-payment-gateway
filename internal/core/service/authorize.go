package service

import (
	"context"
	"errors"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
)

type AuthorizationService struct {
	repo       ports.PaymentRepository
	bankClient ports.BankPort
}

func NewAuthorizationService(repo ports.PaymentRepository, bankClient ports.BankPort) *AuthorizationService {
	return &AuthorizationService{
		repo:       repo,
		bankClient: bankClient,
	}
}

func (s *AuthorizationService) Authorize(
	ctx context.Context,
	orderID, customerID, idempotencyKey string,
	amount int64,
	cardNumber, cvv string,
	expiryMonth, expiryYear int,
) (*domain.Payment, error) {

}

func (s *AuthorizationService) validate(
	orderID, customerID string,
	amount int64,
) error {
	if orderID == "" {
		return errors.New("order_id is required")
	}
	if customerID == "" {
		return errors.New("customer_id is required")
	}
	if amount <= 0 {
		return domain.NewInvalidAmountError(amount)
	}
	return nil
}
