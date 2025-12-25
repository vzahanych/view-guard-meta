package impl

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	"go.uber.org/zap"
)

// handleListScreenshots handles listing labeled screenshots
func (g *WebGatewayImpl) handleListScreenshots(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	filters := make(map[string]interface{})
	if cameraID := c.Query("camera_id"); cameraID != "" {
		filters["camera_id"] = cameraID
	}
	if label := c.Query("label"); label != "" {
		filters["label"] = label
	}
	// Support filtering unlabeled screenshots (for user to label)
	if unlabeled := c.Query("unlabeled"); unlabeled == "true" {
		filters["label"] = "" // Empty label means unlabeled
		filters["unlabeled_only"] = true
	}
	if customLabel := c.Query("custom_label"); customLabel != "" {
		filters["custom_label"] = customLabel
	}
	if description := c.Query("description"); description != "" {
		filters["description"] = description
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filters["limit"] = limit
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filters["offset"] = offset
		}
	}

	screenshotsList, err := g.metaStorage.ListScreenshots(c.Request.Context(), filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to list screenshots: %v", err),
		})
		return
	}

	// Include thumbnails by default so users can see what they're labeling
	// Can be disabled with include_thumbnails=false
	includeThumbnails := c.Query("include_thumbnails") != "false"
	
	response := make([]gin.H, 0, len(screenshotsList))
	for _, ss := range screenshotsList {
		screenshotData := gin.H{
			"id":           ss.ID,
			"camera_id":    ss.CameraID,
			"object_key":   ss.ObjectKey, // Path to image in object storage
			"thumbnail_key": ss.ThumbnailKey, // Path to thumbnail in object storage
			"label":        ss.Label,
			"custom_label": ss.CustomLabel,
			"description":  ss.Description,
			"created_at":   ss.CreatedAt.Format(time.RFC3339),
			"updated_at":   ss.UpdatedAt.Format(time.RFC3339),
		}
		
		// Include thumbnail as base64 if requested
		if includeThumbnails && g.objectStorage != nil {
			thumbnailKey := ss.ThumbnailKey
			if thumbnailKey == "" {
				thumbnailKey = ss.ObjectKey // Fallback to full image if no thumbnail
			}
			
			if reader, err := g.objectStorage.LoadSnapshot(c.Request.Context(), thumbnailKey); err == nil {
				if thumbnailData, err := io.ReadAll(reader); err == nil {
					reader.Close()
					// Encode thumbnail as base64 for JSON response
					thumbnailBase64 := base64.StdEncoding.EncodeToString(thumbnailData)
					screenshotData["thumbnail"] = fmt.Sprintf("data:image/jpeg;base64,%s", thumbnailBase64)
				}
			}
		}
		
		response = append(response, screenshotData)
	}

	c.JSON(http.StatusOK, gin.H{
		"screenshots": response,
		"count":       len(response),
	})
}

// handleGetScreenshot handles getting a single screenshot
func (g *WebGatewayImpl) handleGetScreenshot(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	id := c.Param("id")
	screenshot, found := g.metaStorage.GetScreenshot(c.Request.Context(), id)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Screenshot not found",
		})
		return
	}

	// Include thumbnail by default so users can see what they're labeling
	// Can be disabled with include_thumbnail=false
	includeThumbnail := c.Query("include_thumbnail") != "false"
	includeFullImage := c.Query("include_full_image") == "true"
	
	response := gin.H{
		"id":            screenshot.ID,
		"camera_id":     screenshot.CameraID,
		"object_key":    screenshot.ObjectKey, // Path to full image in object storage
		"thumbnail_key": screenshot.ThumbnailKey, // Path to thumbnail in object storage
		"label":         screenshot.Label,
		"custom_label":  screenshot.CustomLabel,
		"description":   screenshot.Description,
		"metadata":      screenshot.Metadata,
		"created_at":    screenshot.CreatedAt.Format(time.RFC3339),
		"updated_at":    screenshot.UpdatedAt.Format(time.RFC3339),
	}
	
	// Include thumbnail as base64 if requested
	if includeThumbnail && g.objectStorage != nil {
		thumbnailKey := screenshot.ThumbnailKey
		if thumbnailKey == "" {
			thumbnailKey = screenshot.ObjectKey // Fallback to full image if no thumbnail
		}
		
		if reader, err := g.objectStorage.LoadSnapshot(c.Request.Context(), thumbnailKey); err == nil {
			if thumbnailData, err := io.ReadAll(reader); err == nil {
				reader.Close()
				thumbnailBase64 := base64.StdEncoding.EncodeToString(thumbnailData)
				response["thumbnail"] = fmt.Sprintf("data:image/jpeg;base64,%s", thumbnailBase64)
			}
		}
	}
	
	// Include full image as base64 if requested
	if includeFullImage && g.objectStorage != nil {
		if reader, err := g.objectStorage.LoadSnapshot(c.Request.Context(), screenshot.ObjectKey); err == nil {
			if imageData, err := io.ReadAll(reader); err == nil {
				reader.Close()
				imageBase64 := base64.StdEncoding.EncodeToString(imageData)
				response["image"] = fmt.Sprintf("data:image/jpeg;base64,%s", imageBase64)
			}
		}
	}
	
	c.JSON(http.StatusOK, response)
}

// handleGetScreenshotImage handles getting the image file for a screenshot
func (g *WebGatewayImpl) handleGetScreenshotImage(c *gin.Context) {
	if g.metaStorage == nil || g.objectStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage services not available",
		})
		return
	}

	id := c.Param("id")
	screenshot, found := g.metaStorage.GetScreenshot(c.Request.Context(), id)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Screenshot not found",
		})
		return
	}

	reader, err := g.objectStorage.LoadSnapshot(c.Request.Context(), screenshot.ObjectKey)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Screenshot image not found: %v", err),
		})
		return
	}
	defer reader.Close()

	imageData, err := io.ReadAll(reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to read screenshot image: %v", err),
		})
		return
	}

	c.Data(http.StatusOK, "image/jpeg", imageData)
}

// handleGetScreenshotThumbnail handles getting the thumbnail image for a screenshot
func (g *WebGatewayImpl) handleGetScreenshotThumbnail(c *gin.Context) {
	if g.metaStorage == nil || g.objectStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage services not available",
		})
		return
	}

	id := c.Param("id")
	screenshot, found := g.metaStorage.GetScreenshot(c.Request.Context(), id)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Screenshot not found",
		})
		return
	}

	thumbnailKey := screenshot.ThumbnailKey
	if thumbnailKey == "" {
		thumbnailKey = screenshot.ObjectKey
	}

	reader, err := g.objectStorage.LoadSnapshot(c.Request.Context(), thumbnailKey)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Screenshot thumbnail not found: %v", err),
		})
		return
	}
	defer reader.Close()

	thumbnailData, err := io.ReadAll(reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to read screenshot thumbnail: %v", err),
		})
		return
	}

	c.Data(http.StatusOK, "image/jpeg", thumbnailData)
}

// handleSaveScreenshot handles saving a labeled screenshot
// Uses CCTV service to capture screenshot, objectStorage for image, and metaStorage for metadata
func (g *WebGatewayImpl) handleSaveScreenshot(c *gin.Context) {
	if g.metaStorage == nil || g.objectStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage services not available",
		})
		return
	}

	var req struct {
		CameraID    string                 `json:"camera_id" binding:"required"`
		Label       string                 `json:"label" binding:"required"`
		CustomLabel string                 `json:"custom_label,omitempty"`
		Description string                 `json:"description,omitempty"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
		CreatedBy   string                 `json:"created_by,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		g.logger.Error(
			"Invalid screenshot save request",
			zap.Error(err),
			zap.String("camera_id", req.CameraID),
			zap.String("operation", "create"),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	ctx := c.Request.Context()

	// Screenshots must be captured via CCTV service to ensure provenance
	if g.cctvService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "CCTV service not available for capturing screenshots",
		})
		return
	}

	frame, err := g.cctvService.CaptureFrame(ctx, req.CameraID)
	if err != nil {
		g.logger.Error(
			"Failed to capture frame from camera",
			zap.Error(err),
			zap.String("camera_id", req.CameraID),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to capture screenshot from camera: %v", err),
		})
		return
	}
	imageData := frame.Data

	screenshotID := uuid.New().String()
	objectKey := g.objectStorage.GenerateSnapshotKey(req.CameraID, false)
	thumbnailKey := g.objectStorage.GenerateSnapshotKey(req.CameraID, true)

	if err := g.objectStorage.StoreSnapshot(ctx, objectKey, bytes.NewReader(imageData), int64(len(imageData)), "image/jpeg"); err != nil {
		g.logger.Error(
			"Failed to store screenshot image",
			zap.Error(err),
			zap.String("camera_id", req.CameraID),
			zap.String("screenshot_id", screenshotID),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to store screenshot: %v", err),
		})
		return
	}

	now := time.Now()
	screenshotMeta := metastoragetypes.ScreenshotMetadata{
		ID:           screenshotID,
		CameraID:     req.CameraID,
		ObjectKey:    objectKey,
		ThumbnailKey: thumbnailKey,
		Label:        req.Label,
		CustomLabel:  req.CustomLabel,
		Description:  req.Description,
		Metadata:     req.Metadata,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := g.metaStorage.SaveScreenshot(ctx, screenshotMeta); err != nil {
		_ = g.objectStorage.DeleteSnapshot(ctx, objectKey)
		g.logger.Error(
			"Failed to save screenshot metadata",
			zap.Error(err),
			zap.String("camera_id", req.CameraID),
			zap.String("screenshot_id", screenshotID),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to save screenshot metadata: %v", err),
		})
		return
	}

	var datasetStatusForReply *cctvtypes.DatasetStatus
	if g.cctvService != nil {
		minSnapshots := 50
		datasetStatus, err := g.cctvService.GetDatasetStatus(ctx, req.CameraID, minSnapshots)
		if err == nil && datasetStatus != nil {
			datasetStatusForReply = datasetStatus
		}
	}

	response := gin.H{
		"id":             screenshotID,
		"camera_id":      req.CameraID,
		"object_key":     objectKey,
		"label":          req.Label,
		"custom_label":   req.CustomLabel,
		"description":    req.Description,
		"created_at":     now.Format(time.RFC3339),
		"updated_at":     now.Format(time.RFC3339),
		"dataset_status": nil,
	}

	if datasetStatusForReply != nil {
		var lastSynced interface{}
		if !datasetStatusForReply.LastSynced.IsZero() {
			lastSynced = datasetStatusForReply.LastSynced.Format(time.RFC3339)
		}
		response["dataset_status"] = gin.H{
			"label_counts":            datasetStatusForReply.LabelCounts,
			"labeled_snapshot_count":  datasetStatusForReply.LabeledSnapshotCount,
			"required_snapshot_count": datasetStatusForReply.RequiredSnapshotCount,
			"snapshot_required":       datasetStatusForReply.SnapshotRequired,
			"last_synced":             lastSynced,
		}
	}

	c.JSON(http.StatusCreated, response)
}

// handleUpdateScreenshot handles updating a screenshot's label/metadata (labeling)
func (g *WebGatewayImpl) handleUpdateScreenshot(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	id := c.Param("id")

	var req struct {
		Label       *string                `json:"label,omitempty"`
		CustomLabel *string                `json:"custom_label,omitempty"`
		Description *string                `json:"description,omitempty"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		g.logger.Error(
			"Invalid screenshot update request",
			zap.Error(err),
			zap.String("screenshot_id", id),
			zap.String("operation", "update"),
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	ctx := c.Request.Context()

	updated, err := g.metaStorage.UpdateScreenshot(ctx, id, func(ss metastoragetypes.ScreenshotMetadata) metastoragetypes.ScreenshotMetadata {
		if req.Label != nil {
			ss.Label = *req.Label
		}
		if req.CustomLabel != nil {
			ss.CustomLabel = *req.CustomLabel
		}
		if req.Description != nil {
			ss.Description = *req.Description
		}
		if req.Metadata != nil {
			ss.Metadata = req.Metadata
		}
		ss.UpdatedAt = time.Now()
		return ss
	})

	if err != nil {
		g.logger.Error("Failed to update screenshot",
			zap.Error(err),
			zap.String("screenshot_id", id),
			zap.String("operation", "update"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to update screenshot: %v", err),
		})
		return
	}

	var datasetStatusForReply *cctvtypes.DatasetStatus
	if g.cctvService != nil {
		minSnapshots := 50
		datasetStatus, err := g.cctvService.GetDatasetStatus(ctx, updated.CameraID, minSnapshots)
		if err == nil && datasetStatus != nil {
			datasetStatusForReply = datasetStatus
		}
	}

	response := gin.H{
		"id":             updated.ID,
		"camera_id":      updated.CameraID,
		"object_key":     updated.ObjectKey,
		"label":          updated.Label,
		"custom_label":   updated.CustomLabel,
		"description":    updated.Description,
		"created_at":     updated.CreatedAt.Format(time.RFC3339),
		"updated_at":     updated.UpdatedAt.Format(time.RFC3339),
		"dataset_status": nil,
	}

	if datasetStatusForReply != nil {
		var lastSynced interface{}
		if !datasetStatusForReply.LastSynced.IsZero() {
			lastSynced = datasetStatusForReply.LastSynced.Format(time.RFC3339)
		}
		response["dataset_status"] = gin.H{
			"label_counts":            datasetStatusForReply.LabelCounts,
			"labeled_snapshot_count":  datasetStatusForReply.LabeledSnapshotCount,
			"required_snapshot_count": datasetStatusForReply.RequiredSnapshotCount,
			"snapshot_required":       datasetStatusForReply.SnapshotRequired,
			"last_synced":             lastSynced,
		}
	}

	c.JSON(http.StatusOK, response)
}

// handleDeleteScreenshot handles deleting a screenshot
func (g *WebGatewayImpl) handleDeleteScreenshot(c *gin.Context) {
	if g.metaStorage == nil || g.objectStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage services not available",
		})
		return
	}

	id := c.Param("id")
	ctx := c.Request.Context()

	screenshot, found := g.metaStorage.GetScreenshot(ctx, id)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Screenshot not found",
		})
		return
	}

	if err := g.objectStorage.DeleteSnapshot(ctx, screenshot.ObjectKey); err != nil {
		g.logger.Warn("Failed to delete screenshot image from object storage",
			zap.Error(err),
			zap.String("screenshot_id", id),
			zap.String("object_key", screenshot.ObjectKey),
		)
	}

	if screenshot.ThumbnailKey != "" {
		_ = g.objectStorage.DeleteSnapshot(ctx, screenshot.ThumbnailKey)
	}

	if err := g.metaStorage.DeleteScreenshot(ctx, id); err != nil {
		g.logger.Error(
			"Failed to delete screenshot metadata",
			zap.Error(err),
			zap.String("screenshot_id", id),
			zap.String("camera_id", screenshot.CameraID),
			zap.String("operation", "delete"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to delete screenshot: %v", err),
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// buildDatasetStatusResponse builds a JSON-friendly response object for dataset status.
func buildDatasetStatusResponse(cameraID string, datasetStatus *cctvtypes.DatasetStatus) gin.H {
	var lastSynced interface{}
	if datasetStatus != nil && !datasetStatus.LastSynced.IsZero() {
		lastSynced = datasetStatus.LastSynced.Format(time.RFC3339)
	}

	response := gin.H{
		"camera_id": cameraID,
	}

	if datasetStatus != nil {
		response["dataset_status"] = gin.H{
			"label_counts":            datasetStatus.LabelCounts,
			"labeled_snapshot_count":  datasetStatus.LabeledSnapshotCount,
			"required_snapshot_count": datasetStatus.RequiredSnapshotCount,
			"snapshot_required":       datasetStatus.SnapshotRequired,
			"last_synced":             lastSynced,
		}
	} else {
		response["dataset_status"] = nil
	}

	return response
}

// handleGetDatasetStatus returns dataset status for a single camera without modifying state.
func (g *WebGatewayImpl) handleGetDatasetStatus(c *gin.Context) {
	if g.cctvService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
		})
		return
	}

	cameraID := c.Param("id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Camera ID is required",
		})
		return
	}

	minSnapshots := 50

	datasetStatus, err := g.cctvService.GetDatasetStatus(c.Request.Context(), cameraID, minSnapshots)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to get dataset status: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, buildDatasetStatusResponse(cameraID, datasetStatus))
}

// handleSyncDatasetStatus validates local dataset readiness.
func (g *WebGatewayImpl) handleSyncDatasetStatus(c *gin.Context) {
	if g.cctvService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
		})
		return
	}

	cameraID := c.Param("id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Camera ID is required",
		})
		return
	}

	minSnapshots := 50

	datasetStatus, err := g.cctvService.GetDatasetStatus(c.Request.Context(), cameraID, minSnapshots)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to get dataset status for sync: %v", err),
		})
		return
	}

	if datasetStatus == nil || datasetStatus.SnapshotRequired {
		requiredSnapshots := 0
		if datasetStatus != nil {
			requiredSnapshots = datasetStatus.RequiredSnapshotCount
		}
		c.JSON(http.StatusConflict, gin.H{
			"error":              "Dataset not ready for training (snapshot_required=true)",
			"camera_id":          cameraID,
			"dataset_status":     buildDatasetStatusResponse(cameraID, datasetStatus)["dataset_status"],
			"required_snapshots": requiredSnapshots,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id":      cameraID,
		"dataset_synced": false,
		"dataset_status": buildDatasetStatusResponse(cameraID, datasetStatus)["dataset_status"],
		"message":        "Local dataset is ready; upload/sync to VM is handled by the VM service",
	})
}

// Image validation constants
const (
	maxImageSizeBytes = 10 * 1024 * 1024
	minImageWidth     = 32
	minImageHeight    = 32
	maxImageWidth     = 8192
	maxImageHeight    = 8192
)

// decodeBase64Image decodes and validates a base64-encoded image string
func decodeBase64Image(base64Str string) ([]byte, error) {
	parts := strings.Split(base64Str, ",")
	if len(parts) == 2 {
		base64Str = parts[1]
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 encoding: %w", err)
	}

	if len(decoded) > maxImageSizeBytes {
		return nil, fmt.Errorf("image size %d bytes exceeds maximum allowed size of %d bytes", len(decoded), maxImageSizeBytes)
	}

	if len(decoded) == 0 {
		return nil, fmt.Errorf("image data is empty")
	}

	img, format, err := image.Decode(bytes.NewReader(decoded))
	if err != nil {
		return nil, fmt.Errorf("invalid image format: %w (expected JPEG or PNG)", err)
	}

	if format != "jpeg" && format != "png" {
		return nil, fmt.Errorf("unsupported image format: %s (expected JPEG or PNG)", format)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width < minImageWidth || height < minImageHeight {
		return nil, fmt.Errorf("image dimensions %dx%d are too small (minimum: %dx%d)", width, height, minImageWidth, minImageHeight)
	}

	if width > maxImageWidth || height > maxImageHeight {
		return nil, fmt.Errorf("image dimensions %dx%d are too large (maximum: %dx%d)", width, height, maxImageWidth, maxImageHeight)
	}

	return decoded, nil
}


