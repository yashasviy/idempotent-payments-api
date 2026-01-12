# Distributed Fault-Tolerant Payment Ledger

A high-availability financial transaction engine built in Go, designed to handle concurrent money transfers with strict ACID compliance and idempotency guarantees. This system mimics the core architecture of production-grade payment ledgers (Stripe, Square, etc.), focusing on data correctness, race condition prevention, and self-healing from network failures.

## Architecture

The system is composed of three core components:

1. **Ledger API (Go):** Entry point for all transaction requests, handling HTTP/JSON API calls.
2. **Lock Manager (Redis):** Distributed locking mechanism to serialize concurrent requests on the same account.
3. **Persistent Storage (PostgreSQL):** ACID-compliant database storing transaction logs and account balances.

```
Client Request
     |
     v
[Ledger API]
     |
     +---> [Redis Lock Manager]
     |
     +---> [PostgreSQL Ledger]
```

## Engineering Design Decisions

### 1. Race Condition Handling

**Problem:** In a naive implementation, if a user with $100 sends two concurrent $60 transfers, both requests might read the balance as $100 before either writes the new balance, resulting in a -$20 balance.

**Solution:** Distributed Locking with Redis

- Before transaction execution, the system acquires a distributed lock on the sender account ID using Redis SETNX (Set if Not Exists) with a TTL.
- Only one process can hold the lock at a time, creating a critical section that serializes concurrent access to the same account.
- If a lock is held, subsequent requests wait with exponential backoff or return an error, depending on configuration.
- Lock acquisition includes timeout and automatic expiration to prevent deadlocks.

### 2. Idempotency

**Problem:** If the API processes a payment but the response is lost due to network timeout, the client may retry, charging the user twice.

**Solution:** Idempotency Key Tracking

- Every request includes a unique `Idempotency-Key` header.
- Before processing, the system queries the `transactions` table for an existing record with the same key.
- If found, the original successful response is returned without re-executing the money movement.
- The idempotency check happens before distributed lock acquisition, minimizing contention.

### 3. Crash Recovery

**Problem:** If the service crashes after committing the database transaction but before sending the response, the client sees a failure while the money has actually moved.

**Solution:** Recovery Guard with Database Persistence

- All money movements are atomic database transactions that commit before the handler sends a response.
- If the handler panics or crashes post-commit, the database state is durable.
- On retry with the same idempotency key, the recovery check detects the committed transaction and returns the original result.
- This ensures the client eventually receives confirmation of a successful transfer, even across service crashes.

## Testing & Validation

### Chaos Mode

The system includes an optional chaos testing mode (controlled by the `CHAOS_MODE` environment variable) that simulates post-commit crashes:

- When enabled and the `X-Simulate-Chaos: true` header is set, the handler commits the transaction to the database, then intentionally panics.
- The client sees a dropped connection or 500 error.
- On retry with the same idempotency key, the recovery mechanism returns the committed transaction result.
- This validates that the system recovers correctly from transient failures.

### Stress Testing

A built-in stress tester spawns concurrent requests with the same idempotency key:

- Verifies that duplicate requests return the same result without double-charging.
- Confirms distributed lock serialization prevents race conditions.
- Validates data consistency under high concurrency.

## Technology Stack

- **Language:** Go 1.21+
- **Database:** PostgreSQL 15 with Serializable isolation level
- **Caching & Locking:** Redis (Alpine)
- **Infrastructure:** Docker & Docker Compose
- **Build Tools:** Make

## Quick Start

### Prerequisites

- Docker Desktop (running)
- Go 1.21+ (for local development)
- Make (for convenient workflows)

### Using Docker Compose

Start all services (API, PostgreSQL, Redis):

```bash
docker compose up --build
```

The API will be available on `http://localhost:8080`.

### Using the Makefile

For convenience, the project includes a `Makefile` with common targets:

```bash
# Start services with rebuild
make up-build

# Stop services
make down

# Remove services and volumes (clean state)
make clean

# View application logs
make logs

# Run the API locally with chaos mode enabled
make run-local CHAOS_MODE=true

# Run the stress tester
make stress

# Send a chaos test request (requires CHAOS_MODE=true on the API)
make chaos
```

## API Reference

### Transfer Endpoint

**POST /transfer**

Request body:
```json
{
  "from_id": 1,
  "to_id": 2,
  "amount": 100.00
}
```

Required headers:
```
Idempotency-Key: unique-transaction-id
```

Optional headers:
```
X-Simulate-Chaos: true    (only works if CHAOS_MODE=true)
```

Success response (200):
```json
{
  "status": "success",
  "message": "Transfer Complete",
  "amount": 100.00
}
```

Recovered response (200, with X-Db-Hit header):
```json
{
  "status": "success",
  "message": "Transfer Complete (Recovered)",
  "amount": 100.00
}
```

Error responses:
- `400 Bad Request`: Missing or invalid request body or idempotency key.
- `422 Unprocessable Entity`: Insufficient funds.
- `500 Internal Server Error`: Database or transaction failure.

## Configuration

Environment variables:

- `DB_URL`: PostgreSQL connection string (default: `postgres://user:password@localhost:5432/stripe_clone`)
- `REDIS_ADDR`: Redis address (default: `localhost:6379`)
- `CHAOS_MODE`: Enable chaos testing mode (default: `false`)

## Project Structure

```
payment_project/
├── main.go              # API server entry point
├── stress_tester.go     # Concurrent stress test utility
├── api/
│   └── transfer.go      # Transfer handler with idempotency & recovery
├── db/
│   └── db.go            # Database initialization and utilities
├── middleware/
│   └── idempotency.go   # Idempotency middleware (Redis caching)
├── models/
│   └── models.go        # Request/response data structures
├── Dockerfile           # Container image definition
├── docker-compose.yml   # Multi-container orchestration
├── Makefile             # Build and runtime workflows
└── go.mod              # Go module dependencies
```

## Design Patterns

1. **Distributed Locking:** Ensures serialized access to shared resources (account balances).
2. **Idempotency Key Storage:** Prevents duplicate transactions from double-charging.
3. **ACID Transactions:** PostgreSQL transactions with Serializable isolation ensure data consistency.
4. **Graceful Degradation:** System rejects requests rather than corrupting data under failure.
5. **Write-Ahead Logging:** All state changes are durable before response confirmation.

## Known Limitations

1. Chaos mode is disabled by default; set `CHAOS_MODE=true` to enable testing.
2. Redis lock TTL should be tuned based on expected transaction latency.
3. Stress testing uses in-memory concurrency; production deployments should measure under realistic load.

## Future Enhancements

- Multi-region replication for geographic redundancy
- Event sourcing for comprehensive audit trails
- Circuit breaker pattern for Redis unavailability
- Metrics and observability (Prometheus, Grafana)
- Webhook notifications for transaction confirmations

## License

MIT

## Contact

For issues, feature requests, or questions, please open an issue on the project repository.
