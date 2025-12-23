# Handler Test Definitions: HandleAuthorize

## Overview
These test cases define the behavior of the `HandleAuthorize` HTTP handler. They follow the project's DSL-like structure (Setup, Actions, Assertions).

---

## Test Cases

// 1. Successful Authorization
//
// serverIsRunning
// authServiceIsMocked
// validAuthorizeRequestProvided
//
// requestIsSentToAuthorizeEndpoint
//
// expectHTTPStatusCreated
// expectPaymentObjectInResponse
// expectIdempotencyKeyRespected

// 2. Duplicate Request (Polling Success)
//
// serverIsRunning
// authServiceIsMockedToPollSuccess
// duplicateAuthorizeRequestProvided
//
// requestIsSentToAuthorizeEndpoint
//
// expectHTTPStatusCreated
// expectSamePaymentObjectInResponse

// 3. Duplicate Request (Still Processing)
//
// serverIsRunning
// authServiceIsMockedToReturnProcessingError
// duplicateAuthorizeRequestProvided
//
// requestIsSentToAuthorizeEndpoint
//
// expectHTTPStatusAccepted
// expectProcessingErrorCode

// 4. Idempotency Key Mismatch
//
// serverIsRunning
// authServiceIsMockedToReturnMismatchError
// mismatchingAuthorizeRequestProvided
//
// requestIsSentToAuthorizeEndpoint
//
// expectHTTPStatusConflict
// expectMismatchErrorCode

// 5. Validation Error (Missing Field)
//
// serverIsRunning
// authServiceIsMocked
// invalidAuthorizeRequestMissingAmount
//
// requestIsSentToAuthorizeEndpoint
//
// expectHTTPStatusBadRequest
// expectValidationErrorCode

---

## Required DSL Functions

### Setup Functions
- `serverIsRunning`: Initializes the HTTP handler with its dependencies.
- `authServiceIsMocked`: Provides a standard mock for `AuthorizationService`.
- `authServiceIsMockedToPollSuccess`: Mocks the service to simulate a successful poll for an existing request.
- `authServiceIsMockedToReturnProcessingError`: Mocks the service to return `ErrRequestProcessing`.
- `authServiceIsMockedToReturnMismatchError`: Mocks the service to return `ErrCodeIdempotencyMismatch`.
- `validAuthorizeRequestProvided`: Sets up a valid `AuthorizeRequest` body and `Idempotency-Key` header.
- `duplicateAuthorizeRequestProvided`: Sets up a request body with an already-used idempotency key.
- `invalidAuthorizeRequestMissingAmount`: Sets up a request body with a missing `amount` field.

### Action Functions
- `requestIsSentToAuthorizeEndpoint`: Performs the `POST /authorize` request using `httptest`.

### Assertion Functions
- `expectHTTPStatusCreated`: Asserts the response code is `201 Created`.
- `expectHTTPStatusAccepted`: Asserts the response code is `202 Accepted`.
- `expectHTTPStatusConflict`: Asserts the response code is `409 Conflict`.
- `expectHTTPStatusBadRequest`: Asserts the response code is `400 Bad Request`.
- `expectPaymentObjectInResponse`: Verifies the response body contains the expected `Payment` JSON.
- `expectProcessingErrorCode`: Verifies the error code is `REQUEST_PROCESSING`.
- `expectMismatchErrorCode`: Verifies the error code is `IDEMPOTENCY_MISMATCH`.
- `expectValidationErrorCode`: Verifies the error code is `VALIDATION_ERROR`.
- `expectIdempotencyKeyRespected`: Verifies the service was called with the correct idempotency key from header/body.
