package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/config"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
)

type HTTPBankClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewBankClient(cfg config.BankConfig) ports.BankPort {
	return &HTTPBankClient{
		baseURL: cfg.BankBaseURL,
		httpClient: &http.Client{
			Timeout: cfg.BankConnTimeout * time.Second,
		},
	}
}

func (c *HTTPBankClient) Authorize(ctx context.Context, req domain.BankAuthorizationRequest, idempotencyKey string) (*domain.BankAuthorizationResponse, error) {
	return postJSON[domain.BankAuthorizationRequest, domain.BankAuthorizationResponse](
		c, ctx, "/api/v1/authorizations", req, idempotencyKey,
	)
}

func (c *HTTPBankClient) Capture(ctx context.Context, req domain.BankCaptureRequest, idempotencyKey string) (*domain.BankCaptureResponse, error) {
	return postJSON[domain.BankCaptureRequest, domain.BankCaptureResponse](
		c, ctx, "/api/v1/captures", req, idempotencyKey,
	)
}

func (c *HTTPBankClient) Void(ctx context.Context, req domain.BankVoidRequest, idempotencyKey string) (*domain.BankVoidResponse, error) {
	return postJSON[domain.BankVoidRequest, domain.BankVoidResponse](
		c, ctx, "/api/v1/voids", req, idempotencyKey,
	)
}

func (c *HTTPBankClient) Refund(ctx context.Context, req domain.BankRefundRequest, idempotencyKey string) (*domain.BankRefundResponse, error) {
	return postJSON[domain.BankRefundRequest, domain.BankRefundResponse](
		c, ctx, "/api/v1/refunds", req, idempotencyKey,
	)
}

// postJSON is a generic helper for making POST requests to the mock bank API
func postJSON[Req any, Resp any](c *HTTPBankClient, ctx context.Context, path string, req Req, idempotencyKey string) (*Resp, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshalling json: %w", err)
	}

	fullURL := c.baseURL + path
	httpReq, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bank returned status %d: %s", resp.StatusCode, string(body))
	}

	var bankResp Resp
	if err := json.NewDecoder(resp.Body).Decode(&bankResp); err != nil {
		return nil, fmt.Errorf("error decoding json response: %w", err)
	}

	return &bankResp, nil
}
