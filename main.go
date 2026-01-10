package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-redis/redis/v8"
	_ "github.com/jackc/pgx/v4/stdlib"

	"github.com/yashasviy/idempotent-payments-api/api"
	"github.com/yashasviy/idempotent-payments-api/db"
)

func main() {
	// 1. Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})

	// 2. Connect to Postgres
	var database *sql.DB
	var err error

	for i := 0; i < 5; i++ {
		database, err = sql.Open("pgx", os.Getenv("DB_URL"))
		if err == nil {
			err = database.Ping()
		}

		if err == nil {
			fmt.Println("Postgres Connected Successfully!")
			break
		}

		fmt.Println("Waiting for DB...", err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatal("Could not connect to database after retries")
	}
	defer database.Close()

	// 3. Initialize Tables
	db.Initialize(database)

	// 4. Setup Router
	r := chi.NewRouter()

	// TODO: Idempotency Middleware
	r.Post("/transfer", api.TransferHandler(database))

	fmt.Println("Idempotency API running on port 8080...")
	http.ListenAndServe(":8080", r)
}
