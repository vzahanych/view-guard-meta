package capabilities

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
)

func setupTestSyncService(t *testing.T) (*SyncService, *camera.Manager, *screenshots.Service, *state.Manager, func()) {
	tmpDir, err := os.MkdirTemp("", "sync-service-test-*")
	require.NoError(t, err)

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: tmpDir,
			},
			AI: config.AIConfig{
				MinNormalSnapshots: 50,
			},
			Storage: config.StorageConfig{
				MaxDiskUsagePercent: 80.0,
			},
		},
	}

	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)

	stateMgr, err := state.NewManager(cfg, log)
	require.NoError(t, err)

	// Create camera manager
	camMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)

	// Create screenshot service
	screenshotSvc, err := screenshots.NewService(stateMgr, cfg, log)
	require.NoError(t, err)

	// Create sync service (without gRPC client for testing)
	syncSvc := NewSyncService(cfg, camMgr, screenshotSvc, nil, log)

	cleanup := func() {
		stateMgr.Close()
		os.RemoveAll(tmpDir)
	}

	return syncSvc, camMgr, screenshotSvc, stateMgr, cleanup
}

func TestSyncService_BuildDatasetStatus_EmptyDatabase(t *testing.T) {
	syncSvc, camMgr, _, _, cleanup := setupTestSyncService(t)
	defer cleanup()

	ctx := context.Background()

	// Register a camera
	discovered := &camera.DiscoveredCamera{
		ID:           "test-camera-1",
		Manufacturer: "Test",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://test/stream"},
		Capabilities: camera.CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := camMgr.RegisterCamera(ctx, discovered)
	require.NoError(t, err)

	cam, err := camMgr.GetCamera("test-camera-1")
	require.NoError(t, err)

	// Build dataset status for empty database
	status := syncSvc.buildDatasetStatus(ctx, cam)

	assert.NotNil(t, status)
	assert.Empty(t, status.LabelCounts)
	assert.Equal(t, 0, status.LabeledSnapshotCount)
	assert.Equal(t, 50, status.RequiredSnapshotCount) // minSnapshots from config
	assert.True(t, status.SnapshotRequired)
	assert.False(t, status.LastSynced.IsZero())
}

func TestSyncService_BuildDatasetStatus_WithScreenshots(t *testing.T) {
	syncSvc, camMgr, screenshotSvc, stateMgr, cleanup := setupTestSyncService(t)
	defer cleanup()

	ctx := context.Background()

	// Register a camera
	discovered := &camera.DiscoveredCamera{
		ID:           "test-camera-1",
		Manufacturer: "Test",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://test/stream"},
		Capabilities: camera.CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := camMgr.RegisterCamera(ctx, discovered)
	require.NoError(t, err)

	// Create test camera in state (required for foreign key)
	camState := state.CameraState{
		ID:      "test-camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test/stream",
		Enabled: true,
	}
	err = stateMgr.SaveCamera(ctx, camState)
	require.NoError(t, err)

	// Create test image
	imageData := createTestImageBytes(t)

	// Save multiple screenshots with different labels
	screenshots := []*screenshots.Screenshot{
		{CameraID: "test-camera-1", Label: screenshots.LabelNormal},
		{CameraID: "test-camera-1", Label: screenshots.LabelNormal},
		{CameraID: "test-camera-1", Label: screenshots.LabelNormal},
		{CameraID: "test-camera-1", Label: screenshots.LabelThreat},
		{CameraID: "test-camera-1", Label: screenshots.LabelAbnormal},
	}

	for _, s := range screenshots {
		err := screenshotSvc.SaveScreenshot(ctx, s, imageData)
		require.NoError(t, err)
	}

	cam, err := camMgr.GetCamera("test-camera-1")
	require.NoError(t, err)

	// Build dataset status
	status := syncSvc.buildDatasetStatus(ctx, cam)

	assert.NotNil(t, status)
	assert.Equal(t, 3, status.LabelCounts["normal"])
	assert.Equal(t, 1, status.LabelCounts["threat"])
	assert.Equal(t, 1, status.LabelCounts["abnormal"])
	assert.Equal(t, 5, status.LabeledSnapshotCount)
	assert.Equal(t, 50, status.RequiredSnapshotCount)
	assert.True(t, status.SnapshotRequired) // Only 3 normal, need 50
	assert.False(t, status.LastSynced.IsZero())
}

func TestSyncService_BuildDatasetStatus_WithEnoughSnapshots(t *testing.T) {
	syncSvc, camMgr, screenshotSvc, stateMgr, cleanup := setupTestSyncService(t)
	defer cleanup()

	ctx := context.Background()

	// Register a camera
	discovered := &camera.DiscoveredCamera{
		ID:           "test-camera-1",
		Manufacturer: "Test",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://test/stream"},
		Capabilities: camera.CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := camMgr.RegisterCamera(ctx, discovered)
	require.NoError(t, err)

	// Create test camera in state
	camState := state.CameraState{
		ID:      "test-camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test/stream",
		Enabled: true,
	}
	err = stateMgr.SaveCamera(ctx, camState)
	require.NoError(t, err)

	// Create test image
	imageData := createTestImageBytes(t)

	// Save exactly 50 normal snapshots (required count)
	for i := 0; i < 50; i++ {
		s := &screenshots.Screenshot{
			CameraID: "test-camera-1",
			Label:    screenshots.LabelNormal,
		}
		err := screenshotSvc.SaveScreenshot(ctx, s, imageData)
		require.NoError(t, err)
	}

	cam, err := camMgr.GetCamera("test-camera-1")
	require.NoError(t, err)

	// Build dataset status
	status := syncSvc.buildDatasetStatus(ctx, cam)

	assert.NotNil(t, status)
	assert.Equal(t, 50, status.LabelCounts["normal"])
	assert.Equal(t, 50, status.LabeledSnapshotCount)
	assert.Equal(t, 50, status.RequiredSnapshotCount)
	assert.False(t, status.SnapshotRequired) // Have exactly 50, no longer required
}

func TestSyncService_BuildDatasetStatus_WithMoreThanRequired(t *testing.T) {
	syncSvc, camMgr, screenshotSvc, stateMgr, cleanup := setupTestSyncService(t)
	defer cleanup()

	ctx := context.Background()

	// Register a camera
	discovered := &camera.DiscoveredCamera{
		ID:           "test-camera-1",
		Manufacturer: "Test",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://test/stream"},
		Capabilities: camera.CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := camMgr.RegisterCamera(ctx, discovered)
	require.NoError(t, err)

	// Create test camera in state
	camState := state.CameraState{
		ID:      "test-camera-1",
		Name:    "Test Camera",
		RTSPURL: "rtsp://test/stream",
		Enabled: true,
	}
	err = stateMgr.SaveCamera(ctx, camState)
	require.NoError(t, err)

	// Create test image
	imageData := createTestImageBytes(t)

	// Save 55 normal snapshots (more than required 50)
	for i := 0; i < 55; i++ {
		s := &screenshots.Screenshot{
			CameraID: "test-camera-1",
			Label:    screenshots.LabelNormal,
		}
		err := screenshotSvc.SaveScreenshot(ctx, s, imageData)
		require.NoError(t, err)
	}

	cam, err := camMgr.GetCamera("test-camera-1")
	require.NoError(t, err)

	// Build dataset status
	status := syncSvc.buildDatasetStatus(ctx, cam)

	assert.NotNil(t, status)
	assert.Equal(t, 55, status.LabelCounts["normal"])
	assert.Equal(t, 55, status.LabeledSnapshotCount)
	assert.Equal(t, 50, status.RequiredSnapshotCount)
	assert.False(t, status.SnapshotRequired) // Have more than required
}

func TestSyncService_BuildDatasetStatus_WithoutScreenshotService(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sync-service-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: tmpDir,
			},
			AI: config.AIConfig{
				MinNormalSnapshots: 50,
			},
		},
	}

	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)

	stateMgr, err := state.NewManager(cfg, log)
	require.NoError(t, err)
	defer stateMgr.Close()

	camMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)

	// Create sync service without screenshot service
	syncSvc := NewSyncService(cfg, camMgr, nil, nil, log)

	ctx := context.Background()

	// Register a camera
	discovered := &camera.DiscoveredCamera{
		ID:           "test-camera-1",
		Manufacturer: "Test",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://test/stream"},
		Capabilities: camera.CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err = camMgr.RegisterCamera(ctx, discovered)
	require.NoError(t, err)

	cam, err := camMgr.GetCamera("test-camera-1")
	require.NoError(t, err)

	// Build dataset status without screenshot service
	status := syncSvc.buildDatasetStatus(ctx, cam)

	assert.NotNil(t, status)
	assert.Empty(t, status.LabelCounts)
	assert.Equal(t, 0, status.LabeledSnapshotCount)
	assert.Equal(t, 50, status.RequiredSnapshotCount)
	assert.True(t, status.SnapshotRequired)
	assert.False(t, status.LastSynced.IsZero())
}

// Helper function for creating test image bytes (minimal valid JPEG)
func createTestImageBytes(t *testing.T) []byte {
	// Create a minimal valid JPEG image (1x1 pixel)
	// This is a minimal valid JPEG file
	return []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x14, 0x00, 0x01, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x08, 0xFF, 0xC4, 0x00, 0x14, 0x10, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xDA,
		0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00, 0xD2, 0xFF, 0xD9,
	}
}
