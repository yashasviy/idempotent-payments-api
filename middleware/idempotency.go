package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	// IdempotencyHeader is the standard HTTP header for idempotency keys
	IdempotencyHeader = "Idempotency-Key"

	// IdempotencyCacheTTL defines how long responses are cached in Redis
	IdempotencyCacheTTL = 24 * time.Hour

	// LockTimeout prevents indefinite locks if a request crashes
	LockTimeout = 10 * time.Second

	// RedisKeyPrefix for namespacing idempotency keys
	RedisKeyPrefix = "idempotency:"

	// LockKeyPrefix for namespacing distributed locks
	LockKeyPrefix = "lock:"
)

// responseWriterWrapper captures HTTP responses for caching.
// It intercepts both the status code and response body to store in Redis.
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

// WriteHeader captures the HTTP status code before delegating to the underlying writer.
func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the response body while also writing to the client.
func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	rw.body.Write(b)
	return rw.ResponseWriter.Write(b)
}

// Idempotency is middleware that implements request idempotency using Redis.
// It prevents duplicate processing of identical requests by caching responses.
//
// Flow:
//  1. Extract idempotency key from request headers
//  2. Check Redis cache for existing response
//  3. Acquire distributed lock to prevent race conditions
//  4. Process request if not cached
//  5. Store successful responses in Redis with TTL

func Idempotency(rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.Background()

			// Extract idempotency key from standard header
			idempotencyKey := r.Header.Get(IdempotencyHeader)
			if idempotencyKey == "" {
				// No idempotency key provided - process request normally
				next.ServeHTTP(w, r)
				return
			}

			// Namespace keys to avoid collisions
			cacheKey := RedisKeyPrefix + idempotencyKey
			lockKey := LockKeyPrefix + idempotencyKey

			// Check if this request was previously processed
			cachedResponse, err := rdb.Get(ctx, cacheKey).Result()
			if err == nil {
				// Cache hit - return stored response immediately
				log.Printf("[Idempotency] Cache hit for key: %s", idempotencyKey)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Idempotency-Hit", "true")
				w.Write([]byte(cachedResponse))
				return
			}

			// Acquire distributed lock to prevent concurrent duplicate requests
			acquired, err := rdb.SetNX(ctx, lockKey, "processing", LockTimeout).Result()
			if err != nil {
				log.Printf("[Idempotency] Lock acquisition error: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			if !acquired {
				// Another request with same key is currently processing
				log.Printf("[Idempotency] Concurrent request detected: %s", idempotencyKey)
				errorResponse := map[string]string{
					"error":   "conflict",
					"message": "A request with this idempotency key is currently being processed",
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(errorResponse)
				return
			}

			// Ensure lock is released after processing
			defer func() {
				if err := rdb.Del(ctx, lockKey).Err(); err != nil {
					log.Printf("[Idempotency] Failed to release lock: %v", err)
				}
			}()

			// Process the request and capture response
			wrapper := &responseWriterWrapper{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}
			next.ServeHTTP(wrapper, r)

			// Cache successful responses only (2xx status codes)
			if wrapper.statusCode >= 200 && wrapper.statusCode < 300 {
				if err := rdb.Set(ctx, cacheKey, wrapper.body.String(), IdempotencyCacheTTL).Err(); err != nil {
					log.Printf("[Idempotency] Failed to cache response: %v", err)
				} else {
					log.Printf("[Idempotency] Cached response for key: %s (TTL: %v)", idempotencyKey, IdempotencyCacheTTL)
				}
			}
		})
	}
}
