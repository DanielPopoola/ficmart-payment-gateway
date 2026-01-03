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

// ============================================================================
// HAPPY PATH: Authorize → Capture
// ============================================================================

func (suite *E2ETestSuite) TestHappyPath_AuthorizeAndCapture() {
	t := suite.T()

	authReq := api.AuthorizeRequest{
		OrderId:     "order-" + uuid.New().String(),
		CustomerId:  "cust-" + uuid.New().String(),
		Amount:      5000,
		CardNumber:  testdata.ValidCard.CardNumber,
		Cvv:         testdata.ValidCard.CVV,
		ExpiryMonth: testdata.ValidCard.ExpiryMonth,
		ExpiryYear:  testdata.ValidCard.ExpiryYear,
	}

	payment, err := suite.client.Authorize(t, authReq)
	require.NoError(t, err, "Authorization should suceed")

	assert.Equal(t, api.AUTHORIZED, payment.Status)
	assert.NotEmpty(t, payment.BankAuthId)
	assert.NotZero(t, payment.AuthorizedAt)
	assert.NotZero(t, payment.ExpiresAt)

	capturedPayment, err := suite.client.Capture(t, payment.Id, payment.AmountCents)
	require.NoError(t, err, "Capture should succeed")

	assert.Equal(t, api.CAPTURED, capturedPayment.Status)
	assert.NotEmpty(t, capturedPayment.BankCaptureId)
	assert.NotZero(t, capturedPayment.CapturedAt)
}

func (suite *E2ETestSuite) TestHappyPath_AuthorizeAndVoid() {
	t := suite.T()

	authReq := api.AuthorizeRequest{
		OrderId:     "order-" + uuid.New().String(),
		CustomerId:  "cust-" + uuid.New().String(),
		Amount:      5000,
		CardNumber:  testdata.ValidCard.CardNumber,
		Cvv:         testdata.ValidCard.CVV,
		ExpiryMonth: testdata.ValidCard.ExpiryMonth,
		ExpiryYear:  testdata.ValidCard.ExpiryYear,
	}

	payment, err := suite.client.Authorize(t, authReq)
	require.NoError(t, err, "Authorization should suceed")

	assert.Equal(t, api.AUTHORIZED, payment.Status)

	voidedPayment, err := suite.client.Void(t, payment.Id)
	require.NoError(t, err, "Void should succeed")

	assert.Equal(t, api.VOIDED, voidedPayment.Status)
	assert.NotEmpty(t, voidedPayment.BankVoidId)
	assert.NotZero(t, voidedPayment.VoidedAt)
}

// ============================================================================
// FAILURE MODE: Insufficient Funds
// ============================================================================

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
	assert.Contains(t, err.Error(), "Invalid transition")
}
