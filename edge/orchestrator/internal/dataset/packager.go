package dataset

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
)

// DatasetMetadata contains metadata about the dataset
type DatasetMetadata struct {
	EdgeID         string         `json:"edge_id"`
	CameraID       string         `json:"camera_id"`
	TotalSnapshots int            `json:"total_snapshots"`
	LabelCounts    map[string]int `json:"label_counts"`
	SyncedAt       time.Time      `json:"synced_at"`
	Checksum       string         `json:"checksum,omitempty"` // Set after archive creation
}

// ManifestEntry represents a single screenshot in the manifest
type ManifestEntry struct {
	ScreenshotID string    `json:"screenshot_id"`
	Label        string    `json:"label"`
	CustomLabel  string    `json:"custom_label,omitempty"`
	Description  string    `json:"description,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	FilePath     string    `json:"file_path"` // Relative path in archive
}

// Manifest contains the list of all screenshots in the dataset
type Manifest struct {
	CameraID    string          `json:"camera_id"`
	SyncedAt    time.Time       `json:"synced_at"`
	Screenshots []ManifestEntry `json:"screenshots"`
}

// Packager packages screenshots into a dataset archive
type Packager struct {
	logger *logger.Logger
}

// NewPackager creates a new dataset packager
func NewPackager(log *logger.Logger) *Packager {
	return &Packager{
		logger: log,
	}
}

// PackageDataset packages all screenshots for a camera into a tar.gz archive
// Returns the path to the created archive and its SHA-256 checksum
func (p *Packager) PackageDataset(
	edgeID string,
	cameraID string,
	screenshotList []*screenshots.Screenshot,
	outputDir string,
) (archivePath string, checksum string, err error) {
	if len(screenshotList) == 0 {
		return "", "", fmt.Errorf("no screenshots to package for camera %s", cameraID)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate archive filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	archiveFilename := fmt.Sprintf("dataset_%s_%s_%s.tar.gz", edgeID, cameraID, timestamp)
	archivePath = filepath.Join(outputDir, archiveFilename)

	// Create tar.gz archive
	file, err := os.Create(archivePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create archive file: %w", err)
	}
	defer file.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Calculate label counts
	labelCounts := make(map[string]int)
	for _, ss := range screenshotList {
		label := string(ss.Label)
		if ss.CustomLabel != "" {
			label = ss.CustomLabel
		}
		labelCounts[label]++
	}

	// Create manifest entries
	manifestEntries := make([]ManifestEntry, 0, len(screenshotList))
	syncedAt := time.Now()

	// Add metadata.json
	metadata := DatasetMetadata{
		EdgeID:         edgeID,
		CameraID:       cameraID,
		TotalSnapshots: len(screenshotList),
		LabelCounts:    labelCounts,
		SyncedAt:       syncedAt,
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write metadata.json to archive
	if err := p.writeFileToTar(tarWriter, "metadata.json", metadataJSON); err != nil {
		return "", "", fmt.Errorf("failed to write metadata.json: %w", err)
	}

	// Add all screenshot files to archive
	for _, ss := range screenshotList {
		// Read screenshot file
		screenshotData, err := os.ReadFile(ss.FilePath)
		if err != nil {
			p.logger.Warn("Failed to read screenshot file, skipping", "error", err, "screenshot_id", ss.ID, "file_path", ss.FilePath)
			continue
		}

		// Create relative path in archive
		archivePath := filepath.Join("screenshots", fmt.Sprintf("%s.jpg", ss.ID))

		// Write screenshot to archive
		if err := p.writeFileToTar(tarWriter, archivePath, screenshotData); err != nil {
			p.logger.Warn("Failed to write screenshot to archive, skipping", "error", err, "screenshot_id", ss.ID)
			continue
		}

		// Add to manifest
		manifestEntries = append(manifestEntries, ManifestEntry{
			ScreenshotID: ss.ID,
			Label:        string(ss.Label),
			CustomLabel:  ss.CustomLabel,
			Description:  ss.Description,
			CreatedAt:    ss.CreatedAt,
			FilePath:     archivePath,
		})
	}

	// Create and write manifest.json
	manifest := Manifest{
		CameraID:    cameraID,
		SyncedAt:    syncedAt,
		Screenshots: manifestEntries,
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := p.writeFileToTar(tarWriter, "manifest.json", manifestJSON); err != nil {
		return "", "", fmt.Errorf("failed to write manifest.json: %w", err)
	}

	// Close tar writer to flush data
	if err := tarWriter.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Close gzip writer to flush data
	if err := gzipWriter.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Close file
	if err := file.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close archive file: %w", err)
	}

	// Calculate checksum from the archive file
	checksumBytes, err := p.calculateFileChecksum(archivePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	checksum = fmt.Sprintf("%x", checksumBytes)

	// Update metadata with checksum and write it again (optional, for reference)
	// For now, we'll just log it
	p.logger.Info("Dataset packaged successfully",
		"archive_path", archivePath,
		"camera_id", cameraID,
		"total_snapshots", len(screenshotList),
		"checksum", checksum,
		"size_bytes", p.getFileSize(archivePath),
	)

	return archivePath, checksum, nil
}

// writeFileToTar writes a file to the tar archive
func (p *Packager) writeFileToTar(tarWriter *tar.Writer, path string, data []byte) error {
	header := &tar.Header{
		Name:    path,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: time.Now(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	if _, err := tarWriter.Write(data); err != nil {
		return fmt.Errorf("failed to write file data: %w", err)
	}

	return nil
}

// calculateFileChecksum calculates SHA-256 checksum of a file
func (p *Packager) calculateFileChecksum(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}

	return hasher.Sum(nil), nil
}

// getFileSize returns the size of a file in bytes
func (p *Packager) getFileSize(filePath string) int64 {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0
	}
	return info.Size()
}
