package worker_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services/testhelpers"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/mocks"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/worker"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRetryWorker_RecoversStuckCapture(t *testing.T) {
	ctx := context.Background()

	testDB := testhelpers.SetupTestDatabase(t)
	defer testDB.Cleanup(t)

	paymentRepo := postgres.NewPaymentRepository(testDB.DB)
	idempotencyRepo := postgres.NewIdempotencyRepository(testDB.DB)
	mockBank := mocks.NewMockBankClient(t)

	authService := services.NewAuthorizeService(
		paymentRepo,
		idempotencyRepo,
		mockBank,
		testDB.DB,
	)

	idempotencyKey := "idem-test-capture-" + uuid.New().String()

	authCmd := testhelpers.DefaultAuthorizeCommand()

	mockBank.EXPECT().Authorize(
		mock.Anything,
		mock.Anything,
		idempotencyKey,
	).Return(&application.BankAuthorizationResponse{
		Amount:          authCmd.Amount,
		Currency:        authCmd.Currency,
		Status:          "authorized",
		AuthorizationID: "auth-123",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}, nil).Once()

	payment, err := authService.Authorize(ctx, authCmd, idempotencyKey)
	require.NoError(t, err)

	err = payment.MarkCapturing()
	require.NoError(t, err)

	err = paymentRepo.Update(ctx, nil, payment)
	require.NoError(t, err)

	_, err = testDB.DB.Exec(ctx,
		"UPDATE idempotency_keys SET locked_at = $1 WHERE key = $2",
		time.Now().Add(-2*time.Hour),
		idempotencyKey,
	)
	require.NoError(t, err)

	mockBank.EXPECT().Capture(
		mock.Anything,
		mock.Anything,
		idempotencyKey,
	).Return(&application.BankCaptureResponse{
		Amount:          payment.AmountCents,
		Currency:        payment.Currency,
		AuthorizationID: *payment.BankAuthID,
		CaptureID:       "cap-worker-test",
		Status:          "captured",
		CapturedAt:      time.Now(),
	}, nil).Once()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	worker := worker.NewRetryWorker(
		paymentRepo,
		idempotencyRepo,
		mockBank,
		testDB.DB,
		1*time.Minute,
		10,
		logger,
	)

	err = worker.ProcessRetries(ctx)
	require.NoError(t, err)

	updatedPayment, err := paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCaptured, updatedPayment.Status)
	assert.Equal(t, "cap-worker-test", *updatedPayment.BankCaptureID)

	key, err := idempotencyRepo.FindByKey(ctx, idempotencyKey)
	require.NoError(t, err)
	assert.Nil(t, key.LockedAt, "Lock should be released after success")
}

func TestRetryWorker_SchedulesRetryOnTransientError(t *testing.T) {
	ctx := context.Background()

	testDB := testhelpers.SetupTestDatabase(t)
	defer testDB.Cleanup(t)

	paymentRepo := postgres.NewPaymentRepository(testDB.DB)
	idempotencyRepo := postgres.NewIdempotencyRepository(testDB.DB)
	mockBank := mocks.NewMockBankClient(t)

	authService := services.NewAuthorizeService(
		paymentRepo,
		idempotencyRepo,
		mockBank,
		testDB.DB,
	)

	idempotencyKey := "idem-test-capture-" + uuid.New().String()

	authCmd := testhelpers.DefaultAuthorizeCommand()

	mockBank.EXPECT().Authorize(
		mock.Anything,
		mock.Anything,
		idempotencyKey,
	).Return(&application.BankAuthorizationResponse{
		Amount:          authCmd.Amount,
		Currency:        authCmd.Currency,
		Status:          "authorized",
		AuthorizationID: "auth-123",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}, nil).Once()

	payment, err := authService.Authorize(ctx, authCmd, idempotencyKey)
	require.NoError(t, err)

	err = payment.MarkCapturing()
	require.NoError(t, err)

	err = paymentRepo.Update(ctx, nil, payment)
	require.NoError(t, err)

	_, err = testDB.DB.Exec(ctx,
		"UPDATE idempotency_keys SET locked_at = $1 WHERE key = $2",
		time.Now().Add(-2*time.Hour),
		idempotencyKey,
	)
	require.NoError(t, err)

	mockBank.EXPECT().Capture(
		mock.Anything,
		mock.Anything,
		idempotencyKey,
	).Return(nil, &application.BankError{
		Code:       "internal_error",
		Message:    "Bank internal error",
		StatusCode: 500}).Once()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	worker := worker.NewRetryWorker(
		paymentRepo,
		idempotencyRepo,
		mockBank,
		testDB.DB,
		1*time.Minute,
		10,
		logger,
	)

	err = worker.ProcessRetries(ctx)
	require.NoError(t, err)

	updatedPayment, err := paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(t, err)

	assert.Equal(t, domain.StatusCapturing, updatedPayment.Status)
	require.NotNil(t, updatedPayment.NextRetryAt)
	assert.True(t, updatedPayment.NextRetryAt.After(time.Now()))
	assert.Equal(t, 1, updatedPayment.AttemptCount)
}

func TestRetryWorker_FailsOnPermanentError(t *testing.T) {
	ctx := context.Background()

	testDB := testhelpers.SetupTestDatabase(t)
	defer testDB.Cleanup(t)

	paymentRepo := postgres.NewPaymentRepository(testDB.DB)
	idempotencyRepo := postgres.NewIdempotencyRepository(testDB.DB)
	mockBank := mocks.NewMockBankClient(t)

	authService := services.NewAuthorizeService(
		paymentRepo,
		idempotencyRepo,
		mockBank,
		testDB.DB,
	)

	idempotencyKey := "idem-test-capture-" + uuid.New().String()

	authCmd := testhelpers.DefaultAuthorizeCommand()

	mockBank.EXPECT().Authorize(
		mock.Anything,
		mock.Anything,
		idempotencyKey,
	).Return(&application.BankAuthorizationResponse{
		Amount:          authCmd.Amount,
		Currency:        authCmd.Currency,
		Status:          "authorized",
		AuthorizationID: "auth-123",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}, nil).Once()

	payment, err := authService.Authorize(ctx, authCmd, idempotencyKey)
	require.NoError(t, err)

	err = payment.MarkCapturing()
	require.NoError(t, err)

	err = paymentRepo.Update(ctx, nil, payment)
	require.NoError(t, err)

	_, err = testDB.DB.Exec(ctx,
		"UPDATE idempotency_keys SET locked_at = $1 WHERE key = $2",
		time.Now().Add(-2*time.Hour),
		idempotencyKey,
	)
	require.NoError(t, err)

	mockBank.EXPECT().Capture(
		mock.Anything,
		mock.Anything,
		idempotencyKey,
	).Return(nil, &application.BankError{
		Code:       "authorization_expired",
		Message:    "Authorization has expired",
		StatusCode: 400}).Once()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	worker := worker.NewRetryWorker(
		paymentRepo,
		idempotencyRepo,
		mockBank,
		testDB.DB,
		1*time.Minute,
		10,
		logger,
	)

	err = worker.ProcessRetries(ctx)
	require.NoError(t, err)

	updatedPayment, err := paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(t, err)

	assert.Nil(t, updatedPayment.NextRetryAt)
}
