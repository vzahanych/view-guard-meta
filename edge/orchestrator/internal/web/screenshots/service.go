package screenshots

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

// Label represents a screenshot label
type Label string

const (
	LabelNormal   Label = "normal"
	LabelThreat   Label = "threat"
	LabelAbnormal Label = "abnormal"
	LabelCustom   Label = "custom"
)

// Screenshot represents a labeled screenshot
type Screenshot struct {
	ID          string                 `json:"id"`
	CameraID    string                 `json:"camera_id"`
	FilePath    string                 `json:"file_path"`
	Label       Label                  `json:"label"`
	CustomLabel string                 `json:"custom_label,omitempty"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CreatedBy   string                 `json:"created_by,omitempty"`
}

// Service manages labeled screenshots for training data
type Service struct {
	db           *state.Manager
	config       *config.Config
	logger       *logger.Logger
	snapshotsDir string
	exportsDir   string
}

// NewService creates a new screenshot service
func NewService(stateMgr *state.Manager, cfg *config.Config, log *logger.Logger) (*Service, error) {
	// Determine snapshots directory
	snapshotsDir := filepath.Join(cfg.Edge.Orchestrator.DataDir, "snapshots", "labeled")
	if err := os.MkdirAll(snapshotsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshots directory: %w", err)
	}

	exportsDir := cfg.Edge.AI.DatasetExportDir
	if exportsDir == "" {
		exportsDir = filepath.Join(cfg.Edge.Orchestrator.DataDir, "exports")
	}
	if err := os.MkdirAll(exportsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create dataset export directory: %w", err)
	}

	return &Service{
		db:           stateMgr,
		config:       cfg,
		logger:       log,
		snapshotsDir: snapshotsDir,
		exportsDir:   exportsDir,
	}, nil
}

// SaveScreenshot saves a screenshot with a label
func (s *Service) SaveScreenshot(ctx context.Context, screenshot *Screenshot, imageData []byte) error {
	// Generate ID if not provided
	if screenshot.ID == "" {
		screenshot.ID = uuid.New().String()
	}

	// Generate file path
	filename := fmt.Sprintf("%s_%s.jpg", screenshot.CameraID, screenshot.ID)
	filePath := filepath.Join(s.snapshotsDir, filename)

	// Save image to disk
	if err := os.WriteFile(filePath, imageData, 0644); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Serialize metadata to JSON
	metadataJSON := "{}"
	if screenshot.Metadata != nil {
		metadataBytes, err := json.Marshal(screenshot.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	// Save to database
	query := `
		INSERT INTO labeled_screenshots (
			id, camera_id, file_path, label, custom_label, description, metadata, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			label = excluded.label,
			custom_label = excluded.custom_label,
			description = excluded.description,
			metadata = excluded.metadata,
			updated_at = excluded.updated_at
	`

	now := time.Now()
	_, err := s.db.GetDB().ExecContext(ctx, query,
		screenshot.ID, screenshot.CameraID, filePath, string(screenshot.Label),
		screenshot.CustomLabel, screenshot.Description, metadataJSON,
		screenshot.CreatedBy, now, now,
	)
	if err != nil {
		// Clean up file if database insert fails
		os.Remove(filePath)
		return fmt.Errorf("failed to save screenshot to database: %w", err)
	}

	s.logger.Info("Saved labeled screenshot", "id", screenshot.ID, "camera_id", screenshot.CameraID, "label", screenshot.Label)
	return nil
}

// GetScreenshot retrieves a screenshot by ID
func (s *Service) GetScreenshot(ctx context.Context, id string) (*Screenshot, error) {
	query := `
		SELECT id, camera_id, file_path, label, custom_label, description, metadata, created_by, created_at, updated_at
		FROM labeled_screenshots
		WHERE id = ?
	`

	var screenshot Screenshot
	var labelStr, metadataJSON string
	var createdAt, updatedAt time.Time

	err := s.db.GetDB().QueryRowContext(ctx, query, id).Scan(
		&screenshot.ID, &screenshot.CameraID, &screenshot.FilePath,
		&labelStr, &screenshot.CustomLabel, &screenshot.Description,
		&metadataJSON, &screenshot.CreatedBy, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("screenshot not found: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get screenshot: %w", err)
	}

	screenshot.Label = Label(labelStr)
	screenshot.CreatedAt = createdAt
	screenshot.UpdatedAt = updatedAt

	// Parse metadata
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &screenshot.Metadata); err != nil {
			s.logger.Debug("Failed to parse metadata", "error", err)
		}
	}

	return &screenshot, nil
}

// ListScreenshots lists screenshots with optional filters
func (s *Service) ListScreenshots(ctx context.Context, filters *ScreenshotFilters) ([]*Screenshot, error) {
	query := `
		SELECT id, camera_id, file_path, label, custom_label, description, metadata, created_by, created_at, updated_at
		FROM labeled_screenshots
		WHERE 1=1
	`
	args := []interface{}{}

	if filters != nil {
		if filters.CameraID != "" {
			query += " AND camera_id = ?"
			args = append(args, filters.CameraID)
		}
		if filters.Label != "" {
			query += " AND label = ?"
			args = append(args, string(filters.Label))
		}
		if filters.CustomLabel != "" {
			query += " AND custom_label LIKE ?"
			args = append(args, "%"+filters.CustomLabel+"%")
		}
		if !filters.CreatedAfter.IsZero() {
			query += " AND created_at >= ?"
			args = append(args, filters.CreatedAfter)
		}
		if !filters.CreatedBefore.IsZero() {
			query += " AND created_at <= ?"
			args = append(args, filters.CreatedBefore)
		}
	}

	query += " ORDER BY created_at DESC"

	if filters != nil && filters.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filters.Limit)
		if filters.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, filters.Offset)
		}
	}

	rows, err := s.db.GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query screenshots: %w", err)
	}
	defer rows.Close()

	var screenshots []*Screenshot
	for rows.Next() {
		var screenshot Screenshot
		var labelStr, metadataJSON string
		var createdAt, updatedAt time.Time

		err := rows.Scan(
			&screenshot.ID, &screenshot.CameraID, &screenshot.FilePath,
			&labelStr, &screenshot.CustomLabel, &screenshot.Description,
			&metadataJSON, &screenshot.CreatedBy, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan screenshot: %w", err)
		}

		screenshot.Label = Label(labelStr)
		screenshot.CreatedAt = createdAt
		screenshot.UpdatedAt = updatedAt

		// Parse metadata
		if metadataJSON != "" && metadataJSON != "{}" {
			if err := json.Unmarshal([]byte(metadataJSON), &screenshot.Metadata); err != nil {
				s.logger.Debug("Failed to parse metadata", "error", err)
			}
		}

		screenshots = append(screenshots, &screenshot)
	}

	return screenshots, nil
}

// UpdateScreenshot updates a screenshot's label and metadata
func (s *Service) UpdateScreenshot(ctx context.Context, id string, updates *ScreenshotUpdate) error {
	// Build update query dynamically
	query := "UPDATE labeled_screenshots SET updated_at = ?"
	args := []interface{}{time.Now()}

	if updates.Label != nil {
		query += ", label = ?"
		args = append(args, string(*updates.Label))
	}
	if updates.CustomLabel != nil {
		query += ", custom_label = ?"
		args = append(args, *updates.CustomLabel)
	}
	if updates.Description != nil {
		query += ", description = ?"
		args = append(args, *updates.Description)
	}
	if updates.Metadata != nil {
		metadataBytes, err := json.Marshal(updates.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		query += ", metadata = ?"
		args = append(args, string(metadataBytes))
	}

	query += " WHERE id = ?"
	args = append(args, id)

	result, err := s.db.GetDB().ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update screenshot: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("screenshot not found")
	}

	s.logger.Info("Updated screenshot", "id", id)
	return nil
}

// DeleteScreenshot deletes a screenshot and its file
func (s *Service) DeleteScreenshot(ctx context.Context, id string) error {
	// Get screenshot to find file path
	screenshot, err := s.GetScreenshot(ctx, id)
	if err != nil {
		return err
	}

	// Delete from database
	query := "DELETE FROM labeled_screenshots WHERE id = ?"
	result, err := s.db.GetDB().ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete screenshot: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("screenshot not found")
	}

	// Delete file
	if err := os.Remove(screenshot.FilePath); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("Failed to delete screenshot file", "path", screenshot.FilePath, "error", err)
	}

	s.logger.Info("Deleted screenshot", "id", id)
	return nil
}

// GetScreenshotImage reads the image file for a screenshot
func (s *Service) GetScreenshotImage(ctx context.Context, id string) ([]byte, error) {
	screenshot, err := s.GetScreenshot(ctx, id)
	if err != nil {
		return nil, err
	}

	imageData, err := os.ReadFile(screenshot.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image file: %w", err)
	}

	return imageData, nil
}

// ScreenshotFilters contains filters for listing screenshots
type ScreenshotFilters struct {
	CameraID      string
	Label         Label
	CustomLabel   string
	CreatedAfter  time.Time
	CreatedBefore time.Time
	Limit         int
	Offset        int
}

// ScreenshotUpdate contains fields to update
type ScreenshotUpdate struct {
	Label       *Label
	CustomLabel *string
	Description *string
	Metadata    map[string]interface{}
}

// DatasetExportResult represents dataset export details
type DatasetExportResult struct {
	FilePath     string
	SampleCount  int
	ManifestName string
	CreatedAt    time.Time
}

// ExportDataset exports labeled screenshots into a portable archive
func (s *Service) ExportDataset(ctx context.Context, filters *ScreenshotFilters, includeMetadata bool) (*DatasetExportResult, error) {
	screenshots, err := s.ListScreenshots(ctx, filters)
	if err != nil {
		return nil, err
	}
	if len(screenshots) == 0 {
		return nil, fmt.Errorf("no screenshots match the provided filters")
	}

	exportID := time.Now().Format("20060102_150405")
	tempDir := filepath.Join(s.exportsDir, fmt.Sprintf("dataset_%s", exportID))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create export directory: %w", err)
	}

	type manifestEntry struct {
		ID          string                 `json:"id"`
		CameraID    string                 `json:"camera_id"`
		Label       Label                  `json:"label"`
		CustomLabel string                 `json:"custom_label,omitempty"`
		Description string                 `json:"description,omitempty"`
		FileName    string                 `json:"file"`
		CreatedAt   time.Time              `json:"created_at"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}

	manifest := struct {
		GeneratedAt time.Time       `json:"generated_at"`
		Count       int             `json:"count"`
		Entries     []manifestEntry `json:"entries"`
	}{
		GeneratedAt: time.Now(),
	}

	for _, shot := range screenshots {
		destName := fmt.Sprintf("%s_%s.jpg", shot.CameraID, shot.ID)
		destPath := filepath.Join(tempDir, destName)
		if err := copyFile(shot.FilePath, destPath); err != nil {
			s.logger.Warn("Failed to copy screenshot for dataset export", "id", shot.ID, "error", err)
			continue
		}

		entry := manifestEntry{
			ID:          shot.ID,
			CameraID:    shot.CameraID,
			Label:       shot.Label,
			CustomLabel: shot.CustomLabel,
			Description: shot.Description,
			FileName:    destName,
			CreatedAt:   shot.CreatedAt,
		}
		if includeMetadata {
			entry.Metadata = shot.Metadata
		}
		manifest.Entries = append(manifest.Entries, entry)
	}

	manifest.Count = len(manifest.Entries)
	manifestPath := filepath.Join(tempDir, "manifest.json")
	manifestFile, err := os.Create(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest: %w", err)
	}
	if err := json.NewEncoder(manifestFile).Encode(manifest); err != nil {
		manifestFile.Close()
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}
	manifestFile.Close()

	zipPath := filepath.Join(s.exportsDir, fmt.Sprintf("dataset_%s.zip", exportID))
	if err := zipDirectory(tempDir, zipPath); err != nil {
		return nil, fmt.Errorf("failed to create dataset archive: %w", err)
	}

	// Cleanup temp directory but keep archive
	_ = os.RemoveAll(tempDir)

	s.logger.Info("Exported labeled screenshot dataset",
		"file", zipPath,
		"samples", manifest.Count,
	)

	return &DatasetExportResult{
		FilePath:     zipPath,
		SampleCount:  manifest.Count,
		ManifestName: "manifest.json",
		CreatedAt:    manifest.GeneratedAt,
	}, nil
}

func copyFile(src, dest string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	return nil
}

func zipDirectory(srcDir, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		fh, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		fh.Name = relPath
		fh.Method = zip.Deflate

		w, err := writer.CreateHeader(fh)
		if err != nil {
			return err
		}

		if _, err := io.Copy(w, file); err != nil {
			return err
		}
		return nil
	})
}
