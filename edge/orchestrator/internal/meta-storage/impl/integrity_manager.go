package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

// IntegrityError represents an integrity error found during verification.
type IntegrityError struct {
	// Type is the type of integrity error (corrupted_record, missing_bucket, orphaned_record, etc.)
	Type string

	// Bucket is the bucket where the error was found
	Bucket string

	// Key is the record key (if applicable)
	Key string

	// Message is a human-readable error message
	Message string
}

// IntegrityReport contains the results of a database integrity verification.
type IntegrityReport struct {
	// Timestamp is when the verification was performed
	Timestamp time.Time

	// IsHealthy indicates whether the database is healthy (no errors found)
	IsHealthy bool

	// ErrorCount is the total number of integrity errors found
	ErrorCount int

	// Errors is a list of all integrity errors found
	Errors []IntegrityError

	// BucketsChecked is the number of buckets checked
	BucketsChecked int

	// RecordsChecked is the total number of records checked
	RecordsChecked int

	// ProviderHealth is the provider-specific health status
	ProviderHealth string
}

// IntegrityManager manages database integrity verification and corruption detection.
// It performs integrity checks, detects corruption, and provides recovery suggestions.
type IntegrityManager struct {
	provider     types.MetaStorageProvider
	logger       *zap.Logger
	eventEmitter types.StorageEventEmitter // Optional event emitter for emitting corruption events

	// Integrity state
	mu            sync.RWMutex
	lastCheck     time.Time
	lastReport    *IntegrityReport
	errorCount    int
	checkRunning  bool
}

// NewIntegrityManager creates a new integrity manager.
func NewIntegrityManager(provider types.MetaStorageProvider, logger *zap.Logger) *IntegrityManager {
	return &IntegrityManager{
		provider: provider,
		logger:   logger,
	}
}

// SetEventEmitter sets the event emitter for this integrity manager.
// This is optional - if not set, events will not be emitted.
func (i *IntegrityManager) SetEventEmitter(eventEmitter types.StorageEventEmitter) {
	i.eventEmitter = eventEmitter
}

// VerifyDatabaseIntegrity performs a comprehensive integrity check of the database.
// This operation:
// 1. Checks database file integrity (provider-specific)
// 2. Verifies bucket existence and accessibility
// 3. Verifies record format (JSON unmarshaling)
// 4. Checks for orphaned records (references to non-existent objects)
// 5. Returns an integrity report
func (i *IntegrityManager) VerifyDatabaseIntegrity(ctx context.Context) (*IntegrityReport, error) {
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

	// Step 2: Verify bucket existence and accessibility
	buckets := AllStandardBuckets()
	report.BucketsChecked = len(buckets)

	for _, bucketName := range buckets {
		if !i.provider.BucketExists(ctx, bucketName) {
			// Missing bucket is not necessarily an error (may not be created yet)
			// But we log it for awareness
			if i.logger != nil {
				i.logger.Debug("Bucket does not exist",
					zap.String("bucket", bucketName))
			}
			continue
		}

		// Step 3: Verify records in bucket
		bucketErrors, recordsChecked, err := i.verifyBucketRecords(ctx, bucketName)
		if err != nil {
			report.Errors = append(report.Errors, IntegrityError{
				Type:    "bucket_verification_failed",
				Bucket:  bucketName,
				Message: fmt.Sprintf("Failed to verify bucket: %v", err),
			})
			report.ErrorCount++
			continue
		}

		report.Errors = append(report.Errors, bucketErrors...)
		report.ErrorCount += len(bucketErrors)
		report.RecordsChecked += recordsChecked
	}

	// Step 4: Check for orphaned records (references to non-existent objects)
	// This is a placeholder - full implementation would check references between buckets
	// For example, check if device IDs in data_units exist in devices bucket
	orphanedErrors := i.checkOrphanedRecords(ctx)
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
			i.logger.Info("Database integrity check passed",
				zap.Int("buckets_checked", report.BucketsChecked),
				zap.Int("records_checked", report.RecordsChecked))
		} else {
			i.logger.Warn("Database integrity check found errors",
				zap.Int("error_count", report.ErrorCount),
				zap.Int("buckets_checked", report.BucketsChecked),
				zap.Int("records_checked", report.RecordsChecked))
		}
	}

	// Emit corruption detected event if errors found
	if !report.IsHealthy && i.eventEmitter != nil {
		errorDetails := fmt.Sprintf("Found %d integrity errors across %d buckets", report.ErrorCount, report.BucketsChecked)
		i.eventEmitter.EmitStorageEvent("storage.corruption_detected", map[string]interface{}{
			"error_count":   report.ErrorCount,
			"error_details": errorDetails,
		})
	}

	return report, nil
}

// verifyBucketRecords verifies all records in a bucket.
// It checks record format (JSON unmarshaling) and returns any errors found.
func (i *IntegrityManager) verifyBucketRecords(ctx context.Context, bucketName string) ([]IntegrityError, int, error) {
	errors := make([]IntegrityError, 0)
	recordsChecked := 0

	// List all records in the bucket
	keyValues, err := i.provider.List(ctx, bucketName, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list records in bucket %s: %w", bucketName, err)
	}

	for _, kv := range keyValues {
		recordsChecked++

		// Try to unmarshal as JSON to verify format
		var record map[string]interface{}
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			errors = append(errors, IntegrityError{
				Type:    "corrupted_record",
				Bucket:  bucketName,
				Key:     string(kv.Key),
				Message: fmt.Sprintf("Failed to unmarshal record as JSON: %v", err),
			})
			continue
		}

		// Additional validation could be added here based on bucket type
		// For example, verify required fields exist, check data types, etc.
	}

	return errors, recordsChecked, nil
}

// checkOrphanedRecords checks for orphaned records (references to non-existent objects).
// This is a placeholder implementation - full implementation would check cross-bucket references.
func (i *IntegrityManager) checkOrphanedRecords(ctx context.Context) []IntegrityError {
	// TODO: Implement orphaned record checking
	// Examples:
	// - Check if device IDs in data_units exist in devices bucket
	// - Check if model IDs in model_deployments exist
	// - Check if event IDs in security_events are valid
	// For now, return empty list
	return []IntegrityError{}
}

// DetectCorruption detects corruption in the database.
// This method performs integrity checks and identifies corruption indicators.
// Returns ErrCorruptionDetected if corruption is found.
func (i *IntegrityManager) DetectCorruption(ctx context.Context) error {
	report, err := i.VerifyDatabaseIntegrity(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify database integrity: %w", err)
	}

	if !report.IsHealthy {
		// Check for critical corruption indicators
		criticalErrors := 0
		for _, integrityError := range report.Errors {
			switch integrityError.Type {
			case "corrupted_record", "provider_health_check_failed":
				criticalErrors++
			}
		}

		if criticalErrors > 0 {
			if i.logger != nil {
				i.logger.Error("Corruption detected in database",
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

// GetCorruptionRecoverySuggestions returns suggestions for recovering from corruption.
// This provides VM-assisted resync recommendations based on detected corruption.
func (i *IntegrityManager) GetCorruptionRecoverySuggestions(ctx context.Context) ([]string, error) {
	report, err := i.VerifyDatabaseIntegrity(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to verify database integrity: %w", err)
	}

	suggestions := make([]string, 0)

	if !report.IsHealthy {
		// Analyze errors and provide recovery suggestions
		hasCorruptedRecords := false
		hasMissingBuckets := false
		affectedBuckets := make(map[string]bool)

		for _, integrityError := range report.Errors {
			affectedBuckets[integrityError.Bucket] = true

			switch integrityError.Type {
			case "corrupted_record":
				hasCorruptedRecords = true
			case "missing_bucket":
				hasMissingBuckets = true
			}
		}

		if hasCorruptedRecords {
			suggestions = append(suggestions,
				"Corrupted records detected. Request VM-assisted resync for affected buckets.",
				"VM resync will restore metadata from authoritative source.")
		}

		if hasMissingBuckets {
			suggestions = append(suggestions,
				"Missing buckets detected. Run schema migrations to create required buckets.",
				"If migrations fail, request VM-assisted resync.")
		}

		if len(affectedBuckets) > 0 {
			bucketList := ""
			for bucket := range affectedBuckets {
				if bucketList != "" {
					bucketList += ", "
				}
				bucketList += bucket
			}
			suggestions = append(suggestions,
				fmt.Sprintf("Affected buckets: %s", bucketList),
				"Request VM-assisted resync for affected buckets.")
		}

		// General recovery suggestion
		if len(suggestions) == 0 {
			suggestions = append(suggestions,
				"Integrity errors detected. Request VM-assisted resync to restore database integrity.")
		}
	}

	return suggestions, nil
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

