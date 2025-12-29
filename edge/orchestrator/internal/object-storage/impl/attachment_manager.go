package impl

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
	"go.uber.org/zap"
)

// AttachmentManager manages attachment optimization and retention.
// It provides functionality to optimize attachments based on storage quota
// and clean up orphaned attachments.
type AttachmentManager struct {
	provider     types.ObjectStorageProvider
	quotaManager *QuotaManager
	logger       *zap.Logger
}

// NewAttachmentManager creates a new AttachmentManager.
func NewAttachmentManager(provider types.ObjectStorageProvider, quotaManager *QuotaManager, logger *zap.Logger) *AttachmentManager {
	return &AttachmentManager{
		provider:     provider,
		quotaManager: quotaManager,
		logger:       logger,
	}
}

// OptimizeAttachment optimizes an attachment based on current storage quota usage.
// Optimization strategies:
//   - When storage >90% full: compress attachments (gzip)
//   - When storage >95% full: reduce image quality, smaller samples
//
// Returns optimized data, whether compression was applied, and any error.
func (a *AttachmentManager) OptimizeAttachment(ctx context.Context, dataType types.DataType, data []byte) ([]byte, bool, error) {
	if a.quotaManager == nil {
		// No quota manager, no optimization
		return data, false, nil
	}

	// Get current quota status
	quota := a.quotaManager.GetCachedQuotaStatus()
	if quota == nil || quota.Limit == 0 {
		// Can't determine quota, no optimization
		return data, false, nil
	}

	usagePercent := int((quota.Used * 100) / quota.Limit)

	// >95%: reduce image quality, compress all attachments
	if usagePercent >= quota.FullThreshold {
		return a.optimizeForFullStorage(dataType, data)
	}

	// 90-95%: compress attachments
	if usagePercent >= 90 {
		return a.compressAttachment(data)
	}

	// <90%: no optimization needed
	return data, false, nil
}

// compressAttachment compresses data using gzip.
func (a *AttachmentManager) compressAttachment(data []byte) ([]byte, bool, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return nil, false, fmt.Errorf("failed to compress data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, false, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	compressed := buf.Bytes()
	// Only use compression if it actually reduces size
	if len(compressed) < len(data) {
		return compressed, true, nil
	}

	// Compression didn't help, return original
	return data, false, nil
}

// optimizeForFullStorage applies aggressive optimization for full storage.
// For images: reduces quality
// For all types: compresses
func (a *AttachmentManager) optimizeForFullStorage(dataType types.DataType, data []byte) ([]byte, bool, error) {
	optimized := data
	wasOptimized := false

	// For images, reduce quality first
	if dataType == types.DataTypeImage {
		reduced, err := a.reduceImageQuality(data)
		if err == nil && len(reduced) < len(data) {
			optimized = reduced
			wasOptimized = true
			if a.logger != nil {
				a.logger.Info("Reduced image quality due to full storage",
					zap.Int("original_size", len(data)),
					zap.Int("reduced_size", len(reduced)))
			}
		}
	}

	// Compress the result (whether original or quality-reduced)
	compressed, wasCompressed, err := a.compressAttachment(optimized)
	if err != nil {
		return optimized, wasOptimized, err
	}

	return compressed, wasOptimized || wasCompressed, nil
}

// reduceImageQuality reduces the quality of an image to save space.
// Supports JPEG and PNG formats.
func (a *AttachmentManager) reduceImageQuality(data []byte) ([]byte, error) {
	// Decode image
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	var buf bytes.Buffer

	// Re-encode with reduced quality
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		// JPEG: reduce quality to 60% (from default 75-95%)
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60}); err != nil {
			return nil, fmt.Errorf("failed to encode JPEG: %w", err)
		}
	case "png":
		// PNG: use default compression (no quality option for PNG)
		// PNG compression is lossless, so we just re-encode with default compression
		encoder := &png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("failed to encode PNG: %w", err)
		}
	default:
		// Unsupported format, return original
		return data, nil
	}

	reduced := buf.Bytes()
	// Only return reduced if it's actually smaller
	if len(reduced) < len(data) {
		return reduced, nil
	}

	// Reduction didn't help, return original
	return data, nil
}

// CleanupOrphanedAttachments cleans up attachments that are not referenced by events.
// This requires integration with meta-storage to check which attachments are referenced.
// For now, this is a placeholder that can be extended when meta-storage integration is available.
func (a *AttachmentManager) CleanupOrphanedAttachments(ctx context.Context, referencedKeys map[string]bool, retentionDays int) (int, error) {
	if a.logger != nil {
		a.logger.Info("Starting orphaned attachment cleanup",
			zap.Int("referenced_count", len(referencedKeys)))
	}

	// List all security event attachments
	prefix := "security-events/"
	objects, err := a.provider.ListObjects(ctx, prefix)
	if err != nil {
		return 0, fmt.Errorf("failed to list security event attachments: %w", err)
	}

	deletedCount := 0
	now := time.Now()
	cutoffTime := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)

	for _, obj := range objects {
		// Check if attachment is referenced
		if referencedKeys != nil && referencedKeys[obj.Key] {
			continue // Attachment is referenced, keep it
		}

		// Check if attachment is old enough to be considered orphaned
		// Get creation time from metadata
		metadata, err := a.provider.GetObjectMetadata(ctx, obj.Key)
		if err == nil && metadata != nil && !metadata.CreatedAt.IsZero() {
			if metadata.CreatedAt.After(cutoffTime) {
				// Too recent, don't delete yet
				continue
			}
		}

		// Delete orphaned attachment
		if err := a.provider.DeleteObject(ctx, obj.Key); err != nil {
			if a.logger != nil {
				a.logger.Warn("Failed to delete orphaned attachment",
					zap.String("key", obj.Key),
					zap.Error(err))
			}
			continue
		}

		deletedCount++
		if a.logger != nil {
			a.logger.Debug("Deleted orphaned attachment",
				zap.String("key", obj.Key))
		}
	}

	if a.logger != nil {
		a.logger.Info("Completed orphaned attachment cleanup",
			zap.Int("deleted_count", deletedCount))
	}

	return deletedCount, nil
}

// GetOptimizationInfo returns information about whether optimization was applied to an object.
// This checks the object metadata for optimization flags.
func (a *AttachmentManager) GetOptimizationInfo(ctx context.Context, key string) (bool, bool, error) {
	metadata, err := a.provider.GetObjectMetadata(ctx, key)
	if err != nil {
		return false, false, fmt.Errorf("failed to get object metadata: %w", err)
	}

	// Check metadata for optimization flags
	// Note: This requires provider to support metadata map extraction
	// For now, we'll check if the content type indicates compression
	// Full implementation requires metadata map support
	compressed := false
	qualityReduced := false

	// Check content-encoding or metadata flags
	// This is a placeholder - full implementation requires metadata map support
	_ = metadata
	_ = compressed
	_ = qualityReduced

	return compressed, qualityReduced, nil
}

