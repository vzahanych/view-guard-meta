package video

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// ClipRecorder records video clips from streams
type ClipRecorder struct {
	logger     *logger.Logger
	ffmpeg     *FFmpegWrapper
	outputDir  string
	recordings map[string]*Recording // cameraID -> recording
	mu         sync.RWMutex
}

// Recording represents an active recording
type Recording struct {
	CameraID    string
	Input       string
	OutputPath  string
	StartTime   time.Time
	Duration    time.Duration
	ctx         context.Context
	cancel      context.CancelFunc
	cmd         *exec.Cmd
	mu          sync.RWMutex
	completed   bool
}

// ClipMetadata contains metadata about a recorded clip
type ClipMetadata struct {
	CameraID    string
	FilePath    string
	Duration    time.Duration
	SizeBytes   int64
	StartTime   time.Time
	EndTime     time.Time
	Codec       string
	Width       int
	Height      int
	FrameRate   float64
}

// ClipRecorderConfig contains recorder configuration
type ClipRecorderConfig struct {
	OutputDir string // Directory to save clips
}

// NewClipRecorder creates a new clip recorder
func NewClipRecorder(ffmpeg *FFmpegWrapper, config ClipRecorderConfig, log *logger.Logger) (*ClipRecorder, error) {
	// Ensure output directory exists
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	return &ClipRecorder{
		logger:     log,
		ffmpeg:     ffmpeg,
		outputDir:  config.OutputDir,
		recordings: make(map[string]*Recording),
	}, nil
}

// StartRecording starts recording a clip from an input source
func (r *ClipRecorder) StartRecording(cameraID string, input string, duration time.Duration) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already recording
	if _, exists := r.recordings[cameraID]; exists {
		return "", fmt.Errorf("already recording for camera: %s", cameraID)
	}

	// Validate input
	if err := r.ffmpeg.ValidateInput(input); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	// Generate output path
	outputPath := r.generateClipPath(cameraID)

	// Create recording context
	ctx, cancel := context.WithTimeout(context.Background(), duration)

	// Build FFmpeg command
	encoder := r.ffmpeg.GetPreferredEncoder("h264")
	args := r.buildRecordingArgs(input, outputPath, encoder, duration)

	cmd := r.ffmpeg.BuildCommand(ctx, args)

	// Start recording
	if err := cmd.Start(); err != nil {
		cancel()
		return "", fmt.Errorf("failed to start recording: %w", err)
	}

	recording := &Recording{
		CameraID:   cameraID,
		Input:      input,
		OutputPath: outputPath,
		StartTime:  time.Now(),
		Duration:   duration,
		ctx:        ctx,
		cancel:     cancel,
		cmd:        cmd,
	}

	r.recordings[cameraID] = recording

	// Monitor recording completion
	go r.monitorRecording(recording)

	r.logger.Info("Started recording",
		"camera_id", cameraID,
		"input", input,
		"output", outputPath,
		"duration", duration,
	)

	return outputPath, nil
}

// StartRecordingWithWindow starts recording with a time window (pre/post event)
func (r *ClipRecorder) StartRecordingWithWindow(
	cameraID string,
	input string,
	preEventDuration time.Duration,
	postEventDuration time.Duration,
) (string, error) {
	totalDuration := preEventDuration + postEventDuration
	return r.StartRecording(cameraID, input, totalDuration)
}

// StopRecording stops recording for a camera
func (r *ClipRecorder) StopRecording(cameraID string) (*ClipMetadata, error) {
	r.mu.Lock()
	recording, exists := r.recordings[cameraID]
	if !exists {
		r.mu.Unlock()
		return nil, fmt.Errorf("no active recording for camera: %s", cameraID)
	}
	r.mu.Unlock()

	// Cancel recording context
	recording.cancel()

	// Wait for command to finish
	err := recording.cmd.Wait()

	// Mark as completed
	recording.mu.Lock()
	recording.completed = true
	actualDuration := time.Since(recording.StartTime)
	recording.mu.Unlock()

	// Remove from active recordings
	r.mu.Lock()
	delete(r.recordings, cameraID)
	r.mu.Unlock()

	if err != nil {
		// Clean up incomplete file
		os.Remove(recording.OutputPath)
		return nil, fmt.Errorf("recording failed: %w", err)
	}

	// Generate metadata
	metadata, err := r.generateMetadata(recording)
	if err != nil {
		r.logger.Warn("Failed to generate clip metadata", "error", err, "camera_id", cameraID)
		// Return basic metadata even if detailed generation fails
		metadata = &ClipMetadata{
			CameraID:  cameraID,
			FilePath:  recording.OutputPath,
			Duration:  actualDuration,
			StartTime: recording.StartTime,
			EndTime:   time.Now(),
		}
	}

	r.logger.Info("Stopped recording",
		"camera_id", cameraID,
		"output", recording.OutputPath,
		"duration", actualDuration,
	)

	return metadata, nil
}

// IsRecording checks if a camera is currently recording
func (r *ClipRecorder) IsRecording(cameraID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.recordings[cameraID]
	return exists
}

// GetRecording returns the active recording for a camera
func (r *ClipRecorder) GetRecording(cameraID string) (*Recording, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	recording, exists := r.recordings[cameraID]
	return recording, exists
}

// StopAll stops all active recordings
func (r *ClipRecorder) StopAll() map[string]*ClipMetadata {
	r.mu.Lock()
	cameraIDs := make([]string, 0, len(r.recordings))
	for cameraID := range r.recordings {
		cameraIDs = append(cameraIDs, cameraID)
	}
	r.mu.Unlock()

	results := make(map[string]*ClipMetadata)
	for _, cameraID := range cameraIDs {
		metadata, err := r.StopRecording(cameraID)
		if err != nil {
			r.logger.Warn("Failed to stop recording", "error", err, "camera_id", cameraID)
			continue
		}
		results[cameraID] = metadata
	}

	return results
}

// ListRecordings returns all active recording camera IDs
func (r *ClipRecorder) ListRecordings() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cameraIDs := make([]string, 0, len(r.recordings))
	for cameraID := range r.recordings {
		cameraIDs = append(cameraIDs, cameraID)
	}

	return cameraIDs
}

// buildRecordingArgs builds FFmpeg arguments for recording
func (r *ClipRecorder) buildRecordingArgs(input, outputPath, encoder string, duration time.Duration) []string {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", input,
		"-t", fmt.Sprintf("%.2f", duration.Seconds()), // Recording duration
		"-c:v", encoder, // Video codec
		"-preset", "medium", // Encoding preset
		"-crf", "23", // Constant Rate Factor (quality)
		"-c:a", "aac", // Audio codec (if available)
		"-b:a", "128k", // Audio bitrate
		"-movflags", "+faststart", // Enable fast start for web playback
		"-f", "mp4", // Output format
		"-y", // Overwrite output file
		outputPath,
	}

	// Add hardware acceleration flags if using hardware encoder
	if encoder == "h264_vaapi" {
		args = append([]string{"-hwaccel", "vaapi", "-hwaccel_device", "/dev/dri/renderD128"}, args...)
	} else if encoder == "h264_nvenc" {
		args = append([]string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}, args...)
	}

	return args
}

// generateClipPath generates a unique path for a clip
func (r *ClipRecorder) generateClipPath(cameraID string) string {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.mp4", cameraID, timestamp)
	return filepath.Join(r.outputDir, filename)
}

// monitorRecording monitors a recording and handles completion
func (r *ClipRecorder) monitorRecording(recording *Recording) {
	// Create channel for command completion
	cmdDone := make(chan struct{})
	go func() {
		recording.cmd.Wait()
		close(cmdDone)
	}()

	// Wait for context to be done or command to finish
	select {
	case <-recording.ctx.Done():
		// Context cancelled or timeout - kill the process
		if recording.cmd.Process != nil {
			recording.cmd.Process.Kill()
		}
		<-cmdDone // Wait for process to actually terminate
	case <-cmdDone:
		// Command finished naturally
	}

	recording.mu.Lock()
	recording.completed = true
	recording.mu.Unlock()

	// Remove from active recordings
	r.mu.Lock()
	delete(r.recordings, recording.CameraID)
	r.mu.Unlock()

	r.logger.Info("Recording completed",
		"camera_id", recording.CameraID,
		"output", recording.OutputPath,
	)
}

// generateMetadata generates metadata for a recorded clip
func (r *ClipRecorder) generateMetadata(recording *Recording) (*ClipMetadata, error) {
	// Get file info
	fileInfo, err := os.Stat(recording.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Get video properties using FFprobe (if available)
	// For now, we'll use basic metadata
	metadata := &ClipMetadata{
		CameraID:  recording.CameraID,
		FilePath:  recording.OutputPath,
		SizeBytes: fileInfo.Size(),
		StartTime: recording.StartTime,
		EndTime:   time.Now(),
		Duration:   time.Since(recording.StartTime),
		Codec:     "h264",
	}

	// Try to get video properties using FFprobe
	if props, err := r.getVideoProperties(recording.OutputPath); err == nil {
		metadata.Width = props.Width
		metadata.Height = props.Height
		metadata.FrameRate = props.FrameRate
		metadata.Duration = props.Duration
	}

	return metadata, nil
}

// VideoProperties contains video file properties
type VideoProperties struct {
	Width     int
	Height    int
	FrameRate float64
	Duration  time.Duration
}

// getVideoProperties gets video properties using FFprobe
func (r *ClipRecorder) getVideoProperties(filePath string) (*VideoProperties, error) {
	// Try to use ffprobe if available
	// For now, return error to use basic metadata
	// In production, implement FFprobe parsing
	return nil, fmt.Errorf("ffprobe not implemented")
}

// GetOutputDir returns the output directory
func (r *ClipRecorder) GetOutputDir() string {
	return r.outputDir
}

