package bank_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/config"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRetryBankClient_Authorize_Success(t *testing.T) {
	mockClient := mocks.NewBankClient(t)
	retryClient := bank.NewRetryBankClient(mockClient, config.RetryConfig{
		BaseDelay:  1,
		MaxRetries: 3,
	})

	req := application.BankAuthorizationRequest{
		Amount:      5000,
		CardNumber:  "4111111111111111",
		Cvv:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}

	expectedResp := &application.BankAuthorizationResponse{
		Amount:          5000,
		Currency:        "USD",
		Status:          "AUTHORIZED",
		AuthorizationID: "auth-123",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}

	mockClient.EXPECT().
		Authorize(mock.Anything, req, "idem-key").
		Return(expectedResp, nil).
		Once()

	resp, err := retryClient.Authorize(context.Background(), req, "idem-key")

	require.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
}

func TestRetryBankClient_Authorize_RetriesOn5xx(t *testing.T) {
	mockClient := mocks.NewBankClient(t)
	retryClient := bank.NewRetryBankClient(mockClient, config.RetryConfig{
		BaseDelay:  1,
		MaxRetries: 3,
	})

	req := application.BankAuthorizationRequest{
		Amount:      5000,
		CardNumber:  "4111111111111111",
		Cvv:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}

	expectedResp := &application.BankAuthorizationResponse{
		AuthorizationID: "auth-123",
	}

	// First two calls fail with 500
	mockClient.EXPECT().
		Authorize(mock.Anything, req, "idem-key").
		Return(nil, &application.BankError{
			Code:       "internal_error",
			Message:    "Internal server error",
			StatusCode: 500,
		}).
		Twice()

	// Third call succeeds
	mockClient.EXPECT().
		Authorize(mock.Anything, req, "idem-key").
		Return(expectedResp, nil).
		Once()

	resp, err := retryClient.Authorize(context.Background(), req, "idem-key")

	require.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
}

func TestRetryBankClient_Authorize_DoesNotRetryOn4xx(t *testing.T) {
	mockClient := mocks.NewBankClient(t)
	retryClient := bank.NewRetryBankClient(mockClient, config.RetryConfig{
		BaseDelay:  1,
		MaxRetries: 3,
	})

	req := application.BankAuthorizationRequest{
		Amount:      5000,
		CardNumber:  "4111111111111111",
		Cvv:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}

	expectedErr := &application.BankError{
		Code:       "invalid_card",
		Message:    "Invalid card number",
		StatusCode: 400,
	}

	// Should only be called once (no retry on 4xx)
	mockClient.EXPECT().
		Authorize(mock.Anything, req, "idem-key").
		Return(nil, expectedErr).
		Once()

	resp, err := retryClient.Authorize(context.Background(), req, "idem-key")

	require.Error(t, err)
	assert.Nil(t, resp)

	var bankErr *application.BankError
	assert.True(t, errors.As(err, &bankErr))
	assert.Equal(t, expectedErr.Code, bankErr.Code)
}

func TestRetryBankClient_Authorize_ExhaustsRetries(t *testing.T) {
	mockClient := mocks.NewBankClient(t)
	retryClient := bank.NewRetryBankClient(mockClient, config.RetryConfig{
		BaseDelay:  1,
		MaxRetries: 3,
	})

	req := application.BankAuthorizationRequest{
		Amount:      5000,
		CardNumber:  "4111111111111111",
		Cvv:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}

	expectedErr := &application.BankError{
		Code:       "internal_error",
		Message:    "Internal server error",
		StatusCode: 500,
	}

	// All 3 attempts fail
	mockClient.EXPECT().
		Authorize(mock.Anything, req, "idem-key").
		Return(nil, expectedErr).
		Times(3)

	resp, err := retryClient.Authorize(context.Background(), req, "idem-key")

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "maximum retries exceeded")
}

func TestRetryBankClient_Capture_Success(t *testing.T) {
	mockClient := mocks.NewBankClient(t)
	retryClient := bank.NewRetryBankClient(mockClient, config.RetryConfig{
		BaseDelay:  1,
		MaxRetries: 3,
	})

	req := application.BankCaptureRequest{
		Amount:          5000,
		AuthorizationID: "auth-123",
	}

	expectedResp := &application.BankCaptureResponse{
		Amount:          5000,
		Currency:        "USD",
		AuthorizationID: "auth-123",
		CaptureID:       "cap-123",
		Status:          "CAPTURED",
		CapturedAt:      time.Now(),
	}

	mockClient.EXPECT().
		Capture(mock.Anything, req, "idem-key").
		Return(expectedResp, nil).
		Once()

	resp, err := retryClient.Capture(context.Background(), req, "idem-key")

	require.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
}

func TestRetryBankClient_Void_Success(t *testing.T) {
	mockClient := mocks.NewBankClient(t)
	retryClient := bank.NewRetryBankClient(mockClient, config.RetryConfig{
		BaseDelay:  1,
		MaxRetries: 3,
	})

	req := application.BankVoidRequest{
		AuthorizationID: "auth-123",
	}

	expectedResp := &application.BankVoidResponse{
		AuthorizationID: "auth-123",
		Status:          "VOIDED",
		VoidID:          "void-123",
		VoidedAt:        time.Now(),
	}

	mockClient.EXPECT().
		Void(mock.Anything, req, "idem-key").
		Return(expectedResp, nil).
		Once()

	resp, err := retryClient.Void(context.Background(), req, "idem-key")

	require.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
}

func TestRetryBankClient_Refund_Success(t *testing.T) {
	mockClient := mocks.NewBankClient(t)
	retryClient := bank.NewRetryBankClient(mockClient, config.RetryConfig{
		BaseDelay:  1,
		MaxRetries: 3,
	})

	req := application.BankRefundRequest{
		Amount:    5000,
		CaptureID: "cap-123",
	}

	expectedResp := &application.BankRefundResponse{
		Amount:     5000,
		Currency:   "USD",
		Status:     "REFUNDED",
		CaptureID:  "cap-123",
		RefundID:   "ref-123",
		RefundedAt: time.Now(),
	}

	mockClient.EXPECT().
		Refund(mock.Anything, req, "idem-key").
		Return(expectedResp, nil).
		Once()

	resp, err := retryClient.Refund(context.Background(), req, "idem-key")

	require.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
}

func TestRetryBankClient_RespectsContextCancellation(t *testing.T) {
	mockClient := mocks.NewBankClient(t)
	retryClient := bank.NewRetryBankClient(mockClient, config.RetryConfig{
		BaseDelay:  1,
		MaxRetries: 10, // High retry count
	})

	req := application.BankAuthorizationRequest{
		Amount:      5000,
		CardNumber:  "4111111111111111",
		Cvv:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}

	// First call fails
	mockClient.EXPECT().
		Authorize(mock.Anything, req, "idem-key").
		Return(nil, &application.BankError{
			Code:       "internal_error",
			StatusCode: 500,
		}).
		Once()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after first failure
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	resp, err := retryClient.Authorize(ctx, req, "idem-key")

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, context.Canceled, err)
}
