package impl

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"
)

// handleListSnapshotRequests handles listing all pending snapshot requests from VM.
// It now reads directly from meta-storage, decoupling web-gateway from state-mng.
func (g *WebGatewayImpl) handleListSnapshotRequests(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	ctx := c.Request.Context()
	requestDataList, err := g.metaStorage.ListPendingSnapshotRequests(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve pending snapshot requests",
		})
		return
	}

	requests := make([]gin.H, 0, len(requestDataList))
	for _, data := range requestDataList {
		cameraID, _ := data["camera_id"].(string)
		label, _ := data["label"].(string)
		customLabel, _ := data["custom_label"].(string)

		var count int32
		if v, ok := data["count"].(float64); ok {
			count = int32(v)
		} else if v, ok := data["count"].(int32); ok {
			count = v
		}

		var requestedAtStr string
		if t, ok := data["requested_at"].(time.Time); ok {
			requestedAtStr = t.Format(time.RFC3339)
		} else if s, ok := data["requested_at"].(string); ok {
			requestedAtStr = s
		}

		// Count labeled screenshots for this camera to show progress
		var labeledCount int
		if cameraID != "" && g.metaStorage != nil {
			filters := map[string]interface{}{
				"camera_id": cameraID,
			}
			if screenshots, err := g.metaStorage.ListScreenshots(ctx, filters); err == nil {
				for _, ss := range screenshots {
					if ss.Label != "" {
						labeledCount++
					}
				}
			}
		}

		requests = append(requests, gin.H{
			"camera_id":     cameraID,
			"label":         label,
			"custom_label":  customLabel,
			"count":         count, // Minimum required (bare minimum)
			"labeled_count": labeledCount, // Current labeled screenshots
			"requested_at":  requestedAtStr,
			"progress": gin.H{
				"minimum_required": count,
				"current":          labeledCount,
				"ready":            labeledCount >= int(count), // Can mark as ready if minimum met
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"requests": requests,
		"count":    len(requests),
	})
}

// handleGetSnapshotRequest handles getting a pending snapshot request for a specific camera.
// It now reads directly from meta-storage, decoupling web-gateway from state-mng.
func (g *WebGatewayImpl) handleGetSnapshotRequest(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	cameraID := c.Param("camera_id")
	ctx := c.Request.Context()

	data, found := g.metaStorage.GetPendingSnapshotRequest(ctx, cameraID)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "No pending snapshot request found for this camera",
		})
		return
	}

	label, _ := data["label"].(string)
	customLabel, _ := data["custom_label"].(string)

	var count int32
	if v, ok := data["count"].(float64); ok {
		count = int32(v)
	} else if v, ok := data["count"].(int32); ok {
		count = v
	}

	var requestedAtStr string
	if t, ok := data["requested_at"].(time.Time); ok {
		requestedAtStr = t.Format(time.RFC3339)
	} else if s, ok := data["requested_at"].(string); ok {
		requestedAtStr = s
	}

	// Count labeled screenshots for this camera to show progress
	var labeledCount int
	if g.metaStorage != nil {
		filters := map[string]interface{}{
			"camera_id": cameraID,
		}
		if screenshots, err := g.metaStorage.ListScreenshots(ctx, filters); err == nil {
			for _, ss := range screenshots {
				if ss.Label != "" {
					labeledCount++
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id":     cameraID,
		"label":         label,
		"custom_label":  customLabel,
		"count":         count, // Minimum required (bare minimum)
		"labeled_count": labeledCount, // Current labeled screenshots
		"requested_at":  requestedAtStr,
		"progress": gin.H{
			"minimum_required": count,
			"current":          labeledCount,
			"ready":            labeledCount >= int(count), // Can mark as ready if minimum met
		},
	})
}

// handleMarkScreenshotSetReady handles user marking screenshot set as ready for a camera
// This publishes an event to state manager which updates Edge state
func (g *WebGatewayImpl) handleMarkScreenshotSetReady(c *gin.Context) {
	cameraID := c.Param("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Camera ID is required",
		})
		return
	}

	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	if g.eventBus == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Event bus not available",
		})
		return
	}

	ctx := c.Request.Context()

	// Check if there's a pending snapshot request for this camera
	requestData, found := g.metaStorage.GetPendingSnapshotRequest(ctx, cameraID)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "No pending snapshot request found for this camera",
		})
		return
	}

	// Count labeled screenshots for this camera
	filters := map[string]interface{}{
		"camera_id": cameraID,
	}
	screenshots, err := g.metaStorage.ListScreenshots(ctx, filters)
	if err != nil {
		g.logger.Error("Failed to list screenshots",
			zap.String("camera_id", cameraID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to count screenshots: %v", err),
		})
		return
	}

	// Count labeled screenshots (label is not empty)
	labeledCount := 0
	for _, ss := range screenshots {
		if ss.Label != "" {
			labeledCount++
		}
	}

	// Get minimum required count from request
	var minRequired int32
	if v, ok := requestData["count"].(float64); ok {
		minRequired = int32(v)
	} else if v, ok := requestData["count"].(int32); ok {
		minRequired = v
	}

	// Publish event to state manager
	if g.eventBus != nil {
		g.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventTypeDataUnitSetReady,
			Source:    "web-gateway",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"camera_id":     cameraID,
				"labeled_count": labeledCount,
				"min_required":  minRequired,
			},
		})
		g.logger.Info("Screenshot set marked as ready",
			zap.String("camera_id", cameraID),
			zap.Int("labeled_count", labeledCount),
			zap.Int32("min_required", minRequired),
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"camera_id":     cameraID,
		"labeled_count": labeledCount,
		"min_required":  minRequired,
		"message":       "Screenshot set marked as ready. State manager will update Edge status.",
	})
}
