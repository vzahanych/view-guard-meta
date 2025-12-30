package impl

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// GenerateIdempotencyKey generates an idempotency key in the format: {EdgeID}-{operation}-{UUID}
// Example: "edge-123-sync-devices-550e8400-e29b-41d4-a716-446655440000"
func GenerateIdempotencyKey(edgeID string, operation string) string {
	// Generate UUID
	id := uuid.New().String()
	
	// Format: {EdgeID}-{operation}-{UUID}
	// Normalize operation name (lowercase, replace spaces/underscores with hyphens)
	normalizedOp := strings.ToLower(operation)
	normalizedOp = strings.ReplaceAll(normalizedOp, " ", "-")
	normalizedOp = strings.ReplaceAll(normalizedOp, "_", "-")
	
	return fmt.Sprintf("%s-%s-%s", edgeID, normalizedOp, id)
}

// EnsureIdempotencyKey ensures that a request has an idempotency key.
// If the key is empty, it generates a new one using the provided edgeID and operation.
// Returns the idempotency key (either the existing one or a newly generated one).
func EnsureIdempotencyKey(existingKey string, edgeID string, operation string, logger *zap.Logger) string {
	if existingKey != "" {
		logger.Debug("Using provided idempotency key",
			zap.String("key", existingKey),
			zap.String("operation", operation))
		return existingKey
	}
	
	newKey := GenerateIdempotencyKey(edgeID, operation)
	logger.Debug("Generated idempotency key",
		zap.String("key", newKey),
		zap.String("operation", operation),
		zap.String("edge_id", edgeID))
	return newKey
}

// ValidateIdempotencyKey validates that an idempotency key has the correct format.
// Returns true if the key is valid, false otherwise.
func ValidateIdempotencyKey(key string) bool {
	if key == "" {
		return false
	}
	
	// Check if key contains at least two hyphens (EdgeID-operation-UUID)
	parts := strings.Split(key, "-")
	if len(parts) < 3 {
		return false
	}
	
	// Check if the last part (UUID) is a valid UUID
	// UUID format: 8-4-4-4-12 hex characters
	uuidPart := parts[len(parts)-1]
	if len(uuidPart) != 36 { // UUID with hyphens: 8-4-4-4-12 = 36 chars
		// Try to parse as UUID (handles both with and without hyphens)
		_, err := uuid.Parse(uuidPart)
		if err != nil {
			return false
		}
	} else {
		// Validate UUID format
		_, err := uuid.Parse(uuidPart)
		if err != nil {
			return false
		}
	}
	
	return true
}

