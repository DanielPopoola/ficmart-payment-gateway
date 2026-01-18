# Payment Gateway Architecture

## System Overview

This payment gateway is designed to ensure **distributed state consistency** between an e-commerce platform (FicMart) and a banking partner. It addresses the "partial failure" problem—where a system crash occurs after an external action (like charging a card) but before the result is saved locally.

### The Core Problem: Partial Failures
1. Gateway asks bank: "Capture $50 for Auth #123"
2. Bank says: "Success! Capture #456"
3. **Gateway crashes immediately.**
4. Local database still says the payment is "AUTHORIZED".
5. FicMart thinks the payment isn't captured, but the bank has already moved the money.

This system solves this using **Intermediate States**, **Write-Ahead Logging (WAL)**, and **Background Recovery Workers**.

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     FicMart (Client)                        │
└────────────────────────┬────────────────────────────────────┘
                         │ HTTP/REST
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Payment Gateway (Your System)              │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  REST API Layer (HTTP Handlers)                     │  │
│  │  • Receives requests & maps to commands             │  │
│  │  • Enforces Idempotency                             │  │
│  └──────────────────┬──────────────────────────────────┘  │
│                     │                                       │
│  ┌─────────────────▼──────────────────────────────────┐  │
│  │  Application Services (Orchestration)              │  │
│  │  • Authorize, Capture, Void, Refund Services        │  │
│  │  • Transaction Management & Error Classification    │  │
│  └──────────────────┬──────────────────────────────────┘  │
│                     │                                       │
│  ┌─────────────────▼──────────────────────────────────┐  │
│  │  Domain Layer (Pure Business Logic)                │  │
│  │  • Payment Entity & State Machine                  │  │
│  └──────────────────┬──────────────────────────────────┘  │
│                     │                                       │
│  ┌─────────────────▼──────────────────────────────────┐  │
│  │  Infrastructure Layer (External Systems)            │  │
│  │  • PostgreSQL Repositories                         │  │
│  │  • Bank Client (HTTP + Retry logic)                │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  Background Workers (Recovery & Maintenance)        │  │
│  │  • RetryWorker: Resumes stuck intermediate states   │  │
│  │  • ExpirationWorker: Syncs expired bank auths       │  │
│  └─────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
              ┌──────────────────┐
              │   Mock Bank API  │
              └──────────────────┘
```

---

## Layered Breakdown

### 1. Domain Layer (`internal/domain/`)
The "Heart" of the system. It contains the `Payment` entity and the strict state machine that governs its lifecycle.
- **Pure Go**: No dependencies on databases or HTTP.
- **State Machine**: Prevents invalid transitions (e.g., you cannot refund a voided payment).
- **Terminal States**: `CAPTURED`, `VOIDED`, `REFUNDED`, `FAILED`, `EXPIRED`.

### 2. Application Layer (`internal/application/`)
Orchestrates the business flow.
- **Services**: Dedicated services for each operation (`AuthorizeService`, `CaptureService`, etc.).
- **Error Categorizer**: Distinguishes between **Transient** (retryable), **Permanent** (don't retry), and **Business Rule** errors.
- **Idempotency Logic**: Uses `request_hash` to ensure identical requests return cached results, even if they arrive concurrently.

### 3. Infrastructure Layer (`internal/infrastructure/`)
Handles the "outside world."
- **Persistence**: PostgreSQL repositories for Payments and Idempotency Keys.
- **Bank Client**: Wraps raw HTTP calls with a Decorator that provides automatic retries for transient bank failures.

### 4. Background Workers (`internal/worker/`)
The "Cleaning Crew."
- **RetryWorker**: Polls for payments in intermediate states (`CAPTURING`, `VOIDING`, `REFUNDING`). It calls the bank with the original idempotency key to resume the operation.
- **ExpirationWorker**: Finds `AUTHORIZED` payments older than 8 days and reconciles them with the bank's 7-day expiration policy.

---

## Critical Patterns

### Pattern 1: Intermediate States (Intent Signaling)
Instead of jumping directly from `AUTHORIZED` to `CAPTURED`, we use an intermediate `CAPTURING` state.
1. Save state as `CAPTURING` in DB.
2. Call Bank.
3. On Success: Save state as `CAPTURED`.

If we crash at step 2, the `RetryWorker` finds the `CAPTURING` record and knows exactly what to do.

### Pattern 2: Atomic Idempotency Locking
We use a dedicated `idempotency_keys` table.
1. **Acquire Lock**: Insert key + `locked_at` timestamp.
2. **Execute**: Perform the operation.
3. **Release Lock**: Store result and set `locked_at = NULL`.

If a second request arrives while `locked_at` is set, the `waitForCompletion` loop polls until the first request finishes, ensuring the client receives the correct result without double-processing.

### Pattern 3: Write-Ahead Log (WAL) for Authorizations
Since we cannot store card details (PCI compliance), we cannot "retry" an authorization if the gateway crashes.
- We save the payment as `PENDING` *before* calling the bank.
- If we crash, the `RetryWorker` marks `PENDING` payments older than 10 minutes as `FAILED` (Orphaned Authorization Risk), alerting developers to manually check the bank if necessary.

---

## Data Flow: The "Capture" Journey

1. **Client POST /capture**: Handler receives request.
2. **Idempotency Check**: Hash the request. Is this key in the DB? Is it locked?
3. **Transition to CAPTURING**: A DB transaction sets the status to `CAPTURING` and locks the idempotency key.
4. **Call Bank API**: Attempt to charge the card.
5. **Success Handling**: 
    - Update Payment status to `CAPTURED`.
    - Store bank response in `idempotency_keys`.
    - Release the idempotency lock.
6. **Error Handling**: If the error is Permanent (e.g., Auth Expired), mark as `FAILED`. If Transient, leave as `CAPTURING` for the worker to fix.

---

## Database Schema

- **payments**: Stores the current source of truth for every transaction, including bank reference IDs and transition timestamps.
- **idempotency_keys**: Acts as a cache for responses and a distributed lock for concurrent requests. Includes a `request_hash` to prevent key-misuse with different parameters.

---

## Performance & Scalability
- **Database Integrity**: All state transitions and idempotency updates are wrapped in ACID-compliant transactions.
- **Idempotency Efficiency**: `checkIdempotency` is a sub-5ms lookup, protecting the system from redundant heavy operations.