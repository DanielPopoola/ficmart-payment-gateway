# Payment Gateway Design & Tradeoffs

## 1. Architecture: Why structure it this way?
I implemented this gateway using **Domain-Driven Design (DDD)** principles, organizing the code into four distinct layers: domain, application, infrastructure, and interfaces. 

*   **Dependency Isolation:** The domain layer is the core of the system and has zero dependencies. This ensures that business rules (like state transitions) remain stable and are never coupled to external APIs or database schemas.
*   **Service Granularity:** Instead of a monolithic `PaymentService`, I split operations into dedicated services (`AuthorizeService`, `CaptureService`, etc.). This makes mapping services to HTTP handlers easy.

*   **Testability via Abstractions:** By using interfaces like `BankClient`, I can mock the bank during unit tests. Similarly, separating the REST interface ensures I could swap HTTP for gRPC without modifying business logic.
*   **Tradeoff:** I consciously chose a higher degree of initial complexity (more files and indirection) to gain long-term flexibility and testability.

## 2. State Management: How and why?
I use a **Write-Ahead Pattern** and a robust state machine to track payments.

*   **Intent-First Persistence:** Every payment is saved as `PENDING` before the bank is called. This ensures that if the system crashes mid-call, we have a record to reconcile later.
*   **Intermediate States:** I use states like `CAPTURING`, `VOIDING`, and `REFUNDING`. These act as "signals of intent." If a background worker finds a payment stuck in `CAPTURING`, it knows exactly which operation to retry.
*   **Domain-Level Enforcement:** State transitions are guarded within `domain/payment.go`. This prevents invalid business flows (e.g., voiding a captured payment) from ever reaching the bank.
*   **Source of Truth:** While the gateway tracks state, the bank is the ultimate authority. I implement a **Lazy Expiration** policy with a 48-hour grace period (marking auths expired at 9 days instead of the bank’s 7) to account for distributed clock skew while still checking the bank API before a final state change.

## 3. Failure Handling: Retries and Partial Failures
My strategy focuses on **Error Classification** to avoid "poison pill" retries.

*   **Categorization:** Errors are split into **Transient** (500s/timeouts), **Permanent** (400s/insufficient funds), and **Business Rules** (invalid transitions). 
*   **Retry Logic:** I only retry transient errors using **Exponential Backoff with Jitter** (1s → 2s → 4s). This prevents a "thundering herd" effect on the bank's API.
*   **Handling Partial Failures:** If the gateway crashes after a bank call but before updating our DB, background workers handle recovery:
    *   **Scenario (Capture/Void):** The worker finds the stuck intermediate state and retries the operation. Because of idempotency, the bank returns the cached success, and our DB eventually syncs.
    *   **Scenario (Authorize):** Since card details aren't stored, we cannot retry authorizations. These are marked `FAILED` for manual reconciliation to prevent holding customer funds indefinitely.
*   **Efficient Processing:** Workers use `FOR UPDATE SKIP LOCKED` to allow multiple instances to process stuck payments concurrently without blocking each other.

## 4. Idempotency: Implementation and Edge Cases
Idempotency is enforced at the database level using a dedicated `idempotency_keys` table in PostgreSQL.

*   **Mechanism:** I store a `request_hash` (SHA256) alongside the key. This allows the system to detect if a client reuses a key with different request parameters—a critical edge case that triggers an `IDEMPOTENCY_MISMATCH` error.
*   **Concurrency Control:** A `locked_at` column manages concurrent requests. If a second request arrives while the first is processing, it enters a `waitForCompletion()` loop, polling every 100ms until the result is ready or it times out.
*   **Persistence:** Unlike Redis, using PostgreSQL ensures idempotency data survives system restarts. I cache both successes and failures indefinitely, as idempotency keys should never be recycled for different operations.

## 5. What I'd Do Differently in Production
With more time, I would address the following limitations:

*   **Separate Operation Intent:** Currently, the payment state encodes intent (e.g., `CAPTURING`). In production, I’d add an `operation_type` column to the idempotency table. This would keep the domain state clean (only `AUTHORIZED`, `CAPTURED`) while explicitly tracking what the worker needs to do.
*   **Event Sourcing:** I would move to a `payment_events` table. Storing every transition (e.g., `PENDING` -> `CAPTURING` -> `CAPTURED`) provides a full audit trail and makes it easier to debug "orphaned" authorizations where the bank says "Yes" but our gateway marked "Failed" due to timeout.
*   **Infrastructure Optimization:** For a high-scale environment, I’d move idempotency lookups to **Redis** for sub-millisecond latency, keeping PostgreSQL as a durable fallback.
*   **Advanced Chaos Testing:** I would implement failure-injection testing to simulate crashes at the exact millisecond between the bank response and the database commit to further harden the recovery workers.