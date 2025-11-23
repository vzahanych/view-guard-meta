# Go Orchestrator ↔ Python AI Service Communication Protocol

This document describes the data format and communication protocol between the Go orchestrator and Python AI service for video frame inference.

## Overview

The Go orchestrator extracts frames from video streams (RTSP/USB cameras) and sends them to the Python AI service for object detection. The Python service returns detection results, which the Go orchestrator uses to generate events.

## Communication Flow

```
┌─────────────────┐         ┌──────────────────┐         ┌─────────────────┐
│  Go Orchestrator│         │  Python AI       │         │  Event Manager  │
│                 │         │  Service         │         │                 │
├─────────────────┤         ├──────────────────┤         ├─────────────────┤
│                 │         │                  │         │                 │
│ 1. Extract      │         │                  │         │                 │
│    Frame (JPEG) │         │                  │         │                 │
│                 │         │                  │         │                 │
│ 2. Encode to    │────────▶│ 3. Decode &      │         │                 │
│    Base64       │  HTTP   │    Run Inference │         │                 │
│                 │  POST   │                  │         │                 │
│                 │         │                  │         │                 │
│ 5. Receive      │◀────────│ 4. Return        │         │                 │
│    Detections   │  JSON   │    Detections    │         │                 │
│                 │         │                  │         │                 │
│ 6. Generate     │───────────────────────────▶│ 7. Store │
│    Events       │         │                  │    Event │
│                 │         │                  │         │                 │
│ 8. Queue for    │         │                  │         │ 8. Queue for    │
│    Transmission │         │                  │         │    Transmission │
└─────────────────┘         └──────────────────┘         └─────────────────┘
```

## Request Format (Go → Python)

### HTTP Endpoint
```
POST http://localhost:8080/api/v1/inference
Content-Type: application/json
```

### Request Body
```json
{
  "image": "base64_encoded_jpeg_image_string",
  "confidence_threshold": 0.5,  // Optional, overrides default
  "enabled_classes": ["person", "car"]  // Optional, filter by classes
}
```

### Frame Format
- **Format**: JPEG-encoded image
- **Encoding**: Base64 string
- **Source**: Extracted from video stream by `FrameExtractor`
- **Metadata**: Frame includes `CameraID`, `Timestamp`, `Width`, `Height`

### Example Go Code
```go
// In frame_extractor.go or ai_client.go
type InferenceRequest struct {
    Image              string   `json:"image"`
    ConfidenceThreshold *float64 `json:"confidence_threshold,omitempty"`
    EnabledClasses     []string `json:"enabled_classes,omitempty"`
}

// Convert frame to base64
func (f *Frame) ToBase64() string {
    return base64.StdEncoding.EncodeToString(f.Data)
}

// Send to AI service
func sendFrameForInference(frame *Frame, serviceURL string) (*InferenceResponse, error) {
    req := InferenceRequest{
        Image: frame.ToBase64(),
    }
    
    jsonData, _ := json.Marshal(req)
    resp, err := http.Post(serviceURL+"/api/v1/inference", "application/json", bytes.NewBuffer(jsonData))
    // ... handle response
}
```

## Response Format (Python → Go)

### Response Body
```json
{
  "bounding_boxes": [
    {
      "x1": 100.5,
      "y1": 200.3,
      "x2": 300.7,
      "y2": 400.9,
      "confidence": 0.85,
      "class_id": 0,
      "class_name": "person"
    },
    {
      "x1": 500.0,
      "y1": 100.0,
      "x2": 700.0,
      "y2": 300.0,
      "confidence": 0.92,
      "class_id": 2,
      "class_name": "car"
    }
  ],
  "inference_time_ms": 45.2,
  "frame_shape": [480, 640],  // [height, width]
  "model_input_shape": [640, 640],
  "detection_count": 2
}
```

### Response Structure
- **bounding_boxes**: Array of detected objects
  - **x1, y1, x2, y2**: Bounding box coordinates in original frame pixels
  - **confidence**: Detection confidence (0.0 to 1.0)
  - **class_id**: COCO class ID (0 = person, 2 = car, etc.)
  - **class_name**: Human-readable class name
- **inference_time_ms**: Time taken for inference (for performance monitoring)
- **frame_shape**: Original frame dimensions [height, width]
- **model_input_shape**: Model input dimensions [height, width]
- **detection_count**: Number of detections

### Example Go Code
```go
type BoundingBox struct {
    X1         float64 `json:"x1"`
    Y1         float64 `json:"y1"`
    X2         float64 `json:"x2"`
    Y2         float64 `json:"y2"`
    Confidence float64 `json:"confidence"`
    ClassID    int     `json:"class_id"`
    ClassName  string  `json:"class_name"`
}

type InferenceResponse struct {
    BoundingBoxes    []BoundingBox `json:"bounding_boxes"`
    InferenceTimeMs  float64       `json:"inference_time_ms"`
    FrameShape       []int         `json:"frame_shape"`
    ModelInputShape  []int         `json:"model_input_shape"`
    DetectionCount   int           `json:"detection_count"`
}
```

## Event Generation (Epic 1.5)

After receiving detection results, the Go orchestrator generates events:

### Event Structure
```go
type Event struct {
    ID          string    // UUID
    Type        string    // "person_detected", "vehicle_detected", etc.
    CameraID    string
    Timestamp   time.Time
    Confidence  float64
    BoundingBox BoundingBox
    Metadata    map[string]interface{} // Additional metadata
}
```

### Event Generation Logic
1. **Filter detections** by confidence threshold and enabled classes
2. **Create event** for each significant detection (person, vehicle, etc.)
3. **Associate metadata**: camera ID, frame timestamp, bounding boxes
4. **Store event** in SQLite database
5. **Queue event** for transmission to KVM VM

## Implementation Location

### Epic 1.5: Event Management & Queue

**Location**: `edge/orchestrator/internal/events/`

This epic should be implemented in the **Go orchestrator** because:
- Events are generated from AI detection results
- Events need to be stored in SQLite (Go state manager)
- Events need to be queued for transmission to KVM VM (Go gRPC client)
- Event management is part of the orchestrator's coordination responsibilities

### Directory Structure
```
edge/orchestrator/
├── internal/
│   ├── events/              # NEW: Event management (Epic 1.5)
│   │   ├── event.go         # Event structure and definitions
│   │   ├── generator.go     # Event generation from AI detections
│   │   ├── queue.go         # Event queue management
│   │   ├── storage.go       # Event storage in SQLite
│   │   └── service.go       # Event service (implements Service interface)
│   │
│   ├── ai/                  # NEW: AI service client (connects to Python)
│   │   ├── client.go        # HTTP client for Python AI service
│   │   └── types.go         # Request/response types
│   │
│   ├── video/               # Existing: Frame extraction
│   │   ├── frame_extractor.go
│   │   └── frame_distributor.go  # Should call AI client
│   │
│   └── state/               # Existing: State management
│       └── event.go         # Event state storage (already exists)
```

## Integration Points

### 1. Frame Extractor → AI Client
```go
// In frame_distributor.go or new ai_integration.go
func (fd *FrameDistributor) onFrameExtracted(frame *Frame) {
    // Send frame to AI service
    detections, err := aiClient.Infer(frame)
    if err != nil {
        log.Error("AI inference failed", err)
        return
    }
    
    // Generate events from detections
    for _, detection := range detections.BoundingBoxes {
        if detection.Confidence >= threshold {
            event := eventGenerator.CreateEvent(frame, detection)
            eventService.QueueEvent(event)
        }
    }
}
```

### 2. AI Client Implementation
```go
// In internal/ai/client.go
type AIClient struct {
    serviceURL string
    httpClient *http.Client
    logger     *logger.Logger
}

func (c *AIClient) Infer(frame *Frame) (*InferenceResponse, error) {
    // Encode frame to base64
    imageBase64 := base64.StdEncoding.EncodeToString(frame.Data)
    
    // Create request
    req := InferenceRequest{
        Image: imageBase64,
    }
    
    // Send HTTP POST
    // ... HTTP request logic
    
    // Parse response
    // ... JSON unmarshaling
    
    return response, nil
}
```

### 3. Event Service
```go
// In internal/events/service.go
type EventService struct {
    *service.ServiceBase
    generator *EventGenerator
    queue     *EventQueue
    storage   *EventStorage
}

func (s *EventService) OnDetection(detections *InferenceResponse, frame *Frame) {
    // Generate events from detections
    events := s.generator.GenerateEvents(detections, frame)
    
    // Store events
    for _, event := range events {
        s.storage.SaveEvent(event)
        s.queue.Enqueue(event)
    }
}
```

## Configuration

### AI Service URL
Configured in `edge/orchestrator/config/config.yaml`:
```yaml
edge:
  ai:
    service_url: "http://localhost:8080"
    inference_interval: 1s  # How often to send frames
    confidence_threshold: 0.5
    enabled_classes: ["person", "car", "truck"]
```

## Error Handling

- **AI Service Unavailable**: Log error, continue frame extraction, retry later
- **Invalid Response**: Log error, skip frame, continue processing
- **Timeout**: Set HTTP client timeout (e.g., 5 seconds), log timeout, continue
- **Network Errors**: Retry with exponential backoff, fallback to local processing if needed

## Performance Considerations

- **Frame Rate**: Don't send every frame - use `inference_interval` config
- **Concurrent Requests**: Use goroutines for parallel inference requests
- **Queue Size**: Limit event queue size to prevent memory issues
- **Batch Processing**: Consider batching multiple frames (if Python API supports it)

## Next Steps

1. **Implement AI Client** (`internal/ai/client.go`) - HTTP client for Python service
2. **Integrate with Frame Extractor** - Call AI client from frame distributor
3. **Implement Epic 1.5** - Event generation, storage, and queue management
4. **Add Configuration** - AI service URL and inference settings
5. **Add Error Handling** - Retry logic, fallback behavior
6. **Add Tests** - Unit and integration tests for AI client and event generation

