package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/yashasviy/idempotent-payments-api/models"
)

// TransferHandler processes a money transfer with idempotency and recovery guards.
func TransferHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chaosEnabled := os.Getenv("CHAOS_MODE") == "true"

		var req models.TransferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid Body", http.StatusBadRequest)
			return
		}

		idempotencyKey := r.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			http.Error(w, "Missing Idempotency-Key header", http.StatusBadRequest)
			return
		}

		// Recovery check: if the transaction already exists, return the recorded result.
		var existingAmount float64
		err := db.QueryRow("SELECT amount FROM transactions WHERE idempotency_key = $1", idempotencyKey).Scan(&existingAmount)
		switch err {
		case nil:
			log.Println("Transaction recovered from database for idempotency key", idempotencyKey)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Db-Hit", "true")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "success",
				"message": "Transfer Complete (Recovered)",
				"amount":  existingAmount,
			})
			return
		case sql.ErrNoRows:
			// Safe to proceed; no prior record found.
		default:
			http.Error(w, "Database Error", http.StatusInternalServerError)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Database Error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback() // no-op if already committed

		// Deduct from sender while ensuring sufficient balance in a single statement.
		result, err := tx.Exec("UPDATE accounts SET balance = balance - $1 WHERE id = $2 AND balance >= $1", req.Amount, req.FromID)
		if err != nil {
			http.Error(w, "Transaction Failed", http.StatusInternalServerError)
			return
		}

		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, "Insufficient Funds", http.StatusUnprocessableEntity)
			return
		}

		if _, err := tx.Exec("UPDATE accounts SET balance = balance + $1 WHERE id = $2", req.Amount, req.ToID); err != nil {
			http.Error(w, "Transaction Failed", http.StatusInternalServerError)
			return
		}

		if _, err := tx.Exec("INSERT INTO transactions (from_id, to_id, amount, idempotency_key) VALUES ($1, $2, $3, $4)", req.FromID, req.ToID, req.Amount, idempotencyKey); err != nil {
			http.Error(w, "Failed to record transaction", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Commit Failed", http.StatusInternalServerError)
			return
		}

		// Chaos mode: simulate a post-commit network failure when requested.
		if chaosEnabled && r.Header.Get("X-Simulate-Chaos") == "true" {
			log.Println("Simulated crash after commit for idempotency key", idempotencyKey)
			panic("intentional chaos crash")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": "Transfer Complete",
			"amount":  req.Amount,
		})
	}
}
