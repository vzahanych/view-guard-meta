package video

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
)

// CaptureFrameJPEG captures a single JPEG frame from an input source using FFmpeg.
func (f *FFmpegWrapper) CaptureFrameJPEG(ctx context.Context, input string, quality int) ([]byte, error) {
	if quality <= 0 || quality > 100 {
		quality = 85
	}

	if err := f.ValidateInput(input); err != nil {
		return nil, fmt.Errorf("invalid input source: %w", err)
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", input,
		"-frames:v", "1",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-q:v", fmt.Sprintf("%d", quality),
		"-",
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := f.BuildCommand(ctx, args)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg capture failed: %w (%s)", err, stderr.String())
	}

	frameData := stdout.Bytes()
	if len(frameData) == 0 {
		return nil, fmt.Errorf("no frame data captured")
	}

	// Validate it decodes as an image
	if _, _, err := image.Decode(bytes.NewReader(frameData)); err != nil {
		return nil, fmt.Errorf("invalid frame data: %w", err)
	}

	return frameData, nil
}
