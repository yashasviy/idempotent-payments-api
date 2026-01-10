package models

// Transaction represents a money movement event
type Transaction struct {
	ID             string  `json:"id"`
	FromID         int     `json:"from_id"`
	ToID           int     `json:"to_id"`
	Amount         float64 `json:"amount"`
	IdempotencyKey string  `json:"idempotency_key"`
}

// TransferRequest is what the user sends in the API call
type TransferRequest struct {
	FromID int     `json:"from_id"`
	ToID   int     `json:"to_id"`
	Amount float64 `json:"amount"`
}
