package testhelpers

import (
	"context"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank/mocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// CreateAuthorizedPayment uses AuthorizeService to create a real authorized payment
func CreateAuthorizedPayment(
	t *testing.T,
	ctx context.Context,
	authService *services.AuthorizeService,
	mockBank *mocks.MockBankClient,
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

	idempotencyKey := "idem-auth-" + uuid.New().String()

	authResp := &bank.AuthorizationResponse{
		Amount:          100,
		Currency:        "USD",
		Status:          "authorized",
		AuthorizationID: "auth-" + uuid.New().String(),
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}

	mockBank.EXPECT().
		Authorize(mock.Anything, mock.Anything, idempotencyKey).
		Return(authResp, nil).
		Once()

	payment, err := authService.Authorize(ctx, &cmd, idempotencyKey)
	require.NoError(t, err)
	require.NotNil(t, payment)
	require.Equal(t, domain.StatusAuthorized, payment.Status)

	return payment
}

func CreateCapturedPayment(
	t *testing.T,
	ctx context.Context,
	authService *services.AuthorizeService,
	captureService *services.CaptureService,
	mockBank *mocks.MockBankClient,
) *domain.Payment {
	payment := CreateAuthorizedPayment(t, ctx, authService, mockBank)

	idempotencyKey := "idem-capt" + uuid.New().String()

	captureResp := &bank.CaptureResponse{
		Amount:          payment.AmountCents,
		Currency:        payment.Currency,
		AuthorizationID: *payment.BankAuthID,
		Status:          "captured",
		CaptureID:       "cap-123",
		CapturedAt:      time.Now(),
	}

	mockBank.EXPECT().
		Capture(mock.Anything, mock.Anything, idempotencyKey).
		Return(captureResp, nil).
		Once()

	capturedPayment, err := captureService.Capture(ctx, payment.ID, idempotencyKey)
	require.NoError(t, err)

	return capturedPayment
}

func CreateVoidedPayment(
	t *testing.T,
	ctx context.Context,
	authService *services.AuthorizeService,
	voidService *services.VoidService,
	mockBank *mocks.MockBankClient,
) *domain.Payment {
	payment := CreateAuthorizedPayment(t, ctx, authService, mockBank)

	idempotencyKey := "idem-void" + uuid.New().String()

	voidResp := &bank.VoidResponse{
		AuthorizationID: *payment.BankAuthID,
		Status:          "voided",
		VoidID:          "void-123",
		VoidedAt:        time.Now(),
	}

	mockBank.EXPECT().
		Void(mock.Anything, mock.Anything, idempotencyKey).
		Return(voidResp, nil).
		Once()

	voidPayment, err := voidService.Void(ctx, payment.ID, idempotencyKey)
	require.NoError(t, err)

	return voidPayment
}

func CreateRefundedPayment(
	t *testing.T,
	ctx context.Context,
	authService *services.AuthorizeService,
	captureService *services.CaptureService,
	refundService *services.RefundService,
	mockBank *mocks.MockBankClient,
) *domain.Payment {
	payment := CreateCapturedPayment(t, ctx, authService, captureService, mockBank)

	idempotencyKey := "idem-refund" + uuid.New().String()

	refundResp := &bank.RefundResponse{
		Amount:     payment.AmountCents,
		Currency:   payment.Currency,
		Status:     "refunded",
		CaptureID:  "cap-123",
		RefundID:   "ref-123",
		RefundedAt: time.Now(),
	}

	mockBank.EXPECT().
		Refund(mock.Anything, mock.Anything, idempotencyKey).
		Return(refundResp, nil).
		Once()

	refundedPayment, err := refundService.Refund(ctx, payment.ID, idempotencyKey)
	require.NoError(t, err)

	return refundedPayment
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
