package video

import (
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func setupTestFFmpeg(t *testing.T) *FFmpegWrapper {
	log := logger.NewNopLogger()
	ffmpeg, err := NewFFmpegWrapper(log)
	if err != nil {
		t.Skipf("FFmpeg not available, skipping test: %v", err)
	}
	return ffmpeg
}

func setupTestFrameExtractor(t *testing.T, ffmpeg *FFmpegWrapper) *FrameExtractor {
	log := logger.NewNopLogger()
	config := FrameExtractorConfig{
		BufferSize:      5,
		ExtractInterval: 100 * time.Millisecond,
		Preprocess: PreprocessConfig{
			Quality: 85,
		},
	}
	return NewFrameExtractor(ffmpeg, config, log)
}

func setupTestClipRecorder(t *testing.T, ffmpeg *FFmpegWrapper, outputDir string) *ClipRecorder {
	log := logger.NewNopLogger()
	recorder, err := NewClipRecorder(ffmpeg, ClipRecorderConfig{
		OutputDir: outputDir,
	}, log)
	if err != nil {
		t.Fatalf("Failed to create clip recorder: %v", err)
	}
	return recorder
}

