package db

import (
	"database/sql"
	"log"
)

func Initialize(db *sql.DB) {
	// 1. Create Accounts Table
	queryAccounts := `
	CREATE TABLE IF NOT EXISTS accounts (
		id SERIAL PRIMARY KEY,
		balance DECIMAL(10, 2) NOT NULL
	);`

	if _, err := db.Exec(queryAccounts); err != nil {
		log.Fatal("Failed to create accounts table:", err)
	}

	// 2. Create Transactions Table
	// UNIQUE constraint on idempotency_key
	queryTransactions := `
	CREATE TABLE IF NOT EXISTS transactions (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		from_id INT NOT NULL,
		to_id INT NOT NULL,
		amount DECIMAL(10, 2) NOT NULL,
		idempotency_key VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(queryTransactions); err != nil {
		log.Fatal("Failed to create transactions table:", err)
	}

}
