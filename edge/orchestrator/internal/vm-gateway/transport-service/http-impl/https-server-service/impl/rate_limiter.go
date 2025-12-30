package impl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"

	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// TokenBucket represents a token bucket for rate limiting.
// The token bucket algorithm allows bursts up to the bucket size,
// and refills tokens at a constant rate.
type TokenBucket struct {
	capacity     int           // Maximum number of tokens (burst size)
	tokens       float64       // Current number of tokens
	refillRate   float64       // Tokens per second
	lastRefill   time.Time     // Last time tokens were refilled
	mu           sync.Mutex    // Protects token bucket state
}

// NewTokenBucket creates a new token bucket with the specified capacity and refill rate.
func NewTokenBucket(capacity int, requestsPerMinute int) *TokenBucket {
	refillRate := float64(requestsPerMinute) / 60.0 // Tokens per second
	return &TokenBucket{
		capacity:   capacity,
		tokens:      float64(capacity), // Start with full bucket
		refillRate:  refillRate,
		lastRefill:  time.Now(),
	}
}

// Allow checks if a request is allowed and consumes a token if available.
// Returns true if the request is allowed, false otherwise.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	// Refill tokens based on elapsed time
	tb.tokens = tb.tokens + (tb.refillRate * elapsed)
	if tb.tokens > float64(tb.capacity) {
		tb.tokens = float64(tb.capacity)
	}
	tb.lastRefill = now

	// Check if we have enough tokens
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// Reset resets the token bucket to full capacity.
func (tb *TokenBucket) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.tokens = float64(tb.capacity)
	tb.lastRefill = time.Now()
}

// RateLimiter implements rate limiting for VM commands using token bucket algorithm.
// Tracks requests per client (identified by certificate fingerprint) and per endpoint.
type RateLimiter struct {
	config      *vmgatewaytypes.RateLimitConfig
	logger      *zap.Logger
	eventBus    eventbus.EventBus
	buckets     map[string]*TokenBucket // Key: "{clientFingerprint}:{endpoint}"
	bucketsMu   sync.RWMutex
	cleanupTicker *time.Ticker
	cleanupStop   chan struct{}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(
	config *vmgatewaytypes.RateLimitConfig,
	logger *zap.Logger,
	eventBus eventbus.EventBus,
) *RateLimiter {
	rl := &RateLimiter{
		config:    config,
		logger:    logger,
		eventBus:  eventBus,
		buckets:   make(map[string]*TokenBucket),
		cleanupStop: make(chan struct{}),
	}

	// Start cleanup goroutine to remove unused buckets periodically
	rl.cleanupTicker = time.NewTicker(5 * time.Minute)
	go rl.cleanupUnusedBuckets()

	return rl
}

// Stop stops the rate limiter and cleans up resources.
func (rl *RateLimiter) Stop() {
	if rl.cleanupTicker != nil {
		rl.cleanupTicker.Stop()
	}
	close(rl.cleanupStop)
}

// cleanupUnusedBuckets periodically removes unused token buckets to prevent memory leaks.
func (rl *RateLimiter) cleanupUnusedBuckets() {
	for {
		select {
		case <-rl.cleanupTicker.C:
			rl.bucketsMu.Lock()
			// Remove buckets that haven't been used in the last 10 minutes
			// (simple cleanup - in production, could use LRU cache)
			cutoff := time.Now().Add(-10 * time.Minute)
			for key, bucket := range rl.buckets {
				bucket.mu.Lock()
				lastRefill := bucket.lastRefill
				bucket.mu.Unlock()
				if lastRefill.Before(cutoff) {
					delete(rl.buckets, key)
				}
			}
			rl.bucketsMu.Unlock()
		case <-rl.cleanupStop:
			return
		}
	}
}

// getClientFingerprint extracts the client certificate fingerprint from the request.
// Returns empty string if no client certificate is present.
func (rl *RateLimiter) getClientFingerprint(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return ""
	}

	// Use the first peer certificate (client certificate in mTLS)
	cert := r.TLS.PeerCertificates[0]
	fingerprint := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(fingerprint[:])
}

// getBucketKey generates a key for the token bucket based on client fingerprint and endpoint.
func (rl *RateLimiter) getBucketKey(clientFingerprint string, endpoint string) string {
	return fmt.Sprintf("%s:%s", clientFingerprint, endpoint)
}

// getOrCreateBucket gets or creates a token bucket for the given client and endpoint.
func (rl *RateLimiter) getOrCreateBucket(clientFingerprint string, endpoint string) *TokenBucket {
	key := rl.getBucketKey(clientFingerprint, endpoint)

	rl.bucketsMu.RLock()
	bucket, exists := rl.buckets[key]
	rl.bucketsMu.RUnlock()

	if exists {
		return bucket
	}

	// Create new bucket
	limit := rl.config.GetLimitForEndpoint(endpoint)
	burstSize := rl.config.GetBurstSize()
	bucket = NewTokenBucket(burstSize, limit)

	rl.bucketsMu.Lock()
	// Double-check after acquiring write lock
	if existingBucket, exists := rl.buckets[key]; exists {
		rl.bucketsMu.Unlock()
		return existingBucket
	}
	rl.buckets[key] = bucket
	rl.bucketsMu.Unlock()

	return bucket
}

// Allow checks if a request is allowed based on rate limiting rules.
// Returns true if allowed, false if rate limited.
// Also returns the retry-after duration in seconds if rate limited.
func (rl *RateLimiter) Allow(r *http.Request) (bool, int) {
	if !rl.config.Enabled {
		return true, 0
	}

	// Get client fingerprint
	clientFingerprint := rl.getClientFingerprint(r)
	if clientFingerprint == "" {
		// No client certificate - allow request (mTLS should handle this)
		rl.logger.Warn("Rate limiter: no client certificate found in request",
			zap.String("endpoint", r.URL.Path))
		return true, 0
	}

	// Get endpoint path
	endpoint := r.URL.Path

	// Get or create bucket for this client+endpoint combination
	bucket := rl.getOrCreateBucket(clientFingerprint, endpoint)

	// Check if request is allowed
	if bucket.Allow() {
		return true, 0
	}

	// Rate limited - calculate retry-after
	// Estimate retry-after based on refill rate
	limit := rl.config.GetLimitForEndpoint(endpoint)
	retryAfter := 60 / limit // Seconds until next token is available (approximate)
	if retryAfter < 1 {
		retryAfter = 1
	}

	rl.logger.Warn("Rate limit exceeded",
		zap.String("client_fingerprint", clientFingerprint),
		zap.String("endpoint", endpoint),
		zap.Int("limit_per_minute", limit),
		zap.Int("retry_after_seconds", retryAfter))

	// Emit rate limit exceeded event
	if rl.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.RateLimitExceededEventData]{
			Type:      evtbusstypes.EventType("vm_gateway.rate_limit_exceeded"),
			Source:    "vm-gateway",
			Timestamp: time.Now(),
			Data: evtbusstypes.RateLimitExceededEventData{
				ClientFingerprint: clientFingerprint,
				Endpoint:          endpoint,
				LimitPerMinute:    limit,
				RetryAfterSeconds: retryAfter,
			},
		}
		if err := eventbus.PublishTyped(rl.eventBus, event); err != nil {
			rl.logger.Warn("Failed to publish rate limit exceeded event", zap.Error(err))
		}
	}

	return false, retryAfter
}

// GetStats returns rate limiting statistics.
func (rl *RateLimiter) GetStats() (enabled bool, requestsPerMinute int, burstSize int, totalViolations int64, activeBuckets int) {
	if rl.config == nil || !rl.config.Enabled {
		return false, 0, 0, 0, 0
	}

	rl.bucketsMu.RLock()
	activeBuckets = len(rl.buckets)
	rl.bucketsMu.RUnlock()

	return true, rl.config.RequestsPerMinute, rl.config.BurstSize, 0, activeBuckets // TODO: Track totalViolations
}

// RateLimitMiddleware creates an HTTP middleware function for rate limiting.
func RateLimitMiddleware(rateLimiter *RateLimiter, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := rateLimiter.Allow(r)
			if !allowed {
				// Return HTTP 429 Too Many Requests
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				
				errorResponse := map[string]interface{}{
					"error":       "Rate limit exceeded",
					"retry_after": retryAfter,
					"message":     fmt.Sprintf("Too many requests. Please retry after %d seconds.", retryAfter),
				}
				
				if _, err := w.Write([]byte(fmt.Sprintf(`{"error":"%s","retry_after":%d,"message":"%s"}`, 
					errorResponse["error"], retryAfter, errorResponse["message"]))); err != nil {
					logger.Warn("Failed to write rate limit response", zap.Error(err))
				}
				return
			}

			// Request allowed - proceed to next handler
			next.ServeHTTP(w, r)
		})
	}
}

