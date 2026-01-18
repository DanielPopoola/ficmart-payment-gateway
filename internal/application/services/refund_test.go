package services_test

import (
	"context"
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
	suite.paymentRepo = postgres.NewPaymentRepository(suite.testDB.DB)
	suite.idempotencyRepo = postgres.NewIdempotencyRepository(suite.testDB.DB)
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
		suite.testDB.DB,
	)

	suite.captureService = services.NewCaptureService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB,
	)

	suite.refundService = services.NewRefundService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB,
	)
}

func (suite *RefundServiceTestSuite) TearDownTest() {
	suite.testDB.CleanTables(suite.T())
}

// ============================================================================
// HAPPY PATH TESTS
// ============================================================================

func (suite *RefundServiceTestSuite) Test_Refund_Success() {
	t := suite.T()
	ctx := context.Background()

	refundedPayment := testhelpers.CreateRefundedPayment(
		t,
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.refundService,
		suite.mockBank,
	)
	require.NotNil(t, refundedPayment)

	assert.Equal(t, domain.StatusRefunded, refundedPayment.Status)
	assert.Equal(t, "ref-123", *refundedPayment.BankRefundID)
	assert.NotNil(t, refundedPayment.RefundedAt)

	// Verify database state
	savedPayment, err := suite.paymentRepo.FindByID(ctx, refundedPayment.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusRefunded, savedPayment.Status)
	assert.Equal(t, "ref-123", *savedPayment.BankRefundID)
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func (suite *RefundServiceTestSuite) Test_Refund_CannotRefundPendingPayment() {
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

	refundKey := "idem-Refund-" + uuid.New().String()

	_, err = suite.refundService.Refund(ctx, payment.ID, refundKey)

	svcErr, ok := application.IsServiceError(err)
	require.True(t, ok)
	assert.Equal(t, application.ErrCodeInvalidState, svcErr.Code)
}

func (suite *RefundServiceTestSuite) Test_Refund_CannotRefundAlreadyRefundedPayment() {
	t := suite.T()
	ctx := context.Background()

	payment := testhelpers.CreateCapturedPayment(
		t,
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)

	firstKey := "idem-first-" + uuid.New().String()

	refundResp := &bank.RefundResponse{
		Amount:     payment.AmountCents,
		Currency:   payment.Currency,
		CaptureID:  *payment.BankCaptureID,
		RefundID:   "refund-123",
		Status:     "refunded",
		RefundedAt: time.Now(),
	}

	suite.mockBank.EXPECT().
		Refund(mock.Anything, mock.Anything, firstKey).
		Return(refundResp, nil).
		Once()

	_, err := suite.refundService.Refund(ctx, payment.ID, firstKey)
	require.NoError(t, err)

	secondKey := "idem-second-" + uuid.New().String()

	_, err = suite.refundService.Refund(ctx, payment.ID, secondKey)

	require.Error(t, err)

	svcErr, ok := application.IsServiceError(err)
	require.True(t, ok)
	assert.Equal(t, application.ErrCodeInvalidState, svcErr.Code)
}

func (suite *RefundServiceTestSuite) Test_Refund_IdempotencyReturnsCache() {
	t := suite.T()
	ctx := context.Background()

	payment := testhelpers.CreateCapturedPayment(
		t,
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)

	idempotencyKey := "idem-same-key"

	refundResp := &bank.RefundResponse{
		Amount:     payment.AmountCents,
		Currency:   payment.Currency,
		CaptureID:  *payment.BankCaptureID,
		RefundID:   "refund-123",
		Status:     "refunded",
		RefundedAt: time.Now(),
	}

	suite.mockBank.EXPECT().
		Refund(mock.Anything, mock.Anything, idempotencyKey).
		Return(refundResp, nil).
		Once()

	firstResult, err := suite.refundService.Refund(ctx, payment.ID, idempotencyKey)
	require.NoError(t, err)

	secondResult, err := suite.refundService.Refund(ctx, payment.ID, idempotencyKey)
	require.NoError(t, err)

	assert.Equal(t, firstResult.ID, secondResult.ID)
	assert.Equal(t, domain.StatusRefunded, secondResult.Status)
}

func (suite *RefundServiceTestSuite) Test_Refund_PaymentNotFound() {
	t := suite.T()
	ctx := context.Background()

	paymentID := "non-existent-id"

	idempotencyKey := "idem-" + uuid.New().String()

	_, err := suite.refundService.Refund(ctx, paymentID, idempotencyKey)

	svcErr, ok := application.IsServiceError(err)
	require.True(t, ok)
	assert.Equal(t, application.ErrCodeInternal, svcErr.Code)
}

// ============================================================================
// FAILURE RECOVERY TESTS
// ============================================================================

func (suite *RefundServiceTestSuite) Test_Refund_BankReturns500_PaymentStaysRefunding() {
	t := suite.T()
	ctx := context.Background()

	payment := testhelpers.CreateCapturedPayment(
		t,
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)

	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &bank.BankError{
		Code:       "internal_error",
		Message:    "Bank internal error",
		StatusCode: 500,
	}

	suite.mockBank.EXPECT().
		Refund(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	RefundedPayment, err := suite.refundService.Refund(ctx, payment.ID, idempotencyKey)

	require.Error(t, err)

	require.NotNil(t, RefundedPayment)
	assert.Equal(t, domain.StatusRefunding, RefundedPayment.Status)

	savedPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusRefunding, savedPayment.Status)
	assert.Nil(t, savedPayment.BankRefundID)
}

func (suite *RefundServiceTestSuite) Test_Refund_BankReturnsPermanentError_PaymentFails() {
	t := suite.T()
	ctx := context.Background()

	payment := testhelpers.CreateCapturedPayment(
		t,
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)

	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &bank.BankError{
		Code:       "authorization_expired",
		Message:    "Authorization has expired",
		StatusCode: 400,
	}

	suite.mockBank.EXPECT().
		Refund(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	RefundedPayment, err := suite.refundService.Refund(ctx, payment.ID, idempotencyKey)

	require.Error(t, err)

	require.NotNil(t, RefundedPayment)
	assert.Equal(t, domain.StatusFailed, RefundedPayment.Status)
}

func (suite *RefundServiceTestSuite) Test_Refund_ConcurrentRequests_OnlyOneSucceeds() {
	t := suite.T()
	ctx := context.Background()
	payment := testhelpers.CreateCapturedPayment(
		t,
		ctx,
		suite.authorizeService,
		suite.captureService,
		suite.mockBank,
	)

	type result struct {
		payment *domain.Payment
		err     error
	}

	idempotencyKey := "idem-" + uuid.New().String()

	refundResp := &bank.RefundResponse{
		Amount:     payment.AmountCents,
		Currency:   payment.Currency,
		CaptureID:  *payment.BankCaptureID,
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

			payment, err := suite.refundService.Refund(ctx, payment.ID, idempotencyKey)
			results <- result{payment, err}
		}(i)
	}

	var successCount int
	var paymentIDs []string

	for range 2 {
		res := <-results
		if res.err == nil {
			successCount++
			paymentIDs = append(paymentIDs, res.payment.ID)
		}
	}

	assert.Equal(t, 2, successCount)
	assert.Equal(t, paymentIDs[0], paymentIDs[1])
}
