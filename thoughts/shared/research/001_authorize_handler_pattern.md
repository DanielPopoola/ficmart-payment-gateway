---
date: 2025-12-23T10:00:00Z
researcher: Gemini
topic: "Authorize Handler Implementation Patterns"
tags: [research, codebase, handler, authorize, idempotency]
status: complete
---

# Research: Authorize Handler Implementation Patterns

## Research Question
How to implement the `HandleAuthorize` HTTP handler in `internal/adapters/handler/authorize.go` following the project's patterns for request validation, service interaction, error mapping, and idempotency?

## Summary
The `HandleAuthorize` handler is responsible for decoding the JSON request, validating its structure using `validator` tags, invoking the `AuthorizationService`, and mapping domain results/errors to appropriate HTTP responses. Idempotency is handled primarily at the service layer, but the handler must be prepared to handle "Still Processing" (409/202) and "Idempotency Mismatch" (400/409) scenarios.

## Detailed Findings

### Request Validation
- Uses `github.com/go-playground/validator/v10` through a `h.validate` instance.
- Request struct `AuthorizeRequest` in `internal/adapters/handler/authorize.go` already has validation tags.
- Missing required fields should return `400 Bad Request` with a validation error code.

### Service Interaction
- Handler calls `h.authService.Authorize(ctx, ...)` with parameters extracted from the request.
- The service returns a `*domain.Payment` and an `error`.

### Error Mapping
- `internal/adapters/handler/response.go` provides a `respondWithError` helper.
- It maps `domain.DomainError` codes to HTTP status codes:
    - `ErrCodeInvalidAmount`, `ErrCodeMissingRequiredField` -> `400 Bad Request`
    - `ErrCodePaymentNotFound` -> `404 Not Found`
    - `ErrCodeDuplicateIdempotencyKey`, `ErrCodeIdempotencyMismatch`, `ErrCodeInvalidState`, `ErrCodeAmountMismatch`, `ErrCodeInvalidTransition` -> `409 Conflict`
    - `ErrRequestProcessing` -> `202 Accepted` (Note: `respondWithError` currently maps it to `202`, which aligns with "Still Processing")
    - `ErrCodeTimeout` -> `504 Gateway Timeout`

### Idempotency
- `Idempotency-Key` is currently expected in the JSON body of `AuthorizeRequest`.
- `GUIDE.md` mentions "FicMart → POST /authorize (with Idempotency-Key header)", so we should ideally check both header and body, preferring the header if present.

## Code References
- `internal/adapters/handler/authorize.go` - Target file for implementation.
- `internal/core/service/authorize.go` - Service method signature and behavior.
- `internal/adapters/handler/response.go` - Response and error mapping logic.
- `internal/core/domain/errors.go` - Domain error codes.

## Architecture Insights
- **Strict Separation:** Handler stays thin, focusing on HTTP-to-Domain translation.
- **Polling:** The service handles polling, so the handler just waits for the service to return or timeout.
- **Two-Phase Commit:** The gateway commits `PENDING` state before calling the bank, ensuring we have a record even if the bank call fails or times out.

## Open Questions
- Should we strictly enforce the `Idempotency-Key` in the header as per `GUIDE.md`? (I will implement it to check the header first, then the body).
