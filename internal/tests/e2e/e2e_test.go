package e2e

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/tests/e2e/testdata"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type E2ETestSuite struct {
	suite.Suite
	client *TestClient
}

func TestE2ESuite(t *testing.T) {
	// Skip if not running E2E tests
	if os.Getenv("RUN_E2E_TESTS") != "true" {
		t.Skip("Skipping E2E tests (set RUN_E2E_TESTS=true to run)")
	}

	suite.Run(t, new(E2ETestSuite))
}

func (suite *E2ETestSuite) SetupSuite() {
	gatewayURL := "http://localhost:8081"

	suite.client = NewTestClient(gatewayURL)

	// Wait for gateway to be ready
	suite.waitForGateway()
}

func (suite *E2ETestSuite) waitForGateway() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			suite.T().Fatal("Gateway not ready after 30s")
		case <-ticker.C:
			url := "http://localhost:8787/health"
			httpReq, _ := http.NewRequest("GET", url, nil)
			httpReq.Header.Set("Content-Type", "application/json")

			resp, err := suite.client.httpClient.Do(httpReq)
			if err != nil {
				continue
			}
			if resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return
			}
			resp.Body.Close()
		}
	}
}

func (suite *E2ETestSuite) createAuthorizedPayment(orderID, customerID string) *api.Payment {
	t := suite.T()

	authReq := api.AuthorizeRequest{
		OrderId:     orderID,
		CustomerId:  customerID,
		Amount:      5000,
		CardNumber:  testdata.ValidCard.CardNumber,
		Cvv:         testdata.ValidCard.CVV,
		ExpiryMonth: testdata.ValidCard.ExpiryMonth,
		ExpiryYear:  testdata.ValidCard.ExpiryYear,
	}

	payment, err := suite.client.Authorize(t, authReq)
	require.NoError(t, err, "Authorization should succeed")

	assert.Equal(t, api.AUTHORIZED, payment.Status)
	assert.NotEmpty(t, payment.BankAuthId)
	assert.NotZero(t, payment.AuthorizedAt)
	assert.NotZero(t, payment.ExpiresAt)

	return payment
}

// ============================================================================
// HAPPY PATH: Authorize → Capture
// ============================================================================

func (suite *E2ETestSuite) TestHappyPath_AuthorizeAndCapture() {
	t := suite.T()

	orderID := "order-" + uuid.New().String()
	customerID := "cust-" + uuid.New().String()

	payment := suite.createAuthorizedPayment(orderID, customerID)

	capturedPayment, err := suite.client.Capture(t, payment.Id, payment.AmountCents)
	require.NoError(t, err, "Capture should succeed")

	assert.Equal(t, api.CAPTURED, capturedPayment.Status)
	assert.NotEmpty(t, capturedPayment.BankCaptureId)
	assert.NotZero(t, capturedPayment.CapturedAt)
}

func (suite *E2ETestSuite) TestHappyPath_AuthorizeAndVoid() {
	t := suite.T()

	orderID := "order-" + uuid.New().String()
	customerID := "cust-" + uuid.New().String()

	payment := suite.createAuthorizedPayment(orderID, customerID)

	voidedPayment, err := suite.client.Void(t, payment.Id)
	require.NoError(t, err, "Void should succeed")

	assert.Equal(t, api.VOIDED, voidedPayment.Status)
	assert.NotEmpty(t, voidedPayment.BankVoidId)
	assert.NotZero(t, voidedPayment.VoidedAt)
}

func (suite *E2ETestSuite) TestHappyPath_AuthorizeCaptureRefund() {
	t := suite.T()
	orderID := "order-" + uuid.New().String()
	customerID := "cust-" + uuid.New().String()

	payment := suite.createAuthorizedPayment(orderID, customerID)

	_, err := suite.client.Capture(t, payment.Id, payment.AmountCents)
	require.NoError(t, err, "Capture should succeed")

	refundedPayment, err := suite.client.Refund(t, payment.Id, payment.AmountCents)
	require.NoError(t, err, "Refund should succeed")

	assert.Equal(t, api.REFUNDED, refundedPayment.Status)
	assert.NotEmpty(t, refundedPayment.BankRefundId)
	assert.NotZero(t, refundedPayment.RefundedAt)

}

func (suite *E2ETestSuite) Test_FindByCustomerID_WithPagination() {
	t := suite.T()

	// Create 5 payments for same customer
	customerID := "cust-pagination-213"

	for range 5 {
		orderID := "order-" + uuid.New().String()
		suite.createAuthorizedPayment(orderID, customerID)
	}

	page1, err := suite.client.GetByCustomerID(t, customerID, 2, 0)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), page1, 2)

	page2, err := suite.client.GetByCustomerID(t, customerID, 2, 2)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), page2, 2)

	page3, err := suite.client.GetByCustomerID(t, customerID, 2, 4)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), page3, 1) // Only 1 remaining

	// Verify no duplicates across pages
	allPaymentIDs := make(map[string]bool)
	for _, payment := range append(append(page1, page2...), page3...) {
		assert.False(suite.T(), allPaymentIDs[payment.Id.String()], "Duplicate payment ID found")
		allPaymentIDs[payment.Id.String()] = true
	}
}

// ============================================================================
// FAILURE MODE: Insufficient Funds
// ============================================================================

func (suite *E2ETestSuite) TestFailure_ExpiredCard() {
	t := suite.T()

	authReq := api.AuthorizeRequest{
		OrderId:     "order-" + uuid.New().String(),
		CustomerId:  "cust-" + uuid.New().String(),
		Amount:      5000,
		CardNumber:  testdata.ExpiredCard.CardNumber,
		Cvv:         testdata.ExpiredCard.CVV,
		ExpiryMonth: testdata.ExpiredCard.ExpiryMonth,
		ExpiryYear:  testdata.ExpiredCard.ExpiryYear,
	}

	payment, err := suite.client.Authorize(t, authReq)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "card_expired")

	if payment != nil {
		assert.Equal(t, api.FAILED, payment.Status)
	}
}

func (suite *E2ETestSuite) TestFailure_InsufficientFunds() {
	t := suite.T()

	authReq := api.AuthorizeRequest{
		OrderId:     "order-" + uuid.New().String(),
		CustomerId:  "cust-" + uuid.New().String(),
		Amount:      5000,
		CardNumber:  testdata.InsufficientFundsCard.CardNumber,
		Cvv:         testdata.InsufficientFundsCard.CVV,
		ExpiryMonth: testdata.InsufficientFundsCard.ExpiryMonth,
		ExpiryYear:  testdata.InsufficientFundsCard.ExpiryYear,
	}

	payment, err := suite.client.Authorize(t, authReq)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient_funds")

	if payment != nil {
		assert.Equal(t, api.FAILED, payment.Status)
	}
}

func (suite *E2ETestSuite) TestFailure_AmountMismatch() {
	t := suite.T()

	orderID := "order-" + uuid.New().String()
	customerID := "cust-" + uuid.New().String()

	payment := suite.createAuthorizedPayment(orderID, customerID)

	_, err := suite.client.Capture(t, payment.Id, 2000) // Different amount from what was authorized
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Amount mismatch")
}

func (suite *E2ETestSuite) TestFailure_IdempotencyMismatch() {
	t := suite.T()

	authKey := "e2e-auth-" + uuid.New().String()
	captureKey := "e2e-cap-" + uuid.New().String()

	authReq := api.AuthorizeRequest{
		OrderId:     "order-" + uuid.New().String(),
		CustomerId:  "cust-" + uuid.New().String(),
		Amount:      2000,
		CardNumber:  testdata.ValidCard.CardNumber,
		Cvv:         testdata.ValidCard.CVV,
		ExpiryMonth: testdata.ValidCard.ExpiryMonth,
		ExpiryYear:  testdata.ValidCard.ExpiryYear,
	}

	payment1, err := suite.client.AuthorizeWithKey(t, authReq, authKey)
	require.NoError(t, err)

	capReq := api.CaptureRequest{
		PaymentId: payment1.Id,
	}

	capture1, err := suite.client.CaptureWithKey(t, capReq, captureKey)
	require.NoError(t, err)

	capture2, err := suite.client.CaptureWithKey(t, capReq, captureKey)
	require.NoError(t, err)

	assert.Equal(t, capture1.Id, capture2.Id)
	assert.Equal(t, capture1.BankCaptureId, capture2.BankCaptureId)

	differentcapReq := api.CaptureRequest{
		PaymentId: uuid.New(),
	}
	_, err = suite.client.CaptureWithKey(t, differentcapReq, captureKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Idempotency key reused with different request parameters")

}

// ============================================================================
// EDGE CASE: Idempotency
// ============================================================================

func (suite *E2ETestSuite) TestEdgeCase_IdempotencyPreventsDoubleCharge() {
	t := suite.T()

	idempotencyKey := "e2e-idem-" + uuid.New().String()

	authReq := api.AuthorizeRequest{
		OrderId:     "order-" + uuid.New().String(),
		CustomerId:  "cust-" + uuid.New().String(),
		Amount:      2000,
		CardNumber:  testdata.ValidCard.CardNumber,
		Cvv:         testdata.ValidCard.CVV,
		ExpiryMonth: testdata.ValidCard.ExpiryMonth,
		ExpiryYear:  testdata.ValidCard.ExpiryYear,
	}

	payment1, err1 := suite.client.AuthorizeWithKey(t, authReq, idempotencyKey)
	require.NoError(t, err1)

	payment2, err2 := suite.client.AuthorizeWithKey(t, authReq, idempotencyKey)
	require.NoError(t, err2)

	assert.Equal(t, payment1.Id, payment2.Id)
	assert.Equal(t, payment1.BankAuthId, payment2.BankAuthId)
}

// ============================================================================
// EDGE CASE: Cannot Capture Voided Payment
// ============================================================================

func (suite *E2ETestSuite) TestEdgeCase_CannotCaptureVoidedPayment() {
	t := suite.T()

	authReq := api.AuthorizeRequest{
		OrderId:     "order-" + uuid.New().String(),
		CustomerId:  "cust-" + uuid.New().String(),
		Amount:      1000,
		CardNumber:  testdata.ValidCard.CardNumber,
		Cvv:         testdata.ValidCard.CVV,
		ExpiryMonth: testdata.ValidCard.ExpiryMonth,
		ExpiryYear:  testdata.ValidCard.ExpiryYear,
	}

	payment, _ := suite.client.Authorize(t, authReq)
	suite.client.Void(t, payment.Id)

	_, err := suite.client.Capture(t, payment.Id, payment.AmountCents)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid transition")
}
