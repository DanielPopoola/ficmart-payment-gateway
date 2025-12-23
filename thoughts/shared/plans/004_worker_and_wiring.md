# Worker, Wiring, and High-Fidelity Idempotency Plan

## Overview
This plan covers the implementation of the background reconciler worker, the application entry point (`main.go`), and enhancements to the idempotency system to support high-fidelity cached responses and "Lazy Expiration" using the bank as the source of truth.

## Current State Analysis
- **Core Services**: Implemented and tested, but missing background reconciliation logic.
- **Idempotency**: Basic key/hash check exists, but no response payload caching.
- **Bank Client**: Missing GET endpoints for status verification.
- **Worker**: Non-existent.
- **Entry Point**: `cmd/gateway/main.go` was empty (now implemented but needs validation).

## Desired End State
- A background worker that resolves "stuck" payments (`PENDING`, `CAPTURING`, `VOIDING`, `REFUNDING`) and marks `EXPIRED` authorizations.
- High-fidelity idempotent replays that return the original bank response.
- A fully wired `main.go` with graceful shutdown.
- All services updated to be re-entrant and cache-aware.

## Phase 1: Enhanced Idempotency & Domain Updates
- [x] Create migration `002_enhance_idempotency.sql` to add `response_payload`, `status_code`, and `completed_at`.
- [x] Add `StatusRefunding` to domain and update transition rules.
- [x] Implement `UpdateIdempotencyKey` in the repository.
- [x] Update services to cache bank responses upon completion.
- [x] Update services to return cached results on key collision.

## Phase 2: Resilience & Reconciliation (Bank Integration)
- [x] Add `GetAuthorization`, `GetCapture`, and `GetRefund` to `BankPort`.
- [x] Implement GET methods in `HTTPBankClient` and `RetryBankClient`.
- [x] Implement `Reconcile(ctx, payment)` in all core services.
- [x] Update mocks in `mock_test.go` to support new interfaces.

## Phase 3: Background Reconciler Implementation
- [x] Implement `internal/worker/reconciler.go` with ticker loop.
- [x] Implement `reconcileStuckPayments` using `FindPendingPayments`.
- [x] Implement `checkExpiration` using `bankClient.GetAuthorization` (Lazy Expiration).
- [x] Add `isNotFound` logic to handle 404s from the bank.

## Phase 4: Application Wiring (`main.go`)
- [x] Implement `cmd/gateway/main.go` with dependency injection.
- [x] Add `WorkerConfig` to `config.go`.
- [x] Implement graceful shutdown logic (Signals + Context).
- [x] Add missing environment variable defaults or sample `.env`.

## Phase 5: Verification
- [ ] Integration test: Stuck payment recovery.
- [ ] Integration test: Concurrent request handling.
- [ ] E2E Manual flow verification.

## Testing Strategy
- **Unit Tests**: Ensure all services pass with new `Reconcile` and `UpdateIdempotencyKey` methods.
- **Race Tests**: Run `go test -race` during concurrency verification.
- **Worker Simulation**: Manually insert a `CAPTURING` payment and verify the worker moves it to `CAPTURED`.
