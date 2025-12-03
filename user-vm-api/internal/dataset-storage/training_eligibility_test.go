package datasetstorage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database/migrations"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	tunnelgateway "github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
)

// setupTestReceiverWithCapStore creates a test receiver with capability store for training eligibility tests
func setupTestReceiverWithCapStore(t *testing.T) (*Receiver, *tunnelgateway.CapabilityStore, string, func()) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	os.MkdirAll(dataDir, 0755)

	// Create test database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.New(database.DefaultConfig(dbPath))
	require.NoError(t, err)

	// Run migrations
	migrator := migrations.NewMigrator(db)
	ctx := context.Background()
	err = migrator.Up(ctx)
	require.NoError(t, err)

	// Create test edge in database
	now := time.Now().Unix()
	_, err = db.ExecContext(ctx, `
		INSERT INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "test-edge-1", "Test Edge", "test-public-key-123", now, "active", now, now)
	require.NoError(t, err)

	// Create test config
	cfg := &config.Config{
		UserVMAPI: config.UserVMAPIConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: dataDir,
			},
		},
	}

	// Create logger
	log, err := logging.New(logging.LogConfig{
		Level:  "debug",
		Format: "text",
	})
	require.NoError(t, err)

	// Create capability store
	capStore := tunnelgateway.NewCapabilityStore(db)

	// Create edge API server (mock)
	edgeAPIServer := &tunnelgateway.EdgeAPIServer{}

	// Create receiver
	receiver := NewReceiver(cfg, log, db, capStore, edgeAPIServer)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return receiver, capStore, tmpDir, cleanup
}

// TestReceiver_TrainingEligibilityUpdate tests that training eligibility status is updated after dataset upload
func TestReceiver_TrainingEligibilityUpdate(t *testing.T) {
	receiver, capStore, _, cleanup := setupTestReceiverWithCapStore(t)
	defer cleanup()

	ctx := context.Background()
	edgeID := "test-edge-1"
	cameraID := "test-camera-1"

	// First, create a camera status entry (simulating capability sync)
	now := time.Now().Unix()
	_, err := receiver.db.ExecContext(ctx, `
		INSERT INTO edge_camera_status (
			edge_id, camera_id, camera_name, camera_type, camera_status, enabled,
			labeled_snapshot_count, required_snapshot_count, snapshot_required,
			training_eligibility_status, synced_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, edgeID, cameraID, "Test Camera", "rtsp", "connected", 1,
		50, 50, 0, // 50 snapshots, not required anymore
		"needs_snapshots", now, now)
	require.NoError(t, err)

	// Create test archive
	archivePath, checksum := createTestDatasetArchive(t, edgeID, cameraID, 50)
	defer os.Remove(archivePath)

	// Receive dataset
	datasetID, datasetPath, err := receiver.ReceiveDataset(ctx, edgeID, cameraID, archivePath, checksum)
	require.NoError(t, err)
	require.NotEmpty(t, datasetID)
	require.NotEmpty(t, datasetPath)

	// Update training eligibility (this is done by the API handler, but we test it here)
	err = capStore.UpdateTrainingEligibility(ctx, edgeID, cameraID, datasetID, tunnelgateway.TrainingEligibilityReadyForTraining)
	require.NoError(t, err)

	// Verify training eligibility status was updated
	status, err := capStore.GetCameraStatus(ctx, edgeID, cameraID)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, tunnelgateway.TrainingEligibilityReadyForTraining, status.TrainingEligibilityStatus)
	assert.Equal(t, datasetID, status.DatasetID)
}

// TestReceiver_TrainingEligibilityUpdate_NotFound tests error handling when camera not found
func TestReceiver_TrainingEligibilityUpdate_NotFound(t *testing.T) {
	_, capStore, _, cleanup := setupTestReceiverWithCapStore(t)
	defer cleanup()

	ctx := context.Background()
	edgeID := "test-edge-1"
	cameraID := "non-existent-camera"
	datasetID := "test-dataset-123"

	// Try to update training eligibility for non-existent camera
	err := capStore.UpdateTrainingEligibility(ctx, edgeID, cameraID, datasetID, tunnelgateway.TrainingEligibilityReadyForTraining)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
