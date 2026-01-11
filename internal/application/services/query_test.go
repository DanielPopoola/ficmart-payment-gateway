package services_test

import (
	"context"
	"testing"
	"time"

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

type QueryServiceTestSuite struct {
	suite.Suite
	testDB           *testhelpers.TestDatabase
	paymentRepo      *postgres.PaymentRepository
	idempotencyRepo  *postgres.IdempotencyRepository
	mockBank         *mocks.MockBankClient
	authorizeService *services.AuthorizeService
	queryService     *services.QueryService
}

func TestQueryServiceSuite(t *testing.T) {
	suite.Run(t, new(QueryServiceTestSuite))
}

func (suite *QueryServiceTestSuite) SetupSuite() {
	suite.testDB = testhelpers.SetupTestDatabase(suite.T())
	suite.paymentRepo = postgres.NewPaymentRepository(suite.testDB.DB)
	suite.idempotencyRepo = postgres.NewIdempotencyRepository(suite.testDB.DB)
}

func (suite *QueryServiceTestSuite) TearDownSuite() {
	suite.testDB.Cleanup(suite.T())
}

func (suite *QueryServiceTestSuite) SetupTest() {
	suite.mockBank = mocks.NewMockBankClient(suite.T())

	suite.authorizeService = services.NewAuthorizeService(
		suite.paymentRepo,
		suite.idempotencyRepo,
		suite.mockBank,
		suite.testDB.DB,
	)

	suite.queryService = services.NewQueryService(suite.paymentRepo)
}

// TearDownTest runs after each test
func (suite *QueryServiceTestSuite) TearDownTest() {
	suite.testDB.CleanTables(suite.T())
}

// Helper: Create authorized payment using AuthorizeService
func (suite *QueryServiceTestSuite) createAuthorizedPayment(ctx context.Context, orderID, customerID string) *domain.Payment {
	cmd := services.AuthorizeCommand{
		OrderID:     orderID,
		CustomerID:  customerID,
		Amount:      5000,
		Currency:    "USD",
		CardNumber:  "4111111111111111",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}
	idempotencyKey := "idem-" + uuid.New().String()

	// Mock bank response
	authResp := &bank.AuthorizationResponse{
		Amount:          cmd.Amount,
		Currency:        cmd.Currency,
		Status:          "AUTHORIZED",
		AuthorizationID: "auth-" + uuid.New().String(),
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
	}

	suite.mockBank.EXPECT().
		Authorize(mock.Anything, mock.Anything, idempotencyKey).
		Return(authResp, nil).
		Once()

	payment, err := suite.authorizeService.Authorize(ctx, &cmd, idempotencyKey)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), payment)

	return payment
}

// ============================================================================
// HAPPY PATH TESTS
// ============================================================================

func (suite *QueryServiceTestSuite) Test_FindByID_Success() {
	ctx := context.Background()

	// Create payment
	orderID := "order-" + uuid.New().String()
	customerID := "cust-" + uuid.New().String()
	payment := suite.createAuthorizedPayment(ctx, orderID, customerID)

	// Query by ID
	foundPayment, err := suite.queryService.FindByID(ctx, payment.ID)

	// Assert
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), foundPayment)

	assert.Equal(suite.T(), payment.ID, foundPayment.ID)
	assert.Equal(suite.T(), orderID, foundPayment.OrderID)
	assert.Equal(suite.T(), customerID, foundPayment.CustomerID)
	assert.Equal(suite.T(), domain.StatusAuthorized, foundPayment.Status)
}

func (suite *QueryServiceTestSuite) Test_FindByOrderID_Success() {
	ctx := context.Background()

	// Create payment with specific order ID
	orderID := "order-specific-123"
	customerID := "cust-" + uuid.New().String()
	payment := suite.createAuthorizedPayment(ctx, orderID, customerID)

	// Query by order ID
	foundPayment, err := suite.queryService.FindByOrderID(ctx, orderID)

	// Assert
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), foundPayment)

	assert.Equal(suite.T(), payment.ID, foundPayment.ID)
	assert.Equal(suite.T(), orderID, foundPayment.OrderID)
}

func (suite *QueryServiceTestSuite) Test_FindByCustomerID_Success() {
	ctx := context.Background()

	// Create multiple payments for same customer
	customerID := "cust-specific-456"

	payment1 := suite.createAuthorizedPayment(ctx, "order-1", customerID)
	payment2 := suite.createAuthorizedPayment(ctx, "order-2", customerID)
	payment3 := suite.createAuthorizedPayment(ctx, "order-3", customerID)

	// Query by customer ID
	payments, err := suite.queryService.FindByCustomerID(ctx, customerID, 10, 0)

	// Assert
	require.NoError(suite.T(), err)
	require.Len(suite.T(), payments, 3)

	// Verify all payments belong to the customer
	paymentIDs := []string{payment1.ID, payment2.ID, payment3.ID}
	for _, payment := range payments {
		assert.Equal(suite.T(), customerID, payment.CustomerID)
		assert.Contains(suite.T(), paymentIDs, payment.ID)
	}
}

func (suite *QueryServiceTestSuite) Test_FindByCustomerID_WithPagination() {
	ctx := context.Background()

	// Create 5 payments for same customer
	customerID := "cust-pagination-789"

	for i := 0; i < 5; i++ {
		orderID := "order-" + uuid.New().String()
		suite.createAuthorizedPayment(ctx, orderID, customerID)
	}

	// First page (limit=2, offset=0)
	page1, err := suite.queryService.FindByCustomerID(ctx, customerID, 2, 0)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), page1, 2)

	// Second page (limit=2, offset=2)
	page2, err := suite.queryService.FindByCustomerID(ctx, customerID, 2, 2)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), page2, 2)

	// Third page (limit=2, offset=4)
	page3, err := suite.queryService.FindByCustomerID(ctx, customerID, 2, 4)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), page3, 1) // Only 1 remaining

	// Verify no duplicates across pages
	allPaymentIDs := make(map[string]bool)
	for _, payment := range append(append(page1, page2...), page3...) {
		assert.False(suite.T(), allPaymentIDs[payment.ID], "Duplicate payment ID found")
		allPaymentIDs[payment.ID] = true
	}
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func (suite *QueryServiceTestSuite) Test_FindByID_NotFound() {
	ctx := context.Background()

	nonExistentID := uuid.New().String()

	_, err := suite.queryService.FindByID(ctx, nonExistentID)

	require.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, postgres.ErrPaymentNotFound)
}

func (suite *QueryServiceTestSuite) Test_FindByOrderID_NotFound() {
	ctx := context.Background()

	_, err := suite.queryService.FindByOrderID(ctx, "non-existent-order")

	require.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, postgres.ErrPaymentNotFound)
}

func (suite *QueryServiceTestSuite) Test_FindByCustomerID_EmptyResult() {
	ctx := context.Background()

	payments, err := suite.queryService.FindByCustomerID(ctx, "customer-no-payments", 10, 0)

	require.NoError(suite.T(), err)
	assert.Empty(suite.T(), payments)
}

func (suite *QueryServiceTestSuite) Test_FindByCustomerID_OnlyReturnsCustomerPayments() {
	ctx := context.Background()

	customer1 := "cust-one"
	customer2 := "cust-two"

	suite.createAuthorizedPayment(ctx, "order-1", customer1)
	suite.createAuthorizedPayment(ctx, "order-2", customer1)
	suite.createAuthorizedPayment(ctx, "order-3", customer2)

	payments, err := suite.queryService.FindByCustomerID(ctx, customer1, 10, 0)

	// Should only return customer1's payments
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), payments, 2)

	for _, payment := range payments {
		assert.Equal(suite.T(), customer1, payment.CustomerID)
	}
}

func (suite *QueryServiceTestSuite) Test_FindByCustomerID_RespectsLimit() {
	ctx := context.Background()

	customerID := "cust-limit-test"

	for i := 0; i < 10; i++ {
		orderID := "order-" + uuid.New().String()
		suite.createAuthorizedPayment(ctx, orderID, customerID)
	}

	payments, err := suite.queryService.FindByCustomerID(ctx, customerID, 3, 0)

	require.NoError(suite.T(), err)
	assert.Len(suite.T(), payments, 3)
}

func (suite *QueryServiceTestSuite) Test_FindByCustomerID_RespectsOffset() {
	ctx := context.Background()

	customerID := "cust-offset-test"

	for range 5 {
		orderID := "order-" + uuid.New().String()
		suite.createAuthorizedPayment(ctx, orderID, customerID)
	}

	allPayments, err := suite.queryService.FindByCustomerID(ctx, customerID, 10, 0)
	require.NoError(suite.T(), err)
	require.Len(suite.T(), allPayments, 5)

	offsetPayments, err := suite.queryService.FindByCustomerID(ctx, customerID, 10, 2)

	require.NoError(suite.T(), err)
	assert.Len(suite.T(), offsetPayments, 3)

	firstTwoIDs := []string{allPayments[0].ID, allPayments[1].ID}
	for _, payment := range offsetPayments {
		assert.NotContains(suite.T(), firstTwoIDs, payment.ID)
	}
}
