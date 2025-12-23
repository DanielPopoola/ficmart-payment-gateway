# Service Handlers Implementation Plan

## Overview
Implement HTTP handlers for the core payment services (`Authorize`, `Capture`, `Refund`, `Void`) in the `internal/adapters/handler` directory. The handlers will expose the business logic via a RESTful API using the standard `net/http` library.

## Current State Analysis
- **Services:** Core services (`Authorize`, `Capture`, `Refund`, `Void`) are implemented in `internal/core/service`.
- **Handlers:** `internal/adapters/handler` directory exists but is empty.
- **Dependencies:** `net/http` is the chosen transport. `go-playground/validator` is available for input validation.
- **Entry Point:** `cmd/gateway` is empty, so we are building the library/adapter layer first.

## Desired End State
- `PaymentHandler` struct initialized with all service dependencies.
- HTTP methods handling JSON requests and responses for:
    - `POST /authorize`
    - `POST /capture`
    - `POST /refund`
    - `POST /void`
- Standardized error handling mapping domain errors to HTTP status codes.
- Unit tests for all handlers.

## Implementation Approach
We will create a cohesive `PaymentHandler` struct that holds references to all services. We'll use a shared helper for JSON encoding/decoding and error mapping.

## Phase 1: Foundation & Shared Logic [x]

### Overview
Set up the handler structure and common utilities for API responses.

### Changes Required:

#### 1. Handler Struct & Constructor [x]
**File**: `internal/adapters/handler/http.go`
- Define `PaymentHandler` struct with fields for `AuthorizationService`, `CaptureService`, `RefundService`, `VoidService`.
- Create `NewPaymentHandler` constructor.
- Add `ServeHTTP` or separate method definitions.

#### 2. Response Helpers [x]
**File**: `internal/adapters/handler/response.go`
- Implement `JSON(w http.ResponseWriter, status int, data interface{})` helper.
- Implement `Error(w http.ResponseWriter, err error)` helper to map domain errors to HTTP codes (e.g., `domain.ErrInvalidAmount` -> 400).

## Phase 2: Implement Handlers [x]

### Overview
Implement the specific handler methods for each service operation.

### Changes Required:

#### 1. Authorization Handler [x]
**File**: `internal/adapters/handler/authorize.go`
- Define Request/Response structs (DTOs).
- Implement `Authorize(w http.ResponseWriter, r *http.Request)`.
- Extract input from JSON body.
- Call `service.Authorize`.
- Return `201 Created` on success.

#### 2. Capture Handler [x]
**File**: `internal/adapters/handler/capture.go`
- Define Request/Response structs.
- Implement `Capture(w http.ResponseWriter, r *http.Request)`.
- Call `service.Capture`.

#### 3. Refund Handler [x]
**File**: `internal/adapters/handler/refund.go`
- Define Request/Response structs.
- Implement `Refund(w http.ResponseWriter, r *http.Request)`.
- Call `service.Refund`.

#### 4. Void Handler [x]
**File**: `internal/adapters/handler/void.go`
- Define Request/Response structs.
- Implement `Void(w http.ResponseWriter, r *http.Request)`.
- Call `service.Void`.

## Phase 3: Testing [x]

### Overview
Verify the handlers with unit tests using `httptest`.

### Changes Required:

#### 1. Test Suite [x]
**File**: `internal/adapters/handler/handler_test.go`
- Mock the services (or use interfaces if services are interfaces, but they seem to be concrete structs in the service layer. We might need to wrap them or interact with them. *Correction*: The services rely on `ports`, so we can mock the ports and instantiate the services, OR we can refactor services to be behind an interface if needed. Given the current code, services are concrete structs. We can integration test with mocked ports, or just test the handler logic assuming service works).
- *Better Approach*: Since services are concrete structs, we will test the handlers by mocking the *ports* (`PaymentRepository`, `BankPort`) that the services use, and injecting those into the services, which are then injected into the handler.
- Write tests for success and error scenarios for each endpoint.

## Verification
- Run `go test ./internal/adapters/handler/...`
- Ensure all tests pass.
