package services_test

import (
	"context"
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

type RefundServiceTestSuite struct {
	suite.Suite
	testDB           *testhelpers.TestDatabase
	paymentRepo      *postgres.PaymentRepository
	idempotencyRepo  *postgres.IdempotencyRepository
	mockBank         *mocks.MockBankClient
	authorizeService *services.AuthorizeService
	captureService   *services.CaptureService
	refundService    *services.RefundService
}

func TestRefundServiceSuite(t *testing.T) {
	suite.Run(t, new(RefundServiceTestSuite))
}

func (suite *RefundServiceTestSuite) SetupSuite() {
	suite.testDB = testhelpers.SetupTestDatabase(suite.T())
	suite.paymentRepo = postgres.NewPaymentRepository(suite.testDB.DB.Pool)
	suite.idempotencyRepo = postgres.NewIdempotencyRepository(suite.testDB.DB.Pool)
}

func (suite *RefundServiceTestSuite) TearDownSuite() {
	suite.testDB.Cleanup(suite.T())
}

func (suite *RefundServiceTestSuite) SetupTest() {
	suite.mockBank = mocks.NewMockBankClient(suite.T())

	suite.authorizeService = services.NewAuthorizeService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB.Pool,
	)

	suite.captureService = services.NewCaptureService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB.Pool,
	)

	suite.refundService = services.NewRefundService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB.Pool,
	)
}

func (suite *RefundServiceTestSuite) TearDownTest() {
	suite.testDB.CleanTables(suite.T())
}

// ============================================================================
// HAPPY PATH TESTS
// ============================================================================

func (suite *RefundServiceTestSuite) Test_Refund_Success() {
	ctx := context.Background()

	refundedPayment := testhelpers.CreateRefundedPayment(
		suite.T(),
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.refundService,
		suite.mockBank,
	)
	require.NotNil(suite.T(), refundedPayment)

	assert.Equal(suite.T(), domain.StatusRefunded, refundedPayment.Status())
	assert.Equal(suite.T(), "ref-123", *refundedPayment.BankRefundID())
	assert.NotNil(suite.T(), refundedPayment.RefundedAt())

	// Verify database state
	savedPayment, err := suite.paymentRepo.FindByID(ctx, refundedPayment.ID())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), domain.StatusRefunded, savedPayment.Status())
	assert.Equal(suite.T(), "ref-123", *savedPayment.BankRefundID())
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func (suite *RefundServiceTestSuite) Test_Refund_CannotRefundPendingPayment() {
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

	refundCmd := services.RefundCommand{
		PaymentID: payment.ID(),
		Amount:    payment.Amount().Amount,
	}
	refundKey := "idem-Refund-" + uuid.New().String()

	_, err = suite.refundService.Refund(ctx, refundCmd, refundKey)

	require.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, domain.ErrInvalidTransition)
}

func (suite *RefundServiceTestSuite) Test_Refund_CannotRefundAlreadyRefundedPayment() {
	ctx := context.Background()

	payment := testhelpers.CreateCapturedPayment(
		suite.T(),
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)

	cmd := services.RefundCommand{
		PaymentID: payment.ID(),
		Amount:    payment.Amount().Amount,
	}
	firstKey := "idem-first-" + uuid.New().String()

	refundResp := &application.BankRefundResponse{
		Amount:     payment.Amount().Amount,
		Currency:   payment.Amount().Currency,
		CaptureID:  *payment.BankCaptureID(),
		RefundID:   "refund-123",
		Status:     "refunded",
		RefundedAt: time.Now(),
	}

	suite.mockBank.EXPECT().
		Refund(mock.Anything, mock.Anything, firstKey).
		Return(refundResp, nil).
		Once()

	_, err := suite.refundService.Refund(ctx, cmd, firstKey)
	require.NoError(suite.T(), err)

	secondKey := "idem-second-" + uuid.New().String()

	_, err = suite.refundService.Refund(ctx, cmd, secondKey)

	require.Error(suite.T(), err)

	svcErr, ok := application.IsServiceError(err)
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), application.ErrCodeDuplicateBusinessRequest, svcErr.Code)
}

func (suite *RefundServiceTestSuite) Test_Refund_IdempotencyReturnsCache() {
	ctx := context.Background()

	payment := testhelpers.CreateCapturedPayment(
		suite.T(),
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)

	cmd := services.RefundCommand{
		PaymentID: payment.ID(),
		Amount:    payment.Amount().Amount,
	}
	idempotencyKey := "idem-same-key"

	refundResp := &application.BankRefundResponse{
		Amount:     payment.Amount().Amount,
		Currency:   payment.Amount().Currency,
		CaptureID:  *payment.BankCaptureID(),
		RefundID:   "refund-123",
		Status:     "refunded",
		RefundedAt: time.Now(),
	}

	suite.mockBank.EXPECT().
		Refund(mock.Anything, mock.Anything, idempotencyKey).
		Return(refundResp, nil).
		Once()

	firstResult, err := suite.refundService.Refund(ctx, cmd, idempotencyKey)
	require.NoError(suite.T(), err)

	secondResult, err := suite.refundService.Refund(ctx, cmd, idempotencyKey)
	require.NoError(suite.T(), err)

	assert.Equal(suite.T(), firstResult.ID(), secondResult.ID())
	assert.Equal(suite.T(), domain.StatusRefunded, secondResult.Status())
}

func (suite *RefundServiceTestSuite) Test_Refund_PaymentNotFound() {
	ctx := context.Background()

	cmd := services.RefundCommand{
		PaymentID: "non-existent-id",
		Amount:    500,
	}
	idempotencyKey := "idem-" + uuid.New().String()

	_, err := suite.refundService.Refund(ctx, cmd, idempotencyKey)

	svcErr, ok := application.IsServiceError(err)
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), application.ErrCodeInternal, svcErr.Code)
}

// ============================================================================
// FAILURE RECOVERY TESTS
// ============================================================================

func (suite *RefundServiceTestSuite) Test_Refund_BankReturns500_PaymentStaysRefunding() {
	ctx := context.Background()

	payment := testhelpers.CreateCapturedPayment(
		suite.T(),
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)

	cmd := services.RefundCommand{
		PaymentID: payment.ID(),
		Amount:    payment.Amount().Amount,
	}
	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &application.BankError{
		Code:       "internal_error",
		Message:    "Bank internal error",
		StatusCode: 500,
	}

	suite.mockBank.EXPECT().
		Refund(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	RefundedPayment, err := suite.refundService.Refund(ctx, cmd, idempotencyKey)

	require.Error(suite.T(), err)

	require.NotNil(suite.T(), RefundedPayment)
	assert.Equal(suite.T(), domain.StatusRefunding, RefundedPayment.Status())

	savedPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID())
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), domain.StatusRefunding, savedPayment.Status())
	assert.Nil(suite.T(), savedPayment.BankRefundID())
}

func (suite *RefundServiceTestSuite) Test_Refund_BankReturnsPermanentError_PaymentFails() {
	ctx := context.Background()

	payment := testhelpers.CreateCapturedPayment(
		suite.T(),
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)

	cmd := services.RefundCommand{
		PaymentID: payment.ID(),
		Amount:    payment.Amount().Amount,
	}
	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &application.BankError{
		Code:       "authorization_expired",
		Message:    "Authorization has expired",
		StatusCode: 400,
	}

	suite.mockBank.EXPECT().
		Refund(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	RefundedPayment, err := suite.refundService.Refund(ctx, cmd, idempotencyKey)

	require.Error(suite.T(), err)

	require.NotNil(suite.T(), RefundedPayment)
	assert.Equal(suite.T(), domain.StatusFailed, RefundedPayment.Status())
}

func (suite *RefundServiceTestSuite) Test_Refund_ConcurrentRequests_OnlyOneSucceeds() {
	ctx := context.Background()
	payment := testhelpers.CreateCapturedPayment(
		suite.T(),
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)

	cmd := services.RefundCommand{
		PaymentID: payment.ID(),
		Amount:    payment.Amount().Amount,
	}

	type result struct {
		payment *domain.Payment
		err     error
	}

	idempotencyKey := "idem-" + uuid.New().String()

	refundResp := &application.BankRefundResponse{
		Amount:     payment.Amount().Amount,
		Currency:   payment.Amount().Currency,
		CaptureID:  *payment.BankCaptureID(),
		RefundID:   "refund-123",
		Status:     "refunded",
		RefundedAt: time.Now(),
	}

	suite.mockBank.EXPECT().
		Refund(mock.Anything, mock.Anything, idempotencyKey).
		Return(refundResp, nil).
		Once()

	results := make(chan result, 2)

	for i := range 2 {
		go func(goroutineID int) {

			payment, err := suite.refundService.Refund(ctx, cmd, idempotencyKey)
			results <- result{payment, err}
		}(i)
	}

	var successCount int
	var paymentIDs []string

	for range 2 {
		res := <-results
		if res.err == nil {
			successCount++
			paymentIDs = append(paymentIDs, res.payment.ID())
		}
	}

	assert.Equal(suite.T(), 2, successCount)
	assert.Equal(suite.T(), paymentIDs[0], paymentIDs[1])
}
