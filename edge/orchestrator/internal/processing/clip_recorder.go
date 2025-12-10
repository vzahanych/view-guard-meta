package processing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

// ClipRecorderService handles video clip recording for detected events
type ClipRecorderService struct {
	*service.ServiceBase
	logger              *logger.Logger
	frameProcessor      FrameProcessorService
	videoRecorder       VideoRecorderService
	outputDir           string
	preEventDuration    time.Duration
	postEventDuration   time.Duration
	maxClipLength       time.Duration
	activeRecordings    map[string]*ClipRecording // eventID -> recording
	mu                  sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
}

// FrameProcessorService interface for accessing frame buffers
type FrameProcessorService interface {
	GetBuffer(cameraID string) (*FrameBuffer, bool)
}

// VideoRecorderService interface for recording video clips
type VideoRecorderService interface {
	StartRecording(cameraID string, input string, duration time.Duration) (string, error)
	StopRecording(cameraID string) (*video.ClipMetadata, error)
	IsRecording(cameraID string) bool
}

// ClipRecording represents an active clip recording
type ClipRecording struct {
	EventID           string
	CameraID          string
	Event             *events.Event
	PreEventFrames    []*video.Frame
	PostEventStartTime time.Time
	ClipPath          string
	StartTime         time.Time
	Status            RecordingStatus
	mu                sync.RWMutex
}

// RecordingStatus represents the status of a recording
type RecordingStatus string

const (
	RecordingStatusPending   RecordingStatus = "pending"
	RecordingStatusRecording RecordingStatus = "recording"
	RecordingStatusEncoding  RecordingStatus = "encoding"
	RecordingStatusCompleted RecordingStatus = "completed"
	RecordingStatusFailed    RecordingStatus = "failed"
)

// ClipRecorderServiceConfig contains clip recorder configuration
type ClipRecorderServiceConfig struct {
	FrameProcessor    FrameProcessorService
	VideoRecorder     VideoRecorderService
	OutputDir         string
	PreEventDuration  time.Duration // Duration of pre-event buffer to include
	PostEventDuration time.Duration // Duration to record after event
	MaxClipLength     time.Duration // Maximum clip length (0 = no limit)
}

// NewClipRecorderService creates a new clip recorder service
func NewClipRecorderService(config ClipRecorderServiceConfig, log *logger.Logger) (*ClipRecorderService, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Default pre-event duration (5 seconds)
	preEventDuration := config.PreEventDuration
	if preEventDuration == 0 {
		preEventDuration = 5 * time.Second
	}

	// Default post-event duration (10 seconds)
	postEventDuration := config.PostEventDuration
	if postEventDuration == 0 {
		postEventDuration = 10 * time.Second
	}

	// Ensure output directory exists
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	return &ClipRecorderService{
		ServiceBase:        service.NewServiceBase("clip-recorder-service", log),
		logger:             log,
		frameProcessor:     config.FrameProcessor,
		videoRecorder:      config.VideoRecorder,
		outputDir:          config.OutputDir,
		preEventDuration:   preEventDuration,
		postEventDuration:  postEventDuration,
		maxClipLength:      config.MaxClipLength,
		activeRecordings:   make(map[string]*ClipRecording),
		ctx:                ctx,
		cancel:             cancel,
	}, nil
}

// Start starts the clip recorder service
func (crs *ClipRecorderService) Start(ctx context.Context) error {
	crs.LogInfo("Clip recorder service started",
		"output_dir", crs.outputDir,
		"pre_event_duration", crs.preEventDuration,
		"post_event_duration", crs.postEventDuration,
		"max_clip_length", crs.maxClipLength,
	)
	return nil
}

// Stop stops the clip recorder service
func (crs *ClipRecorderService) Stop() error {
	crs.cancel()

	// Stop all active recordings
	crs.mu.Lock()
	eventIDs := make([]string, 0, len(crs.activeRecordings))
	for eventID := range crs.activeRecordings {
		eventIDs = append(eventIDs, eventID)
	}
	crs.mu.Unlock()

	for _, eventID := range eventIDs {
		if err := crs.StopRecording(eventID); err != nil {
			crs.logger.Warn("Failed to stop recording", "error", err, "event_id", eventID)
		}
	}

	crs.LogInfo("Clip recorder service stopped")
	return nil
}

// RecordEventClip records a video clip for a detected event
func (crs *ClipRecorderService) RecordEventClip(ctx context.Context, event *events.Event, cameraStreamInput string) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Check if already recording for this event
	crs.mu.Lock()
	if _, exists := crs.activeRecordings[event.ID]; exists {
		crs.mu.Unlock()
		return fmt.Errorf("already recording clip for event: %s", event.ID)
	}
	crs.mu.Unlock()

	// Get pre-event frames from frame buffer
	preEventFrames := crs.getPreEventFrames(event.CameraID, event.Timestamp)

	// Create recording
	recording := &ClipRecording{
		EventID:            event.ID,
		CameraID:           event.CameraID,
		Event:              event,
		PreEventFrames:     preEventFrames,
		PostEventStartTime: time.Now(),
		StartTime:          time.Now(),
		Status:             RecordingStatusPending,
	}

	// Generate clip path
	clipPath := crs.generateClipPath(event.ID, event.CameraID)
	recording.ClipPath = clipPath

	// Add to active recordings
	crs.mu.Lock()
	crs.activeRecordings[event.ID] = recording
	crs.mu.Unlock()

	crs.logger.Info("Starting event clip recording",
		"event_id", event.ID,
		"camera_id", event.CameraID,
		"pre_event_frames", len(preEventFrames),
		"post_event_duration", crs.postEventDuration,
		"clip_path", clipPath,
	)

	// Start recording in background
	go crs.recordClip(ctx, recording, cameraStreamInput)

	return nil
}

// getPreEventFrames retrieves pre-event frames from the frame buffer
func (crs *ClipRecorderService) getPreEventFrames(cameraID string, eventTime time.Time) []*video.Frame {
	if crs.frameProcessor == nil {
		return []*video.Frame{}
	}

	buffer, ok := crs.frameProcessor.GetBuffer(cameraID)
	if !ok {
		crs.logger.Debug("No frame buffer available for camera", "camera_id", cameraID)
		return []*video.Frame{}
	}

	// Get frames before event time (within pre-event duration)
	cutoffTime := eventTime.Add(-crs.preEventDuration)
	frames := buffer.GetFramesBefore(eventTime)

	// Filter frames within pre-event duration
	preEventFrames := make([]*video.Frame, 0)
	for _, frame := range frames {
		if frame.Timestamp.After(cutoffTime) || frame.Timestamp.Equal(cutoffTime) {
			preEventFrames = append(preEventFrames, frame)
		}
	}

	return preEventFrames
}

// recordClip records the clip (pre-event frames + post-event recording)
func (crs *ClipRecorderService) recordClip(ctx context.Context, recording *ClipRecording, cameraStreamInput string) {
	recording.mu.Lock()
	recording.Status = RecordingStatusRecording
	recording.mu.Unlock()

	// For PoC, we'll use a simplified approach:
	// 1. Save pre-event frames as images (if any)
	// 2. Record post-event from camera stream
	// 3. Combine them into a single clip

	// Step 1: Handle pre-event frames
	// For now, we'll just record from the stream starting slightly before the event
	// In production, we'd combine pre-event frames with post-event recording

	// Step 2: Record post-event from camera stream
	// Calculate recording duration (post-event + some pre-event overlap)
	recordingDuration := crs.postEventDuration
	if crs.maxClipLength > 0 && recordingDuration > crs.maxClipLength {
		recordingDuration = crs.maxClipLength
	}

	// Start recording from camera stream
	// Note: For pre-event, we'd ideally use the frame buffer, but for PoC we'll
	// record from the stream with a slight offset to capture some pre-event context
	clipPath, err := crs.videoRecorder.StartRecording(
		recording.CameraID,
		cameraStreamInput,
		recordingDuration,
	)

	if err != nil {
		recording.mu.Lock()
		recording.Status = RecordingStatusFailed
		recording.mu.Unlock()

		crs.logger.Error("Failed to start clip recording",
			"error", err,
			"event_id", recording.EventID,
			"camera_id", recording.CameraID,
		)

		// Remove from active recordings
		crs.mu.Lock()
		delete(crs.activeRecordings, recording.EventID)
		crs.mu.Unlock()

		return
	}

	// Update recording with actual clip path
	recording.mu.Lock()
	recording.ClipPath = clipPath
	recording.mu.Unlock()

	// Wait for recording to complete
	// The video recorder will handle the recording duration
	// We'll monitor it and update status when done
	go crs.monitorRecording(ctx, recording)
}

// monitorRecording monitors a recording and updates status
func (crs *ClipRecorderService) monitorRecording(ctx context.Context, recording *ClipRecording) {
	// Wait for recording duration
	select {
	case <-ctx.Done():
		return
	case <-crs.ctx.Done():
		return
	case <-time.After(crs.postEventDuration):
		// Recording should be complete
	}

	// Check if recording is still active
	if !crs.videoRecorder.IsRecording(recording.CameraID) {
		// Recording completed
		recording.mu.Lock()
		recording.Status = RecordingStatusCompleted
		recording.mu.Unlock()

		// Get clip metadata
		metadata, err := crs.videoRecorder.StopRecording(recording.CameraID)
		if err != nil {
			crs.logger.Warn("Failed to get clip metadata",
				"error", err,
				"event_id", recording.EventID,
			)
		} else {
			// Update event with clip path
			recording.Event.ClipPath = recording.ClipPath
			crs.logger.Info("Clip recording completed",
				"event_id", recording.EventID,
				"camera_id", recording.CameraID,
				"clip_path", recording.ClipPath,
				"duration", metadata.Duration,
				"size_bytes", metadata.SizeBytes,
			)
		}

		// Remove from active recordings
		crs.mu.Lock()
		delete(crs.activeRecordings, recording.EventID)
		crs.mu.Unlock()
	}
}

// StopRecording stops recording for an event
func (crs *ClipRecorderService) StopRecording(eventID string) error {
	crs.mu.RLock()
	recording, exists := crs.activeRecordings[eventID]
	crs.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no active recording for event: %s", eventID)
	}

	// Stop video recorder
	if crs.videoRecorder.IsRecording(recording.CameraID) {
		metadata, err := crs.videoRecorder.StopRecording(recording.CameraID)
		if err != nil {
			recording.mu.Lock()
			recording.Status = RecordingStatusFailed
			recording.mu.Unlock()
			return fmt.Errorf("failed to stop recording: %w", err)
		}

		// Update event with clip path
		recording.Event.ClipPath = recording.ClipPath

		recording.mu.Lock()
		recording.Status = RecordingStatusCompleted
		recording.mu.Unlock()

		crs.logger.Info("Clip recording stopped",
			"event_id", eventID,
			"clip_path", recording.ClipPath,
			"duration", metadata.Duration,
		)
	}

	// Remove from active recordings
	crs.mu.Lock()
	delete(crs.activeRecordings, eventID)
	crs.mu.Unlock()

	return nil
}

// generateClipPath generates a unique path for a clip
func (crs *ClipRecorderService) generateClipPath(eventID string, cameraID string) string {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s_%s.mp4", eventID, cameraID, timestamp)
	return filepath.Join(crs.outputDir, filename)
}

// GetRecording returns the active recording for an event
func (crs *ClipRecorderService) GetRecording(eventID string) (*ClipRecording, bool) {
	crs.mu.RLock()
	defer crs.mu.RUnlock()
	recording, exists := crs.activeRecordings[eventID]
	return recording, exists
}

// ListRecordings returns all active recording event IDs
func (crs *ClipRecorderService) ListRecordings() []string {
	crs.mu.RLock()
	defer crs.mu.RUnlock()

	eventIDs := make([]string, 0, len(crs.activeRecordings))
	for eventID := range crs.activeRecordings {
		eventIDs = append(eventIDs, eventID)
	}
	return eventIDs
}

// SetPreEventDuration sets the pre-event duration
func (crs *ClipRecorderService) SetPreEventDuration(duration time.Duration) {
	crs.mu.Lock()
	defer crs.mu.Unlock()
	crs.preEventDuration = duration
}

// SetPostEventDuration sets the post-event duration
func (crs *ClipRecorderService) SetPostEventDuration(duration time.Duration) {
	crs.mu.Lock()
	defer crs.mu.Unlock()
	crs.postEventDuration = duration
}

// SetMaxClipLength sets the maximum clip length
func (crs *ClipRecorderService) SetMaxClipLength(duration time.Duration) {
	crs.mu.Lock()
	defer crs.mu.Unlock()
	crs.maxClipLength = duration
}

// GetPreEventDuration returns the pre-event duration
func (crs *ClipRecorderService) GetPreEventDuration() time.Duration {
	crs.mu.RLock()
	defer crs.mu.RUnlock()
	return crs.preEventDuration
}

// GetPostEventDuration returns the post-event duration
func (crs *ClipRecorderService) GetPostEventDuration() time.Duration {
	crs.mu.RLock()
	defer crs.mu.RUnlock()
	return crs.postEventDuration
}

// GetMaxClipLength returns the maximum clip length
func (crs *ClipRecorderService) GetMaxClipLength() time.Duration {
	crs.mu.RLock()
	defer crs.mu.RUnlock()
	return crs.maxClipLength
}

