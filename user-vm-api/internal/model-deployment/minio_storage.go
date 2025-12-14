package modeldeployment

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
	storagesync "github.com/vzahanych/view-guard-meta/user-vm-api/internal/storage-sync"
	"go.uber.org/zap"
)

// MinIOModelStorage handles archiving trained models to MinIO
type MinIOModelStorage struct {
	s3Client     *storagesync.S3Client
	modelStorage *storage.ModelStorage
	logger       *logging.Logger
	bucketName   string
}

// NewMinIOModelStorage creates a new MinIO model storage service
func NewMinIOModelStorage(
	s3Client *storagesync.S3Client,
	modelStorage *storage.ModelStorage,
	logger *logging.Logger,
) (*MinIOModelStorage, error) {
	if s3Client == nil {
		return nil, fmt.Errorf("S3 client is required")
	}
	if modelStorage == nil {
		return nil, fmt.Errorf("model storage is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &MinIOModelStorage{
		s3Client:     s3Client,
		modelStorage: modelStorage,
		logger:       logger,
	}, nil
}

// ArchiveModel archives a trained model to MinIO
// Uploads both model file and metadata to MinIO for long-term persistence
func (m *MinIOModelStorage) ArchiveModel(ctx context.Context, modelID string) error {
	if modelID == "" {
		return fmt.Errorf("model ID is required")
	}

	// Get model file path from model storage
	modelPath := m.modelStorage.GetModelFilePath(modelID)
	if !m.modelStorage.ModelExists(modelID) {
		return fmt.Errorf("model file not found: %s", modelPath)
	}

	// Upload model file to MinIO
	modelObjectKey := fmt.Sprintf("%s/model.onnx", modelID)
	modelFile, err := os.Open(modelPath)
	if err != nil {
		return fmt.Errorf("failed to open model file: %w", err)
	}
	defer modelFile.Close()

	fileInfo, err := modelFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	err = m.s3Client.UploadFile(ctx, modelObjectKey, modelFile, fileInfo.Size(), "application/octet-stream")
	if err != nil {
		m.logger.Warn("Failed to archive model file to MinIO",
			zap.String("model_id", modelID),
			zap.String("object_key", modelObjectKey),
			zap.Error(err),
		)
		return fmt.Errorf("failed to upload model file to MinIO: %w", err)
	}

	m.logger.Info("Archived model file to MinIO",
		zap.String("model_id", modelID),
		zap.String("object_key", modelObjectKey),
		zap.Int64("size", fileInfo.Size()),
	)

	// Upload metadata to MinIO
	metadataObjectKey := fmt.Sprintf("%s/metadata.json", modelID)
	metadataPath := filepath.Join(filepath.Dir(modelPath), "metadata.json")

	// Check if metadata file exists
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		m.logger.Warn("Metadata file not found, skipping metadata archive",
			zap.String("model_id", modelID),
			zap.String("metadata_path", metadataPath),
		)
		// Continue without metadata - model file is the critical part
	} else {
		metadataFile, err := os.Open(metadataPath)
		if err != nil {
			m.logger.Warn("Failed to open metadata file for MinIO archive",
				zap.String("model_id", modelID),
				zap.String("metadata_path", metadataPath),
				zap.Error(err),
			)
			// Continue without metadata - model file is archived
		} else {
			defer metadataFile.Close()

			metadataInfo, err := metadataFile.Stat()
			if err != nil {
				m.logger.Warn("Failed to get metadata file info",
					zap.String("model_id", modelID),
					zap.Error(err),
				)
			} else {
				err = m.s3Client.UploadFile(ctx, metadataObjectKey, metadataFile, metadataInfo.Size(), "application/json")
				if err != nil {
					m.logger.Warn("Failed to archive metadata to MinIO",
						zap.String("model_id", modelID),
						zap.String("object_key", metadataObjectKey),
						zap.Error(err),
					)
					// Continue - model file is archived, metadata is optional
				} else {
					m.logger.Info("Archived model metadata to MinIO",
						zap.String("model_id", modelID),
						zap.String("object_key", metadataObjectKey),
					)
				}
			}
		}
	}

	return nil
}

// GetModelFromMinIO retrieves a model file from MinIO
// Returns a ReadCloser that should be closed by the caller
func (m *MinIOModelStorage) GetModelFromMinIO(ctx context.Context, modelID string) (io.ReadCloser, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model ID is required")
	}

	modelObjectKey := fmt.Sprintf("%s/model.onnx", modelID)

	reader, err := m.s3Client.DownloadFile(ctx, modelObjectKey)
	if err != nil {
		return nil, fmt.Errorf("failed to download model from MinIO: %w", err)
	}

	return reader, nil
}

// ModelExistsInMinIO checks if a model exists in MinIO
func (m *MinIOModelStorage) ModelExistsInMinIO(ctx context.Context, modelID string) (bool, error) {
	if modelID == "" {
		return false, fmt.Errorf("model ID is required")
	}

	modelObjectKey := fmt.Sprintf("%s/model.onnx", modelID)
	return m.s3Client.FileExists(ctx, modelObjectKey)
}

// GetModelSizeFromMinIO gets the size of a model file in MinIO
func (m *MinIOModelStorage) GetModelSizeFromMinIO(ctx context.Context, modelID string) (int64, error) {
	if modelID == "" {
		return 0, fmt.Errorf("model ID is required")
	}

	modelObjectKey := fmt.Sprintf("%s/model.onnx", modelID)
	return m.s3Client.GetFileSize(ctx, modelObjectKey)
}
