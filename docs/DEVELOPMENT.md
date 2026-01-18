# Development Guide

## Prerequisites
- **Go 1.25+**
- **Docker & Docker Compose**
- **Make** (optional, but recommended)
- **Air** (for hot-reload development)

---

## Quick Start

1. **Environment Setup**:
   ```bash
   cp .env.example .env
   ```

2. **Start Infrastructure**:
   ```bash
   # Starts Postgres and the Mock Bank
   make up
   ```

3. **Run the Gateway**:
   ```bash
   # Starts the gateway with hot-reload (requires Air)
   air
   # OR run directly
   go run cmd/gateway/main.go
   ```

4. **Verify**:
   Access the API documentation at `http://localhost:8081/docs`.

---

## Project Structure

```
├── api/                  # OpenAPI spec and generation configs
├── cmd/gateway/          # Application entry point
├── internal/
│   ├── api/              # Generated OpenAPI code (Do not edit)
│   ├── application/      # Service layer, error categorizer, and DTOs
│   │   └── services/     # Business orchestration (Authorize, Capture, etc.)
│   ├── domain/           # Core entities and state machine
│   ├── infrastructure/   # DB repositories and Bank API client
│   ├── interfaces/       # HTTP handlers and middleware
│   ├── worker/           # Background retry and expiration jobs
│   └── db/migrations/    # Postgres SQL migrations
|   └── tests/e2e/        # End-to-end integration tests

```

---

## Core Workflows

### 1. Modifying the API
The gateway is API-first. To change the interface:
1. Edit `api/openapi.yaml`.
2. Run code generation:
   ```bash
   go generate ./...
   ```
3. Implement the updated handler in `internal/interfaces/rest/handlers/`.

### 2. Database Migrations
We use `golang-migrate`.
- **Create a migration**:
  ```bash
  migrate create -ext sql -dir internal/db/migrations -seq your_migration_name
  ```
- **Run migrations**: Migrations run automatically on application startup.

### 3. Running Tests
- **Unit Tests**:
  ```bash
  go test ./internal/domain/...
  ```
- **Integration/Service Tests**:
  ```bash
  go test ./internal/application/services/...
  ```
- **E2E Tests**:
  ```bash
  # Requires the infrastructure (Postgres + Bank) to be running
  RUN_E2E_TESTS=true go test ./internal/tests/e2e/...
  ```

### 4. Code Quality
- **Linting**:
  ```bash
  golangci-lint run
  ```
- **Formatting**:
  ```bash
  go fmt ./...
  ```

---

## Observability & Debugging

### Logging
We use structured logging (`slog`). Logs include:
- `payment_id` and `order_id` for traceability.
- `error_category` for diagnosing retry behavior.
- HTTP request/response metadata.

### Database Inspection
```bash
docker compose exec payment-postgres psql -U postgres -d payment_gateway_db
```

Useful queries:
- **Check stuck payments**: `SELECT id, status FROM payments WHERE status IN ('CAPTURING', 'VOIDING', 'REFUNDING');`
- **View idempotency locks**: `SELECT key, locked_at FROM idempotency_keys WHERE locked_at IS NOT NULL;`

---

## Common Tasks

### Adding a new State Transition
1. Update the `PaymentStatus` constants and `transition` logic in `internal/domain/payment.go`.
2. Update the corresponding repository queries in `internal/infrastructure/persistence/postgres/payment_repository.go`.
3. If it requires a background job, add logic to `internal/worker/retry_worker.go`.

### Handling a new Bank Error
1. Locate the error code in the Mock Bank documentation.
2. Add the mapping to the `CategorizeError` function in `internal/application/error_categorizer.go`.
3. Define whether it is `CategoryTransient` (retryable) or `CategoryPermanent`.

---

## Deployment
The gateway is stateless and supports horizontal scaling. Ensure:
- `GATEWAY_DATABASE__HOST` points to a shared Postgres instance.

- Idempotency is shared across all instances via the central DB.
