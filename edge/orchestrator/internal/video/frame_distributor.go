package video

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// FrameDistributor distributes frames to AI service
type FrameDistributor struct {
	logger      *logger.Logger
	extractors  map[string]*FrameExtractor // cameraID -> extractor
	onFrame     func(*Frame)               // Callback for frames to send to AI
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// FrameDistributorConfig contains distributor configuration
type FrameDistributorConfig struct {
	OnFrame func(*Frame) // Callback for frames to send to AI service
}

// NewFrameDistributor creates a new frame distributor
func NewFrameDistributor(config FrameDistributorConfig, log *logger.Logger) *FrameDistributor {
	ctx, cancel := context.WithCancel(context.Background())

	return &FrameDistributor{
		logger:     log,
		extractors: make(map[string]*FrameExtractor),
		onFrame:    config.OnFrame,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// AddExtractor adds a frame extractor for a camera
func (d *FrameDistributor) AddExtractor(cameraID string, extractor *FrameExtractor) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Stop existing extractor if any
	if existing, ok := d.extractors[cameraID]; ok {
		existing.Stop()
	}

	d.extractors[cameraID] = extractor

	// Set up frame callback
	extractor.onFrame = func(frame *Frame) {
		// Call distributor callback
		if d.onFrame != nil {
			d.onFrame(frame)
		}
	}

	d.logger.Info("Frame extractor added", "camera_id", cameraID)
}

// RemoveExtractor removes a frame extractor for a camera
func (d *FrameDistributor) RemoveExtractor(cameraID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if extractor, ok := d.extractors[cameraID]; ok {
		extractor.Stop()
		delete(d.extractors, cameraID)
		d.logger.Info("Frame extractor removed", "camera_id", cameraID)
	}
}

// StartExtraction starts frame extraction for a camera
func (d *FrameDistributor) StartExtraction(cameraID string, input string) error {
	d.mu.RLock()
	extractor, ok := d.extractors[cameraID]
	d.mu.RUnlock()

	if !ok {
		return fmt.Errorf("extractor not found for camera: %s", cameraID)
	}

	return extractor.Start(input, cameraID)
}

// StopExtraction stops frame extraction for a camera
func (d *FrameDistributor) StopExtraction(cameraID string) {
	d.mu.RLock()
	extractor, ok := d.extractors[cameraID]
	d.mu.RUnlock()

	if ok {
		extractor.Stop()
		d.logger.Info("Frame extraction stopped", "camera_id", cameraID)
	}
}

// StopAll stops all frame extractors
func (d *FrameDistributor) StopAll() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for cameraID, extractor := range d.extractors {
		extractor.Stop()
		d.logger.Info("Frame extraction stopped", "camera_id", cameraID)
	}

	d.cancel()
}

// GetExtractor returns the extractor for a camera
func (d *FrameDistributor) GetExtractor(cameraID string) (*FrameExtractor, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	extractor, ok := d.extractors[cameraID]
	return extractor, ok
}

// ListExtractors returns all camera IDs with active extractors
func (d *FrameDistributor) ListExtractors() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cameraIDs := make([]string, 0, len(d.extractors))
	for cameraID := range d.extractors {
		cameraIDs = append(cameraIDs, cameraID)
	}

	return cameraIDs
}

// GetFrameStats returns statistics for frame extraction
func (d *FrameDistributor) GetFrameStats(cameraID string) (FrameStats, error) {
	d.mu.RLock()
	extractor, ok := d.extractors[cameraID]
	d.mu.RUnlock()

	if !ok {
		return FrameStats{}, fmt.Errorf("extractor not found for camera: %s", cameraID)
	}

	stats := FrameStats{
		CameraID:       cameraID,
		Running:        extractor.IsRunning(),
		BufferSize:     extractor.GetBufferSize(),
		ExtractInterval: extractor.GetExtractInterval(),
		BufferUsage:    len(extractor.frameBuffer),
	}

	return stats, nil
}

// FrameStats contains frame extraction statistics
type FrameStats struct {
	CameraID        string
	Running         bool
	BufferSize      int
	ExtractInterval time.Duration
	BufferUsage     int
}

