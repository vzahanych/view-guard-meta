package datasetstorage

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/service"
	tunnelgateway "github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
	"go.uber.org/zap"
)

// DatasetMetadata represents metadata from the uploaded dataset archive
type DatasetMetadata struct {
	EdgeID         string         `json:"edge_id"`
	CameraID       string         `json:"camera_id"`
	TotalSnapshots int            `json:"total_snapshots"`
	LabelCounts    map[string]int `json:"label_counts"`
	SyncedAt       time.Time      `json:"synced_at"`
	Checksum       string         `json:"checksum,omitempty"`
}

// Receiver handles dataset upload reception and extraction
type Receiver struct {
	config        *config.Config
	logger        *logging.Logger
	db            *database.DB
	capStore      *tunnelgateway.CapabilityStore
	storage       *Storage
	edgeAPIServer *tunnelgateway.EdgeAPIServer
	eventBus      *service.EventBus
}

// NewReceiver creates a new dataset receiver
func NewReceiver(cfg *config.Config, log *logging.Logger, db *database.DB, capStore *tunnelgateway.CapabilityStore, edgeAPIServer *tunnelgateway.EdgeAPIServer) *Receiver {
	storage := NewStorage(cfg, log, db)

	return &Receiver{
		config:        cfg,
		logger:        log,
		db:            db,
		capStore:      capStore,
		storage:       storage,
		edgeAPIServer: edgeAPIServer,
	}
}

// SetEventBus sets the event bus for publishing events
func (r *Receiver) SetEventBus(eventBus *service.EventBus) {
	r.eventBus = eventBus
}

// ReceiveDataset receives and processes an uploaded dataset archive
// Returns the dataset ID and path if successful
func (r *Receiver) ReceiveDataset(ctx context.Context, edgeID string, cameraID string, archivePath string, checksum string) (string, string, error) {
	// Ensure edge exists in database first (for PoC, create if not exists)
	// This prevents foreign key constraint failures
	now := time.Now().Unix()
	_, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, edgeID, "Edge "+edgeID, "unknown-pubkey", now, "active", now, now)
	if err != nil {
		r.logger.Warn("Failed to ensure edge exists", zap.Error(err), zap.String("edge_id", edgeID))
		// Continue anyway - might work if edge already exists
	}

	// Check for duplicate upload (same edge_id, camera_id, and checksum)
	if checksum != "" {
		existingDatasetID, err := r.findExistingDataset(ctx, edgeID, cameraID, checksum)
		if err == nil && existingDatasetID != "" {
			r.logger.Info("Dataset with same checksum already exists, skipping upload",
				zap.String("edge_id", edgeID),
				zap.String("camera_id", cameraID),
				zap.String("checksum", checksum),
				zap.String("existing_dataset_id", existingDatasetID),
			)
			// Return existing dataset ID and path
			existingPath := r.storage.GetDatasetPath(edgeID, cameraID, existingDatasetID)
			return existingDatasetID, existingPath, nil
		}
	}

	// Generate dataset ID
	datasetID := uuid.New().String()

	// Create dataset directory structure: /app/data/datasets/{edge_id}/{camera_id}/{dataset_id}/
	datasetBasePath, err := r.storage.EnsureDatasetDirectory(edgeID, cameraID, datasetID)
	if err != nil {
		return "", "", fmt.Errorf("failed to create dataset directory: %w", err)
	}

	r.logger.Info("Extracting dataset archive",
		zap.String("edge_id", edgeID),
		zap.String("camera_id", cameraID),
		zap.String("dataset_id", datasetID),
		zap.String("archive_path", archivePath),
		zap.String("dataset_path", datasetBasePath),
	)

	// Extract archive
	if err := r.extractArchive(archivePath, datasetBasePath); err != nil {
		// Clean up directory on failure
		os.RemoveAll(datasetBasePath)
		return "", "", fmt.Errorf("failed to extract archive: %w", err)
	}

	// Verify checksum
	if checksum != "" {
		calculatedChecksum, err := r.calculateArchiveChecksum(archivePath)
		if err != nil {
			r.logger.Warn("Failed to calculate checksum for verification", zap.Error(err))
		} else if calculatedChecksum != checksum {
			os.RemoveAll(datasetBasePath)
			return "", "", fmt.Errorf("checksum mismatch: expected %s, got %s", checksum, calculatedChecksum)
		}
		r.logger.Info("Dataset checksum verified", zap.String("checksum", checksum))
	}

	// Read metadata.json
	metadataPath := filepath.Join(datasetBasePath, "metadata.json")
	metadata, err := r.readMetadata(metadataPath)
	if err != nil {
		os.RemoveAll(datasetBasePath)
		return "", "", fmt.Errorf("failed to read metadata: %w", err)
	}

	// Verify dataset structure
	if err := r.storage.VerifyDatasetStructure(datasetBasePath); err != nil {
		os.RemoveAll(datasetBasePath)
		return "", "", fmt.Errorf("dataset structure verification failed: %w", err)
	}

	// Store dataset metadata in database
	if err := r.storeDatasetMetadata(ctx, datasetID, edgeID, cameraID, datasetBasePath, metadata, checksum); err != nil {
		os.RemoveAll(datasetBasePath)
		return "", "", fmt.Errorf("failed to store dataset metadata: %w", err)
	}

	r.logger.Info("Dataset received and stored successfully",
		zap.String("dataset_id", datasetID),
		zap.String("edge_id", edgeID),
		zap.String("camera_id", cameraID),
		zap.Int("total_snapshots", metadata.TotalSnapshots),
		zap.String("dataset_path", datasetBasePath),
	)

	// Publish dataset.uploaded event for downstream training service
	if r.eventBus != nil {
		r.eventBus.Publish(service.Event{
			Type:      "dataset.uploaded",
			Timestamp: time.Now().Unix(),
			Data: map[string]interface{}{
				"edge_id":         edgeID,
				"camera_id":       cameraID,
				"dataset_id":      datasetID,
				"total_snapshots": metadata.TotalSnapshots,
				"label_counts":    metadata.LabelCounts,
			},
		})
	}

	return datasetID, datasetBasePath, nil
}

// extractArchive extracts a tar.gz archive to the target directory
func (r *Receiver) extractArchive(archivePath string, targetDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Sanitize path to prevent directory traversal
		targetPath := filepath.Join(targetDir, header.Name)
		if !filepath.HasPrefix(targetPath, filepath.Clean(targetDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid path in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

		case tar.TypeReg:
			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			// Create file
			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			// Copy file content
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to copy file content: %w", err)
			}

			outFile.Close()

			// Set file permissions
			if err := os.Chmod(targetPath, os.FileMode(header.Mode)); err != nil {
				r.logger.Warn("Failed to set file permissions", zap.String("path", targetPath), zap.Error(err))
			}
		}
	}

	return nil
}

// readMetadata reads and parses metadata.json from the dataset
func (r *Receiver) readMetadata(metadataPath string) (*DatasetMetadata, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var metadata DatasetMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &metadata, nil
}

// calculateArchiveChecksum calculates SHA-256 checksum of the archive file
func (r *Receiver) calculateArchiveChecksum(archivePath string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// storeDatasetMetadata stores dataset metadata in the database
func (r *Receiver) storeDatasetMetadata(ctx context.Context, datasetID string, edgeID string, cameraID string, datasetPath string, metadata *DatasetMetadata, checksum string) error {
	// Marshal label_counts to JSON
	labelCountsJSON := "{}"
	if metadata.LabelCounts != nil {
		labelCountsBytes, err := json.Marshal(metadata.LabelCounts)
		if err != nil {
			return fmt.Errorf("failed to marshal label counts: %w", err)
		}
		labelCountsJSON = string(labelCountsBytes)
	}

	now := time.Now().Unix()

	// Insert into training_datasets table (reusing existing schema)
	query := `
		INSERT INTO training_datasets (
			dataset_id, name, edge_id, dataset_dir_path, label_counts, total_images, status, checksum, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	datasetName := fmt.Sprintf("dataset_%s_%s", edgeID, cameraID)
	_, err := r.db.ExecContext(ctx, query,
		datasetID,
		datasetName,
		edgeID,
		datasetPath,
		labelCountsJSON,
		metadata.TotalSnapshots,
		"ready", // Status: ready for training
		checksum, // Store checksum for duplicate detection
		now,
		now,
	)

	if err != nil {
		return fmt.Errorf("failed to insert dataset metadata: %w", err)
	}

	r.logger.Info("Dataset metadata stored successfully",
		zap.String("dataset_id", datasetID),
		zap.String("edge_id", edgeID),
		zap.String("camera_id", cameraID),
	)

	return nil
}

// findExistingDataset checks if a dataset with the same edge_id, camera_id, and checksum already exists
// Returns the existing dataset ID if found, empty string if not found
func (r *Receiver) findExistingDataset(ctx context.Context, edgeID string, cameraID string, checksum string) (string, error) {
	// First check if checksum column exists (for migration compatibility)
	var columnExists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) > 0 FROM pragma_table_info('training_datasets') WHERE name = 'checksum'
	`).Scan(&columnExists)
	if err != nil {
		// If we can't check, assume column doesn't exist yet and skip duplicate check
		return "", nil
	}

	if !columnExists {
		// Checksum column doesn't exist yet (pre-migration), skip duplicate check
		return "", nil
	}

	const query = `
		SELECT dataset_id
		FROM training_datasets
		WHERE edge_id = ? AND dataset_dir_path LIKE ? AND checksum = ?
		ORDER BY created_at DESC
		LIMIT 1
	`

	// Build pattern to match camera_id in path: %/camera_id/%
	cameraPattern := "%/" + cameraID + "/%"

	var datasetID string
	err = r.db.QueryRowContext(ctx, query, edgeID, cameraPattern, checksum).Scan(&datasetID)
	if err == sql.ErrNoRows {
		return "", nil // No existing dataset found
	}
	if err != nil {
		return "", fmt.Errorf("failed to query existing dataset: %w", err)
	}

	return datasetID, nil
}
