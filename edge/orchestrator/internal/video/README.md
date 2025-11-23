# Video Processing Package

This package provides FFmpeg integration for video decoding, encoding, and processing in the Edge Appliance.

## Features

- **FFmpeg Wrapper**: Exec-based wrapper for FFmpeg (can be replaced with CGO bindings later)
- **Hardware Acceleration Detection**: Automatic detection of Intel QSV (VAAPI) and NVIDIA NVENC
- **Software Fallback**: Automatic fallback to software encoding/decoding when hardware is unavailable
- **Codec Detection**: Automatic detection of available codecs
- **Input Validation**: Validation of RTSP URLs and device paths

## Usage

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"

// Create FFmpeg wrapper
log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
ffmpeg, err := video.NewFFmpegWrapper(log)
if err != nil {
    log.Fatal("Failed to initialize FFmpeg", err)
}

// Check hardware acceleration
hwAccel := ffmpeg.GetHardwareAcceleration()
if hwAccel.IntelQSV {
    log.Info("Intel QSV available")
}

// Get preferred decoder/encoder
decoder := ffmpeg.GetPreferredDecoder("h264") // Returns "h264_vaapi" if QSV available
encoder := ffmpeg.GetPreferredEncoder("h264") // Returns "h264_vaapi" if QSV available

// Validate input
err = ffmpeg.ValidateInput("rtsp://192.168.1.100/stream")
if err != nil {
    log.Error("Invalid input", err)
}

// Build FFmpeg command
ctx := context.Background()
args := []string{
    "-i", "rtsp://192.168.1.100/stream",
    "-c:v", encoder,
    "-f", "mp4",
    "output.mp4",
}
cmd := ffmpeg.BuildCommand(ctx, args)
err = cmd.Run()
```

## Hardware Acceleration

The wrapper automatically detects:
- **Intel QSV (VAAPI)**: For Intel CPUs with integrated graphics
- **NVIDIA NVENC**: For NVIDIA GPUs
- **Software Fallback**: Always available as fallback

## Codec Support

The wrapper detects available codecs and provides preferred encoders/decoders:
- **H.264**: `h264_vaapi` (Intel QSV), `h264_nvenc` (NVIDIA), `libx264` (software)
- **H.265/HEVC**: `hevc_vaapi` (Intel QSV), `hevc_nvenc` (NVIDIA), `libx265` (software)

## Frame Extraction Pipeline

The frame extraction pipeline extracts frames from video streams at configurable intervals, manages frame buffers, and preprocesses frames for AI inference.

### Usage

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"

// Create FFmpeg wrapper
ffmpeg, _ := video.NewFFmpegWrapper(log)

// Create frame extractor
extractor := video.NewFrameExtractor(ffmpeg, video.FrameExtractorConfig{
    BufferSize: 10,                    // Buffer 10 frames
    ExtractInterval: 1 * time.Second,  // Extract 1 frame per second
    Preprocess: video.PreprocessConfig{
        ResizeWidth:  640,              // Resize to 640px width
        ResizeHeight: 480,              // Resize to 480px height
        Quality:      85,                // JPEG quality
    },
    OnFrame: func(frame *video.Frame) {
        // Send frame to AI service
        sendToAI(frame)
    },
}, log)

// Start extraction from RTSP stream or USB device
extractor.Start("rtsp://192.168.1.100/stream", "camera-1")

// Or use frame distributor for multiple cameras
distributor := video.NewFrameDistributor(video.FrameDistributorConfig{
    OnFrame: func(frame *video.Frame) {
        // Distribute to AI service
        sendToAI(frame)
    },
}, log)

// Add extractor for each camera
distributor.AddExtractor("camera-1", extractor)
distributor.StartExtraction("camera-1", "rtsp://192.168.1.100/stream")
```

### Features

- **Configurable Intervals**: Extract frames at any interval (e.g., 1 frame/second)
- **Frame Buffer Management**: Configurable buffer size with automatic oldest-frame dropping
- **Frame Preprocessing**: Resize, normalize, and quality adjustment
- **Frame Distribution**: Distribute frames to AI service via callback
- **Multi-Camera Support**: Manage frame extraction for multiple cameras

## Video Clip Recording

The clip recorder records video clips from streams with MP4/H.264 encoding, supports concurrent recording for multiple cameras, and generates clip metadata.

### Usage

```go
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"

// Create FFmpeg wrapper
ffmpeg, _ := video.NewFFmpegWrapper(log)

// Create clip recorder
recorder, _ := video.NewClipRecorder(ffmpeg, video.ClipRecorderConfig{
    OutputDir: "/path/to/clips",
}, log)

// Start recording for a fixed duration
outputPath, err := recorder.StartRecording("camera-1", "rtsp://192.168.1.100/stream", 10*time.Second)

// Or start recording with time window (pre/post event)
outputPath, err := recorder.StartRecordingWithWindow(
    "camera-1",
    "rtsp://192.168.1.100/stream",
    2*time.Second,  // 2 seconds before event
    5*time.Second,  // 5 seconds after event
)

// Stop recording and get metadata
metadata, err := recorder.StopRecording("camera-1")
// metadata contains: FilePath, Duration, SizeBytes, StartTime, EndTime, etc.

// Check if recording
isRecording := recorder.IsRecording("camera-1")

// List all active recordings
activeRecordings := recorder.ListRecordings()

// Stop all recordings
allMetadata := recorder.StopAll()
```

### Features

- **Start/Stop Recording**: Start and stop recording on demand
- **MP4/H.264 Encoding**: Records to MP4 format with H.264 encoding
- **Hardware Acceleration**: Uses hardware acceleration when available
- **Clip Metadata**: Generates metadata (duration, size, timestamps, camera ID)
- **Concurrent Recording**: Supports recording from multiple cameras simultaneously
- **Time Windows**: Support for pre/post event recording windows

## Future Enhancements

- CGO bindings for direct FFmpeg library access (optional)
- FFprobe integration for detailed video properties
- Stream transcoding
- Advanced frame preprocessing (histogram equalization, etc.)

