package storage

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/color"
	"os"
	"path/filepath"
	"sync"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// SnapshotGenerator generates snapshots from video frames
type SnapshotGenerator struct {
	logger       *logger.Logger
	storage      *StorageService
	mu           sync.RWMutex
	quality      int // JPEG quality (1-100)
	thumbnailSize int // Thumbnail max dimension
}

// SnapshotConfig contains snapshot generator configuration
type SnapshotConfig struct {
	Quality       int // JPEG quality (1-100, default 85)
	ThumbnailSize int // Thumbnail max dimension (default 320)
}

// NewSnapshotGenerator creates a new snapshot generator
func NewSnapshotGenerator(storage *StorageService, config SnapshotConfig, log *logger.Logger) *SnapshotGenerator {
	quality := config.Quality
	if quality == 0 {
		quality = 85 // Default JPEG quality
	}
	if quality < 1 || quality > 100 {
		quality = 85
	}

	thumbnailSize := config.ThumbnailSize
	if thumbnailSize == 0 {
		thumbnailSize = 320 // Default thumbnail size
	}

	return &SnapshotGenerator{
		logger:        log,
		storage:       storage,
		quality:      quality,
		thumbnailSize: thumbnailSize,
	}
}

// GenerateSnapshot generates a JPEG snapshot from a frame
func (s *SnapshotGenerator) GenerateSnapshot(ctx context.Context, frameData []byte, cameraID string, eventID string) (string, error) {
	// Decode JPEG frame
	img, _, err := image.Decode(bytes.NewReader(frameData))
	if err != nil {
		return "", fmt.Errorf("failed to decode frame: %w", err)
	}

	// Generate snapshot path
	snapshotPath := s.storage.GenerateSnapshotPath(cameraID, false)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// Create file
	file, err := os.Create(snapshotPath)
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot file: %w", err)
	}
	defer file.Close()

	// Encode as JPEG
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: s.quality}); err != nil {
		return "", fmt.Errorf("failed to encode snapshot: %w", err)
	}

	// Get file size
	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat snapshot file: %w", err)
	}

	// Save to storage state
	if err := s.storage.SaveSnapshot(ctx, snapshotPath, cameraID, eventID, fileInfo.Size()); err != nil {
		s.logger.Warn("Failed to save snapshot entry", "path", snapshotPath, "error", err)
	}

	s.logger.Info("Generated snapshot", "path", snapshotPath, "camera_id", cameraID, "event_id", eventID)

	return snapshotPath, nil
}

// GenerateThumbnail generates a thumbnail from a frame
func (s *SnapshotGenerator) GenerateThumbnail(ctx context.Context, frameData []byte, cameraID string, eventID string) (string, error) {
	// Decode JPEG frame
	img, _, err := image.Decode(bytes.NewReader(frameData))
	if err != nil {
		return "", fmt.Errorf("failed to decode frame: %w", err)
	}

	// Resize to thumbnail size
	thumbnail := s.resizeImage(img, s.thumbnailSize)

	// Generate thumbnail path
	thumbnailPath := s.storage.GenerateSnapshotPath(cameraID, true)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create thumbnail directory: %w", err)
	}

	// Create file
	file, err := os.Create(thumbnailPath)
	if err != nil {
		return "", fmt.Errorf("failed to create thumbnail file: %w", err)
	}
	defer file.Close()

	// Encode as JPEG with lower quality for thumbnails
	thumbnailQuality := s.quality
	if thumbnailQuality > 70 {
		thumbnailQuality = 70 // Lower quality for thumbnails
	}

	if err := jpeg.Encode(file, thumbnail, &jpeg.Options{Quality: thumbnailQuality}); err != nil {
		return "", fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	// Get file size
	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat thumbnail file: %w", err)
	}

	// Save to storage state
	if err := s.storage.SaveSnapshot(ctx, thumbnailPath, cameraID, eventID, fileInfo.Size()); err != nil {
		s.logger.Warn("Failed to save thumbnail entry", "path", thumbnailPath, "error", err)
	}

	s.logger.Info("Generated thumbnail", "path", thumbnailPath, "camera_id", cameraID, "event_id", eventID)

	return thumbnailPath, nil
}

// resizeImage resizes an image to fit within maxSize while maintaining aspect ratio
func (s *SnapshotGenerator) resizeImage(img image.Image, maxSize int) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Calculate new dimensions
	var newWidth, newHeight int
	if width > height {
		// Landscape
		if width > maxSize {
			newWidth = maxSize
			newHeight = (height * maxSize) / width
		} else {
			return img // No resize needed
		}
	} else {
		// Portrait or square
		if height > maxSize {
			newHeight = maxSize
			newWidth = (width * maxSize) / height
		} else {
			return img // No resize needed
		}
	}

	// Simple resize using nearest neighbor (for basic implementation)
	// In production, you'd use a better resampling algorithm
	resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	
	// Simple nearest neighbor resize
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := (x * width) / newWidth
			srcY := (y * height) / newHeight
			if srcX < width && srcY < height {
				resized.Set(x, y, img.At(srcX, srcY))
			} else {
				resized.Set(x, y, color.RGBA{0, 0, 0, 0})
			}
		}
	}

	return resized
}

