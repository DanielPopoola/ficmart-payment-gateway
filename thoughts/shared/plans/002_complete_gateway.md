# Complete Gateway Implementation Plan

## Overview
Complete the payment gateway by implementing the application entry point, missing query handlers, and database migrations. Verify the full system with integration tests.

## Current State Analysis
- **Core Services:** Implemented and tested (`Authorize`, `Capture`, `Refund`, `Void`).
- **Repositories:** PostgreSQL repository implemented (`internal/adapters/postgres/repository.go`).
- **Bank Client:** HTTP client implemented (`internal/adapters/bank/client.go`).
- **Handlers:** Command handlers implemented (`POST /authorize`, etc.). Query handlers (`GET`) are missing.
- **Entry Point:** `cmd/gateway/main.go` is empty/missing.
- **Config:** `internal/config` exists.
- **Migrations:** Missing.

## Desired End State
- Functional HTTP server running on port 8080 (or configured port).
- `GET /payments/order/{orderID}` endpoint available.
- `GET /payments/customer/{customerID}` endpoint available.
- Database schema created via migrations.
- System handles requests and communicates with Postgres and Bank API.

## Implementation Phases

## Phase 1: Database Setup

### Overview
Create the database schema and migration scripts.

### Changes Required:

#### 1. Migration Files
**File**: `migrations/001_initial_schema.sql`
- Create `payments` table.
- Create `idempotency_keys` table.
- Add necessary indexes.

#### 2. Migration Runner (Optional/Manual)
- Use a simple SQL execution in `main.go` or documentation on how to run it (e.g., `psql`). Given the context, a startup script or `run_shell_command` is appropriate.

## Phase 2: Query Handlers

### Overview
Implement the missing GET endpoints for retrieving payment receipts.

### Changes Required:

#### 1. Add Query Methods to Handler
**File**: `internal/adapters/handler/http.go` & `internal/adapters/handler/query.go` (new file)
- `HandleGetPaymentByOrder(w http.ResponseWriter, r *http.Request)`
- `HandleGetPaymentsByCustomer(w http.ResponseWriter, r *http.Request)`
- Register routes in `RegisterRoutes`.

## Phase 3: Wiring & Entry Point

### Overview
Wire all components together in `main.go` and start the server.

### Changes Required:

#### 1. Main Application
**File**: `cmd/gateway/main.go`
- Load Config.
- Initialize Postgres Connection Pool.
- Initialize `PaymentRepository`.
- Initialize `BankClient`.
- Initialize Services (`Authorize`, `Capture`, `Refund`, `Void`).
- Initialize `PaymentHandler`.
- Setup Router (`http.ServeMux`).
- Start Server with graceful shutdown.

## Phase 4: Integration Verification

### Overview
Verify the system by running it and performing HTTP requests.

### Steps:
1. Start Postgres (assume running or start via docker).
2. Run Migrations.
3. Start Gateway.
4. Run manual/scripted curl tests to Verify:
   - Authorize -> Capture -> Refund flow.
   - Idempotency.
   - Persistence (restart server, check state).
