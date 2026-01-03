# TRADEOFFS.md

## Architecture

### Why did I structure it this way?

Since the project is domain-driven, I researched DDD architecture in Go and this is the standard template across board with clean separation of concerns. It's readable and easy to follow


### Gateway-Owned State

I'm storing payment state in my own database instead of always querying the bank. The database acts as my state machine and helps me enforce valid state transitions. This is part of the "smart assistant" role - invalid state transitions aren't sent to the bank. The tradeoff is that I must keep my state synchronized with the bank's state, which requires careful handling of failures and a reconciliation strategy.

---

## State Management


### How I track payment states:

I save the payment to the database with something to show the intent of the action before calling the bank(e.g of intent - `CAPTURING`, `VOIDING`) as result solving the crash scenario of my service crashing before saving the bank's response. Without this I'd have lost the bank's response forever and FicMart wouldn't be able to proceed with that payment.
I also used background workers for reonciling states. The background worker checks for stuck `PENDING` payments and reconciles them with the bank's state. The worker uses `FOR UPDATE SKIP LOCKED` to prevent multiple worker instances from fighting over the same payment.

### Lazy Expiration with Grace Period

I mark authorizations as `EXPIRED` after 7 days with a 1-hour grace period, but only through the background worker after confirming with the bank. **I do not enforce strict rejection at my API Gateway** When FicMart tries to capture a payment that appears close to expiration based on local timestamps, I attempt the capture anyway and let the bank be the source of truth for the rejection. It is better to waste an API call than to block a valid payment due to clock skew. The background worker proactively marks obviously expired payments (8+ days old) to improve query performance, but API handlers defer to the bank for edge cases.

---

## Idempotency

### How did I implement it?

I made it that each POST request from FicMart to the gateway requires an idempotency key in the request header. This key is in turn stored and sent to the bank. The key have a `UNIQUE` constraint at the db level which ensures no duplicate keys are stored, and on concurrent requests with the same key, the second request polls the db waiting for the first request to complete then returns the same response.

### Edge cases I considered:
- Two requests, request A and B, reaching my gateway almost at the same time with the same credentials
- When my gateway sends request to the bank api, and I didn't save the idempotency key from the Ficmart request, if the bank crashes or my gateway crashes before getting a response, I'd have lost the payment info which is could lead to customer's being double charged and nobody wants that.


---

## Failure Handling

### Retry strategy:

For network timeouts, I retry aggressively to verify the bank's response and retrieve it.

For 5xx errors, the bank explicitly told me "I failed, nothing happened." I use exponential backoff with jitter and stop after 3 attempts before marking the payment as FAILED.

I don't retry 4xx errors (like invalid card or insufficient funds) at all since they're client errors


### How I handled partial failures:

Firstly I record intent before calling bank as I'v mentioned earlier, then I use background workers to poll the database at intervals for payments stuck in intermediate states, which when found, the worker proceeds to call the bank to verify the state of the stuck payments.

---

## What I'd Do Differently In Production:


## State Management

I'd represent only states exposed to FicMart in my domain layer; Then I'd add a `operation_type` column to the `idempotency_keys` table to separate operation intent from payment state

## Others

I'd add something like a payment_events or payment_lifecycle table to show transitions of various payments from one state to the other. This is particularly useful cos in my project, I mark pending payments that timed out as failed so if a authorize call was made to the bank for a new payment and my gateway for any reason didn't receive the response of `authorized`, I can show that this payment went from pending -> failed and if the bank authorized it, I can use it to tell the bank to release the customer's funds.



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

I'd move the idempotency table to Redis for faster reads since reads
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