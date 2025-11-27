package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDatasetStorage(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "datasets")

	storage, err := NewDatasetStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create dataset storage: %v", err)
	}

	if storage.baseDir != baseDir {
		t.Fatalf("Expected baseDir '%s', got '%s'", baseDir, storage.baseDir)
	}

	// Verify directory was created
	if _, err := os.Stat(baseDir); err != nil {
		t.Fatalf("Base directory was not created: %v", err)
	}
}

func TestCreateDataset(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "datasets")

	storage, err := NewDatasetStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create dataset storage: %v", err)
	}

	datasetID := "test-dataset-1"
	datasetPath, err := storage.CreateDataset(datasetID)
	if err != nil {
		t.Fatalf("Failed to create dataset: %v", err)
	}

	expectedPath := filepath.Join(baseDir, datasetID)
	if datasetPath != expectedPath {
		t.Fatalf("Expected path '%s', got '%s'", expectedPath, datasetPath)
	}

	// Verify directory was created
	if _, err := os.Stat(datasetPath); err != nil {
		t.Fatalf("Dataset directory was not created: %v", err)
	}
}

func TestStoreImage(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "datasets")

	storage, err := NewDatasetStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create dataset storage: %v", err)
	}

	datasetID := "test-dataset-1"
	_, err = storage.CreateDataset(datasetID)
	if err != nil {
		t.Fatalf("Failed to create dataset: %v", err)
	}

	// Store image
	imageData := []byte("fake image data")
	imagePath, err := storage.StoreImage(datasetID, "normal", "img-1", imageData)
	if err != nil {
		t.Fatalf("Failed to store image: %v", err)
	}

	// Verify image file was created
	if _, err := os.Stat(imagePath); err != nil {
		t.Fatalf("Image file was not created: %v", err)
	}

	// Verify image data
	readData, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("Failed to read image file: %v", err)
	}
	if string(readData) != string(imageData) {
		t.Fatal("Image data mismatch")
	}
}

func TestGetDatasetInfo(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "datasets")

	storage, err := NewDatasetStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create dataset storage: %v", err)
	}

	datasetID := "test-dataset-1"
	_, err = storage.CreateDataset(datasetID)
	if err != nil {
		t.Fatalf("Failed to create dataset: %v", err)
	}

	// Store some images
	imageData := []byte("fake image data")
	_, err = storage.StoreImage(datasetID, "normal", "img-1", imageData)
	if err != nil {
		t.Fatalf("Failed to store image: %v", err)
	}

	_, err = storage.StoreImage(datasetID, "normal", "img-2", imageData)
	if err != nil {
		t.Fatalf("Failed to store image: %v", err)
	}

	_, err = storage.StoreImage(datasetID, "threat", "img-3", imageData)
	if err != nil {
		t.Fatalf("Failed to store image: %v", err)
	}

	// Get dataset info
	info, err := storage.GetDatasetInfo(datasetID)
	if err != nil {
		t.Fatalf("Failed to get dataset info: %v", err)
	}

	if info.DatasetID != datasetID {
		t.Fatalf("Expected dataset ID '%s', got '%s'", datasetID, info.DatasetID)
	}

	if info.TotalImages != 3 {
		t.Fatalf("Expected 3 images, got %d", info.TotalImages)
	}

	if info.LabelCounts["normal"] != 2 {
		t.Fatalf("Expected 2 normal images, got %d", info.LabelCounts["normal"])
	}

	if info.LabelCounts["threat"] != 1 {
		t.Fatalf("Expected 1 threat image, got %d", info.LabelCounts["threat"])
	}
}

func TestExportImportDataset(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "datasets")
	exportDir := filepath.Join(tmpDir, "exports")

	storage, err := NewDatasetStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create dataset storage: %v", err)
	}

	datasetID := "test-dataset-1"
	_, err = storage.CreateDataset(datasetID)
	if err != nil {
		t.Fatalf("Failed to create dataset: %v", err)
	}

	// Store some images
	imageData := []byte("fake image data")
	_, err = storage.StoreImage(datasetID, "normal", "img-1", imageData)
	if err != nil {
		t.Fatalf("Failed to store image: %v", err)
	}

	// Export dataset
	os.MkdirAll(exportDir, 0755)
	exportPath := filepath.Join(exportDir, "dataset.zip")
	err = storage.ExportDataset(datasetID, exportPath)
	if err != nil {
		t.Fatalf("Failed to export dataset: %v", err)
	}

	// Verify ZIP file was created
	if _, err := os.Stat(exportPath); err != nil {
		t.Fatalf("Export file was not created: %v", err)
	}

	// Import dataset
	newDatasetID := "test-dataset-2"
	err = storage.ImportDataset(newDatasetID, exportPath)
	if err != nil {
		t.Fatalf("Failed to import dataset: %v", err)
	}

	// Verify imported dataset
	info, err := storage.GetDatasetInfo(newDatasetID)
	if err != nil {
		t.Fatalf("Failed to get imported dataset info: %v", err)
	}

	if info.TotalImages != 1 {
		t.Fatalf("Expected 1 image in imported dataset, got %d", info.TotalImages)
	}
}

func TestDeleteDataset(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "datasets")

	storage, err := NewDatasetStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create dataset storage: %v", err)
	}

	datasetID := "test-dataset-1"
	_, err = storage.CreateDataset(datasetID)
	if err != nil {
		t.Fatalf("Failed to create dataset: %v", err)
	}

	// Delete dataset
	err = storage.DeleteDataset(datasetID)
	if err != nil {
		t.Fatalf("Failed to delete dataset: %v", err)
	}

	// Verify dataset was deleted
	_, err = storage.GetDatasetInfo(datasetID)
	if err == nil {
		t.Fatal("Expected error when getting deleted dataset info")
	}
}

func TestListDatasets(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "datasets")

	storage, err := NewDatasetStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create dataset storage: %v", err)
	}

	// Create multiple datasets
	for i := 1; i <= 3; i++ {
		datasetID := "test-dataset-" + string(rune(i+'0'))
		_, err = storage.CreateDataset(datasetID)
		if err != nil {
			t.Fatalf("Failed to create dataset: %v", err)
		}
	}

	// List datasets
	datasets, err := storage.ListDatasets()
	if err != nil {
		t.Fatalf("Failed to list datasets: %v", err)
	}

	if len(datasets) != 3 {
		t.Fatalf("Expected 3 datasets, got %d", len(datasets))
	}
}

func TestValidateDataset(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "datasets")

	storage, err := NewDatasetStorage(baseDir)
	if err != nil {
		t.Fatalf("Failed to create dataset storage: %v", err)
	}

	datasetID := "test-dataset-1"
	_, err = storage.CreateDataset(datasetID)
	if err != nil {
		t.Fatalf("Failed to create dataset: %v", err)
	}

	// Store an image to create label directory
	imageData := []byte("fake image data")
	_, err = storage.StoreImage(datasetID, "normal", "img-1", imageData)
	if err != nil {
		t.Fatalf("Failed to store image: %v", err)
	}

	// Validate dataset
	err = storage.ValidateDataset(datasetID)
	if err != nil {
		t.Fatalf("Failed to validate dataset: %v", err)
	}
}

