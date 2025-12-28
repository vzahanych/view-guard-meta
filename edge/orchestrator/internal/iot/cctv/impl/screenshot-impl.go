package impl

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"
	"time"

	"github.com/google/uuid"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	"go.uber.org/zap"
)

// Image processing constants
const (
	jpegQuality      = 85  // JPEG quality (0-100, higher = better quality but larger file)
	thumbnailMaxSize = 256 // Maximum width/height for thumbnails
)

// SaveScreenshot saves a screenshot with image data and label
func (s *CCTVServiceImpl) SaveScreenshot(ctx context.Context, screenshot *cctvtypes.Screenshot, imageData []byte) error {
	// Generate ID if not provided
	if screenshot.ID == "" {
		screenshot.ID = uuid.New().String()
	}

	// Decode image to extract metadata and process
	img, format, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		s.logger.Error("Failed to decode screenshot image", zap.Error(err), zap.String("screenshot_id", screenshot.ID), zap.String("camera_id", screenshot.CameraID))
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Extract image dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Initialize metadata if nil
	if screenshot.Metadata == nil {
		screenshot.Metadata = make(map[string]interface{})
	}

	// Store image dimensions and original format in metadata
	screenshot.Metadata["width"] = width
	screenshot.Metadata["height"] = height
	screenshot.Metadata["original_format"] = format
	screenshot.Metadata["original_size_bytes"] = len(imageData)

	// Process and optimize image - normalize to JPEG
	var processedImageData []byte
	var finalFormat string = "jpeg"

	if format == "jpeg" {
		// Re-encode JPEG with quality control
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return fmt.Errorf("failed to re-encode JPEG: %w", err)
		}
		processedImageData = buf.Bytes()
	} else {
		// Convert other formats (PNG, etc.) to JPEG
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return fmt.Errorf("failed to convert to JPEG: %w", err)
		}
		processedImageData = buf.Bytes()
	}

	// Calculate compression ratio
	compressionRatio := float64(len(imageData)) / float64(len(processedImageData))
	screenshot.Metadata["compression_ratio"] = compressionRatio
	screenshot.Metadata["processed_size_bytes"] = len(processedImageData)
	screenshot.Metadata["final_format"] = finalFormat

	// Generate object keys
	if screenshot.ObjectKey == "" {
		screenshot.ObjectKey = s.objectStore.GenerateSnapshotKey(screenshot.CameraID, false)
	}

	// Store processed image in object storage
	imageReader := bytes.NewReader(processedImageData)
	if err := s.objectStore.StoreSnapshot(ctx, screenshot.ObjectKey, imageReader, int64(len(processedImageData)), "image/jpeg"); err != nil {
		return fmt.Errorf("failed to store screenshot in object storage: %w", err)
	}

	// Generate thumbnail
	thumbnailData, err := s.generateThumbnail(img)
	if err != nil {
		s.logger.Warn("Failed to generate thumbnail", zap.Error(err), zap.String("screenshot_id", screenshot.ID))
		// Don't fail the save operation if thumbnail generation fails
	} else {
		// Generate thumbnail key
		screenshot.ThumbnailKey = s.objectStore.GenerateSnapshotKey(screenshot.CameraID, true)

		// Store thumbnail in object storage
		thumbnailReader := bytes.NewReader(thumbnailData)
		if err := s.objectStore.StoreSnapshot(ctx, screenshot.ThumbnailKey, thumbnailReader, int64(len(thumbnailData)), "image/jpeg"); err != nil {
			s.logger.Warn("Failed to store thumbnail in object storage", zap.Error(err), zap.String("screenshot_id", screenshot.ID))
		} else {
			screenshot.Metadata["thumbnail_key"] = screenshot.ThumbnailKey
		}
	}

	// Set timestamps
	now := time.Now()
	if screenshot.CreatedAt.IsZero() {
		screenshot.CreatedAt = now
	}
	screenshot.UpdatedAt = now

	// Save metadata to meta-storage
	meta := s.screenshotToMeta(screenshot)
	if err := s.metaStore.SaveScreenshot(ctx, meta); err != nil {
		// Try to clean up object storage on error
		s.objectStore.DeleteSnapshot(ctx, screenshot.ObjectKey)
		if screenshot.ThumbnailKey != "" {
			s.objectStore.DeleteSnapshot(ctx, screenshot.ThumbnailKey)
		}
		return fmt.Errorf("failed to save screenshot metadata: %w", err)
	}

	// Publish event
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.DataUnitSavedEventData]{
			Type:      evtbusstypes.EventTypeDataUnitSaved,
			Source:    s.Name(),
			Timestamp: now,
			Data: evtbusstypes.DataUnitSavedEventData{
				DataUnitID: screenshot.ID,
				DeviceID:   screenshot.CameraID,
				Label:      screenshot.Label,
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish data unit saved event", zap.Error(err))
		}
	}

	s.logger.Info("Screenshot saved", zap.String("screenshot_id", screenshot.ID), zap.String("camera_id", screenshot.CameraID), zap.String("label", screenshot.Label))
	return nil
}

// generateThumbnail generates a thumbnail from an image
func (s *CCTVServiceImpl) generateThumbnail(img image.Image) ([]byte, error) {
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

	// Resize image using simple nearest-neighbor
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
		return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	return buf.Bytes(), nil
}

// GetScreenshot retrieves a screenshot by ID
func (s *CCTVServiceImpl) GetScreenshot(ctx context.Context, screenshotID string) (*cctvtypes.Screenshot, error) {
	meta, found := s.metaStore.GetScreenshot(ctx, screenshotID)
	if !found {
		return nil, fmt.Errorf("screenshot not found: %s", screenshotID)
	}

	return s.metaToScreenshot(meta), nil
}

// ListScreenshots lists screenshots with optional filters
func (s *CCTVServiceImpl) ListScreenshots(ctx context.Context, filters *cctvtypes.ScreenshotFilters) ([]*cctvtypes.Screenshot, error) {
	// Build filter map for meta-storage
	filterMap := make(map[string]interface{})
	if filters != nil {
		if filters.CameraID != nil {
			filterMap["camera_id"] = *filters.CameraID
		}
		if filters.Label != nil {
			filterMap["label"] = *filters.Label
		}
		if filters.CustomLabel != nil {
			filterMap["custom_label"] = *filters.CustomLabel
		}
		if filters.Description != nil {
			filterMap["description"] = *filters.Description
		}
		if filters.StartTime != nil {
			filterMap["start_time"] = *filters.StartTime
		}
		if filters.EndTime != nil {
			filterMap["end_time"] = *filters.EndTime
		}
		if !filters.CreatedAfter.IsZero() {
			filterMap["start_time"] = filters.CreatedAfter
		}
		if !filters.CreatedBefore.IsZero() {
			filterMap["end_time"] = filters.CreatedBefore
		}
	}

	metas, err := s.metaStore.ListScreenshots(ctx, filterMap)
	if err != nil {
		return nil, err
	}

	screenshots := make([]*cctvtypes.Screenshot, len(metas))
	for i, meta := range metas {
		screenshots[i] = s.metaToScreenshot(meta)
	}

	// Apply sorting and pagination if needed (meta-storage doesn't handle this yet)
	// For now, we'll do it in memory
	if filters != nil {
		// TODO: Implement sorting and pagination
	}

	return screenshots, nil
}

// UpdateScreenshot updates a screenshot's label and metadata
func (s *CCTVServiceImpl) UpdateScreenshot(ctx context.Context, screenshotID string, updates *cctvtypes.ScreenshotUpdate) error {
	_, err := s.metaStore.UpdateScreenshot(ctx, screenshotID, func(meta metastorage.ScreenshotMetadata) metastorage.ScreenshotMetadata {
		if updates.Label != nil {
			meta.Label = *updates.Label
		}
		if updates.CustomLabel != nil {
			meta.CustomLabel = *updates.CustomLabel
		}
		if updates.Description != nil {
			meta.Description = *updates.Description
		}
		if updates.Metadata != nil {
			meta.Metadata = updates.Metadata
		}
		return meta
	})

	if err != nil {
		return err
	}

	// Publish event
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.DataUnitUpdatedEventData]{
			Type:      evtbusstypes.EventTypeDataUnitUpdated,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: evtbusstypes.DataUnitUpdatedEventData{
				DataUnitID: screenshotID,
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish data unit updated event", zap.Error(err))
		}
	}

	return nil
}

// DeleteScreenshot deletes a screenshot
func (s *CCTVServiceImpl) DeleteScreenshot(ctx context.Context, screenshotID string) error {
	// Get screenshot to find object keys
	screenshot, err := s.GetScreenshot(ctx, screenshotID)
	if err != nil {
		return err
	}

	// Delete from object storage
	if screenshot.ObjectKey != "" {
		if err := s.objectStore.DeleteSnapshot(ctx, screenshot.ObjectKey); err != nil {
			s.logger.Warn("Failed to delete screenshot from object storage", zap.Error(err), zap.String("screenshot_id", screenshotID))
		}
	}
	if screenshot.ThumbnailKey != "" {
		if err := s.objectStore.DeleteSnapshot(ctx, screenshot.ThumbnailKey); err != nil {
			s.logger.Warn("Failed to delete thumbnail from object storage", zap.Error(err), zap.String("screenshot_id", screenshotID))
		}
	}

	// Delete from meta-storage
	if err := s.metaStore.DeleteScreenshot(ctx, screenshotID); err != nil {
		return err
	}

	// Publish event
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.DataUnitDeletedEventData]{
			Type:      evtbusstypes.EventTypeDataUnitDeleted,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: evtbusstypes.DataUnitDeletedEventData{
				DataUnitID: screenshotID,
				DeviceID:   screenshot.CameraID,
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish data unit deleted event", zap.Error(err))
		}
	}

	return nil
}

// GetScreenshotImage retrieves the full image data for a screenshot
func (s *CCTVServiceImpl) GetScreenshotImage(ctx context.Context, screenshotID string) (*cctvtypes.Frame, error) {
	screenshot, err := s.GetScreenshot(ctx, screenshotID)
	if err != nil {
		return nil, err
	}

	if screenshot.ObjectKey == "" {
		return nil, fmt.Errorf("screenshot has no object key")
	}

	reader, err := s.objectStore.LoadSnapshot(ctx, screenshot.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load screenshot from object storage: %w", err)
	}
	defer reader.Close()

	frameData, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return &cctvtypes.Frame{
		Data:      frameData,
		Timestamp: screenshot.CreatedAt,
		Width:     0, // Width/Height could be extracted from metadata if needed
		Height:    0,
		CameraID:  screenshot.CameraID,
	}, nil
}

// GetScreenshotThumbnail retrieves the thumbnail image data for a screenshot
func (s *CCTVServiceImpl) GetScreenshotThumbnail(ctx context.Context, screenshotID string) (*cctvtypes.Frame, error) {
	screenshot, err := s.GetScreenshot(ctx, screenshotID)
	if err != nil {
		return nil, err
	}

	// Try thumbnail key first
	if screenshot.ThumbnailKey != "" {
		reader, err := s.objectStore.LoadSnapshot(ctx, screenshot.ThumbnailKey)
		if err == nil {
			defer reader.Close()
			frameData, err := io.ReadAll(reader)
			if err != nil {
				return nil, err
			}
			return &cctvtypes.Frame{
				Data:      frameData,
				Timestamp: screenshot.CreatedAt,
				Width:     0, // Width/Height could be extracted from metadata if needed
				Height:    0,
				CameraID:  screenshot.CameraID,
			}, nil
		}
		// Fall through to full image if thumbnail fails
	}

	// Fall back to full image if no thumbnail
	return s.GetScreenshotImage(ctx, screenshotID)
}

// Helper functions for conversion

func (s *CCTVServiceImpl) screenshotToMeta(screenshot *cctvtypes.Screenshot) metastorage.ScreenshotMetadata {
	return metastorage.ScreenshotMetadata{
		ID:           screenshot.ID,
		CameraID:     screenshot.CameraID,
		ObjectKey:    screenshot.ObjectKey,
		ThumbnailKey: screenshot.ThumbnailKey,
		Label:        screenshot.Label,
		CustomLabel:  screenshot.CustomLabel,
		Description:  screenshot.Description,
		Metadata:     screenshot.Metadata,
		CreatedAt:    screenshot.CreatedAt,
		UpdatedAt:    screenshot.UpdatedAt,
	}
}

func (s *CCTVServiceImpl) metaToScreenshot(meta metastorage.ScreenshotMetadata) *cctvtypes.Screenshot {
	return &cctvtypes.Screenshot{
		ID:           meta.ID,
		CameraID:     meta.CameraID,
		ObjectKey:    meta.ObjectKey,
		ThumbnailKey: meta.ThumbnailKey,
		Label:        meta.Label,
		CustomLabel:  meta.CustomLabel,
		Description:  meta.Description,
		Metadata:     meta.Metadata,
		CreatedAt:    meta.CreatedAt,
		UpdatedAt:    meta.UpdatedAt,
	}
}

// Placeholder implementations for methods that need more work

// CaptureScreenshot captures a screenshot from a camera and saves it
// This method captures a frame, stores it in object storage, saves metadata in meta-storage,
// and publishes an event. The screenshot is initially unlabeled.
func (s *CCTVServiceImpl) CaptureScreenshot(ctx context.Context, cameraID string, eventID string) (string, error) {
	// Get camera to validate it exists
	cam, err := s.GetCamera(ctx, cameraID)
	if err != nil {
		return "", fmt.Errorf("camera not found: %w", err)
	}

	// Capture frame from camera
	frame, err := s.CaptureFrame(ctx, cameraID)
	if err != nil {
		return "", fmt.Errorf("failed to capture frame: %w", err)
	}

	// Create screenshot object (initially unlabeled)
	screenshot := &cctvtypes.Screenshot{
		ID:          uuid.New().String(),
		CameraID:    cameraID,
		Label:       "", // Unlabeled initially - user will label it later
		CustomLabel: "",
		Description: fmt.Sprintf("Screenshot captured from camera %s", cam.Name),
		Metadata:    make(map[string]interface{}),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Add event ID to metadata if provided
	if eventID != "" {
		screenshot.Metadata["event_id"] = eventID
	}

	// Save screenshot (this handles object storage, meta-storage, and event publishing)
	if err := s.SaveScreenshot(ctx, screenshot, frame.Data); err != nil {
		return "", fmt.Errorf("failed to save screenshot: %w", err)
	}

	s.logger.Info("Screenshot captured and saved",
		zap.String("screenshot_id", screenshot.ID),
		zap.String("camera_id", cameraID),
		zap.String("event_id", eventID),
	)

	return screenshot.ID, nil
}

func (s *CCTVServiceImpl) CaptureScreenshotWithLabel(ctx context.Context, cameraID string, label string, customLabel string, description string) (string, error) {
	// TODO: Implement labeled screenshot capture from camera stream
	return "", fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) GetDatasetStatus(ctx context.Context, cameraID string, minSnapshots int) (*cctvtypes.DatasetStatus, error) {
	// TODO: Implement dataset status calculation
	return nil, fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) GetStorageStats(ctx context.Context) (*cctvtypes.ScreenshotStorageStats, error) {
	// TODO: Implement storage stats
	return nil, fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) CleanupStorage(ctx context.Context, opts cctvtypes.StorageCleanupOptions) (*cctvtypes.StorageCleanupResult, error) {
	result := &cctvtypes.StorageCleanupResult{}

	metas, err := s.metaStore.ListScreenshots(ctx, nil)
	if err != nil {
		return nil, err
	}

	entryMap := make(map[string]metastorage.StorageEntryMetadata)
	if opts.CleanupOrphanedFiles || opts.CleanupOrphanedRecords || opts.RetentionDays > 0 {
		if entries, err := s.metaStore.ListStorageEntries(ctx, "snapshot"); err == nil {
			for _, entry := range entries {
				entryMap[entry.Path] = entry
			}
		} else if s.logger != nil {
			s.logger.Warn("Failed to list storage entries for cleanup", zap.Error(err))
		}
	}

	referenceKeys := make(map[string]struct{}, len(metas)*2)
	for _, meta := range metas {
		if meta.ObjectKey != "" {
			referenceKeys[meta.ObjectKey] = struct{}{}
		}
		if meta.ThumbnailKey != "" {
			referenceKeys[meta.ThumbnailKey] = struct{}{}
		}
	}

	var cutoff time.Time
	if opts.RetentionDays > 0 {
		cutoff = time.Now().AddDate(0, 0, -opts.RetentionDays)
	}

	for _, meta := range metas {
		if !cutoff.IsZero() && meta.CreatedAt.Before(cutoff) {
			result.FreedBytes += s.deleteScreenshotArtifacts(ctx, meta, entryMap)
			result.OldScreenshotsDeleted++
			continue
		}

		if opts.CleanupOrphanedRecords && s.isOrphanedRecord(ctx, meta, entryMap) {
			result.FreedBytes += s.deleteScreenshotArtifacts(ctx, meta, entryMap)
			result.OrphanedRecordsDeleted++
		}
	}

	if opts.CleanupOrphanedFiles && len(entryMap) > 0 {
		for key, entry := range entryMap {
			if _, ok := referenceKeys[key]; ok {
				continue
			}

			if err := s.objectStore.DeleteSnapshot(ctx, key); err != nil {
				if s.logger != nil {
					s.logger.Warn("Failed to delete orphaned snapshot file", zap.String("key", key), zap.Error(err))
				}
				continue
			}

			// Best-effort cleanup of storage entry metadata
			_ = s.metaStore.DeleteStorageEntry(ctx, key)

			result.OrphanedFilesDeleted++
			result.FreedBytes += entry.SizeBytes
		}
	}

	return result, nil
}

func (s *CCTVServiceImpl) ExportDataset(ctx context.Context, filters *cctvtypes.ScreenshotFilters, includeMetadata bool) (*cctvtypes.DatasetExportResult, error) {
	// TODO: Implement dataset export
	return nil, fmt.Errorf("not implemented")
}

func (s *CCTVServiceImpl) isOrphanedRecord(ctx context.Context, meta metastorage.ScreenshotMetadata, entryMap map[string]metastorage.StorageEntryMetadata) bool {
	if meta.ObjectKey == "" {
		return true
	}

	if len(entryMap) > 0 {
		if _, ok := entryMap[meta.ObjectKey]; ok {
			return false
		}
		return true
	}

	reader, err := s.objectStore.LoadSnapshot(ctx, meta.ObjectKey)
	if err != nil {
		return true
	}
	reader.Close()
	return false
}

func (s *CCTVServiceImpl) deleteScreenshotArtifacts(ctx context.Context, meta metastorage.ScreenshotMetadata, entryMap map[string]metastorage.StorageEntryMetadata) int64 {
	var freed int64

	if entry, ok := entryMap[meta.ObjectKey]; ok {
		freed += entry.SizeBytes
		delete(entryMap, meta.ObjectKey)
	}
	if entry, ok := entryMap[meta.ThumbnailKey]; ok {
		freed += entry.SizeBytes
		delete(entryMap, meta.ThumbnailKey)
	}

	if freed == 0 {
		freed = estimateScreenshotSize(meta)
	}

	if meta.ObjectKey != "" {
		if err := s.objectStore.DeleteSnapshot(ctx, meta.ObjectKey); err != nil && s.logger != nil {
			s.logger.Warn("Failed to delete screenshot object", zap.String("key", meta.ObjectKey), zap.Error(err))
		}
		_ = s.metaStore.DeleteStorageEntry(ctx, meta.ObjectKey)
	}

	if meta.ThumbnailKey != "" {
		if err := s.objectStore.DeleteSnapshot(ctx, meta.ThumbnailKey); err != nil && s.logger != nil {
			s.logger.Warn("Failed to delete thumbnail object", zap.String("key", meta.ThumbnailKey), zap.Error(err))
		}
		_ = s.metaStore.DeleteStorageEntry(ctx, meta.ThumbnailKey)
	}

	_ = s.metaStore.DeleteScreenshot(ctx, meta.ID)

	return freed
}

func estimateScreenshotSize(meta metastorage.ScreenshotMetadata) int64 {
	if meta.Metadata == nil {
		return 0
	}

	if size := toInt64(meta.Metadata["processed_size_bytes"]); size > 0 {
		return size
	}
	return toInt64(meta.Metadata["original_size_bytes"])
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int:
		return int64(val)
	case int64:
		return val
	case float32:
		return int64(val)
	case float64:
		return int64(val)
	default:
		return 0
	}
}
