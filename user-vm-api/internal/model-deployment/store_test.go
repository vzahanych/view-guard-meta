package modeldeployment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
)

func setupTestDB(t *testing.T) (*database.DB, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := database.DefaultConfig(dbPath)
	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	ctx := context.Background()
	if err := db.InitializeSchema(ctx); err != nil {
		db.Close()
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// Create required foreign key records
	// Create edge
	now := time.Now().Unix()
	_, err = db.ExecContext(ctx, `
		INSERT INTO edges (edge_id, name, status, wireguard_public_key, wireguard_endpoint, last_seen, created_at, updated_at)
		VALUES ('test-edge-1', 'Test Edge 1', 'active', 'test-key-1', '10.0.0.1:51820', ?, ?, ?)
	`, now, now, now)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create test edge: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO edges (edge_id, name, status, wireguard_public_key, wireguard_endpoint, last_seen, created_at, updated_at)
		VALUES ('test-edge-2', 'Test Edge 2', 'active', 'test-key-2', '10.0.0.2:51820', ?, ?, ?)
	`, now, now, now)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create test edge: %v", err)
	}

	// Create model (with required fields)
	_, err = db.ExecContext(ctx, `
		INSERT INTO ai_models (model_id, name, version, type, model_file_path, metadata_file_path, status, created_at, updated_at)
		VALUES ('test-model-1', 'Test Model 1', '1.0.0', 'yolo', '/path/to/model1.onnx', '/path/to/metadata1.json', 'ready', ?, ?)
	`, now, now)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create test model: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO ai_models (model_id, name, version, type, model_file_path, metadata_file_path, status, created_at, updated_at)
		VALUES ('test-model-2', 'Test Model 2', '1.0.0', 'yolo', '/path/to/model2.onnx', '/path/to/metadata2.json', 'ready', ?, ?)
	`, now, now)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create test model: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO ai_models (model_id, name, version, type, model_file_path, metadata_file_path, status, created_at, updated_at)
		VALUES ('test-model-3', 'Test Model 3', '1.0.0', 'yolo', '/path/to/model3.onnx', '/path/to/metadata3.json', 'ready', ?, ?)
	`, now, now)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to create test model: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

func TestNewDeploymentStore(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store, err := NewDeploymentStore(db)
	if err != nil {
		t.Fatalf("Failed to create deployment store: %v", err)
	}

	if store == nil {
		t.Fatal("Deployment store should not be nil")
	}

	if store.db == nil {
		t.Fatal("Database should not be nil")
	}
}

func TestNewDeploymentStore_NilDB(t *testing.T) {
	store, err := NewDeploymentStore(nil)
	if err == nil {
		t.Fatal("Expected error for nil database")
	}
	if store != nil {
		t.Fatal("Store should be nil on error")
	}
}

func TestCreateDeployment(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store, err := NewDeploymentStore(db)
	if err != nil {
		t.Fatalf("Failed to create deployment store: %v", err)
	}

	ctx := context.Background()
	job := &DeploymentJob{
		DeploymentID: "test-deployment-1",
		ModelID:      "test-model-1",
		EdgeID:       "test-edge-1",
		Status:       DeploymentStatusPending,
	}

	err = store.CreateDeployment(ctx, job)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Verify deployment was created
	retrieved, err := store.GetDeployment(ctx, job.DeploymentID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}

	if retrieved.DeploymentID != job.DeploymentID {
		t.Fatalf("Expected deployment ID %s, got %s", job.DeploymentID, retrieved.DeploymentID)
	}
	if retrieved.ModelID != job.ModelID {
		t.Fatalf("Expected model ID %s, got %s", job.ModelID, retrieved.ModelID)
	}
	if retrieved.EdgeID != job.EdgeID {
		t.Fatalf("Expected edge ID %s, got %s", job.EdgeID, retrieved.EdgeID)
	}
	if retrieved.Status != job.Status {
		t.Fatalf("Expected status %s, got %s", job.Status, retrieved.Status)
	}
}

func TestCreateDeployment_WithOptionalFields(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store, err := NewDeploymentStore(db)
	if err != nil {
		t.Fatalf("Failed to create deployment store: %v", err)
	}

	ctx := context.Background()
	cameraID := "test-camera-1"
	errorMsg := "test error"
	modelPath := "/path/to/model.onnx"
	version := "1.0.0"
	now := time.Now()

	job := &DeploymentJob{
		DeploymentID:         "test-deployment-2",
		ModelID:              "test-model-2",
		EdgeID:               "test-edge-2",
		CameraID:             &cameraID,
		Status:               DeploymentStatusDeploying,
		DeploymentStartedAt:  &now,
		ErrorMessage:         &errorMsg,
		ModelFilePath:        &modelPath,
		DeploymentVersion:    &version,
	}

	err = store.CreateDeployment(ctx, job)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Verify deployment was created with optional fields
	retrieved, err := store.GetDeployment(ctx, job.DeploymentID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}

	if retrieved.CameraID == nil || *retrieved.CameraID != cameraID {
		t.Fatalf("Expected camera ID %s, got %v", cameraID, retrieved.CameraID)
	}
	if retrieved.ErrorMessage == nil || *retrieved.ErrorMessage != errorMsg {
		t.Fatalf("Expected error message %s, got %v", errorMsg, retrieved.ErrorMessage)
	}
	if retrieved.ModelFilePath == nil || *retrieved.ModelFilePath != modelPath {
		t.Fatalf("Expected model path %s, got %v", modelPath, retrieved.ModelFilePath)
	}
	if retrieved.DeploymentVersion == nil || *retrieved.DeploymentVersion != version {
		t.Fatalf("Expected version %s, got %v", version, retrieved.DeploymentVersion)
	}
}

func TestCreateDeployment_NilJob(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store, err := NewDeploymentStore(db)
	if err != nil {
		t.Fatalf("Failed to create deployment store: %v", err)
	}

	ctx := context.Background()
	err = store.CreateDeployment(ctx, nil)
	if err == nil {
		t.Fatal("Expected error for nil job")
	}
}

func TestGetDeployment_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store, err := NewDeploymentStore(db)
	if err != nil {
		t.Fatalf("Failed to create deployment store: %v", err)
	}

	ctx := context.Background()
	_, err = store.GetDeployment(ctx, "non-existent-deployment")
	if err == nil {
		t.Fatal("Expected error for non-existent deployment")
	}
}

func TestUpdateDeployment(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store, err := NewDeploymentStore(db)
	if err != nil {
		t.Fatalf("Failed to create deployment store: %v", err)
	}

	ctx := context.Background()
	job := &DeploymentJob{
		DeploymentID: "test-deployment-3",
		ModelID:      "test-model-3",
		EdgeID:       "test-edge-1",
		Status:       DeploymentStatusPending,
	}

	err = store.CreateDeployment(ctx, job)
	if err != nil {
		t.Fatalf("Failed to create deployment: %v", err)
	}

	// Update deployment
	now := time.Now()
	job.Status = DeploymentStatusDeployed
	job.DeploymentCompletedAt = &now
	modelPath := "/path/to/deployed/model.onnx"
	job.ModelFilePath = &modelPath

	err = store.UpdateDeployment(ctx, job)
	if err != nil {
		t.Fatalf("Failed to update deployment: %v", err)
	}

	// Verify update
	retrieved, err := store.GetDeployment(ctx, job.DeploymentID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}

	if retrieved.Status != DeploymentStatusDeployed {
		t.Fatalf("Expected status %s, got %s", DeploymentStatusDeployed, retrieved.Status)
	}
	if retrieved.DeploymentCompletedAt == nil {
		t.Fatal("Expected deployment completed at to be set")
	}
	if retrieved.ModelFilePath == nil || *retrieved.ModelFilePath != modelPath {
		t.Fatalf("Expected model path %s, got %v", modelPath, retrieved.ModelFilePath)
	}
}

func TestListDeployments(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store, err := NewDeploymentStore(db)
	if err != nil {
		t.Fatalf("Failed to create deployment store: %v", err)
	}

	ctx := context.Background()

	// Create multiple deployments (using test data from setupTestDB)
	deployments := []*DeploymentJob{
		{DeploymentID: "deploy-1", ModelID: "test-model-1", EdgeID: "test-edge-1", Status: DeploymentStatusPending},
		{DeploymentID: "deploy-2", ModelID: "test-model-1", EdgeID: "test-edge-2", Status: DeploymentStatusDeployed},
		{DeploymentID: "deploy-3", ModelID: "test-model-2", EdgeID: "test-edge-1", Status: DeploymentStatusFailed},
	}

	for _, job := range deployments {
		err = store.CreateDeployment(ctx, job)
		if err != nil {
			t.Fatalf("Failed to create deployment: %v", err)
		}
	}

	// List all deployments
	all, err := store.ListDeployments(ctx, &DeploymentFilters{})
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	if len(all) != 3 {
		t.Fatalf("Expected 3 deployments, got %d", len(all))
	}

	// Filter by edge ID
	edge1, err := store.ListDeployments(ctx, &DeploymentFilters{EdgeID: "test-edge-1"})
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	if len(edge1) != 2 {
		t.Fatalf("Expected 2 deployments for test-edge-1, got %d", len(edge1))
	}

	// Filter by model ID
	model1, err := store.ListDeployments(ctx, &DeploymentFilters{ModelID: "test-model-1"})
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	if len(model1) != 2 {
		t.Fatalf("Expected 2 deployments for test-model-1, got %d", len(model1))
	}

	// Filter by status
	deployed, err := store.ListDeployments(ctx, &DeploymentFilters{Status: DeploymentStatusDeployed})
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	if len(deployed) != 1 {
		t.Fatalf("Expected 1 deployed deployment, got %d", len(deployed))
	}
}

func TestListDeployments_WithPagination(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store, err := NewDeploymentStore(db)
	if err != nil {
		t.Fatalf("Failed to create deployment store: %v", err)
	}

	ctx := context.Background()

	// Create 5 deployments (using test data from setupTestDB)
	for i := 1; i <= 5; i++ {
		job := &DeploymentJob{
			DeploymentID: fmt.Sprintf("deploy-%d", i),
			ModelID:      "test-model-1",
			EdgeID:       "test-edge-1",
			Status:       DeploymentStatusPending,
		}
		err = store.CreateDeployment(ctx, job)
		if err != nil {
			t.Fatalf("Failed to create deployment: %v", err)
		}
	}

	// Test pagination
	page1, err := store.ListDeployments(ctx, &DeploymentFilters{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	if len(page1) != 2 {
		t.Fatalf("Expected 2 deployments on page 1, got %d", len(page1))
	}

	page2, err := store.ListDeployments(ctx, &DeploymentFilters{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	if len(page2) != 2 {
		t.Fatalf("Expected 2 deployments on page 2, got %d", len(page2))
	}

	// Verify no overlap
	if page1[0].DeploymentID == page2[0].DeploymentID {
		t.Fatal("Page 1 and page 2 should not have overlapping deployments")
	}
}

