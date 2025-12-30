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
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/mocks"
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
	suite.paymentRepo = postgres.NewPaymentRepository(suite.testDB.DB.Pool)
	suite.idempotencyRepo = postgres.NewIdempotencyRepository(suite.testDB.DB.Pool)
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
		suite.testDB.DB.Pool,
	)

	suite.voidService = services.NewVoidService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB.Pool,
	)
}

func (suite *voidServiceTestSuite) TearDownTest() {
	suite.testDB.CleanTables(suite.T())
}

// ============================================================================
// HAPPY PATH TESTS
// ============================================================================

func (suite *voidServiceTestSuite) Test_Void_Success() {
	ctx := context.Background()

	voidedPayment := testhelpers.CreateVoidedPayment(
		suite.T(),
		ctx,
		suite.authorizeService,
		suite.voidService,
		suite.mockBank,
	)
	require.NotNil(suite.T(), voidedPayment)

	assert.Equal(suite.T(), domain.StatusVoided, voidedPayment.Status())
	assert.Equal(suite.T(), "void-123", *voidedPayment.BankVoidID())
	assert.NotNil(suite.T(), voidedPayment.VoidedAt())

	// Verify database state
	savedPayment, err := suite.paymentRepo.FindByID(ctx, voidedPayment.ID())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), domain.StatusVoided, savedPayment.Status())
	assert.Equal(suite.T(), "void-123", *savedPayment.BankVoidID())
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func (suite *voidServiceTestSuite) Test_Void_CannotVoidPendingPayment() {
	ctx := context.Background()

	cmd := testhelpers.DefaultAuthorizeCommand()
	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &application.BankError{
		Code:       "internal_error",
		Message:    "Bank error",
		StatusCode: 500,
	}

	suite.mockBank.EXPECT().
		Authorize(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	payment, err := suite.authorizeService.Authorize(ctx, cmd, idempotencyKey)
	require.Error(suite.T(), err)
	require.NotNil(suite.T(), payment)
	require.Equal(suite.T(), domain.StatusPending, payment.Status())

	VoidCmd := services.VoidCommand{
		PaymentID: payment.ID(),
	}
	VoidKey := "idem-Void-" + uuid.New().String()

	_, err = suite.voidService.Void(ctx, VoidCmd, VoidKey)

	require.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, domain.ErrInvalidTransition)
}

func (suite *voidServiceTestSuite) Test_Void_CannotVoidAlreadyVoidedPayment() {
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(suite.T(), ctx, suite.authorizeService, suite.mockBank)

	cmd := services.VoidCommand{
		PaymentID: payment.ID(),
	}
	firstKey := "idem-first-" + uuid.New().String()

	VoidResp := &application.BankVoidResponse{
		AuthorizationID: *payment.BankAuthID(),
		VoidID:          "void-123",
		Status:          "voided",
		VoidedAt:        time.Now(),
	}

	suite.mockBank.EXPECT().
		Void(mock.Anything, mock.Anything, firstKey).
		Return(VoidResp, nil).
		Once()

	_, err := suite.voidService.Void(ctx, cmd, firstKey)
	require.NoError(suite.T(), err)

	secondKey := "idem-second-" + uuid.New().String()

	_, err = suite.voidService.Void(ctx, cmd, secondKey)

	require.Error(suite.T(), err)

	svcErr, ok := application.IsServiceError(err)
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), application.ErrCodeDuplicateBusinessRequest, svcErr.Code)
}

func (suite *voidServiceTestSuite) Test_Void_IdempotencyReturnsCache() {
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(suite.T(), ctx, suite.authorizeService, suite.mockBank)

	cmd := services.VoidCommand{
		PaymentID: payment.ID(),
	}
	idempotencyKey := "idem-same-key"

	VoidResp := &application.BankVoidResponse{
		AuthorizationID: *payment.BankAuthID(),
		VoidID:          "void-123",
		Status:          "voided",
		VoidedAt:        time.Now(),
	}

	suite.mockBank.EXPECT().
		Void(mock.Anything, mock.Anything, idempotencyKey).
		Return(VoidResp, nil).
		Once()

	firstResult, err := suite.voidService.Void(ctx, cmd, idempotencyKey)
	require.NoError(suite.T(), err)

	secondResult, err := suite.voidService.Void(ctx, cmd, idempotencyKey)
	require.NoError(suite.T(), err)

	assert.Equal(suite.T(), firstResult.ID(), secondResult.ID())
	assert.Equal(suite.T(), domain.StatusVoided, secondResult.Status())
}

func (suite *voidServiceTestSuite) Test_Void_PaymentNotFound() {
	ctx := context.Background()

	cmd := services.VoidCommand{
		PaymentID: "non-existent-id",
	}
	idempotencyKey := "idem-" + uuid.New().String()

	_, err := suite.voidService.Void(ctx, cmd, idempotencyKey)

	svcErr, ok := application.IsServiceError(err)
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), application.ErrCodeInternal, svcErr.Code)
}

// ============================================================================
// FAILURE RECOVERY TESTS
// ============================================================================

func (suite *voidServiceTestSuite) Test_Void_BankReturns500_PaymentStaysVoiding() {
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(suite.T(), ctx, suite.authorizeService, suite.mockBank)

	cmd := services.VoidCommand{
		PaymentID: payment.ID(),
	}
	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &application.BankError{
		Code:       "internal_error",
		Message:    "Bank internal error",
		StatusCode: 500,
	}

	suite.mockBank.EXPECT().
		Void(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	voidedPayment, err := suite.voidService.Void(ctx, cmd, idempotencyKey)

	require.Error(suite.T(), err)

	require.NotNil(suite.T(), voidedPayment)
	assert.Equal(suite.T(), domain.StatusVoiding, voidedPayment.Status())

	savedPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), domain.StatusVoiding, savedPayment.Status())
	assert.Nil(suite.T(), savedPayment.BankVoidID())
}

func (suite *voidServiceTestSuite) Test_Void_BankReturnsPermanentError_PaymentFails() {
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(suite.T(), ctx, suite.authorizeService, suite.mockBank)

	cmd := services.VoidCommand{
		PaymentID: payment.ID(),
	}
	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &application.BankError{
		Code:       "authorization_expired",
		Message:    "Authorization has expired",
		StatusCode: 400,
	}

	suite.mockBank.EXPECT().
		Void(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	voidedPayment, err := suite.voidService.Void(ctx, cmd, idempotencyKey)

	require.Error(suite.T(), err)

	require.NotNil(suite.T(), voidedPayment)
	assert.Equal(suite.T(), domain.StatusFailed, voidedPayment.Status())
}

func (suite *voidServiceTestSuite) Test_Void_ConcurrentRequests_OnlyOneSucceeds() {
	ctx := context.Background()

	payment := testhelpers.CreateAuthorizedPayment(suite.T(), ctx, suite.authorizeService, suite.mockBank)

	cmd := services.VoidCommand{
		PaymentID: payment.ID(),
	}
	idempotencyKey := "idem-same-key"

	VoidResp := &application.BankVoidResponse{
		AuthorizationID: *payment.BankAuthID(),
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
			_, err := suite.voidService.Void(ctx, cmd, idempotencyKey)
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

	finalPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), domain.StatusVoided, finalPayment.Status())
}
