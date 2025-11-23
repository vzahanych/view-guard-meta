package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// StorageStateManager implements StateManager using SQLite
type StorageStateManager struct {
	db     *sql.DB
	logger *logger.Logger
}

// NewStorageStateManager creates a new storage state manager
func NewStorageStateManager(db *sql.DB, log *logger.Logger) *StorageStateManager {
	return &StorageStateManager{
		db:     db,
		logger: log,
	}
}

// SaveStorageEntry saves a storage entry to the database
func (s *StorageStateManager) SaveStorageEntry(ctx context.Context, entry StorageEntry) error {
	query := `
		INSERT INTO storage_state (path, file_type, size_bytes, camera_id, event_id, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			size_bytes = excluded.size_bytes,
			camera_id = excluded.camera_id,
			event_id = excluded.event_id,
			expires_at = excluded.expires_at
	`

	var expiresAt interface{}
	if entry.ExpiresAt != nil {
		expiresAt = entry.ExpiresAt
	}

	// Convert empty strings to NULL for foreign keys
	var cameraID interface{} = entry.CameraID
	if entry.CameraID == "" {
		cameraID = nil
	}
	var eventID interface{} = entry.EventID
	if entry.EventID == "" {
		eventID = nil
	}

	_, err := s.db.ExecContext(ctx, query,
		entry.Path,
		entry.FileType,
		entry.SizeBytes,
		cameraID,
		eventID,
		entry.CreatedAt,
		expiresAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save storage entry: %w", err)
	}

	return nil
}

// DeleteStorageEntry deletes a storage entry from the database
func (s *StorageStateManager) DeleteStorageEntry(ctx context.Context, path string) error {
	query := `DELETE FROM storage_state WHERE path = ?`

	_, err := s.db.ExecContext(ctx, query, path)
	if err != nil {
		return fmt.Errorf("failed to delete storage entry: %w", err)
	}

	return nil
}

// ListStorageEntries lists storage entries by file type
func (s *StorageStateManager) ListStorageEntries(ctx context.Context, fileType string) ([]StorageEntry, error) {
	query := `
		SELECT path, file_type, size_bytes, camera_id, event_id, created_at, expires_at
		FROM storage_state
		WHERE file_type = ?
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, fileType)
	if err != nil {
		return nil, fmt.Errorf("failed to query storage entries: %w", err)
	}
	defer rows.Close()

	var entries []StorageEntry
	for rows.Next() {
		var entry StorageEntry
		var expiresAt sql.NullTime
		var cameraID sql.NullString
		var eventID sql.NullString

		err := rows.Scan(
			&entry.Path,
			&entry.FileType,
			&entry.SizeBytes,
			&cameraID,
			&eventID,
			&entry.CreatedAt,
			&expiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan storage entry: %w", err)
		}

		if cameraID.Valid {
			entry.CameraID = cameraID.String
		}
		if eventID.Valid {
			entry.EventID = eventID.String
		}
		if expiresAt.Valid {
			entry.ExpiresAt = &expiresAt.Time
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating storage entries: %w", err)
	}

	return entries, nil
}

// GetStorageStats returns storage statistics
func (s *StorageStateManager) GetStorageStats(ctx context.Context) (*StorageStats, error) {
	// Count clips
	var clipCount int
	clipQuery := `SELECT COUNT(*) FROM storage_state WHERE file_type = 'clip'`
	if err := s.db.QueryRowContext(ctx, clipQuery).Scan(&clipCount); err != nil {
		return nil, fmt.Errorf("failed to count clips: %w", err)
	}

	// Count snapshots
	var snapshotCount int
	snapshotQuery := `SELECT COUNT(*) FROM storage_state WHERE file_type = 'snapshot'`
	if err := s.db.QueryRowContext(ctx, snapshotQuery).Scan(&snapshotCount); err != nil {
		return nil, fmt.Errorf("failed to count snapshots: %w", err)
	}

	// Sum total size
	var totalSize int64
	sizeQuery := `SELECT COALESCE(SUM(size_bytes), 0) FROM storage_state`
	if err := s.db.QueryRowContext(ctx, sizeQuery).Scan(&totalSize); err != nil {
		return nil, fmt.Errorf("failed to sum storage size: %w", err)
	}

	// Note: DiskUsagePercent and AvailableBytes would need disk monitor
	// For now, we'll return 0 and let the caller combine with disk monitor
	return &StorageStats{
		TotalClips:     clipCount,
		TotalSnapshots: snapshotCount,
		TotalSizeBytes: totalSize,
	}, nil
}

