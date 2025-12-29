package impl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

// IntegrityError represents an integrity error found during verification.
type IntegrityError struct {
	// Type is the type of integrity error (hash_mismatch, missing_object, orphaned_object, etc.)
	Type string

	// Key is the object key where the error was found
	Key string

	// Message is a human-readable error message
	Message string
}

// IntegrityReport contains the results of a storage integrity verification.
type IntegrityReport struct {
	// Timestamp is when the verification was performed
	Timestamp time.Time

	// IsHealthy indicates whether the storage is healthy (no errors found)
	IsHealthy bool

	// ErrorCount is the total number of integrity errors found
	ErrorCount int

	// Errors is a list of all integrity errors found
	Errors []IntegrityError

	// ObjectsChecked is the total number of objects checked
	ObjectsChecked int

	// ProviderHealth is the provider-specific health status
	ProviderHealth string
}

// IntegrityManager manages storage integrity verification and corruption detection for object storage.
// It performs hash verification, detects corruption, and provides recovery suggestions.
type IntegrityManager struct {
	provider     types.ObjectStorageProvider
	logger       *zap.Logger
	eventEmitter StorageEventEmitter // Optional event emitter for emitting corruption events

	// Integrity state
	mu           sync.RWMutex
	lastCheck    time.Time
	lastReport   *IntegrityReport
	errorCount   int
	checkRunning bool
}

// NewIntegrityManager creates a new integrity manager for object storage.
func NewIntegrityManager(provider types.ObjectStorageProvider, logger *zap.Logger) *IntegrityManager {
	return &IntegrityManager{
		provider: provider,
		logger:   logger,
	}
}

// SetEventEmitter sets the event emitter for this integrity manager.
// This is optional - if not set, events will not be emitted.
func (i *IntegrityManager) SetEventEmitter(eventEmitter StorageEventEmitter) {
	i.eventEmitter = eventEmitter
}

// CalculateHash calculates the SHA-256 hash of the given data.
func (i *IntegrityManager) CalculateHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// CalculateHashFromReader calculates the SHA-256 hash of data read from a reader.
func (i *IntegrityManager) CalculateHashFromReader(r io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, r); err != nil {
		return "", fmt.Errorf("failed to calculate hash: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// VerifyObjectIntegrity verifies the integrity of a single object by checking its hash.
func (i *IntegrityManager) VerifyObjectIntegrity(ctx context.Context, key string) error {
	// Get object metadata
	metadata, err := i.provider.GetObjectMetadata(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get object metadata: %w", err)
	}

	// If no hash is stored, we can't verify (object may be from before integrity tracking)
	if metadata.Hash == "" {
		return nil // Not an error, just can't verify
	}

	// Load object
	reader, err := i.provider.LoadObject(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to load object: %w", err)
	}
	defer reader.Close()

	// Calculate hash of loaded content
	calculatedHash, err := i.CalculateHashFromReader(reader)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Compare hashes
	if calculatedHash != metadata.Hash {
		return fmt.Errorf("hash mismatch for object %s: expected %s, got %s", key, metadata.Hash, calculatedHash)
	}

	return nil
}

// VerifyStorageIntegrity performs a comprehensive integrity check of the storage.
// This operation:
// 1. Checks provider health
// 2. Samples objects and verifies hashes
// 3. Checks for missing objects referenced by meta-storage (placeholder)
// 4. Checks for orphaned objects (not referenced by meta-storage) (placeholder)
// 5. Returns an integrity report
func (i *IntegrityManager) VerifyStorageIntegrity(ctx context.Context) (*IntegrityReport, error) {
	i.mu.Lock()
	if i.checkRunning {
		i.mu.Unlock()
		return nil, fmt.Errorf("integrity check already running")
	}
	i.checkRunning = true
	i.mu.Unlock()

	defer func() {
		i.mu.Lock()
		i.checkRunning = false
		i.mu.Unlock()
	}()

	report := &IntegrityReport{
		Timestamp: time.Now(),
		Errors:    make([]IntegrityError, 0),
	}

	// Step 1: Check provider health
	if err := i.provider.HealthCheck(ctx); err != nil {
		report.ProviderHealth = "unhealthy"
		report.Errors = append(report.Errors, IntegrityError{
			Type:    "provider_health_check_failed",
			Message: fmt.Sprintf("Provider health check failed: %v", err),
		})
		report.ErrorCount++
	} else {
		report.ProviderHealth = "healthy"
	}

	// Step 2: Sample objects and verify hashes
	// List all objects
	objects, err := i.provider.ListObjects(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	// Sample objects for verification (check all objects that have hashes)
	// In production, we might want to sample a subset for performance
	sampleSize := len(objects)
	if sampleSize > 1000 {
		// For large storage, sample 1000 objects
		sampleSize = 1000
	}

	objectsChecked := 0
	for idx, obj := range objects {
		if idx >= sampleSize {
			break
		}

		objectsChecked++

		// Get metadata to check if hash exists
		metadata, err := i.provider.GetObjectMetadata(ctx, obj.Key)
		if err != nil {
			// Object might have been deleted, skip
			continue
		}

		// Skip objects without hashes (from before integrity tracking)
		if metadata.Hash == "" {
			continue
		}

		// Verify hash
		if err := i.VerifyObjectIntegrity(ctx, obj.Key); err != nil {
			report.Errors = append(report.Errors, IntegrityError{
				Type:    "hash_mismatch",
				Key:     obj.Key,
				Message: err.Error(),
			})
			report.ErrorCount++
		}
	}

	report.ObjectsChecked = objectsChecked

	// Step 3: Check for missing objects referenced by meta-storage
	// TODO: Integrate with meta-storage to check for missing objects
	// This would require querying meta-storage for object keys and verifying they exist
	missingErrors := i.checkMissingObjects(ctx)
	report.Errors = append(report.Errors, missingErrors...)
	report.ErrorCount += len(missingErrors)

	// Step 4: Check for orphaned objects (not referenced by meta-storage)
	// TODO: Integrate with meta-storage to check for orphaned objects
	// This would require querying meta-storage for all object keys and comparing
	orphanedErrors := i.checkOrphanedObjects(ctx)
	report.Errors = append(report.Errors, orphanedErrors...)
	report.ErrorCount += len(orphanedErrors)

	// Determine overall health
	report.IsHealthy = report.ErrorCount == 0

	// Update cached state
	i.mu.Lock()
	i.lastCheck = report.Timestamp
	i.lastReport = report
	i.errorCount = report.ErrorCount
	i.mu.Unlock()

	if i.logger != nil {
		if report.IsHealthy {
			i.logger.Info("Storage integrity check passed",
				zap.Int("objects_checked", report.ObjectsChecked))
		} else {
			i.logger.Warn("Storage integrity check found errors",
				zap.Int("error_count", report.ErrorCount),
				zap.Int("objects_checked", report.ObjectsChecked))
		}
	}

	// Emit corruption detected event if errors found
	if !report.IsHealthy && i.eventEmitter != nil {
		errorDetails := fmt.Sprintf("Found %d integrity errors in %d objects checked", report.ErrorCount, report.ObjectsChecked)
		i.eventEmitter.EmitStorageEvent("storage.corruption_detected", map[string]interface{}{
			"error_count":   report.ErrorCount,
			"error_details": errorDetails,
		})
	}

	return report, nil
}

// checkMissingObjects checks for missing objects referenced by meta-storage.
// This is a placeholder - full implementation would integrate with meta-storage.
func (i *IntegrityManager) checkMissingObjects(ctx context.Context) []IntegrityError {
	// TODO: Integrate with meta-storage to check for missing objects
	// This would require:
	// 1. Query meta-storage for all object keys (from data_units, model_deployments, etc.)
	// 2. For each key, verify the object exists in object storage
	// 3. Return errors for missing objects
	return []IntegrityError{}
}

// checkOrphanedObjects checks for orphaned objects (not referenced by meta-storage).
// This is a placeholder - full implementation would integrate with meta-storage.
func (i *IntegrityManager) checkOrphanedObjects(ctx context.Context) []IntegrityError {
	// TODO: Integrate with meta-storage to check for orphaned objects
	// This would require:
	// 1. List all objects in object storage
	// 2. Query meta-storage for all referenced object keys
	// 3. Find objects in storage that are not referenced in meta-storage
	// 4. Return errors for orphaned objects (or just log them)
	return []IntegrityError{}
}

// DetectCorruption detects corruption in the storage.
// This method performs integrity checks and identifies corruption indicators.
// Returns ErrCorruptionDetected if corruption is found.
func (i *IntegrityManager) DetectCorruption(ctx context.Context) error {
	report, err := i.VerifyStorageIntegrity(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify storage integrity: %w", err)
	}

	if !report.IsHealthy {
		// Check for critical corruption indicators
		criticalErrors := 0
		for _, integrityError := range report.Errors {
			switch integrityError.Type {
			case "hash_mismatch", "provider_health_check_failed":
				criticalErrors++
			}
		}

		if criticalErrors > 0 {
			if i.logger != nil {
				i.logger.Error("Corruption detected in storage",
					zap.Int("critical_errors", criticalErrors),
					zap.Int("total_errors", report.ErrorCount))
			}
			// Emit corruption detected event
			if i.eventEmitter != nil {
				errorDetails := fmt.Sprintf("Found %d critical errors out of %d total errors", criticalErrors, report.ErrorCount)
				i.eventEmitter.EmitStorageEvent("storage.corruption_detected", map[string]interface{}{
					"error_count":   report.ErrorCount,
					"error_details": errorDetails,
				})
			}
			return types.ErrCorruptionDetected
		}
	}

	return nil
}

// StartPeriodicIntegrityChecks starts a background goroutine that periodically runs integrity checks.
// This runs daily by default.
func (i *IntegrityManager) StartPeriodicIntegrityChecks(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour // Default: daily
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Initial check (after first interval)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := i.DetectCorruption(ctx); err != nil {
				if err == types.ErrCorruptionDetected {
					if i.logger != nil {
						i.logger.Error("Initial integrity check detected corruption", zap.Error(err))
					}
				} else if i.logger != nil {
					i.logger.Warn("Initial integrity check failed", zap.Error(err))
				}
			}
		}

		// Periodic checks
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := i.DetectCorruption(ctx); err != nil {
					if err == types.ErrCorruptionDetected {
						if i.logger != nil {
							i.logger.Error("Periodic integrity check detected corruption", zap.Error(err))
						}
					} else if i.logger != nil {
						i.logger.Warn("Periodic integrity check failed", zap.Error(err))
					}
				}
			}
		}
	}()
}

// StopPeriodicIntegrityChecks stops the periodic integrity checks.
// This is called when the service is stopped.
func (i *IntegrityManager) StopPeriodicIntegrityChecks() {
	// The periodic checks are managed by context cancellation,
	// so this is a no-op. The context passed to StartPeriodicIntegrityChecks
	// should be cancelled when the service stops.
}

// GetLastIntegrityReport returns the last integrity verification report.
func (i *IntegrityManager) GetLastIntegrityReport() *IntegrityReport {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if i.lastReport == nil {
		return nil
	}

	// Return a copy
	report := *i.lastReport
	report.Errors = make([]IntegrityError, len(i.lastReport.Errors))
	copy(report.Errors, i.lastReport.Errors)
	return &report
}

// GetErrorCount returns the current integrity error count.
func (i *IntegrityManager) GetErrorCount() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.errorCount
}

// GetLastCheckTime returns when the last integrity check was performed.
func (i *IntegrityManager) GetLastCheckTime() time.Time {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.lastCheck
}

// IsCheckRunning returns whether an integrity check is currently running.
func (i *IntegrityManager) IsCheckRunning() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.checkRunning
}

