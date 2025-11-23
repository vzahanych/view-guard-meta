package camera

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

func setupTestManager(t *testing.T) (*Manager, *state.Manager) {
	tmpDir := t.TempDir()

	cfg := &config.Config{}
	cfg.Edge.Orchestrator.DataDir = tmpDir

	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	stateMgr, err := state.NewManager(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	onvifDiscovery := NewONVIFDiscoveryService(30*time.Second, log)
	usbDiscovery := NewUSBDiscoveryService(30*time.Second, filepath.Join(tmpDir, "dev"), log)

	mgr := NewManager(stateMgr, onvifDiscovery, usbDiscovery, 5*time.Second, log)

	return mgr, stateMgr
}

