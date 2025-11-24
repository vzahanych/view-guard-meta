package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func main() {
	fmt.Println("=== Video Capture & AI Inference Test ===")
	fmt.Println("This will capture video from the 5MP USB Camera and process it through the AI model")
	fmt.Println()

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create logger
	log, err := logger.New(logger.LogConfig{
		Level:  "info",
		Format: "text",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	// Use the 5MP USB Camera device (/dev/video2)
	cameraDevice := "/dev/video2"
	aiServiceURL := cfg.Edge.AI.ServiceURL
	if aiServiceURL == "" {
		aiServiceURL = "http://localhost:8080"
	}

	fmt.Printf("Camera device: %s\n", cameraDevice)
	fmt.Printf("AI Service URL: %s\n", aiServiceURL)
	fmt.Println()

	// Test camera access
	fmt.Println("Testing camera access...")
	if _, err := os.Stat(cameraDevice); err != nil {
		fmt.Fprintf(os.Stderr, "Camera device not accessible: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Camera device accessible")

	// Test AI service
	fmt.Println("Testing AI service connection...")
	resp, err := http.Get(aiServiceURL + "/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "AI service not reachable: %v\n", err)
		os.Exit(1)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "AI service unhealthy: status %d\n", resp.StatusCode)
		os.Exit(1)
	}
	fmt.Println("✅ AI service is healthy")
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Capture frames and process them
	fmt.Println("Starting video capture and AI inference...")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	frameInterval := cfg.Edge.AI.InferenceInterval
	if frameInterval == 0 {
		frameInterval = 1 * time.Second
	}

	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	frameCount := 0
	detectionCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			frameCount++
			fmt.Printf("[Frame %d] Capturing frame...\n", frameCount)

			// Capture a single frame using FFmpeg
			frameData, err := captureFrame(cameraDevice)
			if err != nil {
				fmt.Printf("  ❌ Failed to capture frame: %v\n", err)
				continue
			}

			// Send frame to AI service for inference
			detections, err := runInference(aiServiceURL, frameData)
			if err != nil {
				fmt.Printf("  ❌ Failed to run inference: %v\n", err)
				continue
			}

			// Process detections
			if len(detections) > 0 {
				detectionCount++
				fmt.Printf("  ✅ Detections found: %d\n", len(detections))
				for _, det := range detections {
					fmt.Printf("    - %s (confidence: %.2f%%)\n", det.Class, det.Confidence*100)
				}
			} else {
				fmt.Printf("  ℹ️  No detections\n")
			}
		}
	}
}

// captureFrame captures a single frame from the USB camera using FFmpeg
func captureFrame(devicePath string) ([]byte, error) {
	// Use FFmpeg to capture a single JPEG frame
	// -f v4l2: Video4Linux2 input
	// -i /dev/video2: Input device
	// -frames:v 1: Capture only 1 frame
	// -q:v 2: High quality JPEG
	// -f image2pipe: Output to pipe
	// -vcodec mjpeg: MJPEG codec
	// -: Output to stdout
	cmd := exec.Command("ffmpeg",
		"-f", "v4l2",
		"-input_format", "mjpeg",
		"-video_size", "640x480",
		"-framerate", "30",
		"-i", devicePath,
		"-frames:v", "1",
		"-q:v", "2",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-",
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg start error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		cmd.Process.Kill()
		return nil, fmt.Errorf("ffmpeg timeout")
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("ffmpeg error: %v, stderr: %s", err, stderr.String())
		}
	}

	return stdout.Bytes(), nil
}

// BoundingBoxResponse represents a bounding box from AI service
type BoundingBoxResponse struct {
	X1        float64 `json:"x1"`
	Y1        float64 `json:"y1"`
	X2        float64 `json:"x2"`
	Y2        float64 `json:"y2"`
	Confidence float64 `json:"confidence"`
	ClassID   int     `json:"class_id"`
	ClassName string  `json:"class_name"`
}

// InferenceResponse represents the AI service response
type InferenceResponse struct {
	BoundingBoxes []BoundingBoxResponse `json:"bounding_boxes"`
	InferenceTimeMs float64 `json:"inference_time_ms"`
}

// Detection represents a simplified detection result
type Detection struct {
	Class      string
	Confidence float64
}

// runInference sends a frame to the AI service and returns detections
func runInference(aiServiceURL string, frameData []byte) ([]Detection, error) {
	// Create multipart form data
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add image file
	part, err := writer.CreateFormFile("image", "frame.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(frameData); err != nil {
		return nil, fmt.Errorf("failed to write image data: %w", err)
	}
	writer.Close()

	// Send request
	req, err := http.NewRequest("POST", aiServiceURL+"/api/v1/inference/file", &requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("inference failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var inferenceResp InferenceResponse
	if err := json.NewDecoder(resp.Body).Decode(&inferenceResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to simplified detections
	detections := make([]Detection, 0, len(inferenceResp.BoundingBoxes))
	for _, bbox := range inferenceResp.BoundingBoxes {
		detections = append(detections, Detection{
			Class:      bbox.ClassName,
			Confidence: bbox.Confidence,
		})
	}

	return detections, nil
}

