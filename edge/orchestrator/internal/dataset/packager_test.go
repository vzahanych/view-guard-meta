package dataset

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
)

// createTestScreenshot creates a test screenshot with a temporary image file
func createTestScreenshot(t *testing.T, tmpDir string, id string, label screenshots.Label, customLabel string) *screenshots.Screenshot {
	// Create a dummy image file
	imagePath := filepath.Join(tmpDir, id+".jpg")
	imageData := []byte("fake-image-data-" + id)
	err := os.WriteFile(imagePath, imageData, 0644)
	require.NoError(t, err)

	return &screenshots.Screenshot{
		ID:          id,
		CameraID:    "test-camera-1",
		FilePath:    imagePath,
		Label:       label,
		CustomLabel: customLabel,
		Description: "Test screenshot " + id,
		CreatedAt:   time.Now(),
	}
}

// TestPackager_PackageDataset tests dataset packaging
func TestPackager_PackageDataset(t *testing.T) {
	log := logger.NewNopLogger()
	packager := NewPackager(log)

	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "exports")
	os.MkdirAll(outputDir, 0755)

	// Create test screenshots
	screenshotList := []*screenshots.Screenshot{
		createTestScreenshot(t, tmpDir, "screenshot-1", screenshots.LabelNormal, ""),
		createTestScreenshot(t, tmpDir, "screenshot-2", screenshots.LabelNormal, ""),
		createTestScreenshot(t, tmpDir, "screenshot-3", screenshots.LabelThreat, ""),
		createTestScreenshot(t, tmpDir, "screenshot-4", screenshots.LabelCustom, "custom-label-1"),
	}

	edgeID := "test-edge-1"
	cameraID := "test-camera-1"

	// Package dataset
	archivePath, checksum, err := packager.PackageDataset(edgeID, cameraID, screenshotList, outputDir)
	require.NoError(t, err)
	require.NotEmpty(t, archivePath)
	require.NotEmpty(t, checksum)

	// Verify archive file exists
	assert.FileExists(t, archivePath)

	// Verify archive is valid tar.gz
	file, err := os.Open(archivePath)
	require.NoError(t, err)
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	// Verify archive contents
	filesFound := make(map[string]bool)
	for {
		header, err := tarReader.Next()
		if err != nil {
			break // EOF
		}
		filesFound[header.Name] = true
	}

	// Check required files
	assert.True(t, filesFound["metadata.json"], "metadata.json should be in archive")
	assert.True(t, filesFound["manifest.json"], "manifest.json should be in archive")
	// Screenshots directory is created implicitly when files are added, so we check for files instead
	assert.True(t, filesFound["screenshots/screenshot-1.jpg"], "screenshot-1.jpg should be in archive")
	assert.True(t, filesFound["screenshots/screenshot-2.jpg"], "screenshot-2.jpg should be in archive")
	assert.True(t, filesFound["screenshots/screenshot-3.jpg"], "screenshot-3.jpg should be in archive")
	assert.True(t, filesFound["screenshots/screenshot-4.jpg"], "screenshot-4.jpg should be in archive")

	// Verify metadata.json content
	file2, err := os.Open(archivePath)
	require.NoError(t, err)
	defer file2.Close()

	gzipReader2, err := gzip.NewReader(file2)
	require.NoError(t, err)
	defer gzipReader2.Close()

	tarReader2 := tar.NewReader(gzipReader2)

	var metadata DatasetMetadata
	foundMetadata := false
	for {
		header, err := tarReader2.Next()
		if err != nil {
			break
		}
		if header.Name == "metadata.json" {
			// Read full file (may require multiple reads)
			metadataJSON, err := io.ReadAll(tarReader2)
			require.NoError(t, err)
			err = json.Unmarshal(metadataJSON, &metadata)
			require.NoError(t, err)
			foundMetadata = true
			break
		}
	}
	require.True(t, foundMetadata, "metadata.json should be found in archive")

	assert.Equal(t, edgeID, metadata.EdgeID)
	assert.Equal(t, cameraID, metadata.CameraID)
	assert.Equal(t, 4, metadata.TotalSnapshots)
	assert.Equal(t, 2, metadata.LabelCounts["normal"])
	assert.Equal(t, 1, metadata.LabelCounts["threat"])
	assert.Equal(t, 1, metadata.LabelCounts["custom-label-1"])

	// Verify checksum is valid SHA-256 hex string (64 characters)
	assert.Len(t, checksum, 64, "Checksum should be 64-character hex string")
}

// TestPackager_PackageDataset_EmptyList tests packaging with empty screenshot list
func TestPackager_PackageDataset_EmptyList(t *testing.T) {
	log := logger.NewNopLogger()
	packager := NewPackager(log)

	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "exports")

	// Try to package empty list
	_, _, err := packager.PackageDataset("test-edge-1", "test-camera-1", []*screenshots.Screenshot{}, outputDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no screenshots")
}

// TestPackager_PackageDataset_MissingFile tests handling of missing screenshot files
func TestPackager_PackageDataset_MissingFile(t *testing.T) {
	log := logger.NewNopLogger()
	packager := NewPackager(log)

	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "exports")
	os.MkdirAll(outputDir, 0755)

	// Create screenshot with non-existent file path
	screenshotList := []*screenshots.Screenshot{
		{
			ID:          "missing-screenshot",
			CameraID:    "test-camera-1",
			FilePath:    filepath.Join(tmpDir, "non-existent.jpg"),
			Label:       screenshots.LabelNormal,
			Description: "Missing file",
			CreatedAt:   time.Now(),
		},
		createTestScreenshot(t, tmpDir, "valid-screenshot", screenshots.LabelNormal, ""),
	}

	// Package should succeed but skip missing file
	archivePath, checksum, err := packager.PackageDataset("test-edge-1", "test-camera-1", screenshotList, outputDir)
	require.NoError(t, err)
	require.NotEmpty(t, archivePath)
	require.NotEmpty(t, checksum)

	// Verify archive contains only valid screenshot
	file, err := os.Open(archivePath)
	require.NoError(t, err)
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	filesFound := make(map[string]bool)
	for {
		header, err := tarReader.Next()
		if err != nil {
			break
		}
		filesFound[header.Name] = true
	}

	// Should contain valid screenshot but not missing one
	assert.True(t, filesFound["screenshots/valid-screenshot.jpg"])
	assert.False(t, filesFound["screenshots/missing-screenshot.jpg"])
}

// TestPackager_Checksum tests checksum calculation
func TestPackager_Checksum(t *testing.T) {
	log := logger.NewNopLogger()
	packager := NewPackager(log)

	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "exports")
	os.MkdirAll(outputDir, 0755)

	// Create test screenshot
	screenshotList := []*screenshots.Screenshot{
		createTestScreenshot(t, tmpDir, "screenshot-1", screenshots.LabelNormal, ""),
	}

	// Package dataset
	_, checksum, err := packager.PackageDataset("test-edge-1", "test-camera-1", screenshotList, outputDir)
	require.NoError(t, err)

	// Verify checksum format (SHA-256 hex, 64 chars)
	assert.Len(t, checksum, 64, "Checksum should be 64-character hex string")
	assert.Regexp(t, "^[0-9a-f]{64}$", checksum, "Checksum should be valid hex string")

	// Note: Checksums for same content will differ if metadata includes timestamps
	// This is expected behavior - each package operation includes current time in metadata
}
