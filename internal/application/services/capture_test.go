package services_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services/testhelpers"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/bank/mocks"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CaptureServiceTestSuite struct {
	suite.Suite
	testDB           *testhelpers.TestDatabase
	paymentRepo      *postgres.PaymentRepository
	idempotencyRepo  *postgres.IdempotencyRepository
	mockBank         *mocks.MockBankClient
	authorizeService *services.AuthorizeService
	captureService   *services.CaptureService
}

func TestCaptureServiceSuite(t *testing.T) {
	suite.Run(t, new(CaptureServiceTestSuite))
}

func (suite *CaptureServiceTestSuite) SetupSuite() {
	suite.testDB = testhelpers.SetupTestDatabase(suite.T())
	suite.paymentRepo = postgres.NewPaymentRepository(suite.testDB.DB)
	suite.idempotencyRepo = postgres.NewIdempotencyRepository(suite.testDB.DB)
}

func (suite *CaptureServiceTestSuite) TearDownSuite() {
	suite.testDB.Cleanup(suite.T())
}

func (suite *CaptureServiceTestSuite) SetupTest() {
	suite.mockBank = mocks.NewMockBankClient(suite.T())

	suite.authorizeService = services.NewAuthorizeService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB,
	)

	suite.captureService = services.NewCaptureService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB,
	)
}

func (suite *CaptureServiceTestSuite) TearDownTest() {
	suite.testDB.CleanTables(suite.T())
}

// ============================================================================
// HAPPY PATH TESTS
// ============================================================================

func (suite *CaptureServiceTestSuite) Test_Capture_Success() {
	ctx := context.Background()
	t := suite.T()

	capturedPayment := testhelpers.CreateCapturedPayment(
		t,
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)
	require.NotNil(t, capturedPayment)

	assert.Equal(t, domain.StatusCaptured, capturedPayment.Status)
	assert.Equal(t, "cap-123", *capturedPayment.BankCaptureID)
	assert.NotNil(t, capturedPayment.CapturedAt)

	// Verify database state
	savedPayment, err := suite.paymentRepo.FindByID(ctx, capturedPayment.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCaptured, savedPayment.Status)
	assert.Equal(t, "cap-123", *savedPayment.BankCaptureID)
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func (suite *CaptureServiceTestSuite) Test_Capture_CannotCapturePendingPayment() {
	ctx := context.Background()
	t := suite.T()

	cmd := testhelpers.DefaultAuthorizeCommand()
	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &bank.BankError{
		Code:       "internal_error",
		Message:    "Bank error",
		StatusCode: 500,
	}

	suite.mockBank.EXPECT().
		Authorize(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	payment, err := suite.authorizeService.Authorize(ctx, &cmd, idempotencyKey)
	require.Error(t, err)
	require.NotNil(t, payment)
	require.Equal(t, domain.StatusPending, payment.Status)

	captureKey := "idem-capture-" + uuid.New().String()

	_, err = suite.captureService.Capture(ctx, payment.ID, captureKey)

	svcErr, ok := application.IsServiceError(err)
	require.True(t, ok)
	assert.Equal(t, application.ErrCodeInvalidState, svcErr.Code)
}

func (suite *CaptureServiceTestSuite) Test_Capture_CannotCaptureAlreadyCapturedPayment() {
	ctx := context.Background()
	t := suite.T()

	payment := testhelpers.CreateAuthorizedPayment(t, ctx, suite.authorizeService, suite.mockBank)

	firstKey := "idem-first-" + uuid.New().String()

	captureResp := &bank.CaptureResponse{
		Amount:          payment.AmountCents,
		Currency:        "USD",
		AuthorizationID: *payment.BankAuthID,
		CaptureID:       "cap-123",
		Status:          "captured",
		CapturedAt:      time.Now(),
	}

	suite.mockBank.EXPECT().
		Capture(mock.Anything, mock.Anything, firstKey).
		Return(captureResp, nil).
		Once()

	_, err := suite.captureService.Capture(ctx, payment.ID, firstKey)
	require.NoError(t, err)

	secondKey := "idem-second-" + uuid.New().String()

	_, err = suite.captureService.Capture(ctx, payment.ID, secondKey)

	require.Error(t, err)

	svcErr, ok := application.IsServiceError(err)
	require.True(t, ok)
	assert.Equal(t, application.ErrCodeInvalidState, svcErr.Code)
}

func (suite *CaptureServiceTestSuite) Test_Capture_IdempotencyReturnsCache() {
	ctx := context.Background()
	t := suite.T()

	payment := testhelpers.CreateAuthorizedPayment(t, ctx, suite.authorizeService, suite.mockBank)

	idempotencyKey := "idem-same-key"

	captureResp := &bank.CaptureResponse{
		Amount:          payment.AmountCents,
		Currency:        "USD",
		AuthorizationID: *payment.BankAuthID,
		CaptureID:       "cap-123",
		Status:          "captured",
		CapturedAt:      time.Now(),
	}

	suite.mockBank.EXPECT().
		Capture(mock.Anything, mock.Anything, idempotencyKey).
		Return(captureResp, nil).
		Once()

	firstResult, err := suite.captureService.Capture(ctx, payment.ID, idempotencyKey)
	require.NoError(t, err)

	secondResult, err := suite.captureService.Capture(ctx, payment.ID, idempotencyKey)
	require.NoError(t, err)

	assert.Equal(t, firstResult.ID, secondResult.ID)
	assert.Equal(t, domain.StatusCaptured, secondResult.Status)
}

func (suite *CaptureServiceTestSuite) Test_Capture_PaymentNotFound() {
	ctx := context.Background()
	t := suite.T()

	paymentID := "non-existent-id"
	idempotencyKey := "idem-" + uuid.New().String()

	_, err := suite.captureService.Capture(ctx, paymentID, idempotencyKey)

	svcErr, ok := application.IsServiceError(err)
	require.True(t, ok)
	assert.Equal(t, application.ErrCodeInternal, svcErr.Code)
}

// ============================================================================
// FAILURE RECOVERY TESTS
// ============================================================================

func (suite *CaptureServiceTestSuite) Test_Capture_BankReturns500_PaymentStaysCapturing() {
	ctx := context.Background()
	t := suite.T()

	payment := testhelpers.CreateAuthorizedPayment(t, ctx, suite.authorizeService, suite.mockBank)

	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &bank.BankError{
		Code:       "internal_error",
		Message:    "Bank internal error",
		StatusCode: 500,
	}

	suite.mockBank.EXPECT().
		Capture(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	capturedPayment, err := suite.captureService.Capture(ctx, payment.ID, idempotencyKey)

	require.Error(t, err)

	require.NotNil(t, capturedPayment)
	assert.Equal(t, domain.StatusCapturing, capturedPayment.Status)

	savedPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCapturing, savedPayment.Status)
	assert.Nil(t, savedPayment.BankCaptureID)
}

func (suite *CaptureServiceTestSuite) Test_Capture_BankReturnsPermanentError_IsFailed() {
	ctx := context.Background()
	t := suite.T()

	payment := testhelpers.CreateAuthorizedPayment(t, ctx, suite.authorizeService, suite.mockBank)

	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &bank.BankError{
		Code:       "authorization_expired",
		Message:    "Authorization has expired",
		StatusCode: 400,
	}

	suite.mockBank.EXPECT().
		Capture(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	capturedPayment, err := suite.captureService.Capture(ctx, payment.ID, idempotencyKey)

	require.Error(t, err)

	require.NotNil(t, capturedPayment)
	assert.Equal(t, domain.StatusFailed, capturedPayment.Status)
}

func (suite *CaptureServiceTestSuite) Test_Capture_ConcurrentRequests_OnlyOneSucceeds() {
	ctx := context.Background()
	t := suite.T()

	payment := testhelpers.CreateAuthorizedPayment(t, ctx, suite.authorizeService, suite.mockBank)

	idempotencyKey := "idem-same-key"

	captureResp := &bank.CaptureResponse{
		Amount:          payment.AmountCents,
		Currency:        "USD",
		AuthorizationID: *payment.BankAuthID,
		CaptureID:       "cap-123",
		Status:          "captured",
		CapturedAt:      time.Now(),
	}

	suite.mockBank.EXPECT().
		Capture(mock.Anything, mock.Anything, idempotencyKey).
		Return(captureResp, nil).
		Once()

	var wg sync.WaitGroup
	results := make(chan error, 2)

	for range 2 {
		wg.Go(func() {
			_, err := suite.captureService.Capture(ctx, payment.ID, idempotencyKey)
			results <- err
		})
	}

	wg.Wait()
	close(results)

	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		}
	}

	assert.Equal(t, 2, successCount)

	finalPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCaptured, finalPayment.Status)
}
