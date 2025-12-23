package impl

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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

		requests = append(requests, gin.H{
			"camera_id":    cameraID,
			"label":        label,
			"custom_label": customLabel,
			"count":        count,
			"requested_at": requestedAtStr,
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

	c.JSON(http.StatusOK, gin.H{
		"camera_id":    cameraID,
		"label":        label,
		"custom_label": customLabel,
		"count":        count,
		"requested_at": requestedAtStr,
	})
}
