package impl

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"os/exec"
	"sync"
	"time"

	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	"go.uber.org/zap"
)

// streamingSession represents an active MJPEG stream
type streamingSession struct {
	CameraID    string
	FrameChan   chan []byte
	ctx         context.Context
	cancel      context.CancelFunc
	lastFrame   []byte
	lastFrameMu sync.RWMutex
}

// Done returns a channel that's closed when the stream context is done
func (s *streamingSession) Done() <-chan struct{} {
	return s.ctx.Done()
}

// GetLastFrame gets the last captured frame from a stream
func (s *streamingSession) GetLastFrame() []byte {
	s.lastFrameMu.RLock()
	defer s.lastFrameMu.RUnlock()
	return s.lastFrame
}

// startMJPEGStream starts an MJPEG stream for a camera
func (s *CCTVServiceImpl) startMJPEGStream(ctx context.Context, cameraID string) (*streamingSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if stream already exists
	if stream, ok := s.activeStreams[cameraID]; ok {
		return stream, nil
	}

	// Get camera
	cam, err := s.GetCamera(ctx, cameraID)
	if err != nil {
		return nil, fmt.Errorf("camera not found: %w", err)
	}

	// Get input source
	input := s.getCameraInput(cam)
	if input == "" {
		return nil, fmt.Errorf("no valid input source for camera %s", cameraID)
	}

	// Create stream
	streamCtx, cancel := context.WithCancel(context.Background())
	stream := &streamingSession{
		CameraID:  cameraID,
		FrameChan: make(chan []byte, 10), // Buffer up to 10 frames
		ctx:       streamCtx,
		cancel:    cancel,
	}

	s.activeStreams[cameraID] = stream

	// Start frame capture goroutine
	go s.captureFrames(stream, cam, input)

	s.logger.Info("Started MJPEG stream", zap.String("camera_id", cameraID))
	return stream, nil
}

// stopMJPEGStream stops an MJPEG stream
func (s *CCTVServiceImpl) stopMJPEGStream(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream, ok := s.activeStreams[cameraID]
	if !ok {
		return
	}

	stream.cancel()
	delete(s.activeStreams, cameraID)

	s.logger.Info("Stopped MJPEG stream", zap.String("camera_id", cameraID))
}

// getMJPEGStream gets an active MJPEG stream
func (s *CCTVServiceImpl) getMJPEGStream(cameraID string) (*streamingSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stream, ok := s.activeStreams[cameraID]
	if !ok {
		return nil, fmt.Errorf("stream not found for camera %s", cameraID)
	}

	return stream, nil
}

// captureFrames continuously captures frames from a camera
func (s *CCTVServiceImpl) captureFrames(stream *streamingSession, cam *cctvtypes.Camera, input string) {
	ticker := time.NewTicker(100 * time.Millisecond) // ~10 FPS for MJPEG stream
	defer ticker.Stop()
	defer close(stream.FrameChan)

	for {
		select {
		case <-stream.ctx.Done():
			return
		case <-ticker.C:
			// Capture frame
			frame, err := s.captureFrameForStream(cam, input)
			if err != nil {
				s.logger.Debug("Failed to capture frame", zap.String("camera_id", cam.ID), zap.Error(err))
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

// captureFrameForStream captures a single JPEG frame from a camera
func (s *CCTVServiceImpl) captureFrameForStream(cam *cctvtypes.Camera, input string) ([]byte, error) {
	// Use FFmpeg to capture a single frame
	// For RTSP: ffmpeg -rtsp_transport tcp -i <rtsp_url> -frames:v 1 -f image2pipe -vcodec mjpeg -
	// For USB: ffmpeg -f v4l2 -input_format mjpeg -video_size 640x480 -i <device> -frames:v 1 -f image2pipe -vcodec mjpeg -

	var cmd *exec.Cmd
	if cam.Type == cctvtypes.CameraTypeUSB {
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

// getCameraInput gets the input source for a camera
func (s *CCTVServiceImpl) getCameraInput(cam *cctvtypes.Camera) string {
	if cam.Type == cctvtypes.CameraTypeUSB {
		return cam.DevicePath
	}

	// For RTSP/ONVIF, use the first RTSP URL
	if len(cam.RTSPURLs) > 0 {
		return cam.RTSPURLs[0]
	}

	return ""
}
