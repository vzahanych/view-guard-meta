package storage

import (
	"context"
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func TestNewRetentionPolicy(t *testing.T) {
	policy, err := NewRetentionPolicy(7, 80.0, nil, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create retention policy: %v", err)
	}

	if policy == nil {
		t.Fatal("Retention policy should not be nil")
	}
}

func TestRetentionPolicy_DefaultValues(t *testing.T) {
	// Test with zero values (should use defaults)
	policy, err := NewRetentionPolicy(0, 0, nil, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create retention policy: %v", err)
	}

	if policy.retentionDays != 7 {
		t.Errorf("Expected default retention days 7, got %d", policy.retentionDays)
	}

	if policy.maxDiskUsage != 80.0 {
		t.Errorf("Expected default max disk usage 80.0, got %f", policy.maxDiskUsage)
	}
}

func TestRetentionPolicy_Enforce_NoStateManager(t *testing.T) {
	policy, err := NewRetentionPolicy(7, 80.0, nil, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create retention policy: %v", err)
	}

	ctx := context.Background()
	// Should not error even without state manager
	err = policy.Enforce(ctx)
	if err != nil {
		t.Errorf("Enforce should not error without state manager: %v", err)
	}
}

func TestRetentionPolicy_ShouldPauseRecording(t *testing.T) {
	tmpDir := t.TempDir()
	diskMonitor, err := NewDiskMonitor(tmpDir, 80.0, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create disk monitor: %v", err)
	}

	policy, err := NewRetentionPolicy(7, 80.0, nil, logger.NewNopLogger())
	if err != nil {
		t.Fatalf("Failed to create retention policy: %v", err)
	}

	ctx := context.Background()
	shouldPause, err := policy.ShouldPauseRecording(ctx, diskMonitor)
	if err != nil {
		t.Fatalf("ShouldPauseRecording failed: %v", err)
	}

	// Should not pause in temp directory (has space)
	if shouldPause {
		t.Error("Should not pause recording when disk has space")
	}
}

// Note: More comprehensive tests for retention policy would require
// a mock state manager and actual file creation/deletion tests.
// These are covered in integration tests.

