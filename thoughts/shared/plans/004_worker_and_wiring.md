# Worker, Wiring, and High-Fidelity Idempotency Plan

## Overview
This plan covers the implementation of the background reconciler worker, the application entry point (`main.go`), and enhancements to the idempotency system to support high-fidelity cached responses and "Lazy Expiration" using the bank as the source of truth.

## Current State Analysis
- **Core Services**: Implemented and tested, with background reconciliation logic added.
- **Idempotency**: High-fidelity payload caching implemented.
- **Bank Client**: GET endpoints for status verification implemented.
- **Worker**: Fully implemented and tested.
- **Entry Point**: `cmd/gateway/main.go` implemented and verified.

## Desired End State
- [x] A background worker that resolves "stuck" payments (`PENDING`, `CAPTURING`, `VOIDING`, `REFUNDING`) and marks `EXPIRED` authorizations.
- [x] High-fidelity idempotent replays that return the original bank response.
- [x] A fully wired `main.go` with graceful shutdown.
- [x] All services updated to be re-entrant and cache-aware.

## Phase 1: Enhanced Idempotency & Domain Updates
- [x] Create migration `001_initial_schema.sql` (updated with enhanced columns).
- [x] Add `StatusRefunding` to domain and update transition rules.
- [x] Implement `UpdateIdempotencyKey` in the repository.
- [x] Update services to cache bank responses upon completion.
- [x] Update services to return cached results on key collision.

## Phase 2: Resilience & Reconciliation (Bank Integration)
- [x] Add `GetAuthorization`, `GetCapture`, and `GetRefund` to `BankPort`.
- [x] Implement GET methods in `HTTPBankClient` and `RetryBankClient`.
- [x] Implement `Reconcile(ctx, payment)` in all core services.
- [x] Update mocks in `mocks.go` to support new interfaces.

## Phase 3: Background Reconciler Implementation
- [x] Implement `internal/worker/reconciler.go` with ticker loop.
- [x] Implement `reconcileStuckPayments` using `FindPendingPayments`.
- [x] Implement `checkExpiration` using `bankClient.GetAuthorization` (Lazy Expiration).
- [x] Add `isNotFound` logic to handle 404s from the bank.

## Phase 4: Application Wiring (`main.go`)
- [x] Implement `cmd/gateway/main.go` with dependency injection.
- [x] Add `WorkerConfig` and `koanf` tags to `config.go`.
- [x] Implement graceful shutdown logic (Signals + Context).
- [x] Add sample `.env.example`.

## Phase 5: Verification
- [x] Integration test: Stuck payment recovery.
- [x] Integration test: Concurrent request handling.
- [x] E2E Manual flow verification (verified via FullFlow integration test).

## Testing Strategy
- [x] **Unit Tests**: All services pass.
- [x] **Integration Tests**: Comprehensive suite in `internal/tests` passing against real Postgres and Docker Bank API.
- [x] **Race Verification**: Concurrent tests prove idempotency and double-spend protection.
