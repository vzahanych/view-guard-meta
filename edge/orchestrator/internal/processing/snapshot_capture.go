package processing

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

// SnapshotCaptureService handles snapshot capture for detected events
type SnapshotCaptureService struct {
	*service.ServiceBase
	logger    *logger.Logger
	outputDir string
	jpegQuality int
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// SnapshotCaptureServiceConfig contains snapshot capture configuration
type SnapshotCaptureServiceConfig struct {
	OutputDir   string // Directory to save snapshots
	JPEGQuality int    // JPEG quality (1-100, default 85)
}

// SnapshotMetadata contains metadata about a captured snapshot
type SnapshotMetadata struct {
	EventID      string
	CameraID     string
	EventType    string
	FilePath     string
	SizeBytes    int64
	Width        int
	Height       int
	Timestamp    time.Time
	JPEGQuality  int
}

// NewSnapshotCaptureService creates a new snapshot capture service
func NewSnapshotCaptureService(config SnapshotCaptureServiceConfig, log *logger.Logger) (*SnapshotCaptureService, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Default JPEG quality
	jpegQuality := config.JPEGQuality
	if jpegQuality == 0 {
		jpegQuality = 85 // Default: 85% quality
	}
	if jpegQuality < 1 || jpegQuality > 100 {
		return nil, fmt.Errorf("JPEG quality must be between 1 and 100, got %d", jpegQuality)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	return &SnapshotCaptureService{
		ServiceBase: service.NewServiceBase("snapshot-capture-service", log),
		logger:      log,
		outputDir:   config.OutputDir,
		jpegQuality: jpegQuality,
		ctx:         ctx,
		cancel:      cancel,
	}, nil
}

// Start starts the snapshot capture service
func (scs *SnapshotCaptureService) Start(ctx context.Context) error {
	scs.LogInfo("Snapshot capture service started",
		"output_dir", scs.outputDir,
		"jpeg_quality", scs.jpegQuality,
	)
	return nil
}

// Stop stops the snapshot capture service
func (scs *SnapshotCaptureService) Stop() error {
	scs.cancel()
	scs.LogInfo("Snapshot capture service stopped")
	return nil
}

// CaptureEventSnapshot captures a snapshot for a detected event
func (scs *SnapshotCaptureService) CaptureEventSnapshot(ctx context.Context, event *events.Event, frame *video.Frame) (*SnapshotMetadata, error) {
	if event == nil {
		return nil, fmt.Errorf("event is nil")
	}

	// Use provided frame or try to get from event metadata
	if frame == nil {
		// Try to extract frame from event metadata
		// For now, we'll require frame to be provided
		return nil, fmt.Errorf("frame is required for snapshot capture")
	}

	// Generate snapshot path
	snapshotPath := scs.generateSnapshotPath(event.ID, event.CameraID)

	// Encode frame as JPEG and save
	metadata, err := scs.saveSnapshot(frame, snapshotPath, event)
	if err != nil {
		return nil, fmt.Errorf("failed to save snapshot: %w", err)
	}

	// Update event with snapshot path
	event.SnapshotPath = snapshotPath

	scs.logger.Info("Snapshot captured",
		"event_id", event.ID,
		"camera_id", event.CameraID,
		"event_type", event.EventType,
		"snapshot_path", snapshotPath,
		"size_bytes", metadata.SizeBytes,
		"width", metadata.Width,
		"height", metadata.Height,
	)

	return metadata, nil
}

// saveSnapshot saves a frame as a JPEG snapshot
func (scs *SnapshotCaptureService) saveSnapshot(frame *video.Frame, snapshotPath string, event *events.Event) (*SnapshotMetadata, error) {
	// Decode JPEG frame
	img, err := jpeg.Decode(bytes.NewReader(frame.Data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame: %w", err)
	}

	// Get image dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Create output file
	file, err := os.Create(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot file: %w", err)
	}
	defer file.Close()

	// Encode as JPEG
	err = jpeg.Encode(file, img, &jpeg.Options{Quality: scs.jpegQuality})
	if err != nil {
		// Clean up file on error
		os.Remove(snapshotPath)
		return nil, fmt.Errorf("failed to encode JPEG: %w", err)
	}

	// Get file size
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Create metadata
	metadata := &SnapshotMetadata{
		EventID:     event.ID,
		CameraID:    event.CameraID,
		EventType:   event.EventType,
		FilePath:    snapshotPath,
		SizeBytes:   fileInfo.Size(),
		Width:       width,
		Height:      height,
		Timestamp:   frame.Timestamp,
		JPEGQuality: scs.jpegQuality,
	}

	return metadata, nil
}

// generateSnapshotPath generates a unique path for a snapshot
func (scs *SnapshotCaptureService) generateSnapshotPath(eventID string, cameraID string) string {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s_%s.jpg", eventID, cameraID, timestamp)
	return filepath.Join(scs.outputDir, filename)
}

// SetJPEGQuality sets the JPEG quality
func (scs *SnapshotCaptureService) SetJPEGQuality(quality int) error {
	if quality < 1 || quality > 100 {
		return fmt.Errorf("JPEG quality must be between 1 and 100, got %d", quality)
	}

	scs.mu.Lock()
	defer scs.mu.Unlock()
	scs.jpegQuality = quality
	return nil
}

// GetJPEGQuality returns the current JPEG quality
func (scs *SnapshotCaptureService) GetJPEGQuality() int {
	scs.mu.RLock()
	defer scs.mu.RUnlock()
	return scs.jpegQuality
}

// GetOutputDir returns the output directory
func (scs *SnapshotCaptureService) GetOutputDir() string {
	scs.mu.RLock()
	defer scs.mu.RUnlock()
	return scs.outputDir
}

// CaptureSnapshotFromBytes captures a snapshot from raw JPEG bytes
func (scs *SnapshotCaptureService) CaptureSnapshotFromBytes(
	ctx context.Context,
	event *events.Event,
	frameData []byte,
	timestamp time.Time,
) (*SnapshotMetadata, error) {
	if event == nil {
		return nil, fmt.Errorf("event is nil")
	}

	if len(frameData) == 0 {
		return nil, fmt.Errorf("frame data is empty")
	}

	// Create a frame from bytes
	frame := &video.Frame{
		Data:      frameData,
		Timestamp: timestamp,
		CameraID:  event.CameraID,
	}

	// Decode to get dimensions
	img, err := jpeg.Decode(bytes.NewReader(frameData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode frame data: %w", err)
	}

	bounds := img.Bounds()
	frame.Width = bounds.Dx()
	frame.Height = bounds.Dy()

	// Capture snapshot
	return scs.CaptureEventSnapshot(ctx, event, frame)
}

