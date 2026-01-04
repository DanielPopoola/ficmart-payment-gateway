# FicMart Payment Gateway

A production-grade payment gateway built in Go that handles card payment operations with robust state management, automatic failure recovery, and guaranteed idempotency. This gateway integrates with a mock banking API and implements enterprise-level patterns for handling distributed system failures.

## Why This Exists

Payment processing is deceptively hard. When you authorize a payment, your request might succeed at the bank but fail to save in your database. Or the bank might return a 500 error even though the authorization succeeded. This gateway solves these problems:

- **State Consistency**: Your database always reflects reality, even across crashes
- **Automatic Recovery**: Background workers detect and fix stuck payments
- **Idempotency Guarantees**: Retry-safe operations that never double-charge customers
- **Failure Classification**: Intelligent retry logic that knows when to give up

## Core Features

### 🎯 Complete Payment Lifecycle
- **Authorize**: Reserve funds on a customer's card
- **Capture**: Charge previously authorized funds
- **Void**: Cancel authorization before capture
- **Refund**: Return money after capture

### 🔄 Automatic Failure Recovery
- Background workers detect payments stuck in intermediate states (`CAPTURING`, `VOIDING`, `REFUNDING`)
- Exponential backoff with jitter prevents API overload
- Smart error classification: transient errors are retried, permanent errors fail fast

### 🛡️ Idempotency Guarantees
- Database-level idempotency enforcement using unique constraints
- Request hash validation prevents key reuse with different parameters
- Concurrent request handling with lock-based coordination

### 📊 State Machine Enforcement
```
PENDING → AUTHORIZED → CAPTURED → REFUNDED
              ↓
           VOIDED
```
Invalid transitions (e.g., voiding after capture) are rejected at the domain level.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    FicMart (Client)                     │
└───────────────────────┬─────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────┐
│              Payment Gateway (REST API)                 │
│  ┌───────────────────────────────────────────────────┐  │
│  │              HTTP Handlers Layer                  │  │
│  └──────────────────────┬────────────────────────────┘  │
│                         │                                │
│  ┌──────────────────────▼────────────────────────────┐  │
│  │           Application Services Layer              │  │
│  │  • AuthorizeService  • CaptureService            │  │
│  │  • VoidService       • RefundService             │  │
│  └──────────────────────┬────────────────────────────┘  │
│                         │                                │
│  ┌──────────────────────▼────────────────────────────┐  │
│  │              Domain Layer (Pure)                  │  │
│  │  • Payment Entity   • State Machine              │  │
│  │  • Business Rules   • Domain Errors              │  │
│  └──────────────────────┬────────────────────────────┘  │
│                         │                                │
│  ┌──────────────────────▼────────────────────────────┐  │
│  │         Infrastructure Layer                      │  │
│  │  • PostgreSQL Repos  • Bank HTTP Client          │  │
│  └───────────────────────────────────────────────────┘  │
│                                                           │
│  ┌───────────────────────────────────────────────────┐  │
│  │          Background Workers                       │  │
│  │  • RetryWorker (stuck payments)                  │  │
│  │  • ExpirationWorker (expired auths)              │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
                        │
                        ▼
              ┌──────────────────┐
              │   Mock Bank API  │
              └──────────────────┘
```

**Key Design Decisions:**

1. **Domain-Driven Design**: Business logic lives in the domain layer, completely isolated from HTTP/database concerns
2. **Write-Ahead Pattern**: Every payment is saved as `PENDING` before calling the bank, ensuring we have a record to reconcile
3. **Intermediate States**: States like `CAPTURING` signal intent, allowing workers to resume operations after crashes
4. **Database-Level Concurrency**: PostgreSQL's `FOR UPDATE SKIP LOCKED` enables multiple worker instances to process retries concurrently

See [TRADEOFFS.md](./TRADEOFFS.md) for detailed rationale.

## Quick Start

### Prerequisites

- **Docker & Docker Compose**: 20.10+
- **Go**: 1.23+ (for local development)
- **Make**: Optional (macOS/Linux)

### 1. Start the Mock Bank

The gateway requires the mock bank to be running:

```bash
cd bank
make up
```

Verify at: http://localhost:8787/docs

### 2. Start the Payment Gateway

```bash
make up
```

The gateway will be available at: http://localhost:8081

### 3. Verify Installation

```bash
# View API docs
open http://localhost:8081/docs
```

## API Usage

### Complete Payment Flow

#### 1. Authorize Payment (Reserve Funds)

```bash
curl -X POST http://localhost:8081/authorize \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{
    "order_id": "order-12345",
    "customer_id": "cust-67890",
    "amount": 5000,
    "card_number": "4111111111111111",
    "cvv": "123",
    "expiry_month": 12,
    "expiry_year": 2030
  }'
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "AUTHORIZED",
    "amount_cents": 5000,
    "bank_auth_id": "auth-abc123",
    "expires_at": "2024-01-22T10:30:01Z"
  }
}
```

#### 2. Capture Payment (Charge the Card)

```bash
curl -X POST http://localhost:8081/capture \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{
    "payment_id": "550e8400-e29b-41d4-a716-446655440000",
    "amount": 5000
  }'
```

#### 3. Query Payment Status

```bash
# By payment ID
curl http://localhost:8081/payments/550e8400-e29b-41d4-a716-446655440000

# By order ID
curl http://localhost:8081/payments/order/order-12345

# By customer ID
curl http://localhost:8081/payments/customer/cust-67890?limit=10&offset=0
```

### Test Cards

| Card Number          | CVV | Expiry  | Balance  | Use Case              |
|---------------------|-----|---------|----------|-----------------------|
| 4111111111111111    | 123 | 12/2030 | $10,000  | Happy path            |
| 4242424242424242    | 456 | 06/2030 | $500     | Limited balance       |
| 5555555555554444    | 789 | 09/2030 | $0       | Insufficient funds    |
| 5105105105105100    | 321 | 03/2020 | $5,000   | Expired card          |

## Development

### Run Locally (Hot Reload)

```bash
cd docker
docker compose up

# In another terminal, attach to the container
docker compose exec gateway sh
air  # Hot reload on file changes
```

### Run Tests

```bash
# Unit tests
go test ./internal/... -v

# Integration tests (requires DB)
go test ./internal/application/services/... -v

# E2E tests (requires gateway + bank running)
RUN_E2E_TESTS=true go test ./internal/tests/e2e/... -v
```

### Test Coverage

```bash
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Database Migrations

Migrations run automatically on startup. Files are in `internal/db/migrations/`.

## Project Structure

```
.
├── cmd/gateway/              # Application entry point
├── internal/
│   ├── domain/              # Business logic & state machine (zero dependencies)
│   ├── application/         # Service orchestration & error handling
│   │   └── services/        # AuthorizeService, CaptureService, etc.
│   ├── infrastructure/      # External integrations
│   │   ├── bank/           # Bank API client with retry logic
│   │   └── persistence/    # PostgreSQL repositories
│   ├── interfaces/          # HTTP handlers & middleware
│   └── worker/              # Background retry & expiration workers
├── internal/db/migrations/  # SQL migration files
├── docker/                  # Docker & docker-compose setup
└── internal/tests/          # Integration & E2E tests
```

## Configuration

Configuration is loaded from environment variables with the `GATEWAY_` prefix. Key settings:

```bash
# Server
GATEWAY_SERVER__PORT=8080
GATEWAY_SERVER__READ_TIMEOUT=15s

# Database
GATEWAY_DATABASE__HOST=localhost
GATEWAY_DATABASE__PORT=5432
GATEWAY_DATABASE__MAX_OPEN_CONNS=25

# Bank API
GATEWAY_BANK_CLIENT__BANK_BASE_URL=http://localhost:8787
GATEWAY_BANK_CLIENT__BANK_CONN_TIMEOUT=30s

# Retry Behavior
GATEWAY_RETRY__BASE_DELAY=1        # Initial delay in seconds
GATEWAY_RETRY__MAX_RETRIES=3       # Max retry attempts

# Workers
GATEWAY_WORKER__INTERVAL=30s       # How often to check for stuck payments
GATEWAY_WORKER__BATCH_SIZE=100     # Max payments to process per cycle
```

See [`.env.example`](./.env.example) for the complete list.

## How It Handles Failures

### Scenario 1: Bank Returns 500 Error

1. Gateway saves payment as `PENDING`
2. Bank call fails with 500
3. Payment stays `PENDING` (no state change)
4. **Not retried** (authorization requires card details we don't store)
5. Marked `FAILED` for manual reconciliation

### Scenario 2: Gateway Crashes During Capture

1. Payment transitions to `CAPTURING`
2. Bank responds with success
3. **Gateway crashes before updating DB**
4. Retry worker finds payment stuck in `CAPTURING`
5. Retries with same idempotency key
6. Bank returns cached success (idempotent!)
7. Gateway updates payment to `CAPTURED`

### Scenario 3: Transient Network Error

1. Payment is in `VOIDING`
2. Void call times out
3. Worker schedules retry with exponential backoff: 1s → 2s → 4s
4. Retry succeeds on second attempt
5. Payment marked `VOIDED`

## Design Philosophy

This gateway prioritizes **correctness over performance**:

- ✅ Never double-charge a customer
- ✅ Always reconcile with the bank's state
- ✅ Prefer database transactions over in-memory state
- ✅ Fail loud (return errors) rather than fail silent

For a deep dive into architecture decisions, retry strategies, and production considerations, see [TRADEOFFS.md](./TRADEOFFS.md).

## Common Tasks

```bash
# View logs
make logs

# Restart gateway
make restart

# Connect to database
docker compose exec payment-postgres psql -U postgres -d payment_gateway_db

# Run linter
cd docker && docker compose exec gateway golangci-lint run

# Generate mocks
mockery --config .mockery.yaml
```

## Known Limitations

1. **No Partial Captures/Refunds**: Must capture/refund the full authorized amount
2. **Single Currency**: Only USD is supported
3. **No Card Tokenization**: Card details are not stored (by design)
4. **Authorize Retry Limitation**: Failed authorizations cannot be automatically retried (requires card details)

## Contributing

This is a portfolio project demonstrating payment gateway patterns. While it's not accepting external contributions, feedback via issues is welcome.

## License

Part of the [Backend Engineer Path](https://github.com/benx421/backend-engineer-path) assessment by benx421.

---

**⚠️ Note**: This is a learning project built with a mock bank. Not for production use with real payment processing.