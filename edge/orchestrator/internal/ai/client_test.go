package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
)

func setupTestClient(t *testing.T) (*Client, *httptest.Server) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Mock response
		response := InferenceResponse{
			BoundingBoxes: []BoundingBox{
				{
					X1:         100,
					Y1:         200,
					X2:         300,
					Y2:         400,
					Confidence: 0.85,
					ClassID:    0,
					ClassName:  "person",
				},
			},
			InferenceTimeMs: 45.2,
			FrameShape:      []int{480, 640},
			ModelInputShape: []int{640, 640},
			DetectionCount:  1,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))

	// Create client
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	client := NewClient(ClientConfig{
		ServiceURL:          server.URL,
		Timeout:             5 * time.Second,
		ConfidenceThreshold: 0.5,
	}, log)

	return client, server
}

func TestClient_Infer(t *testing.T) {
	client, server := setupTestClient(t)
	defer server.Close()

	// Create test frame
	frame := &video.Frame{
		Data:      []byte("fake jpeg data"),
		Timestamp: time.Now(),
		CameraID:  "camera-1",
		Width:     640,
		Height:    480,
	}

	ctx := context.Background()
	resp, err := client.Infer(ctx, frame)

	if err != nil {
		t.Fatalf("Inference failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Response is nil")
	}

	if len(resp.BoundingBoxes) != 1 {
		t.Errorf("Expected 1 bounding box, got %d", len(resp.BoundingBoxes))
	}

	if resp.BoundingBoxes[0].ClassName != "person" {
		t.Errorf("Expected class 'person', got '%s'", resp.BoundingBoxes[0].ClassName)
	}

	if resp.DetectionCount != 1 {
		t.Errorf("Expected detection count 1, got %d", resp.DetectionCount)
	}
}

func TestClient_InferWithOptions(t *testing.T) {
	client, server := setupTestClient(t)
	defer server.Close()

	frame := &video.Frame{
		Data:      []byte("fake jpeg data"),
		Timestamp: time.Now(),
		CameraID:  "camera-1",
		Width:     640,
		Height:    480,
	}

	confidence := 0.7
	classes := []string{"person"}

	ctx := context.Background()
	resp, err := client.InferWithOptions(ctx, frame, &confidence, classes)

	if err != nil {
		t.Fatalf("Inference failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Response is nil")
	}
}

func TestClient_InferBatch(t *testing.T) {
	// Create test server with batch endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/inference/batch" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Mock batch response
		response := BatchInferenceResponse{
			Results: []InferenceResponse{
				{
					BoundingBoxes:   []BoundingBox{},
					InferenceTimeMs: 10.0,
					FrameShape:      []int{480, 640},
					ModelInputShape: []int{640, 640},
					DetectionCount:  0,
				},
				{
					BoundingBoxes:   []BoundingBox{},
					InferenceTimeMs: 10.0,
					FrameShape:      []int{480, 640},
					ModelInputShape: []int{640, 640},
					DetectionCount:  0,
				},
			},
			TotalInferenceTimeMs:   20.0,
			AverageInferenceTimeMs: 10.0,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	client := NewClient(ClientConfig{
		ServiceURL: server.URL,
		Timeout:    5 * time.Second,
	}, log)

	frames := []*video.Frame{
		{
			Data:      []byte("fake jpeg 1"),
			Timestamp: time.Now(),
			CameraID:  "camera-1",
			Width:     640,
			Height:    480,
		},
		{
			Data:      []byte("fake jpeg 2"),
			Timestamp: time.Now(),
			CameraID:  "camera-1",
			Width:     640,
			Height:    480,
		},
	}

	ctx := context.Background()
	batchResp, err := client.InferBatch(ctx, frames)

	if err != nil {
		t.Fatalf("Batch inference failed: %v", err)
	}

	if batchResp == nil {
		t.Fatal("Response is nil")
	}

	if len(batchResp.Results) != len(frames) {
		t.Errorf("Expected %d results, got %d", len(frames), len(batchResp.Results))
	}
}

func TestClient_InferWithRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		response := InferenceResponse{
			BoundingBoxes:   []BoundingBox{},
			InferenceTimeMs: 10.0,
			FrameShape:      []int{480, 640},
			ModelInputShape: []int{640, 640},
			DetectionCount:  0,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	client := NewClient(ClientConfig{
		ServiceURL: server.URL,
		Timeout:    5 * time.Second,
	}, log)

	frame := &video.Frame{
		Data:      []byte("fake jpeg"),
		Timestamp: time.Now(),
		CameraID:  "camera-1",
		Width:     640,
		Height:    480,
	}

	ctx := context.Background()
	resp, err := client.InferWithRetry(ctx, frame, 3, 100*time.Millisecond)

	if err != nil {
		t.Fatalf("Inference with retry failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Response is nil")
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestClient_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/ready" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	client := NewClient(ClientConfig{
		ServiceURL: server.URL,
		Timeout:    5 * time.Second,
	}, log)

	ctx := context.Background()
	err := client.HealthCheck(ctx)

	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
}

func TestClient_GetStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/inference/stats" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		stats := InferenceStats{
			TotalInferences: 100,
			TotalTimeMs:     5000.0,
			AverageTimeMs:   50.0,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(stats)
	}))
	defer server.Close()

	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	client := NewClient(ClientConfig{
		ServiceURL: server.URL,
		Timeout:    5 * time.Second,
	}, log)

	ctx := context.Background()
	stats, err := client.GetStats(ctx)

	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Stats is nil")
	}

	if stats.TotalInferences != 100 {
		t.Errorf("Expected 100 inferences, got %d", stats.TotalInferences)
	}
}

func TestFrameEncoding(t *testing.T) {
	// Test that frame encoding works correctly
	frameData := []byte("test jpeg data")
	encoded := base64.StdEncoding.EncodeToString(frameData)

	if encoded == "" {
		t.Fatal("Encoded string is empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	if string(decoded) != string(frameData) {
		t.Errorf("Decoded data doesn't match original")
	}
}

