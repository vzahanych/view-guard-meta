package grpc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"
)

func setupTestStreamingService(t *testing.T) (*StreamingService, *Client) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.WireGuardConfig{
		Enabled:     true,
		KVMEndpoint: "localhost:50051",
	}

	wgClient := wireguard.NewClient(cfg, log)
	grpcClient := NewClient(cfg, wgClient, log)
	streamingService := NewStreamingService(grpcClient, log)

	return streamingService, grpcClient
}

func TestNewStreamingService(t *testing.T) {
	service, _ := setupTestStreamingService(t)

	if service == nil {
		t.Fatal("NewStreamingService returned nil")
	}
	if service.client == nil {
		t.Error("StreamingService client is nil")
	}
	if service.logger == nil {
		t.Error("StreamingService logger is nil")
	}
}

func TestStreamingService_GetClipInfo_FileNotFound(t *testing.T) {
	service, _ := setupTestStreamingService(t)

	ctx := context.Background()
	_, err := service.GetClipInfo(ctx, "test-event", "/nonexistent/path/clip.mp4")

	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestStreamingService_GetClipInfo_ValidFile(t *testing.T) {
	service, _ := setupTestStreamingService(t)

	// Create a temporary file
	tmpDir := t.TempDir()
	clipPath := filepath.Join(tmpDir, "test-clip.mp4")
	testData := []byte("test video data")
	if err := os.WriteFile(clipPath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	ctx := context.Background()
	resp, err := service.GetClipInfo(ctx, "test-event", clipPath)

	if err != nil {
		t.Fatalf("GetClipInfo failed: %v", err)
	}
	if resp == nil {
		t.Fatal("GetClipInfo returned nil response")
	}
	if !resp.Success {
		t.Error("GetClipInfo returned unsuccessful response")
	}
	if resp.SizeBytes != uint64(len(testData)) {
		t.Errorf("Expected size %d, got %d", len(testData), resp.SizeBytes)
	}
}

func TestStreamingService_StreamClip_FileNotFound(t *testing.T) {
	service, _ := setupTestStreamingService(t)

	ctx := context.Background()
	err := service.StreamClip(ctx, "test-event", "/nonexistent/path/clip.mp4", 0)

	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

