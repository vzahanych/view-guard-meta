package storage

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DatasetStorage manages training dataset storage
type DatasetStorage struct {
	baseDir string
}

// DatasetInfo contains dataset metadata
type DatasetInfo struct {
	DatasetID   string
	Name        string
	BasePath    string
	LabelCounts map[string]int
	TotalImages int
	SizeBytes   int64
}

// NewDatasetStorage creates a new dataset storage manager
func NewDatasetStorage(baseDir string) (*DatasetStorage, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("base directory is required")
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &DatasetStorage{
		baseDir: baseDir,
	}, nil
}

// CreateDataset creates a new dataset directory structure
func (ds *DatasetStorage) CreateDataset(datasetID string) (string, error) {
	if datasetID == "" {
		return "", fmt.Errorf("dataset ID is required")
	}

	datasetPath := filepath.Join(ds.baseDir, datasetID)
	if err := os.MkdirAll(datasetPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create dataset directory: %w", err)
	}

	return datasetPath, nil
}

// StoreImage stores an image in the dataset under the specified label
func (ds *DatasetStorage) StoreImage(datasetID, label, imageID string, imageData []byte) (string, error) {
	if datasetID == "" || label == "" || imageID == "" {
		return "", fmt.Errorf("dataset ID, label, and image ID are required")
	}

	// Validate label (sanitize)
	label = sanitizeLabel(label)
	if label == "" {
		return "", fmt.Errorf("invalid label")
	}

	// Create label directory
	labelDir := filepath.Join(ds.baseDir, datasetID, label)
	if err := os.MkdirAll(labelDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create label directory: %w", err)
	}

	// Generate filename
	filename := fmt.Sprintf("image_%s.jpg", imageID)
	imagePath := filepath.Join(labelDir, filename)

	// Write image file
	if err := os.WriteFile(imagePath, imageData, 0644); err != nil {
		return "", fmt.Errorf("failed to write image file: %w", err)
	}

	return imagePath, nil
}

// GetDatasetPath returns the path to a dataset directory
func (ds *DatasetStorage) GetDatasetPath(datasetID string) string {
	return filepath.Join(ds.baseDir, datasetID)
}

// GetLabelPath returns the path to a label directory within a dataset
func (ds *DatasetStorage) GetLabelPath(datasetID, label string) string {
	label = sanitizeLabel(label)
	return filepath.Join(ds.baseDir, datasetID, label)
}

// GetDatasetInfo returns information about a dataset
func (ds *DatasetStorage) GetDatasetInfo(datasetID string) (*DatasetInfo, error) {
	datasetPath := ds.GetDatasetPath(datasetID)

	// Check if dataset exists
	if _, err := os.Stat(datasetPath); err != nil {
		return nil, fmt.Errorf("dataset not found: %w", err)
	}

	// Count images by label
	labelCounts := make(map[string]int)
	totalImages := 0
	var totalSize int64 = 0

	err := filepath.Walk(datasetPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() {
			return nil
		}

		// Check if it's an image file
		if strings.HasSuffix(strings.ToLower(path), ".jpg") ||
			strings.HasSuffix(strings.ToLower(path), ".jpeg") ||
			strings.HasSuffix(strings.ToLower(path), ".png") {
			totalImages++
			totalSize += info.Size()

			// Get label from directory structure
			relPath, err := filepath.Rel(datasetPath, path)
			if err != nil {
				return err
			}

			parts := strings.Split(relPath, string(filepath.Separator))
			if len(parts) >= 1 {
				label := parts[0]
				labelCounts[label]++
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk dataset directory: %w", err)
	}

	return &DatasetInfo{
		DatasetID:   datasetID,
		Name:        filepath.Base(datasetPath),
		BasePath:    datasetPath,
		LabelCounts: labelCounts,
		TotalImages: totalImages,
		SizeBytes:   totalSize,
	}, nil
}

// ExportDataset exports a dataset as a ZIP archive
func (ds *DatasetStorage) ExportDataset(datasetID, outputPath string) error {
	datasetPath := ds.GetDatasetPath(datasetID)

	// Check if dataset exists
	if _, err := os.Stat(datasetPath); err != nil {
		return fmt.Errorf("dataset not found: %w", err)
	}

	// Create ZIP file
	zipFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create ZIP file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk dataset directory and add files to ZIP
	err = filepath.Walk(datasetPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(datasetPath, path)
		if err != nil {
			return err
		}

		// Create file in ZIP
		fileWriter, err := zipWriter.Create(relPath)
		if err != nil {
			return fmt.Errorf("failed to create file in ZIP: %w", err)
		}

		// Read and write file content
		fileData, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		if _, err := fileWriter.Write(fileData); err != nil {
			return fmt.Errorf("failed to write file to ZIP: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to export dataset: %w", err)
	}

	return nil
}

// ImportDataset imports a dataset from a ZIP archive
func (ds *DatasetStorage) ImportDataset(datasetID, zipPath string) error {
	// Open ZIP file
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open ZIP file: %w", err)
	}
	defer zipReader.Close()

	// Create dataset directory
	datasetPath, err := ds.CreateDataset(datasetID)
	if err != nil {
		return fmt.Errorf("failed to create dataset directory: %w", err)
	}

	// Extract files
	for _, file := range zipReader.File {
		// Get file path
		filePath := filepath.Join(datasetPath, file.Name)

		// Create directory if needed
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		// Open file in ZIP
		fileReader, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in ZIP: %w", err)
		}

		// Create output file
		outputFile, err := os.Create(filePath)
		if err != nil {
			fileReader.Close()
			return fmt.Errorf("failed to create output file: %w", err)
		}

		// Copy file content
		_, err = io.Copy(outputFile, fileReader)
		fileReader.Close()
		outputFile.Close()

		if err != nil {
			return fmt.Errorf("failed to copy file: %w", err)
		}
	}

	return nil
}

// DeleteDataset deletes a dataset and all its files
func (ds *DatasetStorage) DeleteDataset(datasetID string) error {
	datasetPath := ds.GetDatasetPath(datasetID)
	return os.RemoveAll(datasetPath)
}

// ListDatasets returns a list of all dataset IDs
func (ds *DatasetStorage) ListDatasets() ([]string, error) {
	entries, err := os.ReadDir(ds.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read base directory: %w", err)
	}

	var datasets []string
	for _, entry := range entries {
		if entry.IsDir() {
			datasets = append(datasets, entry.Name())
		}
	}

	return datasets, nil
}

// GetLabelCounts returns the count of images per label for a dataset
func (ds *DatasetStorage) GetLabelCounts(datasetID string) (map[string]int, error) {
	info, err := ds.GetDatasetInfo(datasetID)
	if err != nil {
		return nil, err
	}
	return info.LabelCounts, nil
}

// ValidateDataset validates a dataset structure
func (ds *DatasetStorage) ValidateDataset(datasetID string) error {
	datasetPath := ds.GetDatasetPath(datasetID)

	// Check if dataset exists
	if _, err := os.Stat(datasetPath); err != nil {
		return fmt.Errorf("dataset not found: %w", err)
	}

	// Check if dataset has at least one label directory
	entries, err := os.ReadDir(datasetPath)
	if err != nil {
		return fmt.Errorf("failed to read dataset directory: %w", err)
	}

	hasLabels := false
	for _, entry := range entries {
		if entry.IsDir() {
			hasLabels = true
			break
		}
	}

	if !hasLabels {
		return fmt.Errorf("dataset has no label directories")
	}

	return nil
}

// sanitizeLabel sanitizes a label name for use as a directory name
func sanitizeLabel(label string) string {
	// Remove invalid characters
	label = strings.TrimSpace(label)
	label = strings.ToLower(label)
	label = strings.ReplaceAll(label, " ", "_")
	label = strings.ReplaceAll(label, "/", "_")
	label = strings.ReplaceAll(label, "\\", "_")
	label = strings.ReplaceAll(label, "..", "_")

	// Common labels
	validLabels := map[string]string{
		"normal":   "normal",
		"threat":   "threat",
		"abnormal": "abnormal",
		"custom":   "custom",
	}

	if normalized, ok := validLabels[label]; ok {
		return normalized
	}

	return label
}

// GetDatasetSize returns the total size of a dataset in bytes
func (ds *DatasetStorage) GetDatasetSize(datasetID string) (int64, error) {
	info, err := ds.GetDatasetInfo(datasetID)
	if err != nil {
		return 0, err
	}
	return info.SizeBytes, nil
}

// GetTotalStorageSize returns the total size of all datasets
func (ds *DatasetStorage) GetTotalStorageSize() (int64, error) {
	datasets, err := ds.ListDatasets()
	if err != nil {
		return 0, err
	}

	var totalSize int64
	for _, datasetID := range datasets {
		size, err := ds.GetDatasetSize(datasetID)
		if err != nil {
			continue // Skip datasets that can't be read
		}
		totalSize += size
	}

	return totalSize, nil
}
