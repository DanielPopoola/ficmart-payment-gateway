package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/config"
)

type HTTPBankClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewBankClient(cfg config.BankConfig) application.BankClient {
	return &HTTPBankClient{
		baseURL: cfg.BankBaseURL,
		httpClient: &http.Client{
			Timeout: cfg.BankConnTimeout * time.Second,
		},
	}
}

func (c *HTTPBankClient) Authorize(ctx context.Context, req application.AuthorizationRequest, idempotencyKey string) (*application.AuthorizationResponse, error) {
	url := fmt.Sprintf("%s/api/v1/authorizations", c.baseURL)
	return sendRequest[application.AuthorizationRequest, application.AuthorizationResponse](c, ctx, http.MethodPost, url, &req, idempotencyKey)
}

func (c *HTTPBankClient) Capture(ctx context.Context, req application.CaptureRequest, idempotencyKey string) (*application.CaptureResponse, error) {
	url := fmt.Sprintf("%s/api/v1/captures", c.baseURL)
	return sendRequest[application.CaptureRequest, application.CaptureResponse](c, ctx, http.MethodPost, url, &req, idempotencyKey)
}

func (c *HTTPBankClient) Void(ctx context.Context, req application.VoidRequest, idempotencyKey string) (*application.VoidResponse, error) {
	url := fmt.Sprintf("%s/api/v1/voids", c.baseURL)
	return sendRequest[application.VoidRequest, application.VoidResponse](c, ctx, http.MethodPost, url, &req, idempotencyKey)
}

func (c *HTTPBankClient) Refund(ctx context.Context, req application.RefundRequest, idempotencyKey string) (*application.RefundResponse, error) {
	url := fmt.Sprintf("%s/api/v1/refunds", c.baseURL)
	return sendRequest[application.RefundRequest, application.RefundResponse](c, ctx, http.MethodPost, url, &req, idempotencyKey)
}

func (c *HTTPBankClient) GetAuthorization(ctx context.Context, authID string) (*application.AuthorizationResponse, error) {
	url := fmt.Sprintf("%s/api/v1/authorizations/%s", c.baseURL, authID)
	return sendRequest[any, application.AuthorizationResponse](c, ctx, http.MethodGet, url, nil, "")
}

func (c *HTTPBankClient) GetCapture(ctx context.Context, captureID string) (*application.CaptureResponse, error) {
	url := fmt.Sprintf("%s/api/v1/captures/%s", c.baseURL, captureID)
	return sendRequest[any, application.CaptureResponse](c, ctx, http.MethodGet, url, nil, "")
}

func (c *HTTPBankClient) GetRefund(ctx context.Context, refundID string) (*application.RefundResponse, error) {
	url := fmt.Sprintf("%s/api/v1/refunds/%s", c.baseURL, refundID)
	return sendRequest[any, application.RefundResponse](c, ctx, http.MethodGet, url, nil, "")
}

func sendRequest[Req any, Resp any](c *HTTPBankClient, ctx context.Context, method, url string, reqBody *Req, idempotencyKey string) (*Resp, error) {
	var bodyReader io.Reader
	if reqBody != nil {
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("error marshalling json: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	if reqBody != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	if idempotencyKey != "" {
		httpReq.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var bankErrResp BankErrorResponse
		if err := json.Unmarshal(body, &bankErrResp); err != nil {
			return nil, fmt.Errorf("bank returned status %d: %s", resp.StatusCode, string(body))
		}
		return nil, &BankError{
			Code:       bankErrResp.Err,
			Message:    bankErrResp.Message,
			StatusCode: resp.StatusCode,
		}
	}

	var bankResp Resp
	if err := json.NewDecoder(resp.Body).Decode(&bankResp); err != nil {
		return nil, fmt.Errorf("error decoding json response: %w", err)
	}

	return &bankResp, nil
}
