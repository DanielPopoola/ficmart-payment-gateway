# Authorize Handler Implementation Plan

## Overview
Implement the `HandleAuthorize` HTTP handler in `internal/adapters/handler/authorize.go`. This handler will be the primary entry point for FicMart to initiate payment authorizations. It will handle request decoding, validation, service invocation, and error/success response mapping, ensuring idempotency is respected.

## Current State Analysis
- `AuthorizeRequest` struct exists with basic validation tags.
- `HandleAuthorize` has a skeletal implementation that decodes the body and calls the service.
- `respondWithError` and `respondWithJSON` helpers exist in `response.go`.
- `AuthorizationService.Authorize` is implemented and handles idempotency polling internally.

## Desired End State
- `HandleAuthorize` fully implemented and following project patterns.
- Robust error handling mapping domain errors to correct HTTP status codes.
- Support for `Idempotency-Key` from both header (preferred) and request body.
- Integration with the `AuthorizationService`.

## What We're NOT Doing
- Implementing the `AuthorizationService` (already exists).
- Implementing the Bank Adapter (already exists).
- Changing the database schema.

## Implementation Approach
1.  **Refine Request Extraction:** Update the handler to look for the `Idempotency-Key` in the header first, then the body.
2.  **Validate Request:** Ensure all required fields are present and valid using the `validator` instance.
3.  **Invoke Service:** Call the `Authorize` method of the `authService`.
4.  **Map Responses:** Use the existing helpers to return successful results or mapped errors.
5.  **Test:** Create acceptance tests for various scenarios (Success, Duplicate Key, Timeout, Validation Error).

## Phase 1: Handler Implementation

### Overview
Complete the logic in `internal/adapters/handler/authorize.go`.

### Changes Required:

#### 1. Authorize Handler
**File**: `internal/adapters/handler/authorize.go`
**Changes**: Update `HandleAuthorize` to implement the full logic.

```go
func (h *PaymentHandler) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
    // 1. Decode JSON body
    var req AuthorizeRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondWithError(w, err)
        return
    }

    // 2. Extract Idempotency-Key (Header takes precedence)
    idemKey := r.Header.Get("Idempotency-Key")
    if idemKey != "" {
        req.IdempotencyKey = idemKey
    }

    // 3. Validate
    if err := h.validate.Struct(req); err != nil {
        respondWithError(w, &domain.DomainError{
            Code:    "VALIDATION_ERROR",
            Message: err.Error(),
        })
        return
    }

    // 4. Call Service
    payment, err := h.authService.Authorize(
        r.Context(),
        req.OrderID,
        req.CustomerID,
        req.IdempotencyKey,
        req.Amount,
        req.CardNumber,
        req.CVV,
        req.ExpiryMonth,
        req.ExpiryYear,
    )

    // 5. Handle Result
    if err != nil {
        respondWithError(w, err)
        return
    }

    respondWithJSON(w, http.StatusCreated, payment)
}
```

## Testing Strategy
### Acceptance Tests (DSL):
- **1. Successful Authorization**
- **2. Duplicate Request (Polling Success)**
- **3. Duplicate Request (Polling Timeout/409)**
- **4. Idempotency Mismatch (400/409)**
- **5. Validation Error (400)**

### Automated Verification:
- Run `go test ./internal/adapters/handler/...`
- Run `go test ./internal/core/service/...` (to ensure service integration works)

### Manual Verification:
- Use `curl` to send a POST request to `/authorize`.
- Verify the response status and body.
- Verify the record in the database.
