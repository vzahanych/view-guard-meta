package storage

import (
	"context"
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func TestNewDiskMonitor(t *testing.T) {
	tmpDir := t.TempDir()

	monitor, err := NewDiskMonitor(tmpDir, 80.0, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create disk monitor: %v", err)
	}

	if monitor == nil {
		t.Fatal("Disk monitor should not be nil")
	}
}

func TestDiskMonitor_GetUsage(t *testing.T) {
	tmpDir := t.TempDir()
	monitor, err := NewDiskMonitor(tmpDir, 80.0, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create disk monitor: %v", err)
	}

	ctx := context.Background()
	usage, err := monitor.GetUsage(ctx)
	if err != nil {
		t.Fatalf("GetUsage failed: %v", err)
	}

	if usage == nil {
		t.Fatal("Usage should not be nil")
	}

	if usage.TotalBytes <= 0 {
		t.Error("TotalBytes should be greater than 0")
	}

	if usage.UsedBytes < 0 {
		t.Error("UsedBytes should not be negative")
	}

	if usage.AvailableBytes < 0 {
		t.Error("AvailableBytes should not be negative")
	}

	if usage.UsagePercent < 0 || usage.UsagePercent > 100 {
		t.Errorf("UsagePercent should be between 0 and 100, got %f", usage.UsagePercent)
	}
}

func TestDiskMonitor_CheckSpace(t *testing.T) {
	tmpDir := t.TempDir()
	monitor, err := NewDiskMonitor(tmpDir, 80.0, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create disk monitor: %v", err)
	}

	ctx := context.Background()
	hasSpace, err := monitor.CheckSpace(ctx)
	if err != nil {
		t.Fatalf("CheckSpace failed: %v", err)
	}

	// Should have space in temp directory
	if !hasSpace {
		t.Error("Should have disk space in temp directory")
	}
}

func TestDiskMonitor_IsDiskFull(t *testing.T) {
	tmpDir := t.TempDir()
	monitor, err := NewDiskMonitor(tmpDir, 80.0, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create disk monitor: %v", err)
	}

	ctx := context.Background()
	isFull, err := monitor.IsDiskFull(ctx)
	if err != nil {
		t.Fatalf("IsDiskFull failed: %v", err)
	}

	// Should not be full in temp directory
	if isFull {
		t.Error("Disk should not be full in temp directory")
	}
}

func TestDiskMonitor_Caching(t *testing.T) {
	tmpDir := t.TempDir()
	monitor, err := NewDiskMonitor(tmpDir, 80.0, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create disk monitor: %v", err)
	}

	ctx := context.Background()

	// First call
	usage1, err := monitor.GetUsage(ctx)
	if err != nil {
		t.Fatalf("GetUsage failed: %v", err)
	}

	// Second call should use cache (within cache duration)
	usage2, err := monitor.GetUsage(ctx)
	if err != nil {
		t.Fatalf("GetUsage failed: %v", err)
	}

	// Should return same values (cached)
	if usage1.TotalBytes != usage2.TotalBytes {
		t.Error("Cached usage should return same TotalBytes")
	}

	if usage1.UsagePercent != usage2.UsagePercent {
		t.Error("Cached usage should return same UsagePercent")
	}
}

