# Payment Gateway Architecture

## System Overview

This is a production-grade payment gateway that processes card payments through a mock banking API. The core challenge it solves is **distributed state consistency** - ensuring your database always reflects reality, even when the network fails, the gateway crashes, or the bank returns errors after processing a request.

### The Core Problem (ELI5)

Imagine you're buying a toy for $50:

1. Gateway asks bank: "Can I hold $50 from this card?"
2. Bank says: "Yes! Authorization #123"
3. **Gateway crashes before saving that the bank said yes**
4. When gateway restarts, it doesn't know the bank already reserved the money

This is a **partial failure**. The bank thinks money is reserved, but your database thinks the payment failed. This gateway solves this problem.

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     FicMart (Client)                        │
│                  "I want to charge $50"                     │
└────────────────────────┬────────────────────────────────────┘
                         │ HTTP/REST
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Payment Gateway (Your System)              │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  REST API Layer (HTTP Handlers)                     │  │
│  │  • Receives requests                                │  │
│  │  • Validates input                                  │  │
│  │  • Returns responses                                │  │
│  └──────────────────┬──────────────────────────────────┘  │
│                     │                                       │
│  ┌─────────────────▼──────────────────────────────────┐  │
│  │  Application Services (Orchestration)              │  │
│  │  • AuthorizeService: Reserve funds                 │  │
│  │  • CaptureService: Charge the card                 │  │
│  │  • VoidService: Cancel reservation                 │  │
│  │  • RefundService: Return money                     │  │
│  │  • QueryService: Get payment status                │  │
│  └──────────────────┬──────────────────────────────────┘  │
│                     │                                       │
│  ┌─────────────────▼──────────────────────────────────┐  │
│  │  Domain Layer (Business Rules - PURE)              │  │
│  │  • Payment entity with state machine               │  │
│  │  • State transition rules                          │  │
│  │  • No external dependencies                        │  │
│  └──────────────────┬──────────────────────────────────┘  │
│                     │                                       │
│  ┌─────────────────▼──────────────────────────────────┐  │
│  │  Infrastructure Layer (External World)             │  │
│  │  • PostgreSQL repositories                         │  │
│  │  • Bank HTTP client (with retry)                   │  │
│  │  • Database transactions                           │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  Background Workers (Failure Recovery)              │  │
│  │  • RetryWorker: Fixes stuck payments                │  │
│  │  • ExpirationWorker: Marks expired authorizations   │  │
│  └─────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
              ┌──────────────────┐
              │   Mock Bank API  │
              │  (External)      │
              └──────────────────┘
```

---

## Layer-by-Layer Breakdown

### 1. REST API Layer (`internal/interfaces/rest/`)

**What it does:** Translates HTTP requests into application commands.

**Example Flow:**
```
POST /authorize
{
  "amount": 5000,
  "card_number": "4111...",
  "order_id": "order-123"
}
    ↓
handlers.AuthorizePayment()
    ↓
Creates: AuthorizeCommand{Amount: 5000, OrderID: "order-123"}
    ↓
Calls: authService.Authorize(cmd, idempotencyKey)
```

**Key Files:**
- `handlers/authorize.go` - Authorize endpoint
- `handlers/capture.go` - Capture endpoint
- `middleware/logging.go` - Logs all requests
- `middleware/recovery.go` - Catches panics

### 2. Application Services (`internal/application/services/`)

**What it does:** Orchestrates business operations across multiple layers.

**ELI5 Example - Authorize Flow:**
```
1. Check: Did I already process this request? (idempotency check)
   └─ If yes → return cached result
   └─ If no → continue

2. Create: New payment in PENDING state
   └─ Save to database immediately (Write-Ahead Log pattern)

3. Lock: Mark idempotency key as "processing"
   └─ Prevents duplicate concurrent requests

4. Call Bank: "Reserve $50 please"
   └─ If bank says yes → update payment to AUTHORIZED
   └─ If bank says no (400 error) → mark payment FAILED
   └─ If bank crashes (500 error) → leave payment PENDING for retry

5. Unlock: Release idempotency key
   └─ Future requests with same key return cached result
```

**Why this order matters:**
- We save payment BEFORE calling the bank
- If we crash after bank approves but before saving, the payment stays PENDING
- Worker finds PENDING payment and knows: "Bank might have approved this, I need to check"

**Key Services:**
- `authorize.go` - Reserve funds on card
- `capture.go` - Actually charge the card
- `void.go` - Cancel a reservation
- `refund.go` - Return money
- `helpers.go` - Shared idempotency logic

### 3. Domain Layer (`internal/domain/`)

**What it does:** Enforces business rules. Has ZERO dependencies on database/HTTP/bank.

**The State Machine (ELI5):**
```
Think of a payment like a traffic light:

PENDING (Red) → Can only go to AUTHORIZED or FAILED
    ↓
AUTHORIZED (Yellow) → Can go to CAPTURING, VOIDING, or EXPIRED
    ↓                      ↓
CAPTURING              VOIDING
    ↓                      ↓
CAPTURED              VOIDED
    ↓
REFUNDING
    ↓
REFUNDED

Once you're at CAPTURED, you can't go to VOIDED (no turning back!)
Once you're at VOIDED/REFUNDED/FAILED/EXPIRED, you can't go anywhere (terminal states)
```

**Code Example:**
```go
// This is enforced in the domain layer
payment.MarkCapturing() // Only works if status is AUTHORIZED
payment.MarkVoiding()   // Only works if status is AUTHORIZED
payment.Capture()       // Only works if status is CAPTURING
```

**Key Files:**
- `payment.go` - The Payment entity with state machine
- `errors.go` - Domain-specific errors

### 4. Infrastructure Layer (`internal/infrastructure/`)

**What it does:** Talks to external systems (database, bank API).

**Bank Client Pattern:**
```
YourCode
    ↓
RetryBankClient (wraps HTTPBankClient)
    ↓ (retries 500 errors automatically)
HTTPBankClient
    ↓ (makes actual HTTP call)
Mock Bank API
```

**Why two clients?**
- `HTTPBankClient` - Simple, makes raw HTTP calls
- `RetryBankClient` - Wrapper that adds retry logic for 500 errors
- **Decorator Pattern** - You don't change HTTPBankClient, you wrap it

**Database Repositories:**
```
PaymentRepository
├─ Create(payment)
├─ Update(payment)
├─ FindByID(id)
├─ FindByOrderID(orderID)
└─ FindByCustomerID(customerID)

IdempotencyRepository
├─ AcquireLock(key, paymentID)
├─ ReleaseLock(key)
└─ FindByKey(key)
```

**Key Files:**
- `bank/client.go` - HTTP client to bank
- `bank/retry.go` - Retry wrapper
- `postgres/payment_repository.go` - Payment CRUD
- `postgres/idempotency_repository.go` - Idempotency key management

### 5. Background Workers (`internal/worker/`)

**What they do:** Fix payments that got stuck due to crashes/failures.

**RetryWorker (ELI5):**
```
Every 30 seconds:
1. Find payments stuck in CAPTURING/VOIDING/REFUNDING
2. For each stuck payment:
   - Call bank again with same idempotency key
   - Bank returns cached response (idempotent!)
   - Update payment to final state (CAPTURED/VOIDED/REFUNDED)
```

**Example Scenario:**
```
10:00 AM - Gateway calls bank to capture $50
10:00 AM - Bank approves and returns capture_id: "cap-123"
10:00 AM - Gateway CRASHES before saving to database
10:01 AM - Gateway restarts. Payment is stuck in "CAPTURING" state
10:01 AM - RetryWorker finds stuck payment
10:01 AM - Worker calls bank again with same idempotency key
10:01 AM - Bank says "I already processed this, here's cap-123"
10:01 AM - Worker updates payment to CAPTURED
```

**ExpirationWorker:**
- Finds AUTHORIZED payments older than 8 days
- Checks bank API: Is this still authorized?
- If bank says "expired" → mark payment as EXPIRED

**Key Files:**
- `retry_worker.go` - Recovers stuck payments
- `expiration_worker.go` - Marks expired authorizations

---

## Critical Design Patterns

### Pattern 1: Write-Ahead Log (WAL)

**Problem:** What if we crash after calling the bank but before saving the result?

**Solution:** Save payment in PENDING state BEFORE calling bank.

```go
// WRONG (old approach)
bankResp := bank.Authorize(card)  // If this succeeds...
db.Save(payment)                  // ...but THIS fails, we lost the authorization!

// CORRECT (WAL pattern)
db.Save(payment, status=PENDING)  // Always save first
bankResp := bank.Authorize(card)  // Then call bank
db.Update(payment, status=AUTHORIZED)  // Then update
```

### Pattern 2: Intermediate States

**Problem:** How do workers know what to retry?

**Solution:** Use intermediate states to signal intent.

```
AUTHORIZED → User clicks "Capture"
    ↓
CAPTURING (intent state - "we're trying to capture")
    ↓
CAPTURED (final state - "we successfully captured")

If we crash in CAPTURING, worker knows:
- We intended to capture
- We should retry the capture operation
```

### Pattern 3: Database-Level Idempotency

**Problem:** User clicks "Charge" button twice. How to prevent double charge?

**Solution:** Unique constraint on idempotency key table.

```sql
CREATE TABLE idempotency_keys (
    key TEXT PRIMARY KEY,  -- Unique constraint
    payment_id UUID,
    request_hash TEXT,
    locked_at TIMESTAMP
);
```

```go
// First request
key = "idem-abc"
db.Insert(key, payment_id)  // ✓ Success

// Second request (duplicate)
key = "idem-abc"
db.Insert(key, payment_id)  // ✗ Fails (unique constraint violation)
return cached_payment        // Return first result instead
```

### Pattern 4: Exponential Backoff

**Problem:** Bank is down. We retry 100 times immediately and overload it.

**Solution:** Wait longer between each retry.

```
Attempt 1: Wait 1 second
Attempt 2: Wait 2 seconds
Attempt 3: Wait 4 seconds
Attempt 4: Wait 8 seconds
...
```

### Pattern 5: Error Classification

**Problem:** Not all errors should be retried.

**Solution:** Categorize errors and only retry transient ones.

```go
// TRANSIENT (retry) - temporary issues
- 500 Internal Server Error
- Timeout errors
- Network failures

// PERMANENT (don't retry) - will never succeed
- 400 Invalid card
- 402 Insufficient funds
- 403 Card expired

// BUSINESS RULE (don't retry) - violates logic
- Invalid state transition (can't capture a voided payment)
- Amount mismatch
```

---

## Data Flow Examples

### Example 1: Successful Authorize → Capture

```
1. Client → POST /authorize
   {
     "order_id": "order-123",
     "amount": 5000,
     "card_number": "4111..."
   }

2. Gateway creates Payment in PENDING state
   payment = {
     id: "pay-abc",
     status: PENDING,
     amount: 5000
   }

3. Gateway calls Bank API
   POST /authorizations
   Response: {
     authorization_id: "auth-xyz",
     status: "AUTHORIZED"
   }

4. Gateway updates Payment to AUTHORIZED
   payment.status = AUTHORIZED
   payment.bank_auth_id = "auth-xyz"

5. Client → POST /capture
   {
     "payment_id": "pay-abc",
     "amount": 5000
   }

6. Gateway transitions Payment to CAPTURING
   payment.status = CAPTURING

7. Gateway calls Bank API
   POST /captures
   Response: {
     capture_id: "cap-123",
     status: "CAPTURED"
   }

8. Gateway updates Payment to CAPTURED
   payment.status = CAPTURED
   payment.bank_capture_id = "cap-123"
```

### Example 2: Crash During Capture (Recovery Scenario)

```
1. Payment is AUTHORIZED (auth_id: "auth-xyz")

2. Client → POST /capture

3. Gateway transitions to CAPTURING
   payment.status = CAPTURING

4. Gateway calls Bank API
   Response: {
     capture_id: "cap-123",
     status: "CAPTURED"
   }

5. *** GATEWAY CRASHES ***
   (Payment stuck in CAPTURING state)

6. RetryWorker runs (30s later)
   - Finds payment in CAPTURING state
   - Calls Bank API again with same idempotency key
   - Bank returns cached response: capture_id "cap-123"
   - Worker updates payment to CAPTURED

7. Final state:
   payment.status = CAPTURED
   payment.bank_capture_id = "cap-123"
```

---

## Database Schema

```sql
-- Payments table (core entity)
CREATE TABLE payments (
    id UUID PRIMARY KEY,
    order_id TEXT NOT NULL,
    customer_id TEXT NOT NULL,
    amount_cents BIGINT NOT NULL,
    currency TEXT DEFAULT 'USD',
    status TEXT NOT NULL,  -- State machine
    
    -- Bank identifiers
    bank_auth_id TEXT,
    bank_capture_id TEXT,
    bank_void_id TEXT,
    bank_refund_id TEXT,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL,
    authorized_at TIMESTAMP,
    captured_at TIMESTAMP,
    voided_at TIMESTAMP,
    refunded_at TIMESTAMP,
    expires_at TIMESTAMP,
    
    -- Retry metadata
    attempt_count INT DEFAULT 0,
    next_retry_at TIMESTAMP
);

-- Idempotency table (prevents duplicates)
CREATE TABLE idempotency_keys (
    key TEXT PRIMARY KEY,  -- Unique constraint
    payment_id UUID REFERENCES payments(id),
    request_hash TEXT NOT NULL,  -- Detects parameter changes
    locked_at TIMESTAMP,  -- Concurrency control
    response_payload JSONB  -- Cached response
);

-- Indexes for performance
CREATE INDEX idx_payments_order_id ON payments(order_id);
CREATE INDEX idx_payments_customer_id ON payments(customer_id);
CREATE INDEX idx_payments_retry_worker ON payments(next_retry_at) 
    WHERE status IN ('CAPTURING', 'VOIDING', 'REFUNDING');
```

---

## Configuration

All configuration uses environment variables with `GATEWAY_` prefix.

**Key Settings:**

```bash
# Server
GATEWAY_SERVER__PORT=8080
GATEWAY_SERVER__READ_TIMEOUT=15s

# Database
GATEWAY_DATABASE__HOST=localhost
GATEWAY_DATABASE__PORT=5432
GATEWAY_DATABASE__MAX_OPEN_CONNS=25

# Bank API
GATEWAY_BANK_CLIENT__BANK_BASE_URL=http://localhost:8787
GATEWAY_BANK_CLIENT__BANK_CONN_TIMEOUT=30s

# Retry Behavior
GATEWAY_RETRY__BASE_DELAY=1        # Initial delay (seconds)
GATEWAY_RETRY__MAX_RETRIES=3       # Max retry attempts
GATEWAY_RETRY__MAX_BACKOFF=10      # Max backoff time (minutes)

# Workers
GATEWAY_WORKER__INTERVAL=30s       # How often to check for stuck payments
GATEWAY_WORKER__BATCH_SIZE=100     # Max payments per cycle
```

---

## Security Considerations

1. **No Card Storage:** Card details never saved (PCI compliance)
2. **Idempotency:** Prevents accidental double charges
3. **Input Validation:** All requests validated at API layer
4. **Structured Logging:** Sensitive data never logged
5. **Timeout Protection:** All HTTP calls have timeouts

---

## Performance Characteristics

- **Authorization:** ~100-300ms (depends on bank API)
- **Capture/Void/Refund:** ~100-300ms
- **Worker Processing:** Batch of 100 payments in ~5-10s
- **Database:** Connection pool of 25, handles ~1000 req/s
- **Idempotency Check:** ~1-5ms (simple SELECT query)

---

## Testing Strategy

1. **Unit Tests:** Domain layer (state transitions)
2. **Integration Tests:** Services with real database (testcontainers)
3. **E2E Tests:** Full flow with mock bank
4. **Chaos Tests:** Simulated crashes, network failures

**Example Test Scenario:**
```go
// Test: Gateway crashes during capture
1. Create AUTHORIZED payment
2. Mock bank to return success
3. Kill goroutine after bank call
4. Verify payment stuck in CAPTURING
5. Run RetryWorker
6. Verify payment moved to CAPTURED
```

---

## Monitoring & Observability

**Structured Logging:**
```json
{
  "level": "info",
  "msg": "request completed",
  "method": "POST",
  "path": "/authorize",
  "status": 201,
  "duration_ms": 245
}
```

**Key Metrics to Track:**
- Payment success/failure rates by operation
- Average time to recover stuck payments
- Number of retries per payment
- Idempotency cache hit rate
- Worker batch processing time

---

## Deployment Architecture

```
┌─────────────────────────────────────────┐
│         Load Balancer (NGINX)           │
└────────────┬────────────────────────────┘
             │
    ┌────────┴────────┐
    │                 │
┌───▼────┐      ┌────▼────┐
│Gateway │      │Gateway  │  (Multiple instances)
│Instance│      │Instance │
└───┬────┘      └────┬────┘
    │                │
    └────────┬───────┘
             │
      ┌──────▼──────┐
      │ PostgreSQL  │
      │  (Primary)  │
      └─────────────┘
```

**Why this works:**
- Stateless instances (all state in database)
- Workers use `FOR UPDATE SKIP LOCKED` (no conflicts)
- Idempotency prevents duplicate processing

---

## Failure Scenarios & Recovery

### Scenario 1: Bank Returns 500 Error

```
Flow:
1. Save payment as PENDING
2. Call bank → 500 error
3. Payment stays PENDING
4. NOT retried automatically (we don't have card details)
5. Marked FAILED after 10 minutes

Why: We don't store card details, so we can't retry authorization
```

### Scenario 2: Gateway Crashes During Capture

```
Flow:
1. Payment transitions to CAPTURING
2. Bank responds with success
3. Gateway crashes before saving
4. RetryWorker finds payment in CAPTURING
5. Calls bank with same idempotency key
6. Bank returns cached success
7. Payment updated to CAPTURED

Why: Idempotency ensures bank doesn't double-charge
```

### Scenario 3: Database Temporarily Down

```
Flow:
1. HTTP request arrives
2. Cannot connect to database
3. Return 500 error to client
4. Client retries with same idempotency key
5. Database back online
6. Request succeeds

Why: No state persisted, so retry is safe
```

---

## Future Improvements

1. **Event Sourcing:** Store every state transition as event
2. **Separate Operation Intent:** Track intent in separate table
3. **Redis for Idempotency:** Sub-millisecond lookups
4. **Partial Captures/Refunds:** Support partial amounts
5. **Multi-Currency:** Handle currencies beyond USD
6. **Webhooks:** Notify FicMart of state changes