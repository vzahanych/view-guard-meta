package modelcatalog

import (
	"path/filepath"
	"testing"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
)

func TestNewModelCatalog(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	if catalog.modelStorage == nil {
		t.Fatal("Model storage should not be nil")
	}

	if catalog.baseDir != baseDir {
		t.Fatalf("Expected baseDir '%s', got '%s'", baseDir, catalog.baseDir)
	}
}

func TestRegisterModel(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	modelID := "test-model-1"
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &storage.ModelMetadata{
		ModelID:    modelID,
		Version:    "1.0.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	// Store model first
	err = modelStorage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Register model in catalog
	err = catalog.RegisterModel(modelID, metadata)
	if err != nil {
		t.Fatalf("Failed to register model: %v", err)
	}
}

func TestGetModel(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	modelID := "baseline-yolov8n"
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &storage.ModelMetadata{
		ModelID:    modelID,
		Version:    "baseline-1.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	// Store model
	err = modelStorage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Get model from catalog
	entry, err := catalog.GetModel(modelID)
	if err != nil {
		t.Fatalf("Failed to get model: %v", err)
	}

	if entry.ModelID != modelID {
		t.Fatalf("Expected model ID '%s', got '%s'", modelID, entry.ModelID)
	}

	if entry.ModelType != "yolo" {
		t.Fatalf("Expected model type 'yolo', got '%s'", entry.ModelType)
	}

	// Baseline models should have baseline status
	if entry.Status != ModelStatusBaseline {
		t.Fatalf("Expected status 'baseline', got '%s'", entry.Status)
	}
}

func TestListModels(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	// Create multiple models
	models := []struct {
		id        string
		modelType string
		cameraID  string
		datasetID string
	}{
		{"baseline-yolov8n", "yolo", "", ""},
		{"trained-model-1", "yolo", "camera-1", "dataset-1"},
		{"trained-model-2", "yolo", "camera-2", "dataset-2"},
	}

	for i, m := range models {
		// Create model data that passes size validation (>= 1KB for ONNX)
		modelData := make([]byte, 2048)
		for j := range modelData {
			modelData[j] = byte((i*100 + j) % 256)
		}
		metadata := &storage.ModelMetadata{
			ModelID:    m.id,
			Version:    "1.0.0",
			ModelType:  m.modelType,
			Framework:  "onnx",
			InputShape: []int{1, 3, 640, 640},
			ONNXFile:   "model.onnx",
			CameraID:   m.cameraID,
		}
		if m.datasetID != "" {
			metadata.TrainingDatasetID = m.datasetID
			metadata.TrainingDate = "2025-12-01T10:00:00Z"
		}

		err = modelStorage.StoreModel(m.id, modelData, metadata)
		if err != nil {
			t.Fatalf("Failed to store model %s: %v", m.id, err)
		}
	}

	// List all models
	entries, err := catalog.ListModels()
	if err != nil {
		t.Fatalf("Failed to list models: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("Expected 3 models, got %d", len(entries))
	}
}

func TestGetBaselineModels(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	// Create baseline and trained models
	baselineID := "baseline-yolov8n"
	// Create model data that passes size validation (>= 1KB for ONNX)
	baselineData := make([]byte, 2048)
	for i := range baselineData {
		baselineData[i] = byte(i % 256)
	}
	baselineMetadata := &storage.ModelMetadata{
		ModelID:    baselineID,
		Version:    "baseline-1.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	err = modelStorage.StoreModel(baselineID, baselineData, baselineMetadata)
	if err != nil {
		t.Fatalf("Failed to store baseline model: %v", err)
	}

	trainedID := "trained-model-1"
	// Create model data that passes size validation (>= 1KB for ONNX)
	trainedData := make([]byte, 2048)
	for i := range trainedData {
		trainedData[i] = byte(i % 256)
	}
	trainedMetadata := &storage.ModelMetadata{
		ModelID:           trainedID,
		Version:           "trained-1.0",
		ModelType:         "yolo",
		Framework:         "onnx",
		InputShape:        []int{1, 3, 640, 640},
		ONNXFile:          "model.onnx",
		TrainingDatasetID: "dataset-1",
		TrainingDate:      "2025-12-01T10:00:00Z",
	}

	err = modelStorage.StoreModel(trainedID, trainedData, trainedMetadata)
	if err != nil {
		t.Fatalf("Failed to store trained model: %v", err)
	}

	// Get baseline models
	baselineModels, err := catalog.GetBaselineModels()
	if err != nil {
		t.Fatalf("Failed to get baseline models: %v", err)
	}

	if len(baselineModels) != 1 {
		t.Fatalf("Expected 1 baseline model, got %d", len(baselineModels))
	}

	if baselineModels[0].ModelID != baselineID {
		t.Fatalf("Expected baseline model ID '%s', got '%s'", baselineID, baselineModels[0].ModelID)
	}
}

func TestGetBaselineModelsByType(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	// Create YOLO baseline model
	yoloID := "baseline-yolov8n"
	// Create model data that passes size validation (>= 1KB for ONNX)
	yoloData := make([]byte, 2048)
	for i := range yoloData {
		yoloData[i] = byte(i % 256)
	}
	yoloMetadata := &storage.ModelMetadata{
		ModelID:    yoloID,
		Version:    "baseline-1.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	err = modelStorage.StoreModel(yoloID, yoloData, yoloMetadata)
	if err != nil {
		t.Fatalf("Failed to store YOLO model: %v", err)
	}

	// Get baseline YOLO models
	yoloModels, err := catalog.GetBaselineModelsByType("yolo")
	if err != nil {
		t.Fatalf("Failed to get baseline YOLO models: %v", err)
	}

	if len(yoloModels) != 1 {
		t.Fatalf("Expected 1 baseline YOLO model, got %d", len(yoloModels))
	}

	if yoloModels[0].ModelType != "yolo" {
		t.Fatalf("Expected model type 'yolo', got '%s'", yoloModels[0].ModelType)
	}
}

func TestGetModelsByCamera(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	cameraID := "camera-1"

	// Create models for different cameras
	model1ID := "trained-model-1"
	// Create model data that passes size validation (>= 1KB for ONNX)
	model1Data := make([]byte, 2048)
	for i := range model1Data {
		model1Data[i] = byte(i % 256)
	}
	model1Metadata := &storage.ModelMetadata{
		ModelID:           model1ID,
		Version:           "1.0.0",
		ModelType:         "yolo",
		Framework:         "onnx",
		InputShape:        []int{1, 3, 640, 640},
		ONNXFile:          "model.onnx",
		CameraID:          cameraID,
		TrainingDatasetID: "dataset-1",
		TrainingDate:      "2025-12-01T10:00:00Z",
	}

	err = modelStorage.StoreModel(model1ID, model1Data, model1Metadata)
	if err != nil {
		t.Fatalf("Failed to store model 1: %v", err)
	}

	model2ID := "trained-model-2"
	// Create model data that passes size validation (>= 1KB for ONNX)
	model2Data := make([]byte, 2048)
	for i := range model2Data {
		model2Data[i] = byte(i % 256)
	}
	model2Metadata := &storage.ModelMetadata{
		ModelID:           model2ID,
		Version:           "1.0.0",
		ModelType:         "yolo",
		Framework:         "onnx",
		InputShape:        []int{1, 3, 640, 640},
		ONNXFile:          "model.onnx",
		CameraID:          "camera-2", // Different camera
		TrainingDatasetID: "dataset-2",
		TrainingDate:      "2025-12-01T10:00:00Z",
	}

	err = modelStorage.StoreModel(model2ID, model2Data, model2Metadata)
	if err != nil {
		t.Fatalf("Failed to store model 2: %v", err)
	}

	// Get models for camera-1
	cameraModels, err := catalog.GetModelsByCamera(cameraID)
	if err != nil {
		t.Fatalf("Failed to get models by camera: %v", err)
	}

	if len(cameraModels) != 1 {
		t.Fatalf("Expected 1 model for camera-1, got %d", len(cameraModels))
	}

	if cameraModels[0].CameraID != cameraID {
		t.Fatalf("Expected camera ID '%s', got '%s'", cameraID, cameraModels[0].CameraID)
	}
}

func TestGetModelsByDataset(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	datasetID := "dataset-1"

	// Create models trained from different datasets
	model1ID := "trained-model-1"
	// Create model data that passes size validation (>= 1KB for ONNX)
	model1Data := make([]byte, 2048)
	for i := range model1Data {
		model1Data[i] = byte(i % 256)
	}
	model1Metadata := &storage.ModelMetadata{
		ModelID:           model1ID,
		Version:           "1.0.0",
		ModelType:         "yolo",
		Framework:         "onnx",
		InputShape:        []int{1, 3, 640, 640},
		ONNXFile:          "model.onnx",
		TrainingDatasetID: datasetID,
		TrainingDate:      "2025-12-01T10:00:00Z",
	}

	err = modelStorage.StoreModel(model1ID, model1Data, model1Metadata)
	if err != nil {
		t.Fatalf("Failed to store model 1: %v", err)
	}

	model2ID := "trained-model-2"
	// Create model data that passes size validation (>= 1KB for ONNX)
	model2Data := make([]byte, 2048)
	for i := range model2Data {
		model2Data[i] = byte(i % 256)
	}
	model2Metadata := &storage.ModelMetadata{
		ModelID:           model2ID,
		Version:           "1.0.0",
		ModelType:         "yolo",
		Framework:         "onnx",
		InputShape:        []int{1, 3, 640, 640},
		ONNXFile:          "model.onnx",
		TrainingDatasetID: "dataset-2", // Different dataset
		TrainingDate:      "2025-12-01T10:00:00Z",
	}

	err = modelStorage.StoreModel(model2ID, model2Data, model2Metadata)
	if err != nil {
		t.Fatalf("Failed to store model 2: %v", err)
	}

	// Get models for dataset-1
	datasetModels, err := catalog.GetModelsByDataset(datasetID)
	if err != nil {
		t.Fatalf("Failed to get models by dataset: %v", err)
	}

	if len(datasetModels) != 1 {
		t.Fatalf("Expected 1 model for dataset-1, got %d", len(datasetModels))
	}

	if datasetModels[0].TrainingDatasetID != datasetID {
		t.Fatalf("Expected training dataset ID '%s', got '%s'", datasetID, datasetModels[0].TrainingDatasetID)
	}
}

func TestGetModelsByStatus(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	// Create baseline model
	baselineID := "baseline-yolov8n"
	// Create model data that passes size validation (>= 1KB for ONNX)
	baselineData := make([]byte, 2048)
	for i := range baselineData {
		baselineData[i] = byte(i % 256)
	}
	baselineMetadata := &storage.ModelMetadata{
		ModelID:    baselineID,
		Version:    "baseline-1.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	err = modelStorage.StoreModel(baselineID, baselineData, baselineMetadata)
	if err != nil {
		t.Fatalf("Failed to store baseline model: %v", err)
	}

	// Create trained model (ready status)
	trainedID := "trained-model-1"
	// Create model data that passes size validation (>= 1KB for ONNX)
	trainedData := make([]byte, 2048)
	for i := range trainedData {
		trainedData[i] = byte(i % 256)
	}
	trainedMetadata := &storage.ModelMetadata{
		ModelID:           trainedID,
		Version:           "trained-1.0",
		ModelType:         "yolo",
		Framework:         "onnx",
		InputShape:        []int{1, 3, 640, 640},
		ONNXFile:          "model.onnx",
		TrainingDatasetID: "dataset-1",
		TrainingDate:      "2025-12-01T10:00:00Z",
	}

	err = modelStorage.StoreModel(trainedID, trainedData, trainedMetadata)
	if err != nil {
		t.Fatalf("Failed to store trained model: %v", err)
	}

	// Get baseline models
	baselineModels, err := catalog.GetModelsByStatus(ModelStatusBaseline)
	if err != nil {
		t.Fatalf("Failed to get baseline models: %v", err)
	}

	if len(baselineModels) != 1 {
		t.Fatalf("Expected 1 baseline model, got %d", len(baselineModels))
	}

	// Get ready models
	readyModels, err := catalog.GetModelsByStatus(ModelStatusReady)
	if err != nil {
		t.Fatalf("Failed to get ready models: %v", err)
	}

	if len(readyModels) != 1 {
		t.Fatalf("Expected 1 ready model, got %d", len(readyModels))
	}
}

func TestGetDefaultBaselineModel(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	// Create baseline YOLO model
	baselineID := "baseline-yolov8n"
	// Create model data that passes size validation (>= 1KB for ONNX)
	baselineData := make([]byte, 2048)
	for i := range baselineData {
		baselineData[i] = byte(i % 256)
	}
	baselineMetadata := &storage.ModelMetadata{
		ModelID:    baselineID,
		Version:    "baseline-1.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	err = modelStorage.StoreModel(baselineID, baselineData, baselineMetadata)
	if err != nil {
		t.Fatalf("Failed to store baseline model: %v", err)
	}

	// Get default baseline model
	defaultModel, err := catalog.GetDefaultBaselineModel()
	if err != nil {
		t.Fatalf("Failed to get default baseline model: %v", err)
	}

	if defaultModel.ModelID != baselineID {
		t.Fatalf("Expected default baseline model ID '%s', got '%s'", baselineID, defaultModel.ModelID)
	}
}

func TestModelExists(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	modelID := "test-model-1"

	// Check non-existent model
	if catalog.ModelExists(modelID) {
		t.Fatal("Model should not exist")
	}

	// Create model
	// Create model data that passes size validation (>= 1KB for ONNX)
	modelData := make([]byte, 2048)
	for i := range modelData {
		modelData[i] = byte(i % 256)
	}
	metadata := &storage.ModelMetadata{
		ModelID:    modelID,
		Version:    "1.0.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	err = modelStorage.StoreModel(modelID, modelData, metadata)
	if err != nil {
		t.Fatalf("Failed to store model: %v", err)
	}

	// Check existing model
	if !catalog.ModelExists(modelID) {
		t.Fatal("Model should exist")
	}
}

func TestScanModels(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	// Create models directly in storage
	models := []string{"model-1", "model-2", "model-3"}
	for i, modelID := range models {
		// Create model data that passes size validation (>= 1KB for ONNX)
		modelData := make([]byte, 2048)
		for j := range modelData {
			modelData[j] = byte((i*100 + j) % 256)
		}
		metadata := &storage.ModelMetadata{
			ModelID:    modelID,
			Version:    "1.0.0",
			ModelType:  "yolo",
			Framework:  "onnx",
			InputShape: []int{1, 3, 640, 640},
			ONNXFile:   "model.onnx",
		}

		err = modelStorage.StoreModel(modelID, modelData, metadata)
		if err != nil {
			t.Fatalf("Failed to store model %s: %v", modelID, err)
		}
	}

	// Scan models
	err = catalog.ScanModels()
	if err != nil {
		t.Fatalf("Failed to scan models: %v", err)
	}

	// Verify all models are discoverable
	for _, modelID := range models {
		if !catalog.ModelExists(modelID) {
			t.Fatalf("Model %s should exist after scanning", modelID)
		}
	}
}

func TestDetermineModelStatus(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "models")

	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create model storage: %v", err)
	}

	catalog, err := NewModelCatalog(modelStorage, baseDir)
	if err != nil {
		t.Fatalf("Failed to create model catalog: %v", err)
	}

	// Test baseline status
	baselineID := "baseline-yolov8n"
	// Create model data that passes size validation (>= 1KB for ONNX)
	baselineData := make([]byte, 2048)
	for i := range baselineData {
		baselineData[i] = byte(i % 256)
	}
	baselineMetadata := &storage.ModelMetadata{
		ModelID:    baselineID,
		Version:    "baseline-1.0",
		ModelType:  "yolo",
		Framework:  "onnx",
		InputShape: []int{1, 3, 640, 640},
		ONNXFile:   "model.onnx",
	}

	err = modelStorage.StoreModel(baselineID, baselineData, baselineMetadata)
	if err != nil {
		t.Fatalf("Failed to store baseline model: %v", err)
	}

	entry, err := catalog.GetModel(baselineID)
	if err != nil {
		t.Fatalf("Failed to get baseline model: %v", err)
	}

	if entry.Status != ModelStatusBaseline {
		t.Fatalf("Expected status 'baseline', got '%s'", entry.Status)
	}

	// Test ready status (trained model with training date)
	trainedID := "trained-model-1"
	// Create model data that passes size validation (>= 1KB for ONNX)
	trainedData := make([]byte, 2048)
	for i := range trainedData {
		trainedData[i] = byte(i % 256)
	}
	trainedMetadata := &storage.ModelMetadata{
		ModelID:           trainedID,
		Version:           "trained-1.0",
		ModelType:         "yolo",
		Framework:         "onnx",
		InputShape:        []int{1, 3, 640, 640},
		ONNXFile:          "model.onnx",
		TrainingDatasetID: "dataset-1",
		TrainingDate:      "2025-12-01T10:00:00Z",
	}

	err = modelStorage.StoreModel(trainedID, trainedData, trainedMetadata)
	if err != nil {
		t.Fatalf("Failed to store trained model: %v", err)
	}

	entry, err = catalog.GetModel(trainedID)
	if err != nil {
		t.Fatalf("Failed to get trained model: %v", err)
	}

	if entry.Status != ModelStatusReady {
		t.Fatalf("Expected status 'ready', got '%s'", entry.Status)
	}
}
