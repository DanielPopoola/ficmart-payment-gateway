package services_test

import (
	"context"
	"errors"
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

type AuthorizeServiceTestSuite struct {
	suite.Suite
	testDB          *testhelpers.TestDatabase
	paymentRepo     *postgres.PaymentRepository
	idempotencyRepo *postgres.IdempotencyRepository
	mockBank        *mocks.MockBankClient
	service         *services.AuthorizeService
}

func TestAuthorizeServiceSuite(t *testing.T) {
	suite.Run(t, new(AuthorizeServiceTestSuite))
}

// SetupSuite runs once before all tests
func (suite *AuthorizeServiceTestSuite) SetupSuite() {
	suite.testDB = testhelpers.SetupTestDatabase(suite.T())
	suite.paymentRepo = postgres.NewPaymentRepository(suite.testDB.DB)
	suite.idempotencyRepo = postgres.NewIdempotencyRepository(suite.testDB.DB)
}

// TearDownSuite runs once after all tests
func (suite *AuthorizeServiceTestSuite) TearDownSuite() {
	suite.testDB.Cleanup(suite.T())
}

// SetupTest runs before each test
func (suite *AuthorizeServiceTestSuite) SetupTest() {
	suite.testDB.CleanTables(suite.T())
	suite.mockBank = mocks.NewMockBankClient(suite.T())
	suite.service = services.NewAuthorizeService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB,
	)
}

// TearDownTest runs after each test
func (suite *AuthorizeServiceTestSuite) TearDownTest() {
	suite.testDB.CleanTables(suite.T())
}

// ============================================================================
// HAPPY PATH TESTS
// ============================================================================

func (suite *AuthorizeServiceTestSuite) Test_Authorize_Success() {
	ctx := context.Background()
	t := suite.T()
	cmd := testhelpers.DefaultAuthorizeCommand()
	idempotencyKey := "idem-" + uuid.New().String()

	// Mock bank success response
	expectedBankResp := &bank.AuthorizationResponse{
		Amount:          cmd.Amount,
		Currency:        cmd.Currency,
		Status:          "AUTHORIZED",
		AuthorizationID: "auth-123",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}

	suite.mockBank.EXPECT().
		Authorize(mock.Anything, mock.Anything, idempotencyKey).
		Return(expectedBankResp, nil).
		Once()

	payment, err := suite.service.Authorize(ctx, &cmd, idempotencyKey)

	require.NoError(t, err)
	require.NotNil(t, payment)

	assert.Equal(t, domain.StatusAuthorized, payment.Status)
	assert.Equal(t, cmd.OrderID, payment.OrderID)
	assert.Equal(t, cmd.CustomerID, payment.CustomerID)
	assert.Equal(t, cmd.Amount, payment.AmountCents)
	assert.Equal(t, "auth-123", *payment.BankAuthID)
	assert.NotNil(t, payment.AuthorizedAt)
	assert.NotNil(t, payment.ExpiresAt)

	savedPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAuthorized, savedPayment.Status)
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func (suite *AuthorizeServiceTestSuite) Test_Authorize_DuplicateIdempotencyKey_ReturnsCachedResponse() {
	ctx := context.Background()
	t := suite.T()
	cmd := testhelpers.DefaultAuthorizeCommand()
	idempotencyKey := "idem-" + uuid.New().String()

	// Mock bank success response
	bankResp := &bank.AuthorizationResponse{
		Amount:          cmd.Amount,
		Currency:        cmd.Currency,
		Status:          "AUTHORIZED",
		AuthorizationID: "auth-123",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}

	suite.mockBank.EXPECT().
		Authorize(mock.Anything, mock.Anything, idempotencyKey).
		Return(bankResp, nil).
		Once()

	firstPayment, err := suite.service.Authorize(ctx, &cmd, idempotencyKey)
	require.NoError(t, err)

	secondPayment, err := suite.service.Authorize(ctx, &cmd, idempotencyKey)
	require.NoError(t, err)

	assert.Equal(t, firstPayment.ID, secondPayment.ID)
	assert.Equal(t, domain.StatusAuthorized, secondPayment.Status)
}

func (suite *AuthorizeServiceTestSuite) Test_Authorize_DifferentRequestSameKey_ReturnsError() {
	t := suite.T()
	ctx := context.Background()
	cmd1 := testhelpers.DefaultAuthorizeCommand()
	idempotencyKey := "idem-" + uuid.New().String()

	bankResp := &bank.AuthorizationResponse{
		Amount:          cmd1.Amount,
		Currency:        cmd1.Currency,
		Status:          "AUTHORIZED",
		AuthorizationID: "auth-123",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}

	suite.mockBank.EXPECT().
		Authorize(mock.Anything, mock.Anything, idempotencyKey).
		Return(bankResp, nil).
		Once()

	_, err := suite.service.Authorize(ctx, &cmd1, idempotencyKey)
	require.NoError(t, err)

	cmd2 := testhelpers.DefaultAuthorizeCommand()
	cmd2.Amount = 9999 // Different amount

	_, err = suite.service.Authorize(ctx, &cmd2, idempotencyKey)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "reused with different")
}

// ============================================================================
// FAILURE RECOVERY TESTS
// ============================================================================

func (suite *AuthorizeServiceTestSuite) Test_Authorize_BankReturns500_PaymentStaysPending() {
	t := suite.T()
	ctx := context.Background()
	cmd := testhelpers.DefaultAuthorizeCommand()
	idempotencyKey := "idem-" + uuid.New().String()

	bankErr := &bank.BankError{
		Code:       "internal_error",
		Message:    "Bank internal error",
		StatusCode: 500,
	}

	suite.mockBank.EXPECT().
		Authorize(mock.Anything, mock.Anything, idempotencyKey).
		Return(nil, bankErr).
		Once()

	payment, err := suite.service.Authorize(ctx, &cmd, idempotencyKey)

	require.Error(t, err)

	require.NotNil(t, payment)
	assert.Equal(t, domain.StatusPending, payment.Status)

	savedPayment, err := suite.paymentRepo.FindByID(ctx, payment.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPending, savedPayment.Status)
	assert.Nil(t, savedPayment.BankAuthID) // No bank ID yet
}

func (suite *AuthorizeServiceTestSuite) Test_Authorize_ContextCancelled_PaymentStaysPending() {
	t := suite.T()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := testhelpers.DefaultAuthorizeCommand()
	idempotencyKey := "idem-" + uuid.New().String()

	payment, err := suite.service.Authorize(ctx, &cmd, idempotencyKey)

	require.Error(t, err)
	svcErr, isSvcErr := application.IsServiceError(err)
	assert.True(t, errors.Is(err, context.Canceled) || (isSvcErr && svcErr.Code == application.ErrCodeIdempotencyMismatch))

	if payment != nil {
		assert.Equal(t, domain.StatusPending, payment.Status)
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func (suite *AuthorizeServiceTestSuite) Test_Authorize_ConcurrentRequests_OnlyOneSucceeds() {
	t := suite.T()
	ctx := context.Background()
	cmd := testhelpers.DefaultAuthorizeCommand()
	idempotencyKey := "idem-same-key"

	bankResp := &bank.AuthorizationResponse{
		Amount:          cmd.Amount,
		Currency:        cmd.Currency,
		Status:          "AUTHORIZED",
		AuthorizationID: "auth-123",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}

	suite.mockBank.EXPECT().
		Authorize(mock.Anything, mock.Anything, idempotencyKey).
		Return(bankResp, nil).
		Once()

	type result struct {
		payment *domain.Payment
		err     error
	}
	results := make(chan result, 2)

	for range 2 {
		go func() {
			payment, err := suite.service.Authorize(ctx, &cmd, idempotencyKey)
			results <- result{payment, err}
		}()
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
