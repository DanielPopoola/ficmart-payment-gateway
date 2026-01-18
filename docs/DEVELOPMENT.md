# Development Guide

## Prerequisites

**Required:**
- Docker & Docker Compose 20.10+
- Go 1.23+ (for local development)
- Make (optional, macOS/Linux)

**Recommended:**
- PostgreSQL client (psql)
- curl or Postman for API testing
- golangci-lint for code quality

---

## Quick Start (5 Minutes)

### 1. Start Mock Bank

The gateway depends on a mock bank API. Start it first:

```bash
cd bank
make up
```

Verify at: http://localhost:8787/docs

### 2. Start Payment Gateway

```bash
# From project root
make up
```

This will:
- Start PostgreSQL container
- Run database migrations
- Start gateway with hot-reload (Air)

Verify at: http://localhost:8081/docs

### 3. Test the Gateway

```bash
# Authorize a payment
curl -X POST http://localhost:8081/authorize \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{
    "order_id": "order-123",
    "customer_id": "cust-456",
    "amount": 5000,
    "card_number": "4111111111111111",
    "cvv": "123",
    "expiry_month": 12,
    "expiry_year": 2030
  }'
```

You should see a `201 Created` response with payment details.

---

## Project Structure (What Lives Where)

```
.
├── cmd/gateway/              # Application entry point
│   └── main.go              # Starts server, workers, wires dependencies
│
├── internal/                # Private application code
│   ├── api/                 # Generated OpenAPI code (DO NOT EDIT)
│   │   ├── dtos.gen.go     # Request/response models
│   │   ├── server.gen.go   # HTTP handlers skeleton
│   │   └── spec.gen.go     # Embedded OpenAPI spec
│   │
│   ├── domain/              # Business logic (PURE - no dependencies)
│   │   ├── payment.go      # Payment entity + state machine
│   │   └── errors.go       # Domain-specific errors
│   │
│   ├── application/         # Service orchestration
│   │   ├── services/       # Business operations
│   │   │   ├── authorize.go
│   │   │   ├── capture.go
│   │   │   ├── void.go
│   │   │   ├── refund.go
│   │   │   ├── query.go
│   │   │   └── helpers.go  # Shared idempotency logic
│   │   └── error_categorizer.go  # Retry logic
│   │
│   ├── infrastructure/      # External systems
│   │   ├── bank/           # Bank API client
│   │   │   ├── client.go   # HTTP client
│   │   │   ├── retry.go    # Retry wrapper
│   │   │   └── dtos.go     # Bank request/response types
│   │   └── persistence/    # Database
│   │       └── postgres/
│   │           ├── connection.go
│   │           ├── payment_repository.go
│   │           └── idempotency_repository.go
│   │
│   ├── interfaces/          # HTTP layer
│   │   └── rest/
│   │       ├── handlers/   # HTTP handlers
│   │       │   ├── authorize.go
│   │       │   ├── capture.go
│   │       │   ├── void.go
│   │       │   ├── refund.go
│   │       │   └── query.go
│   │       └── middleware/ # HTTP middleware
│   │           ├── logging.go
│   │           ├── recovery.go
│   │           └── timeout.go
│   │
│   ├── worker/              # Background jobs
│   │   ├── retry_worker.go
│   │   └── expiration_worker.go
│   │
│   ├── config/              # Configuration
│   │   ├── config.go
│   │   ├── database.go
│   │   └── logger.go
│   │
│   └── db/migrations/       # SQL migrations
│       ├── 001_init.up.sql
│       └── 001_init.down.sql
│
├── api/                     # OpenAPI specification
│   ├── openapi.yaml        # API definition (source of truth)
│   └── cfg/                # Code generation configs
│       ├── dtos.yaml
│       ├── server.yaml
│       └── spec.yaml
│
├── docker/                  # Docker setup
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── init.sh             # Database initialization
│
├── .air.toml               # Hot-reload config
├── .golangci.yaml          # Linter config
├── Makefile                # Common commands
└── go.mod                  # Dependencies
```

---

## Development Workflow

### 1. Local Development (Hot Reload)

The gateway uses [Air](https://github.com/cosmtrek/air) for hot-reload:

```bash
# Start with hot-reload
make up

# The gateway will automatically rebuild on file changes
# Edit any .go file → Auto rebuild → Changes reflected
```

**What gets reloaded:**
- All `.go` files
- `.yaml` configuration
- `.sql` migrations (requires manual `make restart`)

**What doesn't reload:**
- Database schema changes (run `make restart`)
- Docker image changes (run `make up`)

### 2. Working Inside Container

```bash
# Shell into gateway container
make shell

# Now you have access to all tools:
go test ./...
golangci-lint run
air  # Start hot-reload manually
```

### 3. Database Migrations

**Create new migration:**

```bash
# Inside container
make shell

# Create migration files
migrate create -ext sql -dir ./internal/db/migrations -seq add_refund_reason

# This creates:
# 002_add_refund_reason.up.sql
# 002_add_refund_reason.down.sql
```

**Apply migrations:**

```bash
# Migrations run automatically on startup via init.sh
# To run manually:
make shell
migrate -path ./internal/db/migrations \
  -database "postgres://postgres:postgres@payment-postgres:5432/payment_gateway_db?sslmode=disable" \
  up
```

### 4. Regenerating API Code

If you change `api/openapi.yaml`, regenerate code:

```bash
make shell
go generate ./...  # Runs oapi-codegen
```

**What gets regenerated:**
- `internal/api/dtos.gen.go` - Request/response types
- `internal/api/server.gen.go` - HTTP handler interfaces
- `internal/api/spec.gen.go` - Embedded OpenAPI spec

---

## Testing

### Unit Tests (Fast - No Database)

```bash
# Run all unit tests
go test ./internal/domain/... -v

# Run specific test
go test ./internal/domain -run TestPayment_StateTransitions

# With coverage
go test ./internal/domain/... -cover
```

**What to unit test:**
- Domain layer (state transitions)
- Error classification logic
- Pure functions (helpers)

### Integration Tests (Medium - Real Database)

```bash
# Run service tests (uses testcontainers)
go test ./internal/application/services/... -v

# Run specific test
go test ./internal/application/services -run TestAuthorizeService
```

**What's happening:**
- Test spins up PostgreSQL container
- Runs migrations
- Executes test
- Tears down container

**Key patterns:**
```go
// Setup
testDB := testhelpers.SetupTestDatabase(t)
defer testDB.Cleanup(t)

// Each test gets clean tables
testDB.CleanTables(t)
```

### E2E Tests (Slow - Full Stack)

```bash
# Start gateway and bank
make up
cd bank && make up

# Run E2E tests
RUN_E2E_TESTS=true go test ./internal/tests/e2e/... -v
```

**What's tested:**
- Full HTTP flow (client → gateway → bank)
- Idempotency behavior
- Error scenarios
- Multi-step flows (authorize → capture → refund)

### Running All Tests

```bash
# Inside container
make test

# This runs:
# 1. Unit tests
# 2. Integration tests
# 3. E2E tests (if RUN_E2E_TESTS=true)
```

---

## Code Quality

### Linting

```bash
# Run linter
make lint

# Or inside container
golangci-lint run
```

**Configured linters:**
- `errcheck` - Unchecked errors
- `govet` - Suspicious constructs
- `staticcheck` - Advanced checks
- `gosec` - Security issues
- `gocritic` - Opinionated checks

### Formatting

```bash
make fmt

# Or manually
gofmt -w .
```

### Pre-Commit Checklist

Before committing:
```bash
make fmt      # Format code
make lint     # Run linter
make test     # Run tests
```

---

## Common Development Tasks

### Task 1: Add New Payment Operation

**Example: Add "Partial Refund" feature**

1. **Update Domain (State Machine)**

```go
// internal/domain/payment.go

func (p *Payment) PartialRefund(amount int64) error {
    if p.Status != StatusCaptured {
        return ErrInvalidTransition
    }
    if amount > p.AmountCents {
        return ErrInvalidAmount
    }
    // Business logic here
    return nil
}
```

2. **Add Application Service**

```go
// internal/application/services/partial_refund.go

type PartialRefundService struct {
    paymentRepo *postgres.PaymentRepository
    bankClient  bank.BankClient
}

func (s *PartialRefundService) Refund(ctx context.Context, cmd PartialRefundCommand) (*domain.Payment, error) {
    // 1. Check idempotency
    // 2. Validate amount
    // 3. Call bank
    // 4. Update payment
}
```

3. **Add HTTP Handler**

```go
// internal/interfaces/rest/handlers/partial_refund.go

func (h *Handlers) PartialRefundPayment(ctx context.Context, req api.PartialRefundRequest) (*api.Payment, error) {
    cmd := services.PartialRefundCommand{
        PaymentID: req.PaymentId,
        Amount:    req.Amount,
    }
    return h.partialRefundService.Refund(ctx, cmd, req.IdempotencyKey)
}
```

4. **Update OpenAPI Spec**

```yaml
# api/openapi.yaml

/partial-refund:
  post:
    summary: Partial Refund
    requestBody:
      required: true
      content:
        application/json:
          schema:
            type: object
            properties:
              payment_id:
                type: string
              amount:
                type: integer
```

5. **Regenerate Code**

```bash
make shell
go generate ./...
```

### Task 2: Add Database Index

```sql
-- internal/db/migrations/003_add_refund_index.up.sql

CREATE INDEX idx_payments_refund_status 
ON payments(status) 
WHERE status = 'REFUNDING';
```

```sql
-- internal/db/migrations/003_add_refund_index.down.sql

DROP INDEX IF EXISTS idx_payments_refund_status;
```

### Task 3: Add Configuration Option

```go
// internal/config/config.go

type RefundConfig struct {
    MaxPartialRefunds int `koanf:"max_partial_refunds" validate:"required"`
}

type Config struct {
    // ... existing fields
    Refund RefundConfig `koanf:"refund"`
}
```

```bash
# .env

GATEWAY_REFUND__MAX_PARTIAL_REFUNDS=3
```

### Task 4: Debug Stuck Payment

```bash
# Connect to database
docker compose exec payment-postgres psql -U postgres -d payment_gateway_db

# Find stuck payments
SELECT id, status, created_at, next_retry_at, attempt_count
FROM payments
WHERE status IN ('CAPTURING', 'VOIDING', 'REFUNDING');

# Check idempotency lock
SELECT key, locked_at, payment_id
FROM idempotency_keys
WHERE locked_at IS NOT NULL;

# Manually unlock (if needed)
UPDATE idempotency_keys SET locked_at = NULL WHERE key = 'idem-xyz';
```

---

## Environment Variables

### Required Variables

```bash
# Server
GATEWAY_SERVER__PORT=8080
GATEWAY_SERVER__READ_TIMEOUT=15s
GATEWAY_SERVER__WRITE_TIMEOUT=15s
GATEWAY_SERVER__IDLE_TIMEOUT=60s

# Database
GATEWAY_DATABASE__HOST=localhost
GATEWAY_DATABASE__PORT=5432
GATEWAY_DATABASE__USER=postgres
GATEWAY_DATABASE__PASSWORD=postgres
GATEWAY_DATABASE__NAME=payment_gateway_db
GATEWAY_DATABASE__SSL_MODE=disable
GATEWAY_DATABASE__MAX_OPEN_CONNS=25
GATEWAY_DATABASE__MAX_IDLE_CONNS=5
GATEWAY_DATABASE__CONN_MAX_LIFETIME=5m
GATEWAY_DATABASE__CONN_MAX_IDLE_TIME=5m

# Bank Client
GATEWAY_BANK_CLIENT__BANK_BASE_URL=http://localhost:8787
GATEWAY_BANK_CLIENT__BANK_CONN_TIMEOUT=30s

# Retry
GATEWAY_RETRY__BASE_DELAY=1
GATEWAY_RETRY__MAX_RETRIES=3
GATEWAY_RETRY__MAX_BACKOFF=10

# Worker
GATEWAY_WORKER__INTERVAL=30s
GATEWAY_WORKER__BATCH_SIZE=100

# Logger
GATEWAY_LOGGER__LEVEL=info  # debug, info, warn, error
```

### Development vs Production

**Development:**
```bash
GATEWAY_PRIMARY__ENV=development
GATEWAY_LOGGER__LEVEL=debug
GATEWAY_WORKER__INTERVAL=10s  # Faster for testing
```

**Production:**
```bash
GATEWAY_PRIMARY__ENV=production
GATEWAY_LOGGER__LEVEL=info
GATEWAY_WORKER__INTERVAL=30s
GATEWAY_DATABASE__MAX_OPEN_CONNS=50  # More connections
```

---

## Debugging

### 1. Enable Debug Logging

```bash
# In docker-compose.yml
environment:
  - GATEWAY_LOGGER__LEVEL=debug

# Restart
make restart
```

### 2. View Logs

```bash
# Real-time logs
make logs

# Specific service
docker compose logs -f gateway

# Filter by level
docker compose logs gateway | grep ERROR
```

### 3. Debug with Delve

```go
// Add breakpoint in code
import "runtime/debug"
debug.PrintStack()
```

```bash
# Run with debugger
make shell
dlv debug ./cmd/gateway
```

### 4. Inspect HTTP Traffic

```bash
# Enable request/response logging
# Already enabled via middleware/logging.go

# Watch logs
make logs | grep "request completed"
```

---

## Performance Profiling

### CPU Profile

```bash
# Start gateway with profiling
go run ./cmd/gateway &

# Generate profile
curl http://localhost:8080/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze
go tool pprof cpu.prof
```

### Memory Profile

```bash
curl http://localhost:8080/debug/pprof/heap > mem.prof
go tool pprof mem.prof
```

### Database Query Performance

```sql
-- Enable query logging
ALTER DATABASE payment_gateway_db SET log_statement = 'all';

-- View slow queries
SELECT * FROM pg_stat_statements 
WHERE mean_exec_time > 100 
ORDER BY mean_exec_time DESC;
```

---

## Troubleshooting

### Issue: "Connection refused" to database

**Cause:** PostgreSQL not ready or wrong host.

**Fix:**
```bash
# Check database is running
docker compose ps payment-postgres

# Check connection from gateway
make shell
pg_isready -h payment-postgres -U postgres
```

### Issue: "Idempotency key reused"

**Cause:** Same key used with different request parameters.

**Fix:**
```bash
# Clear idempotency cache
docker compose exec payment-postgres psql -U postgres -d payment_gateway_db
DELETE FROM idempotency_keys WHERE key = 'your-key';
```

### Issue: Workers not processing stuck payments

**Cause:** Worker interval too long or payments not meeting criteria.

**Debug:**
```sql
-- Find stuck payments
SELECT id, status, next_retry_at, attempt_count
FROM payments
WHERE status IN ('CAPTURING', 'VOIDING', 'REFUNDING')
ORDER BY created_at DESC;

-- Check idempotency locks
SELECT * FROM idempotency_keys WHERE locked_at IS NOT NULL;
```

**Fix:**
```bash
# Reduce worker interval for testing
GATEWAY_WORKER__INTERVAL=5s make restart
```

### Issue: "payment not found" in logs

**Cause:** Payment ID mismatch or query error.

**Debug:**
```sql
-- Search by order ID
SELECT * FROM payments WHERE order_id = 'your-order-id';

-- Check recent payments
SELECT * FROM payments ORDER BY created_at DESC LIMIT 10;
```

---

## Git Workflow

### Branch Naming

```
feature/add-partial-refunds
bugfix/fix-retry-timeout
hotfix/critical-db-connection
```

### Commit Messages

```
feat: add partial refund support
fix: retry worker now handles timeouts
docs: update architecture diagram
test: add integration test for capture
```

### Pull Request Checklist

- [ ] Code compiles (`go build ./...`)
- [ ] Tests pass (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] Code formatted (`make fmt`)
- [ ] Documentation updated
- [ ] Database migrations included (if applicable)

---

## CI/CD

The project uses GitHub Actions for CI:

**.github/workflows/gateway-ci.yml**

```yaml
on: [push, pull_request]
jobs:
  test:
    - Run unit tests
    - Run integration tests (with PostgreSQL)
    - Upload coverage
  
  lint:
    - Run golangci-lint
  
  build:
    - Build binary
    - Verify binary works
```

**Local simulation:**
```bash
# Run same checks as CI
make fmt
make lint
make test
go build -v -o bin/gateway ./cmd/gateway
```

---

## Docker Commands Reference

```bash
# Start all services
docker compose up -d

# Stop all services
docker compose down

# Restart gateway only
docker compose restart gateway

# View logs
docker compose logs -f gateway

# Shell into gateway
docker compose exec gateway sh

# Shell into database
docker compose exec payment-postgres psql -U postgres -d payment_gateway_db

# Remove all data (fresh start)
docker compose down -v
```

---

## Make Commands Reference

```bash
make up        # Start all services
make down      # Stop all services
make restart   # Restart gateway
make logs      # View gateway logs
make shell     # Shell into gateway container
make test      # Run all tests
make lint      # Run linter
make fmt       # Format code
make build     # Build binary
```

---

## Performance Tips

1. **Database Connection Pool:**
   ```bash
   GATEWAY_DATABASE__MAX_OPEN_CONNS=50  # Increase for production
   ```

2. **Worker Batch Size:**
   ```bash
   GATEWAY_WORKER__BATCH_SIZE=500  # Process more at once
   ```

3. **HTTP Timeouts:**
   ```bash
   GATEWAY_SERVER__READ_TIMEOUT=10s   # Reduce for faster failures
   GATEWAY_BANK_CLIENT__BANK_CONN_TIMEOUT=20s
   ```

4. **Database Indexes:**
   - Already optimized for common queries
   - Add custom indexes for your query patterns

---

## Security Best Practices

1. **Never log sensitive data:**
   ```go
   // WRONG
   logger.Info("processing payment", "card_number", req.CardNumber)
   
   // CORRECT
   logger.Info("processing payment", "payment_id", payment.ID)
   ```

2. **Validate all inputs:**
   ```go
   if req.Amount <= 0 {
       return ErrInvalidAmount
   }
   ```

3. **Use prepared statements:**
   - pgx automatically uses prepared statements

4. **Set resource limits:**
   ```bash
   GATEWAY_SERVER__READ_TIMEOUT=15s
   GATEWAY_WORKER__BATCH_SIZE=100
   ```

---

## Next Steps

1. Read [ARCHITECTURE.md](./ARCHITECTURE.md) for design decisions
2. Read [TRADEOFFS.md](./TRADEOFFS.md) for production considerations
3. Explore [api/openapi.yaml](./api/openapi.yaml) for API reference
4. Run E2E tests: `RUN_E2E_TESTS=true make test`
5. Try breaking things and see how recovery works!

---

## Getting Help

1. **Check logs:** `make logs`
2. **Inspect database:** `make shell` → `psql`
3. **Read tests:** Best documentation is in `*_test.go` files
4. **OpenAPI docs:** http://localhost:8081/docs