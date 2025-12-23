# Acceptance Test Definitions

## Overview
These test cases are designed to verify the core resilience and consistency requirements of the Payment Gateway, specifically focusing on concurrency, idempotency, and crash recovery.

---

## Test Cases

// 1. Concurrent Double-Spend Prevention
//
// databaseHasAuthorizedPayment
// bankIsSlowToRespond
// twoSimultaneousCaptureRequestsArriveWithSameKey
//
// triggerConcurrentCaptures
//
// expectExactlyOneBankCallMade
// expectBothRequestsToReturnSameSuccessfulResponse
// expectPaymentStatusToBeCaptured

// 2. High-Fidelity Idempotency Verification
//
// databaseHasCapturedPaymentWithCachedResponse
// duplicateCaptureRequestArrivesWithSameKey
//
// sendDuplicateRequest
//
// expectNoBankCallMade
// expectResponseToMatchCachedPayload
// expectHttpStatusToBeOK

// 3. Crash Recovery (Zombie PENDING)
//
// databaseHasStuckPendingPaymentOlderThanOneMinute
// bankHasRecordOfSuccessfulAuthorization
// backgroundWorkerIsTriggered
//
// runReconciler
//
// expectWorkerToQueryBankWithIdempotencyKey
// expectPaymentStatusUpdatedToAuthorized
// expectIdempotencyKeyMarkedAsCompleted

// 4. Lazy Expiration Confirmation
//
// databaseHasAuthorizedPaymentPastExpiryDate
// bankConfirmsAuthorizationIsExpired
// backgroundWorkerIsTriggered
//
// runReconciler
//
// expectWorkerToCallBankGetAuthorization
// expectPaymentStatusUpdatedToExpired

// 5. Idempotency Key Mismatch Rejection
//
// databaseHasExistingIdempotencyKeyRecord
// newRequestArrivesWithSameKeyButDifferentAmount
//
// sendMismatchingRequest
//
// expectNoBankCallMade
// expectHttpStatusToBadRequest
// expectIdempotencyMismatchErrorCode

---

## Required DSL Functions

### Setup Functions
- `databaseHasAuthorizedPayment`: Seeds the database with a payment in `AUTHORIZED` status.
- `databaseHasCapturedPaymentWithCachedResponse`: Seeds both `payments` and `idempotency_keys` (with `response_payload`) tables.
- `databaseHasStuckPendingPaymentOlderThanOneMinute`: Seeds a payment in `PENDING` status with a `created_at` timestamp in the past.
- `databaseHasAuthorizedPaymentPastExpiryDate`: Seeds a payment where `expires_at` is in the past.
- `bankIsSlowToRespond`: Configures the mock bank to delay responses to simulate race conditions.
- `bankHasRecordOfSuccessfulAuthorization`: Mocks the bank's `GET` or `POST` (idempotent) to return a success for a given key.
- `bankConfirmsAuthorizationIsExpired`: Mocks the bank's `GET /authorizations/{id}` to return an expired status or 404.
- `twoSimultaneousCaptureRequestsArriveWithSameKey`: Prepares two goroutines with identical `Capture` payloads and `Idempotency-Key` headers.

### Action Functions
- `triggerConcurrentCaptures`: Executes multiple goroutines calling the handler or service simultaneously.
- `sendDuplicateRequest`: Sends a second request with an identical idempotency key.
- `sendMismatchingRequest`: Sends a request with an existing key but different payload.
- `runReconciler`: Invokes the `worker.Reconciler.run` method manually.

### Assertion Functions
- `expectExactlyOneBankCallMade`: Verifies the bank mock was only invoked once despite multiple incoming requests.
- `expectBothRequestsToReturnSameSuccessfulResponse`: Verifies that both concurrent callers received the same `200 OK` and data.
- `expectPaymentStatusToBeCaptured`: Verifies the final state in the database.
- `expectNoBankCallMade`: Verifies the bank mock was never touched (result came from cache).
- `expectResponseToMatchCachedPayload`: Verifies the response body is identical to what was stored in `idempotency_keys`.
- `expectWorkerToQueryBankWithIdempotencyKey`: Verifies the worker used the correct recovery path.
- `expectIdempotencyKeyMarkedAsCompleted`: Verifies the `completed_at` column is now set.
