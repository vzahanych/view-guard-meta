# AI Service Client

This package provides an HTTP client for communicating with the Python AI inference service.

## Overview

The AI client sends video frames (JPEG images) to the Python AI service for object detection and receives detection results with bounding boxes, confidence scores, and class information.

## Components

### Client (`client.go`)

Main HTTP client for the AI service:
- `Infer()` - Single frame inference
- `InferWithOptions()` - Inference with custom confidence threshold and class filters
- `InferBatch()` - Batch inference for multiple frames
- `InferWithRetry()` - Inference with automatic retry logic
- `GetStats()` - Retrieve inference statistics
- `HealthCheck()` - Check AI service health

### Types (`types.go`)

Data structures matching the Python API:
- `InferenceRequest` - Request format
- `InferenceResponse` - Response format
- `BoundingBox` - Detection bounding box
- `DetectionResult` - Detection result with frame context

### Frame Processor (`integration.go`)

High-level frame processor that:
- Integrates with `FrameDistributor`
- Handles rate limiting (inference interval per camera)
- Calls detection callback with results
- Manages frame processing lifecycle

## Usage

### Basic Usage

```go
// Create client
client := ai.NewClient(ai.ClientConfig{
    ServiceURL: "http://localhost:8080",
    Timeout: 30 * time.Second,
    ConfidenceThreshold: 0.5,
    EnabledClasses: []string{"person", "car"},
}, logger)

// Perform inference
frame := &video.Frame{
    Data: jpegBytes,
    Timestamp: time.Now(),
    CameraID: "camera-1",
    Width: 640,
    Height: 480,
}

response, err := client.Infer(ctx, frame)
if err != nil {
    log.Error("Inference failed", err)
    return
}

// Process detections
for _, box := range response.BoundingBoxes {
    log.Info("Detection",
        "class", box.ClassName,
        "confidence", box.Confidence,
        "bbox", fmt.Sprintf("%.0f,%.0f,%.0f,%.0f", box.X1, box.Y1, box.X2, box.Y2),
    )
}
```

### Integration with Frame Distributor

```go
// Create frame processor
processor := ai.NewFrameProcessor(ai.FrameProcessorConfig{
    Client: client,
    Interval: 1 * time.Second, // 1 frame per second per camera
    OnDetection: func(result *ai.DetectionResult) {
        // Handle detection result
        // This is where Epic 1.5 event generation will be called
    },
}, logger)

// Integrate with frame distributor
distributor := video.NewFrameDistributor(video.FrameDistributorConfig{
    OnFrame: func(frame *video.Frame) {
        processor.ProcessFrame(ctx, frame)
    },
}, logger)
```

### With Retry Logic

```go
// Inference with automatic retries
response, err := client.InferWithRetry(
    ctx,
    frame,
    3,              // max retries
    1*time.Second,  // retry delay
)
```

## Configuration

Configure the AI service URL and settings in `config.yaml`:

```yaml
edge:
  ai:
    service_url: "http://localhost:8080"
    inference_interval: 1s
    confidence_threshold: 0.5
```

Or via environment variables:
- `EDGE_AI_SERVICE_URL` - AI service URL
- `EDGE_AI_INFERENCE_INTERVAL` - Inference interval
- `EDGE_AI_CONFIDENCE_THRESHOLD` - Confidence threshold

## Error Handling

The client handles:
- Network errors with retry logic
- HTTP errors (non-200 status codes)
- Timeout errors (configurable timeout)
- JSON parsing errors

All errors are logged and returned to the caller for appropriate handling.

## Performance

- **Rate Limiting**: Frame processor enforces minimum interval between inferences per camera
- **Concurrent Processing**: Frame processing happens in goroutines to avoid blocking
- **Batch Support**: Use `InferBatch()` for processing multiple frames efficiently
- **Statistics**: Monitor inference performance with `GetStats()`

## Next Steps

This client will be integrated with:
- **Epic 1.5**: Event generation from detection results
- **Frame Distributor**: Automatic frame processing
- **Event Service**: Event creation and queueing

