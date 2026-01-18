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

type voidServiceTestSuite struct {
	suite.Suite
	testDB           *testhelpers.TestDatabase
	paymentRepo      *postgres.PaymentRepository
	idempotencyRepo  *postgres.IdempotencyRepository
	mockBank         *mocks.MockBankClient
	authorizeService *services.AuthorizeService
	voidService      *services.VoidService
}

func TestVoidServiceSuite(t *testing.T) {
	suite.Run(t, new(voidServiceTestSuite))
}

func (suite *voidServiceTestSuite) SetupSuite() {
	suite.testDB = testhelpers.SetupTestDatabase(suite.T())
	suite.paymentRepo = postgres.NewPaymentRepository(suite.testDB.DB)
	suite.idempotencyRepo = postgres.NewIdempotencyRepository(suite.testDB.DB)
}

func (suite *voidServiceTestSuite) TearDownSuite() {
	suite.testDB.Cleanup(suite.T())
}

func (suite *voidServiceTestSuite) SetupTest() {
	suite.mockBank = mocks.NewMockBankClient(suite.T())

	suite.authorizeService = services.NewAuthorizeService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB,
	)

	suite.voidService = services.NewVoidService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB,
	)
}

func (suite *voidServiceTestSuite) TearDownTest() {
	suite.testDB.CleanTables(suite.T())
}

// ============================================================================
// HAPPY PATH TESTS
// ============================================================================

func (suite *voidServiceTestSuite) Test_Void_Success() {
	t := suite.T()
	ctx := context.Background()

	voidedPayment := testhelpers.CreateVoidedPayment(
		t,
		ctx,
		suite.authorizeService,
		suite.voidService,
		suite.mockBank,
	)
	require.NotNil(t, voidedPayment)

	assert.Equal(t, domain.StatusVoided, voidedPayment.Status)
	assert.Equal(t, "void-123", *voidedPayment.BankVoidID)
	assert.NotNil(t, voidedPayment.VoidedAt)

	// Verify database state
	savedPayment, err := suite.paymentRepo.FindByID(ctx, voidedPayment.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusVoided, savedPayment.Status)
	assert.Equal(t, "void-123", *savedPayment.BankVoidID)
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func (suite *voidServiceTestSuite) Test_Void_CannotVoidPendingPayment() {
	t := suite.T()
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
	require.Error(t, err)
	require.NotNil(t, payment)
	require.Equal(t, domain.StatusPending, payment.Status)

	VoidKey := "idem-Void-" + uuid.New().String()

	_, err = suite.voidService.Void(ctx, payment.ID, VoidKey)

	svcErr, ok := application.IsServiceError(err)
	require.True(t, ok)
	assert.Equal(t, application.ErrCodeInvalidState, svcErr.Code)
}

func (suite *voidServiceTestSuite) Test_Void_CannotVoidAlreadyVoidedPayment() {
	t := suite.T()
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(t, ctx, suite.authorizeService, suite.mockBank)

	firstKey := "idem-first-" + uuid.New().String()

	VoidResp := &bank.VoidResponse{
		AuthorizationID: *payment.BankAuthID,
		VoidID:          "void-123",
		Status:          "voided",
		VoidedAt:        time.Now(),
	}

	suite.mockBank.EXPECT().
		Void(mock.Anything, mock.Anything, firstKey).
		Return(VoidResp, nil).
		Once()

	_, err := suite.voidService.Void(ctx, payment.ID, firstKey)
	require.NoError(t, err)

	secondKey := "idem-second-" + uuid.New().String()

	_, err = suite.voidService.Void(ctx, payment.ID, secondKey)

	require.Error(t, err)

	svcErr, ok := application.IsServiceError(err)
	require.True(t, ok)
	assert.Equal(t, application.ErrCodeInvalidState, svcErr.Code)
}

func (suite *voidServiceTestSuite) Test_Void_IdempotencyReturnsCache() {
	t := suite.T()
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(t, ctx, suite.authorizeService, suite.mockBank)

	idempotencyKey := "idem-same-key"

	VoidResp := &bank.VoidResponse{
		AuthorizationID: *payment.BankAuthID,
		VoidID:          "void-123",
		Status:          "voided",
		VoidedAt:        time.Now(),
	}

	suite.mockBank.EXPECT().
		Void(mock.Anything, mock.Anything, idempotencyKey).
		Return(VoidResp, nil).
		Once()

	firstResult, err := suite.voidService.Void(ctx, payment.ID, idempotencyKey)
	require.NoError(t, err)

	secondResult, err := suite.voidService.Void(ctx, payment.ID, idempotencyKey)
	require.NoError(t, err)

	assert.Equal(t, firstResult.ID, secondResult.ID)
	assert.Equal(t, domain.StatusVoided, secondResult.Status)
}

func (suite *voidServiceTestSuite) Test_Void_PaymentNotFound() {
	t := suite.T()
	ctx := context.Background()

	paymentID := "non-existent-id"

	idempotencyKey := "idem-" + uuid.New().String()

	_, err := suite.voidService.Void(ctx, paymentID, idempotencyKey)

	svcErr, ok := application.IsServiceError(err)
	require.True(t, ok)
	assert.Equal(t, application.ErrCodeInternal, svcErr.Code)
}

// ============================================================================
// FAILURE RECOVERY TESTS
// ============================================================================

func (suite *voidServiceTestSuite) Test_Void_BankReturns500_PaymentStaysVoiding() {
	t := suite.T()
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(t, ctx, suite.authorizeService, suite.mockBank)

	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &bank.BankError{
		Code:       "internal_error",
		Message:    "Bank internal error",
		StatusCode: 500,
	}

	suite.mockBank.EXPECT().
		Void(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	voidedPayment, err := suite.voidService.Void(ctx, payment.ID, idempotencyKey)

	require.Error(t, err)

	require.NotNil(t, voidedPayment)
	assert.Equal(t, domain.StatusVoiding, voidedPayment.Status)

	savedPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusVoiding, savedPayment.Status)
	assert.Nil(t, savedPayment.BankVoidID)
}

func (suite *voidServiceTestSuite) Test_Void_BankReturnsPermanentError_PaymentFails() {
	t := suite.T()
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(t, ctx, suite.authorizeService, suite.mockBank)

	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &bank.BankError{
		Code:       "authorization_expired",
		Message:    "Authorization has expired",
		StatusCode: 400,
	}

	suite.mockBank.EXPECT().
		Void(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	voidedPayment, err := suite.voidService.Void(ctx, payment.ID, idempotencyKey)

	require.Error(t, err)

	require.NotNil(t, voidedPayment)
	assert.Equal(t, domain.StatusFailed, voidedPayment.Status)
}

func (suite *voidServiceTestSuite) Test_Void_ConcurrentRequests_OnlyOneSucceeds() {
	t := suite.T()
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(t, ctx, suite.authorizeService, suite.mockBank)

	idempotencyKey := "idem-same-key"

	VoidResp := &bank.VoidResponse{
		AuthorizationID: *payment.BankAuthID,
		VoidID:          "void-123",
		Status:          "voided",
		VoidedAt:        time.Now(),
	}

	suite.mockBank.EXPECT().
		Void(mock.Anything, mock.Anything, idempotencyKey).
		Return(VoidResp, nil).
		Once()

	var wg sync.WaitGroup
	results := make(chan error, 2)

	for range 2 {
		wg.Go(func() {
			_, err := suite.voidService.Void(ctx, payment.ID, idempotencyKey)
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
	assert.Equal(t, domain.StatusVoided, finalPayment.Status)
}
