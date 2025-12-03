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
)

// setupTestStorage creates a test storage with temporary database
func setupTestStorage(t *testing.T) (*Storage, string, func()) {
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

	// Create storage
	storage := NewStorage(cfg, log, db)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return storage, tmpDir, cleanup
}

// TestStorage_GetDatasetPath tests dataset path generation
func TestStorage_GetDatasetPath(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	edgeID := "test-edge-1"
	cameraID := "test-camera-1"
	datasetID := "test-dataset-123"

	expectedPath := filepath.Join(storage.dataDir, "datasets", edgeID, cameraID, datasetID)
	actualPath := storage.GetDatasetPath(edgeID, cameraID, datasetID)

	assert.Equal(t, expectedPath, actualPath)
}

// TestStorage_EnsureDatasetDirectory tests directory creation
func TestStorage_EnsureDatasetDirectory(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	edgeID := "test-edge-1"
	cameraID := "test-camera-1"
	datasetID := "test-dataset-123"

	datasetPath, err := storage.EnsureDatasetDirectory(edgeID, cameraID, datasetID)
	require.NoError(t, err)
	require.NotEmpty(t, datasetPath)

	// Verify directory exists
	assert.DirExists(t, datasetPath)

	// Verify screenshots subdirectory exists
	screenshotsDir := filepath.Join(datasetPath, "screenshots")
	assert.DirExists(t, screenshotsDir)

	// Verify path structure
	expectedPath := filepath.Join(storage.dataDir, "datasets", edgeID, cameraID, datasetID)
	assert.Equal(t, expectedPath, datasetPath)
}

// TestStorage_EnsureDatasetDirectory_Idempotent tests that creating directory twice is safe
func TestStorage_EnsureDatasetDirectory_Idempotent(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	edgeID := "test-edge-1"
	cameraID := "test-camera-1"
	datasetID := "test-dataset-123"

	// Create directory first time
	datasetPath1, err := storage.EnsureDatasetDirectory(edgeID, cameraID, datasetID)
	require.NoError(t, err)

	// Create directory second time (should be idempotent)
	datasetPath2, err := storage.EnsureDatasetDirectory(edgeID, cameraID, datasetID)
	require.NoError(t, err)

	assert.Equal(t, datasetPath1, datasetPath2)
	assert.DirExists(t, datasetPath1)
}

// TestStorage_VerifyDatasetStructure tests dataset structure verification
func TestStorage_VerifyDatasetStructure(t *testing.T) {
	storage, tmpDir, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create test dataset directory structure
	datasetPath := filepath.Join(tmpDir, "test-dataset")
	os.MkdirAll(datasetPath, 0755)
	screenshotsDir := filepath.Join(datasetPath, "screenshots")
	os.MkdirAll(screenshotsDir, 0755)

	// Create metadata.json
	metadataPath := filepath.Join(datasetPath, "metadata.json")
	err := os.WriteFile(metadataPath, []byte(`{"edge_id": "test-edge-1"}`), 0644)
	require.NoError(t, err)

	// Verify structure (should pass)
	err = storage.VerifyDatasetStructure(datasetPath)
	require.NoError(t, err)
}

// TestStorage_VerifyDatasetStructure_MissingMetadata tests verification failure for missing metadata
func TestStorage_VerifyDatasetStructure_MissingMetadata(t *testing.T) {
	storage, tmpDir, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create dataset directory without metadata.json
	datasetPath := filepath.Join(tmpDir, "test-dataset")
	os.MkdirAll(datasetPath, 0755)
	screenshotsDir := filepath.Join(datasetPath, "screenshots")
	os.MkdirAll(screenshotsDir, 0755)

	// Verify structure (should fail)
	err := storage.VerifyDatasetStructure(datasetPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata.json")
}

// TestStorage_VerifyDatasetStructure_MissingScreenshotsDir tests verification failure for missing screenshots directory
func TestStorage_VerifyDatasetStructure_MissingScreenshotsDir(t *testing.T) {
	storage, tmpDir, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create dataset directory without screenshots directory
	datasetPath := filepath.Join(tmpDir, "test-dataset")
	os.MkdirAll(datasetPath, 0755)

	// Create metadata.json
	metadataPath := filepath.Join(datasetPath, "metadata.json")
	err := os.WriteFile(metadataPath, []byte(`{"edge_id": "test-edge-1"}`), 0644)
	require.NoError(t, err)

	// Verify structure (should fail)
	err = storage.VerifyDatasetStructure(datasetPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "screenshots")
}

// TestStorage_GetDatasetInfo tests retrieving dataset info from database
func TestStorage_GetDatasetInfo(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	datasetID := "test-dataset-123"
	edgeID := "test-edge-1"
	datasetPath := filepath.Join(storage.dataDir, "datasets", edgeID, "camera-1", datasetID)

	// Insert test dataset into database
	now := time.Now().Unix()
	_, err := storage.db.ExecContext(ctx, `
		INSERT INTO training_datasets (
			dataset_id, name, edge_id, dataset_dir_path, label_counts, total_images, status, checksum, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, datasetID, "test-dataset", edgeID, datasetPath, `{"normal": 50}`, 50, "ready", "test-checksum", now, now)
	require.NoError(t, err)

	// Retrieve dataset info
	info, err := storage.GetDatasetInfo(ctx, datasetID)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, datasetID, info.DatasetID)
	assert.Equal(t, edgeID, info.EdgeID)
	assert.Equal(t, datasetPath, info.DatasetPath)
	assert.Equal(t, 50, info.TotalSnapshots)
	assert.Equal(t, "ready", info.Status)
	assert.Equal(t, 50, info.LabelCounts["normal"])
}

// TestStorage_GetDatasetInfo_NotFound tests error handling for non-existent dataset
func TestStorage_GetDatasetInfo_NotFound(t *testing.T) {
	storage, _, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	nonExistentID := "non-existent-dataset"

	info, err := storage.GetDatasetInfo(ctx, nonExistentID)
	require.Error(t, err)
	assert.Nil(t, info)
	assert.Contains(t, err.Error(), "failed to get dataset info")
}
