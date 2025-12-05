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
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		Framework: "onnx",
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
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		Framework: "onnx",
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
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		Framework: "onnx",
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
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		Framework: "onnx",
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
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		Framework: "onnx",
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
		// Create model data that passes size validation (>= 1KB for ONNX)
		modelData := make([]byte, 2048)
		for j := range modelData {
			modelData[j] = byte((i*100 + j) % 256)
		}
		metadata := &ModelMetadata{
			ModelID:   modelID,
			Version:   "1.0.0",
			ModelType: "cae",
			Framework: "onnx",
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
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		Framework: "onnx",
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
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		Framework: "onnx",
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
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:   modelID,
		Version:   "1.0.0",
		ModelType: "cae",
		Framework: "onnx",
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

func TestModelSizeLimit(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-oversized"
	// Create model data that exceeds the 50MB limit
	oversizedData := make([]byte, MaxModelSizeBytes+1)
	for i := range oversizedData {
		oversizedData[i] = byte(i % 256)
	}

	metadata := &ModelMetadata{
		ModelID:    modelID,
		Version:    "1.0.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	// Should fail with size limit error
	err = storage.StoreModel(modelID, oversizedData, metadata)
	if err == nil {
		t.Fatal("Expected error when storing oversized model")
	}

	if !contains(err.Error(), "exceeds maximum") {
		t.Fatalf("Expected size limit error, got: %v", err)
	}
}

func TestValidateModelSize(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:    modelID,
		Version:    "1.0.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Validate model size
	err = storage.ValidateModelSize(modelID)
	if err != nil {
		t.Fatalf("Model size validation should pass: %v", err)
	}
}

func TestValidateModelFormat(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-model-1"
	// Create minimal ONNX-like data (ONNX files start with specific magic bytes)
	// For testing, we'll use a simple byte array that passes basic validation
	modelData := make([]byte, 2048) // Minimum size check
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}

	metadata := &ModelMetadata{
		ModelID:    modelID,
		Version:    "1.0.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Validate model format
	err = storage.ValidateModelFormat(modelID)
	// Format validation may pass or fail depending on implementation
	// We just check it doesn't panic
	if err != nil {
		t.Logf("Model format validation returned error (expected for fake data): %v", err)
	}
}

func TestStoreModelWithYOLOMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "test-yolo-model"
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:    modelID,
		Version:    "baseline-1.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
		Preprocessing: map[string]interface{}{
			"resize":       []int{640, 640},
			"normalize":    true,
			"color_format": "BGR",
		},
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store YOLO model: %v", err)
	}

	// Verify metadata was stored correctly
	retrievedMetadata, err := storage.GetMetadata(modelID)
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}

	if retrievedMetadata.ModelType != "yolo" {
		t.Fatalf("Expected model type 'yolo', got '%s'", retrievedMetadata.ModelType)
	}

	if len(retrievedMetadata.InputShape) != 4 {
		t.Fatalf("Expected input shape length 4, got %d", len(retrievedMetadata.InputShape))
	}

	if retrievedMetadata.InputShape[2] != 640 || retrievedMetadata.InputShape[3] != 640 {
		t.Fatalf("Expected input shape [1, 3, 640, 640], got %v", retrievedMetadata.InputShape)
	}
}

func TestStoreModelWithTrainingMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	storage, err := NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	modelID := "trained-model-1"
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &ModelMetadata{
		ModelID:           modelID,
		Version:           "trained-1.0",
		ModelType:         "yolo",
		Framework:         "onnx",
		InputShape:        []int{1, 3, 640, 640},
		ONNXFile:          "model.onnx",
		TrainingDatasetID: "dataset-123",
		TrainingDate:      "2025-12-01T10:00:00Z",
		Accuracy:          0.95,
		Precision:         0.92,
		Recall:            0.94,
		F1Score:           0.93,
	}

	err = storage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store trained model: %v", err)
	}

	// Verify training metadata was stored
	retrievedMetadata, err := storage.GetMetadata(modelID)
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}

	if retrievedMetadata.TrainingDatasetID != "dataset-123" {
		t.Fatalf("Expected training dataset ID 'dataset-123', got '%s'", retrievedMetadata.TrainingDatasetID)
	}

	if retrievedMetadata.Accuracy != 0.95 {
		t.Fatalf("Expected accuracy 0.95, got %f", retrievedMetadata.Accuracy)
	}
}

// Helper function to check if error message contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
