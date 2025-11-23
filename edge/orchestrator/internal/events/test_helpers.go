package events

import (
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

func setupTestManager(t *testing.T) *state.Manager {
	tmpDir := t.TempDir()

	properCfg := &config.Config{}
	properCfg.Edge.Orchestrator.DataDir = tmpDir

	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	mgr, err := state.NewManager(properCfg, log)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	return mgr
}

