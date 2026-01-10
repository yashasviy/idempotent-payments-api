package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/yashasviy/idempotent-payments-api/models"
)

func TransferHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req models.TransferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid Body", http.StatusBadRequest)
			return
		}

		// Get the Idempotency Key from headers
		idempotencyKey := r.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			http.Error(w, "Missing Idempotency-Key header", http.StatusBadRequest)
			return
		}

		// start ACID transaction
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Database Error", http.StatusInternalServerError)
			return
		}

		// 1. Deduct from Sender
		// verify they have enough balance AND deduct in one query
		result, err := tx.Exec("UPDATE accounts SET balance = balance - $1 WHERE id = $2 AND balance >= $1", req.Amount, req.FromID)
		if err != nil {
			tx.Rollback()
			http.Error(w, "Transaction Failed", http.StatusInternalServerError)
			return
		}

		// Check for sufficient funds
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			tx.Rollback()
			http.Error(w, "Insufficient Funds", http.StatusUnprocessableEntity)
			return
		}

		// 2. Add to Receiver
		if _, err := tx.Exec("UPDATE accounts SET balance = balance + $1 WHERE id = $2", req.Amount, req.ToID); err != nil {
			tx.Rollback()
			http.Error(w, "Transaction Failed", http.StatusInternalServerError)
			return
		}

		// 3. Record the Transaction in the Ledger
		// ensures a permanent record
		if _, err := tx.Exec("INSERT INTO transactions (from_id, to_id, amount, idempotency_key) VALUES ($1, $2, $3, $4)", req.FromID, req.ToID, req.Amount, idempotencyKey); err != nil {
			tx.Rollback()
			http.Error(w, "Failed to record transaction", http.StatusInternalServerError)
			return
		}

		// 4. Commit the transaction
		if err := tx.Commit(); err != nil {
			http.Error(w, "Commit Failed", http.StatusInternalServerError)
			return
		}

		// Success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": "Transfer Complete",
			"amount":  req.Amount,
		})
	}
}
