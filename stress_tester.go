// Package main provides a concurrent stress test for idempotent payment APIs.
// It validates that the system prevents duplicate transactions even under
// high concurrency by sending multiple simultaneous requests with the same
// idempotency key.
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/jackc/pgx/v4/stdlib"
)

const (
	// DefaultURL is the target API endpoint
	DefaultURL = "http://localhost:8080/transfer"

	// DefaultConcurrency is the number of concurrent requests
	DefaultConcurrency = 50

	// DefaultAmount is the payment amount to test
	DefaultAmount = 10
)

// TestConfig holds the stress test configuration
type TestConfig struct {
	URL                string
	IdempotencyKey     string
	ConcurrentRequests int
	FromID             int
	ToID               int
	Amount             float64
}

// TestResults tracks the outcomes of all requests
type TestResults struct {
	SuccessCount  int32
	CacheHitCount int32
	ConflictCount int32
	ErrorCount    int32
	Duration      time.Duration
}

func main() {
	// Parse command-line flags
	config := TestConfig{}
	flag.StringVar(&config.URL, "url", DefaultURL, "API endpoint URL")
	flag.StringVar(&config.IdempotencyKey, "key", "stress-test-key-999", "Idempotency key for all requests")
	flag.IntVar(&config.ConcurrentRequests, "concurrent", DefaultConcurrency, "Number of concurrent requests")
	flag.IntVar(&config.FromID, "from", 1, "Sender account ID")
	flag.IntVar(&config.ToID, "to", 2, "Receiver account ID")
	flag.Float64Var(&config.Amount, "amount", DefaultAmount, "Payment amount")
	skipSetup := flag.Bool("skip-setup", false, "Skip database setup (accounts already exist)")
	flag.Parse()

	fmt.Println("  IDEMPOTENT PAYMENT API - CONCURRENT STRESS TEST")

	// Setup database if needed
	if !*skipSetup {
		fmt.Println("Setting up test environment...")
		if err := setupTestEnvironment(config.FromID, config.ToID, config.Amount); err != nil {
			log.Fatalf("Failed to setup test environment: %v", err)
		}
		fmt.Println("Test accounts created successfully")
	}

	fmt.Printf("Endpoint:       %s\n", config.URL)
	fmt.Printf("Idempotency:    %s\n", config.IdempotencyKey)
	fmt.Printf("Concurrency:    %d requests\n", config.ConcurrentRequests)
	fmt.Printf("Payment:        $%.2f from Account %d to Account %d\n", config.Amount, config.FromID, config.ToID)
	fmt.Println("---------------------------------------------------------------")

	results := runStressTest(config)
	printResults(results, config.ConcurrentRequests)
}

// setupTestEnvironment creates test accounts if they don't exist
func setupTestEnvironment(fromID, toID int, initialBalance float64) error {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = "postgres://user:password@localhost:5432/stripe_clone?sslmode=disable"
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Create tables if they don't exist
	createAccountsTable := `
		CREATE TABLE IF NOT EXISTS accounts (
			id SERIAL PRIMARY KEY,
			balance DECIMAL(10, 2) NOT NULL
		);`

	createTransactionsTable := `
		CREATE TABLE IF NOT EXISTS transactions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			from_id INT NOT NULL,
			to_id INT NOT NULL,
			amount DECIMAL(10, 2) NOT NULL,
			idempotency_key VARCHAR(255) UNIQUE NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`

	if _, err := db.Exec(createAccountsTable); err != nil {
		return fmt.Errorf("failed to create accounts table: %w", err)
	}

	if _, err := db.Exec(createTransactionsTable); err != nil {
		return fmt.Errorf("failed to create transactions table: %w", err)
	}

	// Check if accounts exist, create if not
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM accounts WHERE id IN ($1, $2)", fromID, toID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check accounts: %w", err)
	}

	if count < 2 {
		// Calculate required balance for sender
		requiredBalance := initialBalance * 100

		// Insert or update accounts
		_, err = db.Exec(`
			INSERT INTO accounts (id, balance) VALUES ($1, $2), ($3, 0)
			ON CONFLICT (id) DO UPDATE SET balance = EXCLUDED.balance`,
			fromID, requiredBalance, toID)
		if err != nil {
			return fmt.Errorf("failed to create accounts: %w", err)
		}
	}

	return nil
}

// runStressTest executes concurrent requests and returns aggregated results
func runStressTest(config TestConfig) TestResults {
	var (
		results TestResults
		wg      sync.WaitGroup
		start   = time.Now()
	)

	fmt.Printf("\nLaunching %d concurrent requests...\n", config.ConcurrentRequests)

	for i := 0; i < config.ConcurrentRequests; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()
			executeRequest(config, requestID, &results)
		}(i)
	}

	wg.Wait()
	results.Duration = time.Since(start)

	return results
}

// executeRequest sends a single HTTP request and updates results atomically
func executeRequest(config TestConfig, requestID int, results *TestResults) {
	payload := map[string]interface{}{
		"from_id": config.FromID,
		"to_id":   config.ToID,
		"amount":  config.Amount,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[Request %d] Failed to marshal JSON: %v", requestID, err)
		atomic.AddInt32(&results.ErrorCount, 1)
		return
	}

	req, err := http.NewRequest("POST", config.URL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("[Request %d] Failed to create request: %v", requestID, err)
		atomic.AddInt32(&results.ErrorCount, 1)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", config.IdempotencyKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[Request %d] HTTP error: %v", requestID, err)
		atomic.AddInt32(&results.ErrorCount, 1)
		return
	}
	defer resp.Body.Close()

	// Classify response
	switch {
	case resp.Header.Get("X-Idempotency-Hit") == "true":
		atomic.AddInt32(&results.CacheHitCount, 1)
	case resp.StatusCode == http.StatusOK:
		atomic.AddInt32(&results.SuccessCount, 1)
	case resp.StatusCode == http.StatusConflict:
		atomic.AddInt32(&results.ConflictCount, 1)
	default:
		log.Printf("[Request %d] Unexpected status: %d", requestID, resp.StatusCode)
		atomic.AddInt32(&results.ErrorCount, 1)
	}
}

// printResults displays formatted test results and pass/fail verdict
func printResults(results TestResults, totalRequests int) {
	fmt.Println("                    TEST RESULTS")
	fmt.Printf("Duration:                     %v\n", results.Duration)
	fmt.Printf("Requests per second:          %.2f\n", float64(totalRequests)/results.Duration.Seconds())
	fmt.Printf("[SUCCESS] Successful (First Process):  %d\n", results.SuccessCount)
	fmt.Printf("[CACHED]  Cache Hits (Redis Cached):   %d\n", results.CacheHitCount)
	fmt.Printf("[BLOCKED] Conflicts (Redis Locked):    %d\n", results.ConflictCount)
	fmt.Printf("[ERROR]   Network/Timeout Errors:      %d\n", results.ErrorCount)

	expectedDuplicates := int32(totalRequests) - 1
	actualDuplicates := results.CacheHitCount + results.ConflictCount

	if results.SuccessCount == 1 && actualDuplicates == expectedDuplicates && results.ErrorCount == 0 {
		fmt.Println("TEST PASSED: System is fully idempotent and thread-safe")
		fmt.Println("  * Only 1 transaction processed")
		fmt.Println("  * All duplicates handled correctly")
		fmt.Println("  * No double-spending detected")
	} else {
		fmt.Println("TEST FAILED: System has critical issues")
		if results.SuccessCount > 1 {
			fmt.Printf("  * WARNING: Multiple transactions processed (%d)\n", results.SuccessCount)
			fmt.Println("  * CRITICAL: Double-spending vulnerability detected")
		}
		if results.ErrorCount > 0 {
			fmt.Printf("  * Network/timeout errors: %d\n", results.ErrorCount)
		}
		if actualDuplicates != expectedDuplicates {
			fmt.Printf("  * Expected %d duplicates, got %d\n", expectedDuplicates, actualDuplicates)
		}
	}
}
