package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

// TestEnvironment provides a test environment for integration tests
type TestEnvironment struct {
	TempDir     string
	Config      *config.Config
	StateMgr    *state.Manager
	Logger      *logger.Logger
	CleanupFunc func()
}

// SetupTestEnvironment creates a test environment
func SetupTestEnvironment(t *testing.T) *TestEnvironment {
	tmpDir := t.TempDir()

	// Create subdirectories
	dataDir := filepath.Join(tmpDir, "data")
	clipsDir := filepath.Join(tmpDir, "clips")
	snapshotsDir := filepath.Join(tmpDir, "snapshots")

	_ = os.MkdirAll(dataDir, 0755)
	_ = os.MkdirAll(clipsDir, 0755)
	_ = os.MkdirAll(snapshotsDir, 0755)

	// Create test config
	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				LogLevel:  "debug",
				LogFormat: "text",
				DataDir:   dataDir,
			},
			Storage: config.StorageConfig{
				ClipsDir:            clipsDir,
				SnapshotsDir:        snapshotsDir,
				RetentionDays:       7,
				MaxDiskUsagePercent: 80.0,
			},
		},
		Log: config.LogConfig{
			Level:  "debug",
			Format: "text",
		},
	}

	// Create logger
	log := logger.NewNopLogger()

	// Create state manager
	stateMgr, err := state.NewManager(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	cleanup := func() {
		stateMgr.Close()
	}

	return &TestEnvironment{
		TempDir:     tmpDir,
		Config:      cfg,
		StateMgr:    stateMgr,
		Logger:      log,
		CleanupFunc: cleanup,
	}
}

// Cleanup cleans up the test environment
func (e *TestEnvironment) Cleanup() {
	if e.CleanupFunc != nil {
		e.CleanupFunc()
	}
}

// WaitForCondition waits for a condition to become true
func WaitForCondition(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		select {
		case <-ticker.C:
			continue
		}
	}

	return false
}

// ContextWithTimeout creates a context with timeout for tests
func ContextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

