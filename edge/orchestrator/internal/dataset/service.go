package dataset

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
)

// Service manages dataset packaging and uploading
type Service struct {
	packager *Packager
	uploader *Uploader
	logger   *logger.Logger
	config   *config.Config
	edgeID   string // Edge appliance identifier
}

// NewService creates a new dataset service
func NewService(cfg *config.Config, log *logger.Logger, edgeID string) *Service {
	return &Service{
		packager: NewPackager(log),
		uploader: NewUploader(cfg, log),
		logger:   log,
		config:   cfg,
		edgeID:   edgeID,
	}
}

// UploadDatasetForCamera packages and uploads all screenshots for a camera to the VM
// Returns the dataset ID if successful, or an error
func (s *Service) UploadDatasetForCamera(ctx context.Context, cameraID string, screenshotList []*screenshots.Screenshot) (string, error) {
	if len(screenshotList) == 0 {
		return "", fmt.Errorf("no screenshots to upload for camera %s", cameraID)
	}

	// Determine output directory for temporary archive
	outputDir := s.config.Edge.AI.DatasetExportDir
	if outputDir == "" {
		outputDir = filepath.Join(s.config.Edge.Orchestrator.DataDir, "exports")
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	// Package dataset
	s.logger.Info("Packaging dataset for upload",
		"camera_id", cameraID,
		"screenshot_count", len(screenshotList),
		"edge_id", s.edgeID,
	)

	archivePath, checksum, err := s.packager.PackageDataset(
		s.edgeID,
		cameraID,
		screenshotList,
		outputDir,
	)
	if err != nil {
		return "", fmt.Errorf("failed to package dataset: %w", err)
	}

	// Clean up archive file after upload (defer)
	defer func() {
		if err := os.Remove(archivePath); err != nil {
			s.logger.Warn("Failed to clean up archive file", "error", err, "archive_path", archivePath)
		}
	}()

	// Upload dataset to VM
	s.logger.Info("Uploading dataset to VM",
		"camera_id", cameraID,
		"archive_path", archivePath,
		"checksum", checksum,
	)

	result, err := s.uploader.UploadDataset(ctx, archivePath, cameraID, checksum, s.edgeID)
	if err != nil {
		return "", fmt.Errorf("failed to upload dataset: %w", err)
	}

	if !result.Success {
		return "", fmt.Errorf("dataset upload failed: %w", result.Error)
	}

	s.logger.Info("Dataset uploaded successfully",
		"camera_id", cameraID,
		"dataset_id", result.DatasetID,
		"checksum", checksum,
	)

	return result.DatasetID, nil
}
