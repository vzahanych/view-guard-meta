package screenshots

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
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
	db                  *state.Manager
	config              *config.Config
	logger              *logger.Logger
	snapshotsDir        string
	exportsDir          string
	thumbnailsDir       string
	diskMonitor         *storage.DiskMonitor
	maxDiskUsagePercent float64
}

// Image processing constants (Substep 2.2.2.4.4)
const (
	jpegQuality      = 85  // JPEG quality (0-100, higher = better quality but larger file)
	thumbnailMaxSize = 256 // Maximum width/height for thumbnails
)

// NewService creates a new screenshot service
func NewService(stateMgr *state.Manager, cfg *config.Config, log *logger.Logger) (*Service, error) {
	// Determine snapshots directory
	snapshotsDir := filepath.Join(cfg.Edge.Orchestrator.DataDir, "snapshots", "labeled")
	if err := os.MkdirAll(snapshotsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshots directory: %w", err)
	}

	// Create thumbnails directory (Substep 2.2.2.4.4)
	thumbnailsDir := filepath.Join(cfg.Edge.Orchestrator.DataDir, "snapshots", "thumbnails")
	if err := os.MkdirAll(thumbnailsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create thumbnails directory: %w", err)
	}

	exportsDir := cfg.Edge.AI.DatasetExportDir
	if exportsDir == "" {
		exportsDir = filepath.Join(cfg.Edge.Orchestrator.DataDir, "exports")
	}
	if err := os.MkdirAll(exportsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create dataset export directory: %w", err)
	}

	// Initialize disk monitor for screenshot storage using the same threshold as generic storage
	maxUsage := cfg.Edge.Storage.MaxDiskUsagePercent
	if maxUsage <= 0 {
		maxUsage = 80.0
	}

	diskMonitor, err := storage.NewDiskMonitor(cfg.Edge.Orchestrator.DataDir, maxUsage, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create disk monitor for screenshots: %w", err)
	}

	return &Service{
		db:                  stateMgr,
		config:              cfg,
		logger:              log,
		snapshotsDir:        snapshotsDir,
		thumbnailsDir:       thumbnailsDir,
		exportsDir:          exportsDir,
		diskMonitor:         diskMonitor,
		maxDiskUsagePercent: maxUsage,
	}, nil
}

// SaveScreenshot saves a screenshot with a label (Substep 2.2.2.4.4: image processing and optimization)
func (s *Service) SaveScreenshot(ctx context.Context, screenshot *Screenshot, imageData []byte) error {
	// Generate ID if not provided
	if screenshot.ID == "" {
		screenshot.ID = uuid.New().String()
	}

	// Decode image to extract metadata and process (Substep 2.2.2.4.4)
	img, format, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		s.logger.Error("Failed to decode screenshot image", "error", err, "screenshot_id", screenshot.ID, "camera_id", screenshot.CameraID, "operation", "create", "created_by", screenshot.CreatedBy)
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Extract image dimensions (Substep 2.2.2.4.4)
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Initialize metadata if nil
	if screenshot.Metadata == nil {
		screenshot.Metadata = make(map[string]interface{})
	}

	// Store image dimensions and original format in metadata (Substep 2.2.2.4.4)
	screenshot.Metadata["width"] = width
	screenshot.Metadata["height"] = height
	screenshot.Metadata["original_format"] = format
	screenshot.Metadata["original_size_bytes"] = len(imageData)

	// Process and optimize image (Substep 2.2.2.4.4)
	// Image Processing Strategy:
	// 1. All images are normalized to JPEG format for consistency and smaller file sizes
	// 2. PNG images are converted to JPEG (PNG is lossless but larger)
	// 3. JPEG images are re-encoded with quality control (85% quality) for compression
	// 4. Other formats (GIF, BMP, etc.) are converted to JPEG
	// 5. Compression ratio and size metadata are stored for analysis
	//
	// Quality setting (jpegQuality = 85):
	// - Balances file size vs. image quality
	// - 85% provides good quality with significant size reduction
	// - Lower values = smaller files but lower quality
	// - Higher values = better quality but larger files
	var processedImageData []byte
	var finalFormat string

	if format == "png" {
		// Convert PNG to JPEG for smaller file size (Substep 2.2.2.4.4)
		// PNG is lossless but typically 2-3x larger than JPEG
		// Conversion to JPEG reduces storage requirements significantly
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return fmt.Errorf("failed to convert PNG to JPEG: %w", err)
		}
		processedImageData = buf.Bytes()
		finalFormat = "jpeg"
		screenshot.Metadata["converted_from"] = "png"
	} else if format == "jpeg" {
		// Re-encode JPEG with quality control for compression (Substep 2.2.2.4.4)
		// Re-encoding ensures consistent quality and can reduce file size
		// if the original was saved at higher quality
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return fmt.Errorf("failed to re-encode JPEG: %w", err)
		}
		processedImageData = buf.Bytes()
		finalFormat = "jpeg"
	} else {
		// For other formats (GIF, BMP, WebP, etc.), convert to JPEG
		// Ensures all screenshots are in a consistent, widely-supported format
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return fmt.Errorf("failed to convert to JPEG: %w", err)
		}
		processedImageData = buf.Bytes()
		finalFormat = "jpeg"
		screenshot.Metadata["converted_from"] = format
	}

	// Store processed image size in metadata (Substep 2.2.2.4.4)
	// These metrics help track storage efficiency and compression effectiveness
	screenshot.Metadata["processed_size_bytes"] = len(processedImageData)
	screenshot.Metadata["compression_ratio"] = float64(len(processedImageData)) / float64(len(imageData))
	// compression_ratio < 1.0 means file was compressed (smaller)
	// compression_ratio > 1.0 means file grew (rare, but possible with very small originals)

	// Generate file path
	filename := fmt.Sprintf("%s_%s.jpg", screenshot.CameraID, screenshot.ID)
	filePath := filepath.Join(s.snapshotsDir, filename)

	// Expose file path on the screenshot struct for callers
	screenshot.FilePath = filePath

	// Save processed image to disk
	if err := os.WriteFile(filePath, processedImageData, 0644); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Generate thumbnail (Substep 2.2.2.4.4)
	thumbnailPath, err := s.generateThumbnail(screenshot.ID, screenshot.CameraID, img)
	if err != nil {
		s.logger.Warn("Failed to generate thumbnail", "error", err, "screenshot_id", screenshot.ID)
		// Don't fail the save operation if thumbnail generation fails
	} else {
		screenshot.Metadata["thumbnail_path"] = thumbnailPath
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
	_, err = s.db.GetDB().ExecContext(ctx, query,
		screenshot.ID, screenshot.CameraID, filePath, string(screenshot.Label),
		screenshot.CustomLabel, screenshot.Description, metadataJSON,
		screenshot.CreatedBy, now, now,
	)
	if err != nil {
		// Clean up file if database insert fails
		os.Remove(filePath)
		s.logger.Error("Failed to save screenshot to database", "error", err, "screenshot_id", screenshot.ID, "camera_id", screenshot.CameraID, "operation", "create", "created_by", screenshot.CreatedBy, "label", screenshot.Label)
		return fmt.Errorf("failed to save screenshot to database: %w", err)
	}

	// Log disk usage warnings if we're approaching or exceeding the configured threshold
	s.checkDiskUsage(ctx)

	// Structured logging for screenshot creation (Substep 2.2.2.7.2)
	s.logger.Info("Screenshot created", "operation", "create", "screenshot_id", screenshot.ID, "camera_id", screenshot.CameraID,
		"label", screenshot.Label, "custom_label", screenshot.CustomLabel, "created_by", screenshot.CreatedBy,
		"original_size", len(imageData), "processed_size", len(processedImageData), "format", finalFormat,
		"file_path", filePath)
	return nil
}

// checkDiskUsage logs warnings when disk usage approaches or exceeds the configured threshold
func (s *Service) checkDiskUsage(ctx context.Context) {
	if s.diskMonitor == nil || s.maxDiskUsagePercent <= 0 {
		return
	}

	usage, err := s.diskMonitor.GetUsage(ctx)
	if err != nil {
		s.logger.Debug("Failed to get disk usage for screenshots", "error", err)
		return
	}

	// Hard warning when usage is at or above the configured limit
	if usage.UsagePercent >= s.maxDiskUsagePercent {
		s.logger.Warn("Disk usage for screenshots data directory is above configured threshold",
			"usage_percent", usage.UsagePercent,
			"max_usage_percent", s.maxDiskUsagePercent,
			"total_bytes", usage.TotalBytes,
			"used_bytes", usage.UsedBytes,
			"available_bytes", usage.AvailableBytes,
		)
		return
	}

	// Soft warning when we're close to the threshold (90% of max)
	if usage.UsagePercent >= s.maxDiskUsagePercent*0.9 {
		s.logger.Info("Disk usage for screenshots data directory is nearing configured threshold",
			"usage_percent", usage.UsagePercent,
			"max_usage_percent", s.maxDiskUsagePercent,
			"total_bytes", usage.TotalBytes,
			"used_bytes", usage.UsedBytes,
			"available_bytes", usage.AvailableBytes,
		)
	}
}

// generateThumbnail generates a thumbnail for faster list view loading (Substep 2.2.2.4.4)
func (s *Service) generateThumbnail(screenshotID, cameraID string, img image.Image) (string, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Calculate thumbnail dimensions maintaining aspect ratio
	var thumbWidth, thumbHeight int
	if width > height {
		thumbWidth = thumbnailMaxSize
		thumbHeight = (height * thumbnailMaxSize) / width
	} else {
		thumbHeight = thumbnailMaxSize
		thumbWidth = (width * thumbnailMaxSize) / height
	}

	// Resize image using simple nearest-neighbor for now (could use better algorithm)
	thumbImg := image.NewRGBA(image.Rect(0, 0, thumbWidth, thumbHeight))
	for y := 0; y < thumbHeight; y++ {
		for x := 0; x < thumbWidth; x++ {
			srcX := (x * width) / thumbWidth
			srcY := (y * height) / thumbHeight
			thumbImg.Set(x, y, img.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	// Encode thumbnail as JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumbImg, &jpeg.Options{Quality: 75}); err != nil {
		return "", fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	// Save thumbnail
	filename := fmt.Sprintf("%s_%s_thumb.jpg", cameraID, screenshotID)
	thumbnailPath := filepath.Join(s.thumbnailsDir, filename)
	if err := os.WriteFile(thumbnailPath, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("failed to save thumbnail: %w", err)
	}

	return thumbnailPath, nil
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
		if filters.Description != "" {
			query += " AND description LIKE ?"
			args = append(args, "%"+filters.Description+"%")
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

	// Handle sorting
	sortBy := "created_at"
	sortOrder := "DESC"
	if filters != nil {
		if filters.SortBy != "" {
			// Validate sort field to prevent SQL injection
			allowedSortFields := map[string]bool{
				"created_at":   true,
				"camera_id":    true,
				"label":        true,
				"custom_label": true,
				"updated_at":   true,
			}
			if allowedSortFields[filters.SortBy] {
				sortBy = filters.SortBy
			}
		}
		if filters.SortOrder != "" {
			if filters.SortOrder == "asc" || filters.SortOrder == "desc" {
				sortOrder = strings.ToUpper(filters.SortOrder)
			}
		}
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

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

	// Get screenshot before update for logging context
	screenshot, err := s.GetScreenshot(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get screenshot for update", "error", err, "screenshot_id", id, "operation", "update")
		return fmt.Errorf("screenshot not found: %w", err)
	}

	result, err := s.db.GetDB().ExecContext(ctx, query, args...)
	if err != nil {
		s.logger.Error("Failed to update screenshot in database", "error", err, "screenshot_id", id, "camera_id", screenshot.CameraID, "operation", "update")
		return fmt.Errorf("failed to update screenshot: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		s.logger.Error("Failed to get rows affected after update", "error", err, "screenshot_id", id, "camera_id", screenshot.CameraID, "operation", "update")
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		s.logger.Warn("Screenshot not found for update", "screenshot_id", id, "operation", "update")
		return fmt.Errorf("screenshot not found")
	}

	// Get updated screenshot for logging
	updatedScreenshot, err := s.GetScreenshot(ctx, id)
	if err == nil {
		// Structured logging for screenshot update (Substep 2.2.2.7.2)
		s.logger.Info("Screenshot updated", "operation", "update", "screenshot_id", id, "camera_id", updatedScreenshot.CameraID,
			"old_label", screenshot.Label, "new_label", updatedScreenshot.Label,
			"old_custom_label", screenshot.CustomLabel, "new_custom_label", updatedScreenshot.CustomLabel,
			"created_by", updatedScreenshot.CreatedBy)
	} else {
		s.logger.Info("Screenshot updated", "operation", "update", "screenshot_id", id, "camera_id", screenshot.CameraID)
	}
	return nil
}

// DeleteScreenshot deletes a screenshot and its file
func (s *Service) DeleteScreenshot(ctx context.Context, id string) error {
	// Get screenshot to find file path and get context for logging
	screenshot, err := s.GetScreenshot(ctx, id)
	if err != nil {
		s.logger.Error("Failed to get screenshot for deletion", "error", err, "screenshot_id", id, "operation", "delete")
		return err
	}

	// Delete from database
	query := "DELETE FROM labeled_screenshots WHERE id = ?"
	result, err := s.db.GetDB().ExecContext(ctx, query, id)
	if err != nil {
		s.logger.Error("Failed to delete screenshot from database", "error", err, "screenshot_id", id, "camera_id", screenshot.CameraID, "operation", "delete", "created_by", screenshot.CreatedBy)
		return fmt.Errorf("failed to delete screenshot: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		s.logger.Error("Failed to get rows affected after deletion", "error", err, "screenshot_id", id, "camera_id", screenshot.CameraID, "operation", "delete")
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		s.logger.Warn("Screenshot not found for deletion", "screenshot_id", id, "operation", "delete")
		return fmt.Errorf("screenshot not found")
	}

	// Delete file
	if err := os.Remove(screenshot.FilePath); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("Failed to delete screenshot file", "path", screenshot.FilePath, "error", err, "screenshot_id", id, "camera_id", screenshot.CameraID, "operation", "delete")
	}

	// Structured logging for screenshot deletion (Substep 2.2.2.7.2)
	s.logger.Info("Screenshot deleted", "operation", "delete", "screenshot_id", id, "camera_id", screenshot.CameraID,
		"label", screenshot.Label, "custom_label", screenshot.CustomLabel, "created_by", screenshot.CreatedBy,
		"file_path", screenshot.FilePath)
	return nil
}

// GetLabelCounts returns the number of labeled screenshots per label for a camera
func (s *Service) GetLabelCounts(ctx context.Context, cameraID string) (map[Label]int, error) {
	query := `
		SELECT label, COUNT(*)
		FROM labeled_screenshots
		WHERE camera_id = ?
		GROUP BY label
	`

	rows, err := s.db.GetDB().QueryContext(ctx, query, cameraID)
	if err != nil {
		return nil, fmt.Errorf("failed to query label counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[Label]int)
	for rows.Next() {
		var label string
		var count int
		if err := rows.Scan(&label, &count); err != nil {
			return nil, fmt.Errorf("failed to scan label count: %w", err)
		}
		counts[Label(label)] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate label counts: %w", err)
	}

	return counts, nil
}

// DatasetStatus represents the dataset readiness status for a camera
type DatasetStatus struct {
	LabelCounts           map[string]int
	LabeledSnapshotCount  int
	RequiredSnapshotCount int
	SnapshotRequired      bool
	LastSynced            time.Time
}

// GetDatasetStatus calculates and returns the dataset status for a camera
// This is a shared helper method that can be used by both SyncService and handlers
//
// Dataset Progress Calculation:
// - Only "normal" labeled screenshots count toward the required snapshot count
// - snapshot_required = true if labeled_snapshot_count < required_snapshot_count
// - snapshot_required = false if labeled_snapshot_count >= required_snapshot_count
// - All label types (normal, threat, abnormal, custom) are counted in label_counts
//
// Example: If minSnapshots=50 and camera has 45 normal screenshots:
//   - labeled_snapshot_count = 45
//   - required_snapshot_count = 50
//   - snapshot_required = true (45 < 50)
//
// See: docs/SCREENSHOT_USER_GUIDE.md for detailed explanation
func (s *Service) GetDatasetStatus(ctx context.Context, cameraID string, minSnapshots int) (*DatasetStatus, error) {
	stats := &DatasetStatus{
		LabelCounts:           make(map[string]int),
		RequiredSnapshotCount: minSnapshots,
		LastSynced:            time.Now(),
	}

	// Validate and set default minimum snapshot count
	// Default: 50 normal snapshots required per camera (configurable via min_normal_snapshots)
	if minSnapshots <= 0 {
		minSnapshots = 50 // Default
		stats.RequiredSnapshotCount = minSnapshots
	}

	// Get label counts for all label types (normal, threat, abnormal, custom)
	counts, err := s.GetLabelCounts(ctx, cameraID)
	if err != nil {
		s.logger.Info("Failed to get label counts", "camera_id", cameraID, "error", err)
		// On error, assume more snapshots are required
		stats.SnapshotRequired = true
		return stats, nil // Return status with snapshot_required=true on error
	}

	// Populate label counts and extract normal label count
	// Note: Only "normal" labeled screenshots count toward the required snapshot count
	for label, count := range counts {
		stats.LabelCounts[string(label)] = count
		if label == LabelNormal {
			// This is the count used for snapshot_required calculation
			stats.LabeledSnapshotCount = count
		}
	}

	// Determine if more snapshots are required
	// snapshot_required = true means the camera needs more "normal" labeled snapshots
	stats.SnapshotRequired = stats.LabeledSnapshotCount < minSnapshots
	return stats, nil
}

// CameraStorageStats holds screenshot storage stats per camera
type CameraStorageStats struct {
	CameraID        string `json:"camera_id"`
	ScreenshotCount int    `json:"screenshot_count"`
	TotalSizeBytes  int64  `json:"total_size_bytes"`
}

// StorageStats holds aggregated screenshot storage statistics
type StorageStats struct {
	TotalScreenshots    int                           `json:"total_screenshots"`
	TotalSizeBytes      int64                         `json:"total_size_bytes"`
	Cameras             map[string]CameraStorageStats `json:"cameras"`
	OldestScreenshotAt  string                        `json:"oldest_screenshot_at,omitempty"`
	NewestScreenshotAt  string                        `json:"newest_screenshot_at,omitempty"`
	OrphanedRecordCount int                           `json:"orphaned_record_count"`

	// Disk-level statistics (shared with generic storage)
	DiskTotalBytes      int64   `json:"disk_total_bytes"`
	DiskUsedBytes       int64   `json:"disk_used_bytes"`
	DiskAvailableBytes  int64   `json:"disk_available_bytes"`
	DiskUsagePercent    float64 `json:"disk_usage_percent"`
	MaxDiskUsagePercent float64 `json:"max_disk_usage_percent"`
}

// StorageCleanupOptions controls screenshot storage cleanup behavior
type StorageCleanupOptions struct {
	CleanupOrphanedFiles   bool
	CleanupOrphanedRecords bool
	RetentionDays          int
}

// StorageCleanupResult describes the outcome of a storage cleanup operation
type StorageCleanupResult struct {
	OrphanedFilesDeleted   int   `json:"orphaned_files_deleted"`
	OrphanedRecordsDeleted int   `json:"orphaned_records_deleted"`
	OldScreenshotsDeleted  int   `json:"old_screenshots_deleted"`
	FreedBytes             int64 `json:"freed_bytes"`
}

// GetStorageStats returns aggregated storage statistics for labeled screenshots (Substep 2.2.2.4.5)
func (s *Service) GetStorageStats(ctx context.Context) (*StorageStats, error) {
	stats := &StorageStats{
		Cameras:             make(map[string]CameraStorageStats),
		MaxDiskUsagePercent: s.maxDiskUsagePercent,
	}

	query := `
		SELECT camera_id, file_path, created_at
		FROM labeled_screenshots
	`

	rows, err := s.db.GetDB().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query screenshot storage stats: %w", err)
	}
	defer rows.Close()

	var (
		totalScreenshots int
		totalSizeBytes   int64
		orphanedRecords  int
		oldestTime       time.Time
		newestTime       time.Time
	)

	first := true

	for rows.Next() {
		var cameraID, filePath string
		var createdAt time.Time

		if err := rows.Scan(&cameraID, &filePath, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan screenshot row: %w", err)
		}

		totalScreenshots++

		// Track oldest/newest timestamps
		if first {
			oldestTime = createdAt
			newestTime = createdAt
			first = false
		} else {
			if createdAt.Before(oldestTime) {
				oldestTime = createdAt
			}
			if createdAt.After(newestTime) {
				newestTime = createdAt
			}
		}

		// Get file size (if file is missing, treat as orphaned record)
		sizeBytes := int64(0)
		info, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				orphanedRecords++
			} else {
				s.logger.Debug("Failed to stat screenshot file when computing storage stats",
					"path", filePath, "error", err)
			}
		} else {
			sizeBytes = info.Size()
			totalSizeBytes += sizeBytes
		}

		cameraStats := stats.Cameras[cameraID]
		cameraStats.CameraID = cameraID
		cameraStats.ScreenshotCount++
		cameraStats.TotalSizeBytes += sizeBytes
		stats.Cameras[cameraID] = cameraStats
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate screenshot storage stats: %w", err)
	}

	stats.TotalScreenshots = totalScreenshots
	stats.TotalSizeBytes = totalSizeBytes
	stats.OrphanedRecordCount = orphanedRecords

	if !first {
		stats.OldestScreenshotAt = oldestTime.Format(time.RFC3339)
		stats.NewestScreenshotAt = newestTime.Format(time.RFC3339)
	}

	// Attach disk-level stats if disk monitor is available
	if s.diskMonitor != nil {
		if usage, err := s.diskMonitor.GetUsage(ctx); err == nil {
			stats.DiskTotalBytes = usage.TotalBytes
			stats.DiskUsedBytes = usage.UsedBytes
			stats.DiskAvailableBytes = usage.AvailableBytes
			stats.DiskUsagePercent = usage.UsagePercent
		} else {
			s.logger.Debug("Failed to get disk usage for screenshot storage stats", "error", err)
		}
	}

	return stats, nil
}

// CleanupStorage performs storage cleanup for labeled screenshots, including
// orphaned files, orphaned database records, and optional age-based retention.
// This implements Substep 2.2.2.4.5 storage management and cleanup.
func (s *Service) CleanupStorage(ctx context.Context, opts StorageCleanupOptions) (*StorageCleanupResult, error) {
	result := &StorageCleanupResult{}

	// Safe defaults: if nothing is specified, clean up orphaned files and records only
	if !opts.CleanupOrphanedFiles && !opts.CleanupOrphanedRecords && opts.RetentionDays == 0 {
		opts.CleanupOrphanedFiles = true
		opts.CleanupOrphanedRecords = true
	}

	// 1) Age-based retention: delete old screenshots first so that follow-up orphan checks
	// see a consistent state and don't double count files.
	if opts.RetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -opts.RetentionDays)

		query := `
			SELECT id, file_path
			FROM labeled_screenshots
			WHERE created_at < ?
		`

		rows, err := s.db.GetDB().QueryContext(ctx, query, cutoff)
		if err != nil {
			return nil, fmt.Errorf("failed to query old screenshots for retention cleanup: %w", err)
		}
		defer rows.Close()

		type rowData struct {
			id       string
			filePath string
		}

		var toDelete []rowData
		for rows.Next() {
			var id, filePath string
			if err := rows.Scan(&id, &filePath); err != nil {
				return nil, fmt.Errorf("failed to scan old screenshot row: %w", err)
			}
			toDelete = append(toDelete, rowData{id: id, filePath: filePath})
		}

		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to iterate old screenshot rows: %w", err)
		}

		for _, row := range toDelete {
			// Get file size before deletion (if possible)
			var sizeBytes int64
			if info, err := os.Stat(row.filePath); err == nil {
				sizeBytes = info.Size()
			}

			if err := os.Remove(row.filePath); err != nil && !os.IsNotExist(err) {
				s.logger.Warn("Failed to delete old screenshot file during retention cleanup",
					"path", row.filePath, "error", err)
			}

			res, err := s.db.GetDB().ExecContext(ctx,
				"DELETE FROM labeled_screenshots WHERE id = ?", row.id)
			if err != nil {
				s.logger.Warn("Failed to delete old screenshot record during retention cleanup",
					"id", row.id, "error", err)
				continue
			}

			affected, _ := res.RowsAffected()
			if affected > 0 {
				result.OldScreenshotsDeleted++
				result.FreedBytes += sizeBytes
			}
		}
	}

	// 2) Orphaned database records (records referencing files that no longer exist)
	if opts.CleanupOrphanedRecords {
		query := `
			SELECT id, file_path
			FROM labeled_screenshots
		`

		rows, err := s.db.GetDB().QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("failed to query screenshots for orphaned record cleanup: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var id, filePath string
			if err := rows.Scan(&id, &filePath); err != nil {
				return nil, fmt.Errorf("failed to scan screenshot row for orphaned record cleanup: %w", err)
			}

			if _, err := os.Stat(filePath); err != nil {
				if os.IsNotExist(err) {
					res, err := s.db.GetDB().ExecContext(ctx,
						"DELETE FROM labeled_screenshots WHERE id = ?", id)
					if err != nil {
						s.logger.Warn("Failed to delete orphaned screenshot record",
							"id", id, "path", filePath, "error", err)
						continue
					}

					affected, _ := res.RowsAffected()
					if affected > 0 {
						result.OrphanedRecordsDeleted++
					}
				} else {
					s.logger.Debug("Failed to stat screenshot file during orphaned record cleanup",
						"path", filePath, "error", err)
				}
			}
		}

		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to iterate screenshot rows for orphaned record cleanup: %w", err)
		}
	}

	// 3) Orphaned files (files on disk without a corresponding database record)
	if opts.CleanupOrphanedFiles {
		err := filepath.Walk(s.snapshotsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip directories
			if info.IsDir() {
				return nil
			}

			// Check if this file path exists in the database
			var count int
			query := `
				SELECT COUNT(1)
				FROM labeled_screenshots
				WHERE file_path = ?
			`
			if err := s.db.GetDB().QueryRowContext(ctx, query, path).Scan(&count); err != nil {
				s.logger.Warn("Failed to query screenshot record for file during orphaned file cleanup",
					"path", path, "error", err)
				return nil
			}

			// If there is no record, delete the file
			if count == 0 {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					s.logger.Warn("Failed to delete orphaned screenshot file",
						"path", path, "error", err)
					return nil
				}

				result.OrphanedFilesDeleted++
				result.FreedBytes += info.Size()
			}

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("failed to walk screenshot storage directory for orphaned file cleanup: %w", err)
		}
	}

	return result, nil
}

// GetScreenshotThumbnail reads the thumbnail image for a screenshot (Substep 2.2.2.4.4)
func (s *Service) GetScreenshotThumbnail(ctx context.Context, id string) ([]byte, error) {
	screenshot, err := s.GetScreenshot(ctx, id)
	if err != nil {
		return nil, err
	}

	// Try to get thumbnail path from metadata
	var thumbnailPath string
	if screenshot.Metadata != nil {
		if path, ok := screenshot.Metadata["thumbnail_path"].(string); ok && path != "" {
			thumbnailPath = path
		}
	}

	// If no thumbnail path in metadata, construct it
	if thumbnailPath == "" {
		filename := fmt.Sprintf("%s_%s_thumb.jpg", screenshot.CameraID, screenshot.ID)
		thumbnailPath = filepath.Join(s.thumbnailsDir, filename)
	}

	// Read thumbnail file
	data, err := os.ReadFile(thumbnailPath)
	if err != nil {
		// If thumbnail doesn't exist, fall back to full image
		return s.GetScreenshotImage(ctx, id)
	}

	return data, nil
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
	Description   string // Search term for description (LIKE search)
	CreatedAfter  time.Time
	CreatedBefore time.Time
	SortBy        string // Sort field: "created_at", "camera_id", "label", "custom_label"
	SortOrder     string // Sort order: "asc" or "desc"
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
