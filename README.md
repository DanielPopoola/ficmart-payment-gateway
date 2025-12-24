# FicMart Payment Gateway

A payment gateway service built in Go that handles authorization, capture, void, and refund operations for FicMart's e-commerce platform. This gateway integrates with a mock banking API and implements robust state management, idempotency, and failure recovery patterns.

## Table of Contents

- [Features](#features)
- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [API Documentation](#api-documentation)
- [Project Structure](#project-structure)
- [Configuration](#configuration)
- [Testing](#testing)
- [Design Decisions](#design-decisions)
- [Contributing](#contributing)

## Features

- **Payment Operations**: Support for authorize, capture, void, and refund transactions
- **State Machine**: Enforces valid payment state transitions
- **Idempotency**: Prevents duplicate charges through idempotency key mechanism
- **Failure Recovery**: Background reconciliation worker for stuck payments
- **Concurrent Safety**: Database-level concurrency control with optimistic locking
- **Query Support**: Retrieve payments by order ID or customer ID
- **Comprehensive Logging**: Structured logging for debugging and monitoring

## Architecture

The gateway follows Clean Architecture principles with clear separation of concerns:

```
┌─────────────┐
│   FicMart   │
│  (Client)   │
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────────┐
│      Payment Gateway (This)         │
│  ┌───────────────────────────────┐  │
│  │  HTTP Handlers                │  │
│  └───────────┬───────────────────┘  │
│              │                       │
│  ┌───────────▼───────────────────┐  │
│  │  Service Layer                │  │
│  │  (Business Logic)             │  │
│  └───────────┬───────────────────┘  │
│              │                       │
│  ┌───────────▼───────────────────┐  │
│  │  Repository (Data Access)     │  │
│  └───────────┬───────────────────┘  │
│              │                       │
│  ┌───────────▼───────────────────┐  │
│  │  PostgreSQL Database          │  │
│  └───────────────────────────────┘  │
│                                      │
│  ┌───────────────────────────────┐  │
│  │  Background Reconciler        │  │
│  └───────────────────────────────┘  │
└──────────────┬───────────────────────┘
               │
               ▼
        ┌─────────────┐
        │  Mock Bank  │
        │     API     │
        └─────────────┘
```

### Key Components

- **HTTP Handlers**: REST API endpoints for payment operations
- **Service Layer**: Business logic and state management
- **Repository**: Data access abstraction over PostgreSQL
- **Bank Client**: HTTP client with retry logic for bank API
- **Reconciler**: Background worker for recovering stuck payments

## Prerequisites

- **Go**: 1.23 or higher
- **Docker**: 20.10+ (with Docker Compose)
- **PostgreSQL**: 16+ (provided via Docker)
- **Mock Bank API**: Running on `localhost:8787`

## Getting Started

### 1. Clone the Repository

```bash
git clone <repository-url>
cd ficmart-payment-gateway
```

### 2. Set Up Environment Variables

Copy the example environment file and configure it:

```bash
cp .env.example .env
```

Edit `.env` to match your local setup. The defaults work for Docker Compose.

### 3. Start the Mock Bank API

The payment gateway requires the mock bank API to be running:

```bash
cd bank
make up
```

Verify the bank is running at `http://localhost:8787/docs`

### 4. Start the Payment Gateway

#### Using Docker Compose (Recommended)

```bash
cd docker
./dev.sh
```

This will:
- Start PostgreSQL
- Run database migrations
- Start the payment gateway on port 8080

#### Running Locally (Development)

```bash
# Install dependencies
go mod download

# Run migrations (requires PostgreSQL running)
# See migrations/001_initial_schema.sql

# Start the application
go run cmd/gateway/main.go
```

### 5. Verify Installation

Check the health endpoint:

```bash
curl http://localhost:8080/health
```

View API documentation:

```bash
open http://localhost:8080/docs/index.html
```

## API Documentation

### Swagger UI

Interactive API documentation is available at `http://localhost:8080/docs/index.html` when the service is running.

### Core Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/authorize` | Reserve funds on a card |
| POST | `/capture` | Charge previously authorized funds |
| POST | `/void` | Cancel an authorization |
| POST | `/refund` | Return money after capture |
| GET | `/payments/order/{orderID}` | Get payment by order ID |
| GET | `/payments/customer/{customerID}` | List payments by customer |

### Example: Authorize a Payment

```bash
curl -X POST http://localhost:8080/authorize \
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

### Test Cards

| Card Number | CVV | Expiry | Balance | Use Case |
|-------------|-----|--------|---------|----------|
| 4111111111111111 | 123 | 12/2030 | $10,000 | Happy path |
| 4242424242424242 | 456 | 06/2030 | $500 | Limited balance |
| 5555555555554444 | 789 | 09/2030 | $0 | Insufficient funds |
| 5105105105105100 | 321 | 03/2020 | $5,000 | Expired card |

## Project Structure

```
.
├── cmd/
│   └── gateway/
│       └── main.go              # Application entry point
├── internal/
│   ├── adapters/
│   │   ├── bank/                # Bank API client
│   │   ├── handler/             # HTTP handlers
│   │   └── postgres/            # PostgreSQL repository
│   ├── config/                  # Configuration loading
│   ├── core/
│   │   ├── domain/              # Domain models and errors
│   │   ├── ports/               # Interface definitions
│   │   └── service/             # Business logic
│   └── worker/                  # Background reconciliation
├── migrations/                  # SQL migration files
├── docker/                      # Docker configuration
├── docs/                        # Swagger documentation
├── tests/                       # Integration tests
├── go.mod
├── go.sum
├── .env.example
├── README.md
└── TRADEOFFS.md                 # Design decisions document
```

## Configuration

Configuration is loaded from environment variables prefixed with `GATEWAY_`. See `.env.example` for all available options.

### Key Configuration Sections

- **Server**: Port, timeouts
- **Database**: Connection settings, pool configuration
- **Bank Client**: Base URL, connection timeout
- **Retry**: Base delay, max retries
- **Worker**: Reconciliation interval, batch size

## Testing

### Run Unit Tests

```bash
go test ./internal/... -v
```

### Run Integration Tests

Integration tests require PostgreSQL and the mock bank to be running:

```bash
# Start dependencies
cd bank && make up
cd docker && docker-compose up -d postgres

# Run tests
go test ./internal/tests -v
```

### Run Specific Test

```bash
go test ./internal/core/service -run TestAuthorizeService_Authorize_Success -v
```

### Test Coverage

```bash
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Design Decisions

For detailed information about architecture, state management, failure handling, and idempotency implementation, see [TRADEOFFS.md](TRADEOFFS.md).

### Key Design Choices

1. **Write-Ahead Pattern**: Save payment intent before calling bank
2. **Database-Level Concurrency**: Use PostgreSQL constraints instead of application locks
3. **Background Reconciliation**: Worker process recovers stuck payments
4. **Lazy Expiration**: Let bank be source of truth for expiration edge cases
5. **Idempotency via Database**: Unique constraints prevent duplicate requests

## Development

### Hot Reload (Development Mode)

The project uses [Air](https://github.com/cosmtrek/air) for hot reloading during development:

```bash
air
```

Configuration is in `.air.toml`.

### Database Migrations

Migrations are applied automatically on startup. To create a new migration:

1. Create a new SQL file in `migrations/` with a sequential number prefix
2. Write forward migrations only (no rollback for this project)
3. Restart the gateway to apply

### Adding a New Endpoint

1. Define request/response types in `internal/adapters/handler/`
2. Implement service logic in `internal/core/service/`
3. Add handler in `internal/adapters/handler/`
4. Register route in `RegisterRoutes()`
5. Add Swagger annotations
6. Regenerate docs: `swag init -g cmd/gateway/main.go`

## Contributing

This is a portfolio/assessment project and is not accepting external contributions. However, feedback and suggestions are welcome via issues.

## License

This project is part of the [Backend Engineer Path](https://github.com/benx421/backend-engineer-path) by benx421.

## Acknowledgments

- Mock Bank API provided as part of the project specification
- Project specification by benx421
- Built as part of the Backend Engineer Path assessment

---

**Note**: This is a learning project demonstrating payment gateway patterns. It is not intended for production use with real payment processing.