# Makefile for Payment Engine (production-friendly)

COMPOSE ?= docker compose
GO       ?= go
APP      ?= ./main.go
STRESS   ?= ./stress_tester.go
DB_URL   ?= postgres://user:password@localhost:5432/stripe_clone
REDIS_ADDR ?= localhost:6379
CHAOS_MODE ?= false

.PHONY: up up-build down clean build run-local logs stress chaos

# Start services using Docker (cached build)
up:
	@echo "Starting services..."
	$(COMPOSE) up -d

# Rebuild images and start services
up-build:
	@echo "Rebuilding images and starting services..."
	$(COMPOSE) up -d --build

# Stop services
down:
	@echo "Stopping services..."
	$(COMPOSE) down

# Remove services and volumes
clean:
	@echo "Stopping services and removing volumes..."
	$(COMPOSE) down -v

# Build the API binary locally
build:
	@echo "Building API binary..."
	$(GO) build -o bin/payment-api $(APP)

# Run the API locally (uses local DB/Redis addresses by default)
run-local:
	@echo "Running API locally..."
	DB_URL=$(DB_URL) REDIS_ADDR=$(REDIS_ADDR) CHAOS_MODE=$(CHAOS_MODE) $(GO) run $(APP)

# Tail application logs (Docker)
logs:
	$(COMPOSE) logs -f app

# Run the stress tester
stress:
	@echo "Launching stress tester..."
	$(GO) run $(STRESS)

# Fire a chaos request (requires CHAOS_MODE=true on the server)
# Note: set CHAOS_MODE=true on the API process before using this target.
chaos:
	@echo "Sending chaos transaction..."
	curl -v -X POST http://localhost:8080/transfer \
		-H "Content-Type: application/json" \
		-H "Idempotency-Key: chaos-mk-1" \
		-H "X-Simulate-Chaos: true" \
		-d '{"from_id":1,"to_id":2,"amount":100}'
