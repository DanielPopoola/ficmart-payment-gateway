package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestClient wraps HTTP calls to gateway
type TestClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewTestClient(baseURL string) *TestClient {
	return &TestClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Authorize calls /authorize endpoint with different idempotencyKey on each call
func (c *TestClient) Authorize(t *testing.T, req api.AuthorizeRequest) (*api.Payment, error) {
	idempotencyKey := "e2e-auth-" + uuid.New().String()

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", c.baseURL+"/authorize", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var errResp api.ErrorResponse
		json.Unmarshal(bodyBytes, &errResp)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var paymentResp api.PaymentResponse
	require.NoError(t, json.Unmarshal(bodyBytes, &paymentResp))
	return &paymentResp.Data, nil
}

// Capture calls /capture endpoint
func (c *TestClient) Capture(t *testing.T, paymentID uuid.UUID, amount int64) (*api.Payment, error) {
	idempotencyKey := "e2e-cap-" + uuid.New().String()

	req := api.CaptureRequest{
		PaymentId: paymentID,
		Amount:    amount,
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", c.baseURL+"/capture", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var errResp api.ErrorResponse
		json.Unmarshal(bodyBytes, &errResp)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var paymentResp api.PaymentResponse
	require.NoError(t, json.Unmarshal(bodyBytes, &paymentResp))
	return &paymentResp.Data, nil
}

func (c *TestClient) Void(t *testing.T, paymentID uuid.UUID) (*api.Payment, error) {
	idempotencyKey := "e2e-void-" + uuid.New().String()

	req := api.VoidRequest{
		PaymentId: paymentID,
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", c.baseURL+"/void", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var errResp api.ErrorResponse
		json.Unmarshal(bodyBytes, &errResp)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var paymentResp api.PaymentResponse
	require.NoError(t, json.Unmarshal(bodyBytes, &paymentResp))
	return &paymentResp.Data, nil
}

func (c *TestClient) Refund(t *testing.T, paymentID uuid.UUID, amount int64) (*api.Payment, error) {
	idempotencyKey := "e2e-ref-" + uuid.New().String()

	req := api.RefundRequest{
		PaymentId: paymentID,
		Amount:    amount,
	}

	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", c.baseURL+"/refund", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var errResp api.ErrorResponse
		json.Unmarshal(bodyBytes, &errResp)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var paymentResp api.PaymentResponse
	require.NoError(t, json.Unmarshal(bodyBytes, &paymentResp))
	return &paymentResp.Data, nil
}

func (c *TestClient) GetByOrderID(t *testing.T, orderID string) (*api.Payment, error) {
	url := fmt.Sprintf("%s/payments/order/%s", c.baseURL, orderID)
	httpReq, _ := http.NewRequest("GET", url, nil)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var errResp api.ErrorResponse
		json.Unmarshal(bodyBytes, &errResp)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var paymentResp api.PaymentResponse
	require.NoError(t, json.Unmarshal(bodyBytes, &paymentResp))
	return &paymentResp.Data, nil

}

func (c *TestClient) GetByCustomerID(t *testing.T, customerID string, limit, offset int) ([]api.Payment, error) {
	url := fmt.Sprintf("%s/payments/customer/%s?limit=%d&offset=%d",
		c.baseURL, customerID, limit, offset)

	httpReq, _ := http.NewRequest("GET", url, nil)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var errResp api.ErrorResponse
		json.Unmarshal(bodyBytes, &errResp)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var response struct {
		Success bool          `json:"success"`
		Data    []api.Payment `json:"data"`
	}
	require.NoError(t, json.Unmarshal(bodyBytes, &response))
	return response.Data, nil
}

func (c *TestClient) AuthorizeWithKey(t *testing.T, req api.AuthorizeRequest, idempotencyKey string) (*api.Payment, error) {
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", c.baseURL+"/authorize", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var errResp api.ErrorResponse
		json.Unmarshal(bodyBytes, &errResp)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var paymentResp api.PaymentResponse
	require.NoError(t, json.Unmarshal(bodyBytes, &paymentResp))
	return &paymentResp.Data, nil
}

func (c *TestClient) CaptureWithKey(t *testing.T, req api.CaptureRequest, idempotencyKey string) (*api.Payment, error) {
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", c.baseURL+"/capture", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var errResp api.ErrorResponse
		json.Unmarshal(bodyBytes, &errResp)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var paymentResp api.PaymentResponse
	require.NoError(t, json.Unmarshal(bodyBytes, &paymentResp))
	return &paymentResp.Data, nil
}
