package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewModelStorage(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	if storage.baseDir != baseDir {
		t.Fatalf("Expected baseDir '%s', got '%s'", baseDir, storage.baseDir)
	}

	// Verify directory was created
	if _, err := os.Stat(baseDir); err != nil {
		t.Fatalf("Base directory was not created: %v", err)
	}
}

func TestCreateModelDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"
	modelPath, err := storage.CreateModelDirectory(modelID)
	if err != nil {
		t.Fatalf("Failed to create model directory: %v", err)
	}

	expectedPath := filepath.Join(baseDir, modelID)
	if modelPath != expectedPath {
		t.Fatalf("Expected path '%s', got '%s'", expectedPath, modelPath)
	}

	// Verify directory was created
	if _, err := os.Stat(modelPath); err != nil {
		t.Fatalf("Model directory was not created: %v", err)
	}
}

func TestStoreModel(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"
	modelData := []byte("fake onnx model data")
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		ONNXFile:  "model.onnx",
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Verify model file was created
	modelPath := storage.GetModelFilePath(modelID)
	if _, err := os.Stat(modelPath); err != nil {
		t.Fatalf("Model file was not created: %v", err)
	}

	// Verify metadata file was created
	metadataPath := storage.GetMetadataPath(modelID)
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("Metadata file was not created: %v", err)
	}
}

func TestGetModelInfo(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"
	modelData := []byte("fake onnx model data")
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		ONNXFile:  "model.onnx",
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Get model info
	info, err := storage.GetModelInfo(modelID)
	if err != nil {
		t.Fatalf("Failed to get model info: %v", err)
	}

	if info.ModelID != modelID {
		t.Fatalf("Expected model ID '%s', got '%s'", modelID, info.ModelID)
	}

	if info.Metadata.Version != "1.0.0" {
		t.Fatalf("Expected version '1.0.0', got '%s'", info.Metadata.Version)
	}

	if info.Metadata.ModelType != "cae" {
		t.Fatalf("Expected model type 'cae', got '%s'", info.Metadata.ModelType)
	}
}

func TestGetMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"
	modelData := []byte("fake onnx model data")
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		ONNXFile:  "model.onnx",
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Get metadata
	retrievedMetadata, err := storage.GetMetadata(modelID)
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}

	if retrievedMetadata.ModelID != modelID {
		t.Fatalf("Expected model ID '%s', got '%s'", modelID, retrievedMetadata.ModelID)
	}

	if retrievedMetadata.Version != "1.0.0" {
		t.Fatalf("Expected version '1.0.0', got '%s'", retrievedMetadata.Version)
	}
}

func TestReadModel(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"
	modelData := []byte("fake onnx model data")
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		ONNXFile:  "model.onnx",
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Read model
	readData, err := storage.ReadModel(modelID)
	if err != nil {
		t.Fatalf("Failed to read model: %v", err)
	}

	if string(readData) != string(modelData) {
		t.Fatal("Model data mismatch")
	}
}

func TestDeleteModel(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"
	modelData := []byte("fake onnx model data")
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		ONNXFile:  "model.onnx",
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Delete model
	err = storage.DeleteModel(modelID)
	if err != nil {
		t.Fatalf("Failed to delete model: %v", err)
	}

	// Verify model was deleted
	_, err = storage.GetModelInfo(modelID)
	if err == nil {
		t.Fatal("Expected error when getting deleted model info")
	}
}

func TestListModels(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	// Create multiple models
	for i := 1; i <= 3; i++ {
		modelID := "test-model-" + string(rune(i+'0'))
		modelData := []byte("fake onnx model data")
		metadata := &ModelMetadata{
			ModelID:   modelID,
			Version:   "1.0.0",
			ModelType: "cae",
			ONNXFile:  "model.onnx",
		}

		err = storage.StoreModel(modelID, modelData, metadata)
		if err != nil {
			t.Fatalf("Failed to store model: %v", err)
		}
	}

	// List models
	models, err := storage.ListModels()
	if err != nil {
		t.Fatalf("Failed to list models: %v", err)
	}

	if len(models) != 3 {
		t.Fatalf("Expected 3 models, got %d", len(models))
	}
}

func TestValidateModel(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"
	modelData := []byte("fake onnx model data")
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		ONNXFile:  "model.onnx",
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Validate model
	err = storage.ValidateModel(modelID)
	if err != nil {
		t.Fatalf("Failed to validate model: %v", err)
	}
}

func TestUpdateMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"
	modelData := []byte("fake onnx model data")
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		ONNXFile:  "model.onnx",
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Update metadata
	updatedMetadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.1.0",
		ModelType: "cae",
		ONNXFile:  "model.onnx",
	}

	err = storage.UpdateMetadata(modelID, updatedMetadata)
	if err != nil {
		t.Fatalf("Failed to update metadata: %v", err)
	}

	// Verify update
	retrievedMetadata, err := storage.GetMetadata(modelID)
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}

	if retrievedMetadata.Version != "1.1.0" {
		t.Fatalf("Expected version '1.1.0', got '%s'", retrievedMetadata.Version)
	}
}

func TestModelExists(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"

	// Check non-existent model
	if storage.ModelExists(modelID) {
		t.Fatal("Model should not exist")
	}

	// Create model
	modelData := []byte("fake onnx model data")
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		ONNXFile:  "model.onnx",
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Check existing model
	if !storage.ModelExists(modelID) {
		t.Fatal("Model should exist")
	}
}

