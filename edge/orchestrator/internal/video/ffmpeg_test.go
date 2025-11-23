package video

import (
	"context"
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func TestNewFFmpegWrapper(t *testing.T) {
	log := logger.NewNopLogger()
	ffmpeg, err := NewFFmpegWrapper(log)
	if err != nil {
		t.Skipf("FFmpeg not available, skipping test: %v", err)
	}

	if ffmpeg == nil {
		t.Fatal("NewFFmpegWrapper returned nil")
	}

	if ffmpeg.ffmpegPath == "" {
		t.Error("FFmpeg path should be set")
	}
}

func TestFFmpegWrapper_GetHardwareAcceleration(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)

	hwAccel := ffmpeg.GetHardwareAcceleration()

	// Software should always be available
	if !hwAccel.Software {
		t.Error("Software fallback should always be available")
	}

	// Intel QSV and NVIDIA NVENC are optional
	// Just verify the method returns without error
	_ = hwAccel
}

func TestFFmpegWrapper_IsCodecAvailable(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)

	// Test common codecs
	codecs := []string{"h264", "libx264", "h264_vaapi", "h264_nvenc", "hevc", "libx265"}

	for _, codec := range codecs {
		available := ffmpeg.IsCodecAvailable(codec)
		// We can't assert availability, but we can verify the method works
		_ = available
	}
}

func TestFFmpegWrapper_GetPreferredDecoder(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)

	// Test H.264 decoder selection
	decoder := ffmpeg.GetPreferredDecoder("h264")
	if decoder == "" {
		t.Error("Preferred decoder should not be empty")
	}

	// Test HEVC decoder selection
	hevcDecoder := ffmpeg.GetPreferredDecoder("hevc")
	if hevcDecoder == "" {
		t.Error("Preferred HEVC decoder should not be empty")
	}

	// Test unknown codec (should return codec name as fallback)
	unknownDecoder := ffmpeg.GetPreferredDecoder("unknown")
	if unknownDecoder != "unknown" {
		t.Errorf("Expected 'unknown', got '%s'", unknownDecoder)
	}
}

func TestFFmpegWrapper_GetPreferredEncoder(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)

	// Test H.264 encoder selection
	encoder := ffmpeg.GetPreferredEncoder("h264")
	if encoder == "" {
		t.Error("Preferred encoder should not be empty")
	}

	// Test HEVC encoder selection
	hevcEncoder := ffmpeg.GetPreferredEncoder("hevc")
	if hevcEncoder == "" {
		t.Error("Preferred HEVC encoder should not be empty")
	}

	// Test unknown codec (should return codec name as fallback)
	unknownEncoder := ffmpeg.GetPreferredEncoder("unknown")
	if unknownEncoder != "unknown" {
		t.Errorf("Expected 'unknown', got '%s'", unknownEncoder)
	}
}

func TestFFmpegWrapper_BuildCommand(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)

	ctx := context.Background()
	args := []string{"-version"}

	cmd := ffmpeg.BuildCommand(ctx, args)
	if cmd == nil {
		t.Fatal("BuildCommand returned nil")
	}

	if cmd.Path == "" {
		t.Error("Command path should not be empty")
	}

	// Verify args are set (should include path + provided args)
	if len(cmd.Args) < len(args)+1 {
		t.Errorf("Expected at least %d args, got %d", len(args)+1, len(cmd.Args))
	}

	// Verify last arg matches
	if len(cmd.Args) > 0 && cmd.Args[len(cmd.Args)-1] != args[len(args)-1] {
		t.Errorf("Expected last arg '%s', got '%s'", args[len(args)-1], cmd.Args[len(cmd.Args)-1])
	}
}

func TestFFmpegWrapper_GetVersion(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)

	version, err := ffmpeg.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}

	if version == "" {
		t.Error("Version should not be empty")
	}
}

func TestFFmpegWrapper_ValidateInput_Invalid(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)

	// Test invalid input
	err := ffmpeg.ValidateInput("invalid://not-a-valid-url")
	if err == nil {
		t.Error("ValidateInput should return error for invalid input")
	}
}

func TestFFmpegWrapper_CodecDetection(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)

	// Test that codec detection ran during initialization
	// We can't assert specific codecs, but we can verify the method works
	available := ffmpeg.IsCodecAvailable("h264")
	_ = available // May be true or false depending on system
}

