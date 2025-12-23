---
date: 2025-12-23T12:00:00Z
researcher: Gemini
topic: "Worker and Wiring Architecture"
tags: [research, codebase, worker, reconciler, main, wiring]
status: complete
---

# Research: Worker and Wiring Architecture

## Research Question
Plan and implement the background worker and the wiring of everything in `cmd/gateway`.

## Summary
The background worker will be a ticker-based reconciler that periodically checks for stuck payments in intermediate states (`PENDING`, `CAPTURING`, `VOIDING`, `REFUNDING`) and uses the service's `Reconcile` methods to resolve them. The `main.go` entry point will handle configuration loading, database connection pooling, dependency injection across all layers, and graceful shutdown.

## Detailed Findings

### Background Worker (`internal/worker/reconciler.go`)
- **Responsibility**: Eventual consistency for payments that failed or crashed during processing.
- **Mechanism**: Ticker loop (e.g., every 30-60 seconds).
- **Batching**: Uses `repo.FindPendingPayments` to fetch a limited number of stuck payments.
- **Strategy**: Calls `service.Reconcile(ctx, payment)` based on the status.
- **Lazy Expiration**: Fetches `AUTHORIZED` payments and verifies with `bankClient.GetAuthorization`.

### Wiring (`cmd/gateway/main.go`)
- **Config**: Loaded via `koanf` from environment variables.
- **DB**: `pgxpool` managed via `internal/adapters/postgres/connection.go`.
- **Adapters**: `RetryBankClient` wraps `HTTPBankClient`.
- **Services**: All domain services (`AuthorizationService`, `CaptureService`, etc.) are instantiated here.
- **HTTP**: `PaymentHandler` registers routes on `http.ServeMux`.
- **Graceful Shutdown**: Catches `SIGINT`/`SIGTERM`, stops the worker, shuts down the HTTP server with timeout, and closes the DB pool.

## Code References
- `internal/adapters/postgres/repository.go:174` - `FindPendingPayments` implementation.
- `internal/core/service/authorize.go` - `Reconcile` method for authorizations.
- `internal/adapters/handler/http.go` - Handler registration logic.
- `internal/config/config.go` - Main configuration struct.

## Architecture Insights
- **Re-entrancy**: Services are designed to be re-entrant, allowing the worker to safely retry operations with the same idempotency key.
- **Clean Shutdown**: Using `context.WithCancel` for the worker ensures it stops processing immediately when the application shuts down.
- **Pool Management**: `pgxpool` is shared across all repositories, providing efficient connection management.

## Open Questions
- Should the worker interval be configurable via environment variables? (Recommended).
- What is the desired batch size for `FindPendingPayments`? (Defaulting to 100).
