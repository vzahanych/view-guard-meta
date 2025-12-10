package processing

import (
	"context"
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	svc "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

func TestNewSnapshotCaptureService(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	tmpDir := t.TempDir()

	config := SnapshotCaptureServiceConfig{
		OutputDir:   tmpDir,
		JPEGQuality: 85,
	}

	service, err := NewSnapshotCaptureService(config, log)
	if err != nil {
		t.Fatalf("NewSnapshotCaptureService failed: %v", err)
	}

	if service == nil {
		t.Fatal("NewSnapshotCaptureService returned nil")
	}

	if service.outputDir != tmpDir {
		t.Errorf("Expected outputDir '%s', got '%s'", tmpDir, service.outputDir)
	}

	if service.jpegQuality != 85 {
		t.Errorf("Expected JPEG quality 85, got %d", service.jpegQuality)
	}
}

func TestSnapshotCaptureService_StartStop(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	tmpDir := t.TempDir()

	config := SnapshotCaptureServiceConfig{
		OutputDir:   tmpDir,
		JPEGQuality: 85,
	}

	service, err := NewSnapshotCaptureService(config, log)
	if err != nil {
		t.Fatalf("NewSnapshotCaptureService failed: %v", err)
	}

	ctx := context.Background()
	err = service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// ServiceBase may not set status automatically, so we just check it doesn't error
	status := service.GetStatus().GetStatus()
	if status == svc.StatusError {
		t.Errorf("Service should not be in error status")
	}

	err = service.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	status = service.GetStatus().GetStatus()
	if status != svc.StatusStopped {
		t.Errorf("Expected status %s, got %s", svc.StatusStopped, status)
	}
}

func TestSnapshotCaptureService_SetJPEGQuality(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	tmpDir := t.TempDir()

	config := SnapshotCaptureServiceConfig{
		OutputDir:   tmpDir,
		JPEGQuality: 85,
	}

	service, err := NewSnapshotCaptureService(config, log)
	if err != nil {
		t.Fatalf("NewSnapshotCaptureService failed: %v", err)
	}

	newQuality := 90
	service.SetJPEGQuality(newQuality)

	if service.jpegQuality != newQuality {
		t.Errorf("Expected JPEG quality %d, got %d", newQuality, service.jpegQuality)
	}
}

func TestSnapshotCaptureService_InvalidJPEGQuality(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	tmpDir := t.TempDir()

	config := SnapshotCaptureServiceConfig{
		OutputDir:   tmpDir,
		JPEGQuality: 150, // Invalid quality
	}

	_, err := NewSnapshotCaptureService(config, log)
	if err == nil {
		t.Error("NewSnapshotCaptureService should fail with invalid JPEG quality")
	}
}

