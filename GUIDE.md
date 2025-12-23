# Payment Gateway Implementation Guide

This document summarizes your design decisions and provides a coding roadmap. Reference this while implementing to stay aligned with your architecture.

---

## Core Design Principles

### 1. The Gateway's Role
**Smart Assistant, Not Dumb Proxy**
- Validate state transitions before calling the bank
- Prevent wasted API calls by checking local state first
- Return meaningful errors to FicMart
- **Exception:** Near expiration boundaries, defer to bank as source of truth

### 2. State Management Strategy
**Write-Ahead + Reconciliation**
- Save intent (`PENDING`) before making bank calls
- Commit to DB, release connection, then call bank
- Use background worker to fix "zombie" PENDING states
- Never hold DB transactions during network I/O

### 3. Concurrency Safety
**Database-Level Enforcement**
- UNIQUE constraint on idempotency keys
- `INSERT ... ON CONFLICT DO NOTHING`
- `FOR UPDATE SKIP LOCKED` in worker queries
- No application-level locking

---

## Database Schema

### Payments Table
```sql
CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id VARCHAR(255) NOT NULL,
    customer_id VARCHAR(255) NOT NULL,
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status VARCHAR(50) NOT NULL,
    
    -- Bank reference IDs
    bank_auth_id VARCHAR(255),
    bank_capture_id VARCHAR(255),
    bank_void_id VARCHAR(255),
    bank_refund_id VARCHAR(255),
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    authorized_at TIMESTAMP,
    captured_at TIMESTAMP,
    voided_at TIMESTAMP,
    refunded_at TIMESTAMP,
    expires_at TIMESTAMP,
    
    -- Retry tracking
    attempt_count INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMP,
    last_error_category VARCHAR(50),
    
    -- Indexes for common queries
    INDEX idx_order_id (order_id),
    INDEX idx_customer_id (customer_id),
    INDEX idx_status_created (status, created_at)
);
```

### Idempotency Keys Table
```sql
CREATE TABLE idempotency_keys (
    key VARCHAR(255) PRIMARY KEY,
    request_payload JSONB NOT NULL,
    response_payload JSONB,
    status_code INT,
    locked_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP
);
```

**Key Points:**
- `response_payload` is NULL until request completes
- Duplicate requests poll for `response_payload IS NOT NULL`
- Store full bank response for debugging

---

## State Machine

### Valid States
```
PENDING      → Initial state, intent recorded
AUTHORIZED   → Bank approved, funds held
CAPTURED     → Funds charged
VOIDED       → Authorization cancelled
REFUNDED     → Money returned after capture
FAILED       → Permanent failure (4xx errors)
EXPIRED      → Authorization too old (7+ days)
```

### State Transition Rules

**Domain Logic (enforce in code):**
```go
func (p *Payment) CanTransitionTo(target Status) error {
    transitions := map[Status][]Status{
        StatusPending:    {StatusAuthorized, StatusFailed},
        StatusAuthorized: {StatusCaptured, StatusVoided, StatusExpired},
        StatusCaptured:   {StatusRefunded},
        StatusVoided:     {}, // terminal
        StatusRefunded:   {}, // terminal
        StatusFailed:     {}, // terminal
        StatusExpired:    {}, // terminal
    }
    
    allowed := transitions[p.Status]
    for _, valid := range allowed {
        if valid == target {
            return nil
        }
    }
    return fmt.Errorf("cannot transition from %s to %s", p.Status, target)
}
```

---

## API Flow Patterns

### Pattern 1: Authorize Flow
```
1. FicMart → POST /authorize (with Idempotency-Key header)
2. Gateway checks idempotency table
   - If key exists and complete: Return cached response
   - If key exists and incomplete: Poll for completion (5s timeout)
   - If key new: Proceed
3. Gateway: INSERT INTO idempotency_keys (key, request_payload)
   - ON CONFLICT: Go to step 2
4. Gateway: INSERT INTO payments (status=PENDING, ...)
5. Gateway: COMMIT transaction
6. Gateway: Call bank API (retries on timeout/5xx)
7. Gateway: UPDATE payments SET status=AUTHORIZED/FAILED, ...
8. Gateway: UPDATE idempotency_keys SET response_payload=..., status_code=...
9. Gateway: Return response to FicMart
```

**Critical:** Steps 4-5 commit before step 6 (bank call)

### Pattern 2: Capture/Void/Refund Flow
```
1. FicMart → POST /capture (with payment_id)
2. Gateway: SELECT * FROM payments WHERE id=... FOR UPDATE
3. Gateway: Validate current state allows transition
   - If near expiration: Attempt anyway, let bank reject
   - If clearly invalid: Return 400 error immediately
4. Gateway: UPDATE payments SET status=CAPTURING (optional intermediate state)
5. Gateway: COMMIT transaction
6. Gateway: Call bank API
7. Gateway: UPDATE payments SET status=CAPTURED/FAILED, ...
8. Gateway: Return response
```

---

## Error Handling Strategy

### Network Timeouts
```go
// Aggressive retry with same idempotency key
maxAttempts := 5
for attempt := 1; attempt <= maxAttempts; attempt++ {
    response, err := callBank(ctx, idempotencyKey)
    if err == nil {
        return response
    }
    
    if isTimeout(err) {
        time.Sleep(exponentialBackoff(attempt))
        continue
    }
    
    return err // Non-timeout error, don't retry
}
```

**Rationale:** Same idempotency key prevents double-charge

### Bank 5xx Errors
```go
// Backoff with jitter
maxAttempts := 3
for attempt := 1; attempt <= maxAttempts; attempt++ {
    response, err := callBank(ctx, idempotencyKey)
    if err == nil {
        return response
    }
    
    if is5xx(err) && attempt < maxAttempts {
        jitter := rand.Intn(100)
        delay := time.Duration(math.Pow(2, float64(attempt))) * time.Second
        time.Sleep(delay + time.Duration(jitter)*time.Millisecond)
        continue
    }
    
    return err
}
```

### Bank 4xx Errors
```go
// No retry, immediate failure
response, err := callBank(ctx, idempotencyKey)
if is4xx(err) {
    // Update payment status to FAILED
    return err // Return to FicMart immediately
}
```

---

## Background Worker Logic

### Worker Query
```sql
SELECT * FROM payments
WHERE status = 'PENDING'
  AND created_at < NOW() - INTERVAL '1 minute'
  AND (next_retry_at IS NULL OR next_retry_at < NOW())
  AND created_at > NOW() - INTERVAL '24 hours'
FOR UPDATE SKIP LOCKED
LIMIT 100;
```

**Key Points:**
- Only process payments > 1 minute old (give main handler time)
- Respect `next_retry_at` for backoff
- Stop after 24 hours (idempotency window)
- `SKIP LOCKED` prevents worker contention

### Worker Actions
```go
for _, payment := range stuckPayments {
    // Retrieve original idempotency key from payment
    idempotencyKey := payment.IdempotencyKey
    
    // Retry with same key
    response, err := callBank(ctx, idempotencyKey)
    
    if err != nil {
        // Update retry metadata
        payment.AttemptCount++
        payment.NextRetryAt = calculateNextRetry(payment.AttemptCount)
        payment.LastErrorCategory = categorizeError(err)
        continue
    }
    
    // Success! Update payment
    payment.Status = response.Status
    payment.BankAuthID = response.AuthorizationID
    payment.AuthorizedAt = time.Now()
}
```

### Expiration Checker (Separate Worker)
```sql
SELECT * FROM payments
WHERE status = 'AUTHORIZED'
  AND created_at < NOW() - INTERVAL '8 days'
LIMIT 100;
```

Mark as `EXPIRED` after confirming with bank or if significantly past window.

---

## Context Handling Rules

### Every Function Takes Context
```go
// Handlers
func (h *Handler) Authorize(ctx context.Context, req AuthorizeRequest) (Response, error)

// Services
func (s *Service) Authorize(ctx context.Context, ...) (*Payment, error)

// Repositories
func (r *Repo) Create(ctx context.Context, payment *Payment) error

// Bank Client
func (c *BankClient) Authorize(ctx context.Context, req BankRequest) (BankResponse, error)
```

### Polling with Context
```go
func waitForResponse(ctx context.Context, key string) (*Response, error) {
    timeout := time.After(5 * time.Second)
    ticker := time.NewTicker(200 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return nil, ctx.Err() // Client disconnected
        case <-timeout:
            return nil, ErrTimeout
        case <-ticker.C:
            resp, err := checkDB(ctx, key)
            if err == nil && resp.IsComplete() {
                return resp, nil
            }
        }
    }
}
```

---

## Required Tests

### 1. Concurrent Idempotency Test
```go
func TestConcurrentAuthorize(t *testing.T) {
    var wg sync.WaitGroup
    results := make(chan Result, 2)
    
    idempotencyKey := "test-key-123"
    
    // Launch 2 concurrent requests
    for i := 0; i < 2; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            resp, err := client.Authorize(ctx, req, idempotencyKey)
            results <- Result{resp, err}
        }()
    }
    
    wg.Wait()
    close(results)
    
    // Collect results
    var responses []Result
    for r := range results {
        responses = append(responses, r)
    }
    
    // Both should succeed with identical responses
    assert.Equal(t, 2, len(responses))
    assert.Equal(t, responses[0].ID, responses[1].ID)
}
```

### 2. Worker Recovery Test
```go
func TestWorkerRecoversStuckPayment(t *testing.T) {
    // Insert PENDING payment directly
    payment := &Payment{Status: StatusPending, CreatedAt: time.Now().Add(-2 * time.Minute)}
    repo.Create(ctx, payment)
    
    // Simulate bank will succeed on retry
    mockBank.On("Authorize", ...).Return(successResponse, nil)
    
    // Run worker
    worker.ProcessStuckPayments(ctx)
    
    // Verify payment updated
    updated, _ := repo.FindByID(ctx, payment.ID)
    assert.Equal(t, StatusAuthorized, updated.Status)
}
```

### 3. Expiration Boundary Test
```go
func TestCaptureNearExpiration(t *testing.T) {
    // Payment authorized 6 days 23 hours ago (within grace period)
    payment := &Payment{
        Status: StatusAuthorized,
        CreatedAt: time.Now().Add(-167 * time.Hour),
    }
    
    // Should attempt capture, let bank decide
    mockBank.On("Capture", ...).Return(successResponse, nil)
    
    resp, err := handler.Capture(ctx, payment.ID)
    
    assert.NoError(t, err)
    assert.Equal(t, StatusCaptured, resp.Status)
}
```

---

## Project Structure

```
/cmd/gateway/
    main.go                 # Entry point, wire dependencies

/internal/
    /core/
        /domain/
            payment.go      # Payment struct, state machine
            errors.go       # Custom error types
        /ports/
            bank.go         # BankClient interface
            repository.go   # PaymentRepository interface
    
    /service/
        authorize.go        # Business logic for authorize
        capture.go          # Business logic for capture
        void.go
        refund.go
    
    /adapters/
        /handler/
            types.go              ← AuthorizeRequest, CaptureRequest, etc.
            authorization_handler.go
            http.go         # HTTP handlers (Gin/Chi)
            middleware.go   # Idempotency middleware
        /repo/
            postgres.go     # PostgreSQL implementation
        /bank/
            client.go       # Mock bank client
    
    /worker/
        reconciler.go       # Background worker

/migrations/
    001_create_payments.sql
    002_create_idempotency_keys.sql

/tests/
    integration_test.go
    concurrency_test.go
```

---

## Implementation Checklist

### Phase 1: Foundation
- [ ] Database schema and migrations
- [ ] Payment domain model with state machine
- [ ] Repository interface and PostgreSQL implementation
- [ ] Bank client with retry logic

### Phase 2: Core Operations
- [ ] POST /authorize endpoint
- [ ] Idempotency middleware
- [ ] POST /capture endpoint
- [ ] POST /void endpoint
- [ ] POST /refund endpoint

### Phase 3: Resilience
- [ ] Background worker for stuck payments
- [ ] Expiration checker worker
- [ ] Context cancellation in all operations
- [ ] Error categorization and retry logic

### Phase 4: Testing
- [ ] Concurrent idempotency test
- [ ] Worker recovery test
- [ ] Expiration boundary test
- [ ] Integration tests with real DB and mock bank

---

## Key Reminders While Coding

1. **Never hold DB transactions during bank calls**
2. **Always check `ctx.Done()` in loops**
3. **Use `FOR UPDATE SKIP LOCKED` in worker**
4. **Store full bank responses in idempotency table**
5. **Let bank reject near-expiration captures**
6. **Same idempotency key for all retries**
7. **4xx = no retry, 5xx = backoff, timeout = aggressive**

---

## Success Criteria

Your implementation passes if:
- ✅ Concurrent requests with same key return identical responses
- ✅ Worker recovers payments stuck in PENDING
- ✅ No database connections held during bank calls
- ✅ Context cancellation works throughout
- ✅ State machine prevents invalid transitions
- ✅ Bank is source of truth for expiration edge cases