package testhelpers

import (
	"context"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// CreateAuthorizedPayment uses AuthorizeService to create a real authorized payment
func CreateAuthorizedPayment(
	t *testing.T,
	ctx context.Context,
	authService *services.AuthorizeService,
	mockBank *application.BankClient,
) *domain.Payment {

	cmd := services.AuthorizeCommand{
		OrderID:     "order-" + uuid.New().String(),
		CustomerID:  "cust-" + uuid.New().String(),
		Amount:      5000,
		Currency:    "USD",
		CardNumber:  "4111111111111111",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}

	idempotencyKey := "idem-" + uuid.New().String()

	_ = &application.BankAuthorizationResponse{
		Amount:          100,
		Currency:        "USD",
		Status:          "AUTHORIZED",
		AuthorizationID: "auth-" + uuid.New().String(),
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}

	payment, err := authService.Authorize(ctx, cmd, idempotencyKey)
	require.NoError(t, err)
	require.NotNil(t, payment)
	require.Equal(t, domain.StatusAuthorized, payment.Status())

	return payment
}

// DefaultAuthorizeCommand returns a valid authorize command for testing
func DefaultAuthorizeCommand() services.AuthorizeCommand {
	return services.AuthorizeCommand{
		OrderID:     "order-" + uuid.New().String(),
		CustomerID:  "cust-" + uuid.New().String(),
		Amount:      100,
		Currency:    "USD",
		CardNumber:  "4111111111111111",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}
}

// DefaultCaptureCommand returns a valid capture command for testing
func DefaultCaptureCommand(paymentID string) services.CaptureCommand {
	return services.CaptureCommand{
		PaymentID: paymentID,
		Amount:    100,
	}
}
