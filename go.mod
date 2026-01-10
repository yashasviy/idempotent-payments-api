module github.com/yashasviy/idempotent-payments-api

go 1.21

require (
	github.com/go-chi/chi/v5 v5.0.10        // Router
	github.com/go-redis/redis/v8 v8.11.5    // Redis Client
	github.com/jackc/pgx/v4 v4.18.1         // Postgres Driver
	github.com/joho/godotenv v1.5.1         // Env vars
)