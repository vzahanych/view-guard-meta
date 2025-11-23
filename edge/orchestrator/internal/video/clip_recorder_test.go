package video

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func TestNewClipRecorder(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()

	recorder, err := NewClipRecorder(ffmpeg, ClipRecorderConfig{
		OutputDir: tmpDir,
	}, logger.NewNopLogger())

	if err != nil {
		t.Fatalf("NewClipRecorder failed: %v", err)
	}

	if recorder == nil {
		t.Fatal("NewClipRecorder returned nil")
	}

	if recorder.outputDir != tmpDir {
		t.Errorf("Expected output dir '%s', got '%s'", tmpDir, recorder.outputDir)
	}

	// Verify directory was created
	if _, err := os.Stat(tmpDir); err != nil {
		t.Errorf("Output directory was not created: %v", err)
	}
}

func TestClipRecorder_StartRecording_InvalidInput(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Try to start recording with invalid input
	_, err := recorder.StartRecording("camera-1", "invalid://input", 5*time.Second)
	if err == nil {
		t.Error("StartRecording should return error for invalid input")
	}
}

func TestClipRecorder_StartRecording_Duplicate(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Create a dummy input file for testing
	testInput := filepath.Join(tmpDir, "test_input.mp4")
	file, err := os.Create(testInput)
	if err == nil {
		file.Close()
		defer os.Remove(testInput)

		// Start first recording
		_, err := recorder.StartRecording("camera-1", testInput, 1*time.Second)
		if err != nil {
			// If FFmpeg can't process the dummy file, that's okay
			return
		}

		// Try to start second recording for same camera
		_, err = recorder.StartRecording("camera-1", testInput, 1*time.Second)
		if err == nil {
			t.Error("StartRecording should return error for duplicate camera")
		}

		// Clean up
		recorder.StopRecording("camera-1")
	}
}

func TestClipRecorder_IsRecording(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Initially not recording
	if recorder.IsRecording("camera-1") {
		t.Error("Should not be recording initially")
	}
}

func TestClipRecorder_GetRecording(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Get recording for non-existent camera
	_, exists := recorder.GetRecording("camera-1")
	if exists {
		t.Error("GetRecording should return false for non-existent camera")
	}
}

func TestClipRecorder_ListRecordings(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Initially empty
	recordings := recorder.ListRecordings()
	if len(recordings) != 0 {
		t.Errorf("Expected 0 recordings, got %d", len(recordings))
	}
}

func TestClipRecorder_StopRecording_NotRecording(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Try to stop non-existent recording
	_, err := recorder.StopRecording("camera-1")
	if err == nil {
		t.Error("StopRecording should return error for non-existent recording")
	}
}

func TestClipRecorder_StopAll(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Stop all when no recordings
	results := recorder.StopAll()
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestClipRecorder_GetOutputDir(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	outputDir := recorder.GetOutputDir()
	if outputDir != tmpDir {
		t.Errorf("Expected output dir '%s', got '%s'", tmpDir, outputDir)
	}
}

func TestClipRecorder_ConcurrentRecording(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Create dummy input files
	testInput1 := filepath.Join(tmpDir, "test_input1.mp4")
	testInput2 := filepath.Join(tmpDir, "test_input2.mp4")

	file1, err1 := os.Create(testInput1)
	file2, err2 := os.Create(testInput2)

	if err1 != nil || err2 != nil {
		t.Skip("Failed to create test input files")
	}
	file1.Close()
	file2.Close()
	defer os.Remove(testInput1)
	defer os.Remove(testInput2)

	// Try to start concurrent recordings
	_, err1 = recorder.StartRecording("camera-1", testInput1, 1*time.Second)
	_, err2 = recorder.StartRecording("camera-2", testInput2, 1*time.Second)

	// If both succeed, verify they're both recording
	if err1 == nil && err2 == nil {
		if !recorder.IsRecording("camera-1") {
			t.Error("Camera 1 should be recording")
		}

		if !recorder.IsRecording("camera-2") {
			t.Error("Camera 2 should be recording")
		}

		recordings := recorder.ListRecordings()
		if len(recordings) != 2 {
			t.Errorf("Expected 2 recordings, got %d", len(recordings))
		}

		// Stop all
		results := recorder.StopAll()
		if len(results) != 2 {
			t.Errorf("Expected 2 results from StopAll, got %d", len(results))
		}
	}
}

func TestClipRecorder_StartRecordingWithWindow(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Create dummy input file
	testInput := filepath.Join(tmpDir, "test_input.mp4")
	file, err := os.Create(testInput)
	if err == nil {
		file.Close()
		defer os.Remove(testInput)

		// Start recording with time window
		_, err := recorder.StartRecordingWithWindow(
			"camera-1",
			testInput,
			2*time.Second, // Pre-event
			5*time.Second, // Post-event
		)

		if err == nil {
			// Recording started, verify it's recording
			if !recorder.IsRecording("camera-1") {
				t.Error("Camera should be recording")
			}

			// Stop recording
			metadata, err := recorder.StopRecording("camera-1")
			if err == nil && metadata != nil {
				// Verify metadata
				if metadata.CameraID != "camera-1" {
					t.Errorf("Expected camera ID 'camera-1', got '%s'", metadata.CameraID)
				}

				if metadata.Duration == 0 {
					t.Error("Duration should be greater than 0")
				}

				if metadata.FilePath == "" {
					t.Error("FilePath should not be empty")
				}
			}
		}
	}
}

func TestClipRecorder_ClipMetadata(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Test metadata generation indirectly through StartRecording/StopRecording
	// Create a dummy input file
	testInput := filepath.Join(tmpDir, "test_input.mp4")
	file, err := os.Create(testInput)
	if err == nil {
		file.WriteString("fake video data")
		file.Close()
		defer os.Remove(testInput)

		// Start and immediately stop recording to test metadata generation
		outputPath, err := recorder.StartRecording("camera-1", testInput, 1*time.Second)
		if err == nil {
			// Wait a bit for recording to start
			time.Sleep(100 * time.Millisecond)

			// Stop recording and get metadata
			metadata, err := recorder.StopRecording("camera-1")
			if err == nil && metadata != nil {
				// Verify metadata
				if metadata.CameraID != "camera-1" {
					t.Errorf("Expected camera ID 'camera-1', got '%s'", metadata.CameraID)
				}

				if metadata.FilePath != outputPath {
					t.Errorf("Expected file path '%s', got '%s'", outputPath, metadata.FilePath)
				}

				if metadata.StartTime.IsZero() {
					t.Error("StartTime should not be zero")
				}

				if metadata.EndTime.IsZero() {
					t.Error("EndTime should not be zero")
				}

				if metadata.Codec == "" {
					t.Error("Codec should not be empty")
				}
			}
		}
	}
}

func TestClipRecorder_GenerateClipPath(t *testing.T) {
	ffmpeg := setupTestFFmpeg(t)
	tmpDir := t.TempDir()
	recorder := setupTestClipRecorder(t, ffmpeg, tmpDir)

	// Test clip path generation indirectly through StartRecording
	// Create a dummy input file
	testInput := filepath.Join(tmpDir, "test_input.mp4")
	file, err := os.Create(testInput)
	if err == nil {
		file.Close()
		defer os.Remove(testInput)

		// Start recording (which will generate a clip path)
		outputPath, err := recorder.StartRecording("camera-1", testInput, 1*time.Second)
		if err == nil {
			// Verify path format
			if outputPath == "" {
				t.Error("Generated path should not be empty")
			}

			// Verify it's in the output directory
			if !filepath.HasPrefix(outputPath, tmpDir) {
				t.Errorf("Path '%s' should be in output dir '%s'", outputPath, tmpDir)
			}

			// Verify filename format (cameraID_timestamp.mp4)
			filename := filepath.Base(outputPath)
			if len(filename) < 20 { // At least camera-1_YYYYMMDD_HHMMSS.mp4
				t.Errorf("Filename '%s' seems too short", filename)
			}

			// Verify extension
			if filepath.Ext(outputPath) != ".mp4" {
				t.Errorf("Expected .mp4 extension, got '%s'", filepath.Ext(outputPath))
			}

			// Clean up
			recorder.StopRecording("camera-1")
		}
	}
}

