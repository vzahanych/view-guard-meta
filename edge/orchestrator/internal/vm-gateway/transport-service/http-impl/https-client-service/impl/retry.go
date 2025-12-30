package impl

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"

	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// RetryableError indicates whether an error should be retried.
type RetryableError struct {
	Err      error
	Retryable bool
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// isTransientError determines if an error is transient and should be retried.
func isTransientError(err error, statusCode int) bool {
	// Network errors are transient
	if err != nil {
		if netErr, ok := err.(net.Error); ok {
			if netErr.Timeout() || netErr.Temporary() {
				return true
			}
		}
		// DNS errors, connection refused, etc.
		if _, ok := err.(*net.OpError); ok {
			return true
		}
		return false
	}

	// HTTP status codes: 5xx are transient, 4xx are permanent (except 429)
	if statusCode >= 500 {
		return true
	}
	if statusCode == 429 { // Too Many Requests - retry with backoff
		return true
	}
	if statusCode == 408 { // Request Timeout - retry
		return true
	}

	// 4xx errors (except 429, 408) are permanent failures
	if statusCode >= 400 && statusCode < 500 {
		return false
	}

	// Other status codes (2xx, 3xx) are success - no retry needed
	return false
}

// isPermanentError determines if an error is permanent and should not be retried.
func isPermanentError(err error, statusCode int) bool {
	// Auth failures (401, 403) are permanent
	if statusCode == 401 || statusCode == 403 {
		return true
	}

	// 4xx errors (except 429, 408) are permanent
	if statusCode >= 400 && statusCode < 500 && statusCode != 429 && statusCode != 408 {
		return true
	}

	return false
}

// calculateBackoff calculates the backoff duration for a given attempt.
// Uses exponential backoff with optional jitter.
func calculateBackoff(
	attempt int,
	initialBackoff time.Duration,
	maxBackoff time.Duration,
	multiplier float64,
	jitterEnabled bool,
) time.Duration {
	// Calculate exponential backoff: initial * (multiplier ^ attempt)
	backoff := float64(initialBackoff) * math.Pow(multiplier, float64(attempt))
	
	// Cap at max backoff
	if backoff > float64(maxBackoff) {
		backoff = float64(maxBackoff)
	}

	duration := time.Duration(backoff)

	// Add jitter to prevent thundering herd
	if jitterEnabled {
		// Add random jitter: ±25% of the backoff duration
		jitterRange := float64(duration) * 0.25
		jitter := time.Duration((rand.Float64() * 2 * jitterRange) - jitterRange)
		duration = duration + jitter
		// Ensure duration doesn't go negative
		if duration < 0 {
			duration = 0
		}
	}

	return duration
}

// RetryConfig represents retry configuration for a specific operation.
type RetryConfig struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	JitterEnabled     bool
}

// RetryWithBackoff executes a function with retry logic and exponential backoff.
// It retries on transient errors and stops on permanent errors or when max retries is reached.
func RetryWithBackoff(
	ctx context.Context,
	config RetryConfig,
	logger *zap.Logger,
	operationName string,
	fn func() error,
) error {
	var lastErr error
	var lastStatusCode int

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Execute the operation
		err := fn()

		// If no error, success
		if err == nil {
			if attempt > 0 {
				logger.Info("Operation succeeded after retry",
					zap.String("operation", operationName),
					zap.Int("attempt", attempt+1))
			}
			return nil
		}

		lastErr = err

		// Extract HTTP status code if available
		// This is a simplified check - in practice, you'd need to extract it from the error
		// For now, we'll check if it's a transient error based on the error type
		lastStatusCode = 0
		if retryableErr, ok := err.(*RetryableError); ok {
			if !retryableErr.Retryable {
				// Permanent error - don't retry
				logger.Warn("Permanent error, not retrying",
					zap.String("operation", operationName),
					zap.Error(err))
				return err
			}
		}

		// Check if error is transient
		if !isTransientError(err, lastStatusCode) {
			// Permanent error - don't retry
			logger.Warn("Permanent error, not retrying",
				zap.String("operation", operationName),
				zap.Error(err))
			return err
		}

		// If this was the last attempt, return the error
		if attempt >= config.MaxRetries {
			logger.Error("Operation failed after all retries",
				zap.String("operation", operationName),
				zap.Int("max_retries", config.MaxRetries),
				zap.Error(err))
			return fmt.Errorf("operation failed after %d retries: %w", config.MaxRetries, err)
		}

		// Calculate backoff duration
		backoff := calculateBackoff(
			attempt,
			config.InitialBackoff,
			config.MaxBackoff,
			config.BackoffMultiplier,
			config.JitterEnabled,
		)

		logger.Warn("Operation failed, retrying",
			zap.String("operation", operationName),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", config.MaxRetries),
			zap.Duration("backoff", backoff),
			zap.Error(err))

		// Wait for backoff duration or context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("operation cancelled: %w", ctx.Err())
		case <-time.After(backoff):
			// Continue to next retry
		}
	}

	return lastErr
}

// RetryHTTPRequest executes an HTTP request with retry logic.
// It extracts status codes from HTTP responses to determine retryability.
func RetryHTTPRequest(
	ctx context.Context,
	retryConfig RetryConfig,
	logger *zap.Logger,
	operationName string,
	httpClient *http.Client,
	req *http.Request,
) (*http.Response, error) {
	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		// Execute the HTTP request
		resp, err := httpClient.Do(req.WithContext(ctx))

		// If no error and successful status, return success
		if err == nil && resp != nil {
			// Check if status code indicates success
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if attempt > 0 {
					logger.Info("HTTP request succeeded after retry",
						zap.String("operation", operationName),
						zap.Int("attempt", attempt+1),
						zap.Int("status_code", resp.StatusCode))
				}
				return resp, nil
			}

			// Check if status code indicates permanent failure
			if isPermanentError(nil, resp.StatusCode) {
				logger.Warn("HTTP request failed with permanent error, not retrying",
					zap.String("operation", operationName),
					zap.Int("status_code", resp.StatusCode))
				return resp, fmt.Errorf("permanent error: HTTP %d", resp.StatusCode)
			}

			// Transient error - will retry
			lastResp = resp
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else if err != nil {
			// Network error
			lastErr = err
			lastResp = nil
		} else {
			// No error but no response (shouldn't happen)
			lastErr = fmt.Errorf("no response received")
			lastResp = nil
		}

		// Check if error is transient
		statusCode := 0
		if lastResp != nil {
			statusCode = lastResp.StatusCode
		}
		if !isTransientError(lastErr, statusCode) {
			// Permanent error - don't retry
			if lastResp != nil {
				return lastResp, lastErr
			}
			return nil, lastErr
		}

		// If this was the last attempt, return the error
		if attempt >= retryConfig.MaxRetries {
			logger.Error("HTTP request failed after all retries",
				zap.String("operation", operationName),
				zap.Int("max_retries", retryConfig.MaxRetries),
				zap.Error(lastErr))
			if lastResp != nil {
				return lastResp, fmt.Errorf("request failed after %d retries: %w", retryConfig.MaxRetries, lastErr)
			}
			return nil, fmt.Errorf("request failed after %d retries: %w", retryConfig.MaxRetries, lastErr)
		}

		// Calculate backoff duration
		backoff := calculateBackoff(
			attempt,
			retryConfig.InitialBackoff,
			retryConfig.MaxBackoff,
			retryConfig.BackoffMultiplier,
			retryConfig.JitterEnabled,
		)

		logger.Warn("HTTP request failed, retrying",
			zap.String("operation", operationName),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", retryConfig.MaxRetries),
			zap.Duration("backoff", backoff),
			zap.Error(lastErr))

		// Close previous response body if exists
		if lastResp != nil && lastResp.Body != nil {
			lastResp.Body.Close()
		}

		// Wait for backoff duration or context cancellation
		select {
		case <-ctx.Done():
			if lastResp != nil {
				return lastResp, fmt.Errorf("request cancelled: %w", ctx.Err())
			}
			return nil, fmt.Errorf("request cancelled: %w", ctx.Err())
		case <-time.After(backoff):
			// Continue to next retry
		}
	}

	if lastResp != nil {
		return lastResp, lastErr
	}
	return nil, lastErr
}

// GetRetryConfigForAuthentication returns retry configuration for authentication.
// Uses custom backoff: 10s, 20s, 40s, max 5min.
func GetRetryConfigForAuthentication(baseConfig vmgatewaytypes.RetryConfig) RetryConfig {
	return RetryConfig{
		MaxRetries:        baseConfig.GetMaxRetries(),
		InitialBackoff:    10 * time.Second, // Custom: 10s initial
		MaxBackoff:        5 * time.Minute,  // Custom: 5min max
		BackoffMultiplier: baseConfig.GetBackoffMultiplier(),
		JitterEnabled:     baseConfig.IsJitterEnabled(),
	}
}

// GetRetryConfigForVMAPI returns retry configuration for VM API calls.
// Uses standard backoff: 1s, 2s, 4s, 8s, max 60s.
func GetRetryConfigForVMAPI(baseConfig vmgatewaytypes.RetryConfig) RetryConfig {
	return RetryConfig{
		MaxRetries:        baseConfig.GetMaxRetries(),
		InitialBackoff:    baseConfig.GetInitialBackoff(), // Standard: 1s
		MaxBackoff:        baseConfig.GetMaxBackoff(),      // Standard: 60s
		BackoffMultiplier: baseConfig.GetBackoffMultiplier(),
		JitterEnabled:     baseConfig.IsJitterEnabled(),
	}
}

