// Concrete implementation client of the bank client interface
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
)

type BankClient interface {
	Authorize(ctx context.Context, req AuthorizationRequest, idempotencyKey string) (*AuthorizationResponse, error)
	Capture(ctx context.Context, req CaptureRequest, idempotencyKey string) (*CaptureResponse, error)
	Void(ctx context.Context, req VoidRequest, idempotencyKey string) (*VoidResponse, error)
	Refund(ctx context.Context, req RefundRequest, idempotencyKey string) (*RefundResponse, error)

	GetAuthorization(ctx context.Context, authID string) (*AuthorizationResponse, error)
}

type HTTPBankClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewBankClient(cfg config.BankConfig) BankClient {
	return &HTTPBankClient{
		baseURL: cfg.BankBaseURL,
		httpClient: &http.Client{
			Timeout: cfg.BankConnTimeout * time.Second,
		},
	}
}

func (c *HTTPBankClient) Authorize(ctx context.Context, req AuthorizationRequest, idempotencyKey string) (*AuthorizationResponse, error) {
	url := fmt.Sprintf("%s/api/v1/authorizations", c.baseURL)
	return sendRequest[AuthorizationRequest, AuthorizationResponse](c, ctx, http.MethodPost, url, &req, idempotencyKey)
}

func (c *HTTPBankClient) Capture(ctx context.Context, req CaptureRequest, idempotencyKey string) (*CaptureResponse, error) {
	url := fmt.Sprintf("%s/api/v1/captures", c.baseURL)
	return sendRequest[CaptureRequest, CaptureResponse](c, ctx, http.MethodPost, url, &req, idempotencyKey)
}

func (c *HTTPBankClient) Void(ctx context.Context, req VoidRequest, idempotencyKey string) (*VoidResponse, error) {
	url := fmt.Sprintf("%s/api/v1/voids", c.baseURL)
	return sendRequest[VoidRequest, VoidResponse](c, ctx, http.MethodPost, url, &req, idempotencyKey)
}

func (c *HTTPBankClient) Refund(ctx context.Context, req RefundRequest, idempotencyKey string) (*RefundResponse, error) {
	url := fmt.Sprintf("%s/api/v1/refunds", c.baseURL)
	return sendRequest[RefundRequest, RefundResponse](c, ctx, http.MethodPost, url, &req, idempotencyKey)
}

func (c *HTTPBankClient) GetAuthorization(ctx context.Context, authID string) (*AuthorizationResponse, error) {
	url := fmt.Sprintf("%s/api/v1/authorizations/%s", c.baseURL, authID)
	return sendRequest[any, AuthorizationResponse](c, ctx, http.MethodGet, url, nil, "")
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

	defer func() {
		_ = resp.Body.Close() //nolint:errcheck // Closing the response body; error can be ignored here.
	}()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, &BankError{
				Code:       "READ_ERROR",
				Message:    fmt.Sprintf("failed to read response body: %v", err),
				StatusCode: resp.StatusCode,
			}
		}
		var bankErrResp BankErrorResponse
		if err := json.Unmarshal(body, &bankErrResp); err != nil {
			return nil, &BankError{
				Code:       "UNKNOWN",
				Message:    string(body),
				StatusCode: resp.StatusCode,
			}
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
