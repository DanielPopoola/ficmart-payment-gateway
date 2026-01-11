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

	capturedPayment := testhelpers.CreateCapturedPayment(
		suite.T(),
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)
	require.NotNil(suite.T(), capturedPayment)

	assert.Equal(suite.T(), domain.StatusCaptured, capturedPayment.Status)
	assert.Equal(suite.T(), "cap-123", *capturedPayment.BankCaptureID)
	assert.NotNil(suite.T(), capturedPayment.CapturedAt)

	// Verify database state
	savedPayment, err := suite.paymentRepo.FindByID(ctx, capturedPayment.ID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), domain.StatusCaptured, savedPayment.Status)
	assert.Equal(suite.T(), "cap-123", *savedPayment.BankCaptureID)
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func (suite *CaptureServiceTestSuite) Test_Capture_CannotCapturePendingPayment() {
	ctx := context.Background()

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
	require.Error(suite.T(), err)
	require.NotNil(suite.T(), payment)
	require.Equal(suite.T(), domain.StatusPending, payment.Status)

	captureCmd := services.CaptureCommand{
		PaymentID: payment.ID,
		Amount:    payment.AmountCents,
	}
	captureKey := "idem-capture-" + uuid.New().String()

	_, err = suite.captureService.Capture(ctx, captureCmd, captureKey)

	svcErr, ok := application.IsServiceError(err)
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), application.ErrCodeInvalidState, svcErr.Code)
}

func (suite *CaptureServiceTestSuite) Test_Capture_CannotCaptureAlreadyCapturedPayment() {
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(suite.T(), ctx, suite.authorizeService, suite.mockBank)

	cmd := services.CaptureCommand{
		PaymentID: payment.ID,
		Amount:    payment.AmountCents,
	}
	firstKey := "idem-first-" + uuid.New().String()

	captureResp := &bank.CaptureResponse{
		Amount:          cmd.Amount,
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

	_, err := suite.captureService.Capture(ctx, cmd, firstKey)
	require.NoError(suite.T(), err)

	secondKey := "idem-second-" + uuid.New().String()

	_, err = suite.captureService.Capture(ctx, cmd, secondKey)

	require.Error(suite.T(), err)

	svcErr, ok := application.IsServiceError(err)
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), application.ErrCodeInvalidState, svcErr.Code)
}

func (suite *CaptureServiceTestSuite) Test_Capture_IdempotencyReturnsCache() {
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(suite.T(), ctx, suite.authorizeService, suite.mockBank)

	cmd := services.CaptureCommand{
		PaymentID: payment.ID,
		Amount:    payment.AmountCents,
	}
	idempotencyKey := "idem-same-key"

	captureResp := &bank.CaptureResponse{
		Amount:          cmd.Amount,
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

	firstResult, err := suite.captureService.Capture(ctx, cmd, idempotencyKey)
	require.NoError(suite.T(), err)

	secondResult, err := suite.captureService.Capture(ctx, cmd, idempotencyKey)
	require.NoError(suite.T(), err)

	assert.Equal(suite.T(), firstResult.ID, secondResult.ID)
	assert.Equal(suite.T(), domain.StatusCaptured, secondResult.Status)
}

func (suite *CaptureServiceTestSuite) Test_Capture_PaymentNotFound() {
	ctx := context.Background()

	cmd := services.CaptureCommand{
		PaymentID: "non-existent-id",
		Amount:    5000,
	}
	idempotencyKey := "idem-" + uuid.New().String()

	_, err := suite.captureService.Capture(ctx, cmd, idempotencyKey)

	svcErr, ok := application.IsServiceError(err)
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), application.ErrCodeInternal, svcErr.Code)
}

// ============================================================================
// FAILURE RECOVERY TESTS
// ============================================================================

func (suite *CaptureServiceTestSuite) Test_Capture_BankReturns500_PaymentStaysCapturing() {
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(suite.T(), ctx, suite.authorizeService, suite.mockBank)

	cmd := services.CaptureCommand{
		PaymentID: payment.ID,
		Amount:    payment.AmountCents,
	}
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

	capturedPayment, err := suite.captureService.Capture(ctx, cmd, idempotencyKey)

	require.Error(suite.T(), err)

	require.NotNil(suite.T(), capturedPayment)
	assert.Equal(suite.T(), domain.StatusCapturing, capturedPayment.Status)

	savedPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), domain.StatusCapturing, savedPayment.Status)
	assert.Nil(suite.T(), savedPayment.BankCaptureID)
}

func (suite *CaptureServiceTestSuite) Test_Capture_BankReturnsPermanentError_IsFailed() {
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(suite.T(), ctx, suite.authorizeService, suite.mockBank)

	cmd := services.CaptureCommand{
		PaymentID: payment.ID,
		Amount:    payment.AmountCents,
	}
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

	capturedPayment, err := suite.captureService.Capture(ctx, cmd, idempotencyKey)

	require.Error(suite.T(), err)

	require.NotNil(suite.T(), capturedPayment)
	assert.Equal(suite.T(), domain.StatusFailed, capturedPayment.Status)
}

func (suite *CaptureServiceTestSuite) Test_Capture_ConcurrentRequests_OnlyOneSucceeds() {
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(suite.T(), ctx, suite.authorizeService, suite.mockBank)

	cmd := services.CaptureCommand{
		PaymentID: payment.ID,
		Amount:    payment.AmountCents,
	}
	idempotencyKey := "idem-same-key"

	captureResp := &bank.CaptureResponse{
		Amount:          cmd.Amount,
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
			_, err := suite.captureService.Capture(ctx, cmd, idempotencyKey)
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

	assert.Equal(suite.T(), 2, successCount)

	finalPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), domain.StatusCaptured, finalPayment.Status)
}
