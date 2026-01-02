package bank

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/config"
)

type RetryBankClient struct {
	inner      application.BankClient
	baseDelay  time.Duration
	maxRetries int
}

func NewRetryBankClient(inner application.BankClient, cfg config.RetryConfig) application.BankClient {
	return &RetryBankClient{
		inner:      inner,
		baseDelay:  time.Duration(cfg.BaseDelay) * time.Second,
		maxRetries: int(cfg.MaxRetries),
	}
}

// Authorize with retry logic
func (r *RetryBankClient) Authorize(ctx context.Context, req application.BankAuthorizationRequest, idempotencyKey string) (*application.BankAuthorizationResponse, error) {
	return retry(
		r,
		ctx,
		func(ctx context.Context) (*application.BankAuthorizationResponse, error) {
			return r.inner.Authorize(ctx, req, idempotencyKey)
		},
	)
}

// Capture with retry logic
func (r *RetryBankClient) Capture(ctx context.Context, req application.BankCaptureRequest, idempotencyKey string) (*application.BankCaptureResponse, error) {
	return retry(
		r,
		ctx,
		func(ctx context.Context) (*application.BankCaptureResponse, error) {
			return r.inner.Capture(ctx, req, idempotencyKey)
		},
	)
}

// Void with retry logic
func (r *RetryBankClient) Void(ctx context.Context, req application.BankVoidRequest, idempotencyKey string) (*application.BankVoidResponse, error) {
	return retry(
		r,
		ctx,
		func(ctx context.Context) (*application.BankVoidResponse, error) {
			return r.inner.Void(ctx, req, idempotencyKey)
		},
	)
}

// Refund with retry logic
func (r *RetryBankClient) Refund(ctx context.Context, req application.BankRefundRequest, idempotencyKey string) (*application.BankRefundResponse, error) {
	return retry(
		r,
		ctx,
		func(ctx context.Context) (*application.BankRefundResponse, error) {
			return r.inner.Refund(ctx, req, idempotencyKey)
		},
	)
}

func (r *RetryBankClient) GetAuthorization(ctx context.Context, authID string) (*application.BankAuthorizationResponse, error) {
	return retry(
		r,
		ctx,
		func(ctx context.Context) (*application.BankAuthorizationResponse, error) {
			return r.inner.GetAuthorization(ctx, authID)
		},
	)
}

// Generic retry helper
func retry[T any](r *RetryBankClient, ctx context.Context, operation func(ctx context.Context) (*T, error)) (*T, error) {
	var lastErr error

	for attempt := 0; attempt < r.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := operation(ctx)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		if !isRetryable(err) {
			return nil, err
		}

		if attempt < r.maxRetries-1 {
			time.Sleep(r.backoff(attempt))
		}
	}

	return nil, fmt.Errorf("maximum retries exceeded: %w", lastErr)
}

// Helper: to check retryable errors
func isRetryable(err error) bool {
	var bankErr *application.BankError
	if errors.As(err, &bankErr) {
		if bankErr.StatusCode >= 500 {
			return true
		}

		if bankErr.Code == "internal_error" {
			return true
		}

		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	return true
}

// Backoff calculation with exponential delay and jitter
func (r *RetryBankClient) backoff(attempt int) time.Duration {
	base := r.baseDelay * time.Duration(1<<attempt)

	jitter := time.Duration(rand.Intn(1000)) * time.Millisecond

	return base + jitter
}
