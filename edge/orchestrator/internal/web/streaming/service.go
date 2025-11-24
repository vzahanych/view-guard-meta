package streaming

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"os/exec"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

// Service manages camera streaming for the web UI
type Service struct {
	logger    *logger.Logger
	cameraMgr *camera.Manager
	ffmpeg    *video.FFmpegWrapper
	streams   map[string]*Stream // Active streams by camera ID
	mu        sync.RWMutex
}

// Stream represents an active MJPEG stream
type Stream struct {
	CameraID    string
	FrameChan   chan []byte
	ctx         context.Context
	cancel      context.CancelFunc
	lastFrame   []byte
	lastFrameMu sync.RWMutex
}

// Done returns a channel that's closed when the stream context is done
func (s *Stream) Done() <-chan struct{} {
	return s.ctx.Done()
}

// NewService creates a new streaming service
func NewService(cameraMgr *camera.Manager, ffmpeg *video.FFmpegWrapper, log *logger.Logger) *Service {
	return &Service{
		logger:    log,
		cameraMgr: cameraMgr,
		ffmpeg:    ffmpeg,
		streams:   make(map[string]*Stream),
	}
}

// GetFrame captures a single JPEG frame from a camera
func (s *Service) GetFrame(cameraID string) ([]byte, error) {
	// Get camera
	cam, err := s.cameraMgr.GetCamera(cameraID)
	if err != nil {
		return nil, fmt.Errorf("camera not found: %w", err)
	}

	// Get input source (RTSP URL or USB device path)
	input := s.getCameraInput(cam)
	if input == "" {
		return nil, fmt.Errorf("no valid input source for camera %s", cameraID)
	}

	// Use FFmpeg to capture a single frame
	// For RTSP: ffmpeg -rtsp_transport tcp -i <rtsp_url> -frames:v 1 -f image2pipe -vcodec mjpeg -
	// For USB: ffmpeg -f v4l2 -input_format mjpeg -video_size 640x480 -i <device> -frames:v 1 -f image2pipe -vcodec mjpeg -

	var cmd *exec.Cmd
	if cam.Type == camera.CameraTypeUSB {
		// USB camera
		cmd = exec.Command("ffmpeg",
			"-f", "v4l2",
			"-input_format", "mjpeg",
			"-video_size", "640x480",
			"-i", input,
			"-frames:v", "1",
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-q:v", "2", // High quality
			"-",
		)
	} else {
		// RTSP/ONVIF camera
		cmd = exec.Command("ffmpeg",
			"-rtsp_transport", "tcp",
			"-i", input,
			"-frames:v", "1",
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-q:v", "2", // High quality
			"-",
		)
	}

	// Set timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w, stderr: %s", err, stderr.String())
	}

	frameData := stdout.Bytes()
	if len(frameData) == 0 {
		return nil, fmt.Errorf("no frame data captured")
	}

	// Validate JPEG
	if _, err := jpeg.Decode(bytes.NewReader(frameData)); err != nil {
		return nil, fmt.Errorf("invalid JPEG data: %w", err)
	}

	return frameData, nil
}

// StartStream starts an MJPEG stream for a camera
func (s *Service) StartStream(cameraID string) (*Stream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if stream already exists
	if stream, ok := s.streams[cameraID]; ok {
		return stream, nil
	}

	// Get camera
	cam, err := s.cameraMgr.GetCamera(cameraID)
	if err != nil {
		return nil, fmt.Errorf("camera not found: %w", err)
	}

	// Get input source
	input := s.getCameraInput(cam)
	if input == "" {
		return nil, fmt.Errorf("no valid input source for camera %s", cameraID)
	}

	// Create stream
	ctx, cancel := context.WithCancel(context.Background())
	stream := &Stream{
		CameraID:  cameraID,
		FrameChan: make(chan []byte, 10), // Buffer up to 10 frames
		ctx:       ctx,
		cancel:    cancel,
	}

	s.streams[cameraID] = stream

	// Start frame capture goroutine
	go s.captureFrames(stream, cam, input)

	s.logger.Info("Started MJPEG stream", "camera_id", cameraID)
	return stream, nil
}

// StopStream stops an MJPEG stream
func (s *Service) StopStream(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream, ok := s.streams[cameraID]
	if !ok {
		return
	}

	stream.cancel()
	delete(s.streams, cameraID)

	s.logger.Info("Stopped MJPEG stream", "camera_id", cameraID)
}

// GetStream gets an active stream
func (s *Service) GetStream(cameraID string) (*Stream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stream, ok := s.streams[cameraID]
	if !ok {
		return nil, fmt.Errorf("stream not found for camera %s", cameraID)
	}

	return stream, nil
}

// captureFrames continuously captures frames from a camera
func (s *Service) captureFrames(stream *Stream, cam *camera.Camera, input string) {
	ticker := time.NewTicker(100 * time.Millisecond) // ~10 FPS for MJPEG stream
	defer ticker.Stop()
	defer close(stream.FrameChan)

	for {
		select {
		case <-stream.ctx.Done():
			return
		case <-ticker.C:
			// Capture frame
			frame, err := s.GetFrame(cam.ID)
			if err != nil {
				s.logger.Debug("Failed to capture frame", "camera_id", cam.ID, "error", err)
				continue
			}

			// Update last frame
			stream.lastFrameMu.Lock()
			stream.lastFrame = frame
			stream.lastFrameMu.Unlock()

			// Send frame (non-blocking)
			select {
			case stream.FrameChan <- frame:
			default:
				// Channel full, skip this frame
			}
		}
	}
}

// GetLastFrame gets the last captured frame from a stream
func (s *Stream) GetLastFrame() []byte {
	s.lastFrameMu.RLock()
	defer s.lastFrameMu.RUnlock()
	return s.lastFrame
}

// getCameraInput gets the input source for a camera
func (s *Service) getCameraInput(cam *camera.Camera) string {
	if cam.Type == camera.CameraTypeUSB {
		return cam.DevicePath
	}

	// For RTSP/ONVIF, use the first RTSP URL
	if len(cam.RTSPURLs) > 0 {
		return cam.RTSPURLs[0]
	}

	return ""
}
