package impl

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
)

// BucketMigrator handles migration of data from legacy bucket names to new bucket names.
// This ensures backward compatibility during the transition period.
type BucketMigrator struct {
	provider types.MetaStorageProvider
	logger   *zap.Logger
}

// NewBucketMigrator creates a new bucket migrator.
func NewBucketMigrator(provider types.MetaStorageProvider, logger *zap.Logger) *BucketMigrator {
	return &BucketMigrator{
		provider: provider,
		logger:   logger,
	}
}

// MigrateBuckets migrates data from legacy bucket names to new bucket names.
// This operation is idempotent and safe to run multiple times.
//
// Migration process:
// 1. For each legacy bucket that exists:
//    a. Check if the new bucket exists
//    b. If new bucket doesn't exist, create it
//    c. Copy all records from legacy bucket to new bucket
//    d. Verify all records were copied successfully
// 2. After successful migration, legacy buckets are kept for backward compatibility
//    (they can be deleted manually after verification)
//
// Returns an error if migration fails for any bucket.
func (m *BucketMigrator) MigrateBuckets(ctx context.Context) error {
	migrationMap := BucketMigrationMap()

	for oldBucket, newBucket := range migrationMap {
		// Check if legacy bucket exists
		if !m.provider.BucketExists(ctx, oldBucket) {
			m.logger.Debug("Legacy bucket does not exist, skipping migration",
				zap.String("legacy_bucket", oldBucket),
				zap.String("new_bucket", newBucket))
			continue
		}

		// Check if new bucket exists, create if not
		if !m.provider.BucketExists(ctx, newBucket) {
			m.logger.Info("Creating new bucket for migration",
				zap.String("new_bucket", newBucket))
			if err := m.provider.CreateBucket(ctx, newBucket); err != nil {
				return fmt.Errorf("failed to create new bucket %s: %w", newBucket, err)
			}
		}

		// Migrate data from legacy bucket to new bucket
		if err := m.migrateBucketData(ctx, oldBucket, newBucket); err != nil {
			return fmt.Errorf("failed to migrate data from %s to %s: %w", oldBucket, newBucket, err)
		}

		m.logger.Info("Successfully migrated bucket",
			zap.String("legacy_bucket", oldBucket),
			zap.String("new_bucket", newBucket))
	}

	return nil
}

// migrateBucketData copies all records from the old bucket to the new bucket.
// This operation is idempotent - if a record already exists in the new bucket,
// it will be overwritten with the value from the old bucket.
func (m *BucketMigrator) migrateBucketData(ctx context.Context, oldBucket, newBucket string) error {
	// List all records in the old bucket
	keyValues, err := m.provider.List(ctx, oldBucket, nil)
	if err != nil {
		return fmt.Errorf("failed to list records in old bucket: %w", err)
	}

	if len(keyValues) == 0 {
		m.logger.Debug("No records to migrate",
			zap.String("old_bucket", oldBucket),
			zap.String("new_bucket", newBucket))
		return nil
	}

	// Copy each record to the new bucket
	migratedCount := 0
	for _, kv := range keyValues {
		// Check if record already exists in new bucket
		existingValue, err := m.provider.Get(ctx, newBucket, kv.Key)
		if err == nil && existingValue != nil {
			// Record exists, compare values
			if string(existingValue) == string(kv.Value) {
				// Values match, skip (already migrated)
				continue
			}
			// Values differ, overwrite with value from old bucket
			m.logger.Debug("Overwriting existing record in new bucket",
				zap.String("new_bucket", newBucket),
				zap.String("key", string(kv.Key)))
		}

		// Copy record to new bucket
		if err := m.provider.Put(ctx, newBucket, kv.Key, kv.Value); err != nil {
			return fmt.Errorf("failed to copy record to new bucket (key: %s): %w", string(kv.Key), err)
		}
		migratedCount++
	}

	m.logger.Info("Migrated records from legacy bucket to new bucket",
		zap.String("old_bucket", oldBucket),
		zap.String("new_bucket", newBucket),
		zap.Int("total_records", len(keyValues)),
		zap.Int("migrated_records", migratedCount))

	return nil
}

// VerifyMigration verifies that all records from legacy buckets have been migrated to new buckets.
// Returns an error if verification fails.
func (m *BucketMigrator) VerifyMigration(ctx context.Context) error {
	migrationMap := BucketMigrationMap()

	for oldBucket, newBucket := range migrationMap {
		// Skip if legacy bucket doesn't exist
		if !m.provider.BucketExists(ctx, oldBucket) {
			continue
		}

		// List records in both buckets
		oldRecords, err := m.provider.List(ctx, oldBucket, nil)
		if err != nil {
			return fmt.Errorf("failed to list records in legacy bucket %s: %w", oldBucket, err)
		}

		newRecords, err := m.provider.List(ctx, newBucket, nil)
		if err != nil {
			return fmt.Errorf("failed to list records in new bucket %s: %w", newBucket, err)
		}

		// Create a map of new bucket records for quick lookup
		newRecordsMap := make(map[string][]byte)
		for _, kv := range newRecords {
			newRecordsMap[string(kv.Key)] = kv.Value
		}

		// Verify all old records exist in new bucket with matching values
		missingCount := 0
		mismatchCount := 0
		for _, oldKV := range oldRecords {
			key := string(oldKV.Key)
			newValue, exists := newRecordsMap[key]
			if !exists {
				missingCount++
				m.logger.Warn("Record missing in new bucket",
					zap.String("old_bucket", oldBucket),
					zap.String("new_bucket", newBucket),
					zap.String("key", key))
			} else if string(newValue) != string(oldKV.Value) {
				mismatchCount++
				m.logger.Warn("Record value mismatch between buckets",
					zap.String("old_bucket", oldBucket),
					zap.String("new_bucket", newBucket),
					zap.String("key", key))
			}
		}

		if missingCount > 0 || mismatchCount > 0 {
			return fmt.Errorf("migration verification failed for %s -> %s: %d missing, %d mismatched",
				oldBucket, newBucket, missingCount, mismatchCount)
		}

		m.logger.Info("Migration verification passed",
			zap.String("old_bucket", oldBucket),
			zap.String("new_bucket", newBucket),
			zap.Int("total_records", len(oldRecords)))
	}

	return nil
}

// RollbackMigration rolls back a migration by copying records from new buckets back to legacy buckets.
// This is useful if migration needs to be undone.
// WARNING: This operation will overwrite existing records in legacy buckets.
func (m *BucketMigrator) RollbackMigration(ctx context.Context) error {
	migrationMap := BucketMigrationMap()

	for oldBucket, newBucket := range migrationMap {
		// Skip if new bucket doesn't exist
		if !m.provider.BucketExists(ctx, newBucket) {
			continue
		}

		// Ensure legacy bucket exists
		if !m.provider.BucketExists(ctx, oldBucket) {
			if err := m.provider.CreateBucket(ctx, oldBucket); err != nil {
				return fmt.Errorf("failed to create legacy bucket %s: %w", oldBucket, err)
			}
		}

		// Copy records from new bucket back to legacy bucket
		if err := m.migrateBucketData(ctx, newBucket, oldBucket); err != nil {
			return fmt.Errorf("failed to rollback migration from %s to %s: %w", newBucket, oldBucket, err)
		}

		m.logger.Info("Rolled back migration",
			zap.String("new_bucket", newBucket),
			zap.String("legacy_bucket", oldBucket))
	}

	return nil
}

