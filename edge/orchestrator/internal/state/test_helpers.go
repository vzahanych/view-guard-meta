package state

import (
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func setupTestManager(t *testing.T) *Manager {
	tmpDir := t.TempDir()

	cfg := &config.Config{}
	cfg.Edge.Orchestrator.DataDir = tmpDir

	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	mgr, err := NewManager(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	return mgr
}

