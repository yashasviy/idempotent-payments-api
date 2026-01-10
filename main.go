package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-redis/redis/v8"
	_ "github.com/jackc/pgx/v4/stdlib"
)

func main() {
	// 1. Test Redis Connection
	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})
	_, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		log.Printf("❌ Redis Connection Failed: %v", err)
	} else {
		log.Println("✅ Redis Connected!")
	}

	// 2. Test Postgres Connection
	db, err := sql.Open("pgx", os.Getenv("DB_URL"))
	if err != nil {
		log.Printf("❌ DB Driver Error: %v", err)
	}
	if err = db.Ping(); err != nil {
		log.Printf("❌ Postgres Ping Failed: %v", err)
	} else {
		log.Println("✅ Postgres Connected!")
	}

	// 3. Start Server
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Payments Idempotency API is Running!")
	})

	log.Println("🚀 Server running on port 8080...")
	http.ListenAndServe(":8080", nil)
}
