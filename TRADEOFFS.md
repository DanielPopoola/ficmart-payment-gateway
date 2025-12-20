# TRADEOFFS.md

## Architecture

### Why did I structure it this way?

From my FastAPI experience, I try to ensure clean separation of concerns and I looked at other codebases for examples. For the adapter it's tailored to PostgreSQL because of the driver I used(pgx), then for the handler, I made the http layer own its request types - because it matches Go convention.

### Gateway-Owned State

I'm storing payment state in my own database instead of always querying the bank. The database acts as my state machine and helps me enforce valid state transitions. This is part of the "smart assistant" role - invalid state transitions aren't sent to the bank. The tradeoff is that I must keep my state synchronized with the bank's state, which requires careful handling of failures and a reconciliation strategy.

---

## State Management

### Write-Ahead Pattern: Save PENDING Before Calling Bank

I save the payment to the database with `status=PENDING` before calling the bank. This solves the critical crash scenario: if my system crashes before receiving the bank's response, I can retry with the same idempotency key. Without this, I'd have lost the bank's response forever and FicMart wouldn't be able to proceed with that payment. The PENDING record is my "intent log" that survives crashes.

**Critical Implementation Detail:** I commit the PENDING state to the database *before* calling the bank, then release the database connection. The bank call happens outside any database transaction to avoid holding connections during long network operations. This creates a window where payments can be stuck in PENDING if the app crashes, which is why the reconciliation worker is essential.

### Background Worker for Stuck Payments

The background worker checks for stuck `PENDING` payments and reconciles them with the bank's state. This is necessary because data inconsistency between the bank and my gateway can occur during crashes or network failures. The bank and gateway should always agree, so when the gateway sees pending payments older than 1 minute, it verifies their actual state with the bank and updates accordingly. The worker uses `FOR UPDATE SKIP LOCKED` to prevent multiple worker instances from fighting over the same payment.

### Lazy Expiration with Grace Period

I mark authorizations as `EXPIRED` after 7 days with a 1-hour grace period, but only through the background worker after confirming with the bank. **I do not enforce strict rejection at my API Gateway** When FicMart tries to capture a payment that appears close to expiration based on local timestamps, I attempt the capture anyway and let the bank be the source of truth for the rejection. It is better to waste an API call than to block a valid payment due to clock skew. The background worker proactively marks obviously expired payments (8+ days old) to improve query performance, but API handlers defer to the bank for edge cases.

---

## Idempotency

### UNIQUE Constraint + INSERT ON CONFLICT

I use `INSERT ... ON CONFLICT DO NOTHING` with a UNIQUE constraint on the idempotency key. This prevents race conditions when two requests with the same idempotency key try to insert at the same time. The database atomically rejects the duplicate, and only one request proceeds to call the bank. This is enforced at the database level, not application level.

### Polling for Duplicate Requests with Timeout

When a duplicate request arrives (same idempotency key), Request B polls the database waiting for Request A's response. The polling implementation respects `ctx.Done()` so if the client disconnects, we stop polling immediately. There's a hard timeout of 5 seconds - if Request A hasn't completed by then, Request B returns a 503 Service Unavailable rather than hanging indefinitely. The polling interval is 200ms to balance responsiveness with database load.


---

## Failure Handling

### Network Timeouts: Aggressive Retry

I retry aggressively for network timeouts to verify the bank's response and retrieve it. Timeouts are ambiguous - the bank might have successfully processed the request, but the response was lost. By retrying with the same idempotency key, I can safely get the cached response without risking double-capture or double-holding of funds. The idempotency key makes aggressive retries safe.

### Bank 5xx Errors: Backoff Strategy

For 5xx errors, the bank explicitly told me "I failed, nothing happened." I use exponential backoff with jitter and stop after 3 attempts before marking the payment as FAILED. This is standard practice for transient failures - give the bank time to recover without overwhelming it.

### No Retry on 4xx Errors

I don't retry 4xx errors (like invalid card or insufficient funds) at all. These are permanent business logic errors that won't be fixed by retrying. Retrying would waste resources and isn't standard practice.

### 24-Hour Worker Cutoff

The worker stops retrying stuck payments after 24 hours. This is based on the bank's idempotency cache TTL. After the idempotency window closes, retrying risks creating a duplicate authorization if the bank doesn't remember the original request. Beyond this window, human intervention is safer than automated recovery.


### How I handled partial failures?

I used an executor interface and a method (a closure really - it takes a function and executes it within a database transaction (WithTx in internal/adapters/postgres/repository.go).  This was to prevent situations like I'll save to my idempotency table but my db crashes before saving to the payments table, so they are wrapped in a transaction like Django's transaction.atomic() either both succeed or not
---

## What I'd Do Differently

### Observability

If this were going to production, I'd add comprehensive monitoring:
- **p50, p95, p99 latency** for all bank API calls to detect degradation
- **Retry attempt distributions** to understand failure patterns
- **State transition metrics** to track how long payments spend in each state
- **Alerting** for payments stuck in PENDING beyond expected thresholds
- **Database connection pool metrics** to detect exhaustion from long transactions

### Testing Strategy

For this implementation, I must include:
- **Concurrent double-spend test:** Two goroutines simultaneously try to capture the same payment; exactly one succeeds
- **Idempotency verification:** Same request with same key returns cached response
- **Crash simulation:** Insert PENDING, kill process, verify worker recovers state

Future additions would include:
- **Chaos testing** with network failures injected at specific points
- **Time-based tests** to verify expiration logic works correctly
- **Contract tests** against the actual bank API

### Performance Optimization

I'd move the idempotency table to Redis for faster reads since they are temporary

---

## Biggest Limitation

The biggest risk in my current design is **data inconsistency**. If my gateway's state diverges from the bank's state (due to bugs in the reconciliation worker, clock skew exceeding the grace period, or database connection pool exhaustion causing failed updates), it could lead to:
- FicMart shipping goods on an expired authorization
- Double-charging customers if the worker retries outside the idempotency window
- Lost money if a valid authorization is incorrectly marked as expired

The reconciliation worker is critical, and any bugs in that component have direct financial impact. The "commit first, then call bank" pattern creates a deliberate window of inconsistency that the worker must resolve. This is an acceptable tradeoff because holding database connections during bank calls would exhaust the connection pool and cause cascading failures under load.

---

## Key Implementation Constraints

1. **Never hold database transactions during network calls** - Always commit the intent, call external APIs, then update in a new transaction
2. **Respect context cancellation** - All operations must check `ctx.Done()`
3. **Database-level uniqueness enforcement** - Idempotency is enforced by UNIQUE constraints, not application logic
4. **Bank is source of truth for edge cases** - Don't reject requests based purely on local state near expiration boundaries
5. **Required test coverage** - Must prove idempotency works under concurrent load