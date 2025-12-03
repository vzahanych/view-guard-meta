package datasetstorage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"go.uber.org/zap"
)

// Storage manages dataset storage organization on the filesystem
type Storage struct {
	config  *config.Config
	logger  *logging.Logger
	db      *database.DB
	dataDir string
}

// NewStorage creates a new dataset storage manager
func NewStorage(cfg *config.Config, log *logging.Logger, db *database.DB) *Storage {
	dataDir := cfg.UserVMAPI.Orchestrator.DataDir
	if dataDir == "" {
		dataDir = "/app/data"
	}

	return &Storage{
		config:  cfg,
		logger:  log,
		db:      db,
		dataDir: dataDir,
	}
}

// GetDatasetPath returns the filesystem path for a dataset
// Format: /app/data/datasets/{edge_id}/{camera_id}/{dataset_id}/
func (s *Storage) GetDatasetPath(edgeID string, cameraID string, datasetID string) string {
	return filepath.Join(s.dataDir, "datasets", edgeID, cameraID, datasetID)
}

// EnsureDatasetDirectory creates the dataset directory structure if it doesn't exist
func (s *Storage) EnsureDatasetDirectory(edgeID string, cameraID string, datasetID string) (string, error) {
	datasetPath := s.GetDatasetPath(edgeID, cameraID, datasetID)
	if err := os.MkdirAll(datasetPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create dataset directory: %w", err)
	}

	// Create screenshots subdirectory
	screenshotsDir := filepath.Join(datasetPath, "screenshots")
	if err := os.MkdirAll(screenshotsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create screenshots directory: %w", err)
	}

	return datasetPath, nil
}

// VerifyDatasetStructure verifies that a dataset directory has the expected structure
func (s *Storage) VerifyDatasetStructure(datasetPath string) error {
	// Check for metadata.json
	metadataPath := filepath.Join(datasetPath, "metadata.json")
	if _, err := os.Stat(metadataPath); err != nil {
		return fmt.Errorf("metadata.json not found: %w", err)
	}

	// Check for screenshots directory
	screenshotsDir := filepath.Join(datasetPath, "screenshots")
	if _, err := os.Stat(screenshotsDir); err != nil {
		return fmt.Errorf("screenshots directory not found: %w", err)
	}

	// Check for manifest.json (optional)
	manifestPath := filepath.Join(datasetPath, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		s.logger.Warn("manifest.json not found in dataset (optional)", zap.String("dataset_path", datasetPath))
	}

	return nil
}

// GetDatasetInfo retrieves dataset information from the database
func (s *Storage) GetDatasetInfo(ctx context.Context, datasetID string) (*DatasetInfo, error) {
	const query = `
		SELECT dataset_id, name, edge_id, dataset_dir_path, label_counts, 
		       total_images, status, created_at, updated_at
		FROM training_datasets
		WHERE dataset_id = ?
	`

	var info DatasetInfo
	var labelCountsJSON string
	var createdAt, updatedAt int64
	var labelCounts sql.NullString

	err := s.db.QueryRowContext(ctx, query, datasetID).Scan(
		&info.DatasetID,
		&info.Name,
		&info.EdgeID,
		&info.DatasetPath,
		&labelCounts,
		&info.TotalSnapshots,
		&info.Status,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset info: %w", err)
	}

	if labelCounts.Valid {
		labelCountsJSON = labelCounts.String
	}

	// Parse label_counts JSON
	if labelCountsJSON != "" {
		if err := json.Unmarshal([]byte(labelCountsJSON), &info.LabelCounts); err != nil {
			s.logger.Warn("Failed to parse label_counts", zap.Error(err))
		}
	}

	info.CreatedAt = time.Unix(createdAt, 0)
	info.UpdatedAt = time.Unix(updatedAt, 0)

	return &info, nil
}

// DatasetInfo contains information about a stored dataset
type DatasetInfo struct {
	DatasetID      string
	Name           string
	EdgeID         string
	DatasetPath    string
	LabelCounts    map[string]int
	TotalSnapshots int
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
