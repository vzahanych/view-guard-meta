package impl

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	statemngtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	"go.uber.org/zap"
)

// handleListEvents handles listing events with filtering and pagination
func (g *WebGatewayImpl) handleListEvents(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	var cameraID, eventType string
	var startTime, endTime time.Time
	limit := 100

	if cameraIDParam := c.Query("camera_id"); cameraIDParam != "" {
		cameraID = cameraIDParam
	}

	if eventTypeParam := c.Query("event_type"); eventTypeParam != "" {
		eventType = eventTypeParam
	}

	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if parsed, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = parsed
		}
	}

	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if parsed, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = parsed
		}
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	filters := make(map[string]interface{})
	if cameraID != "" {
		filters["camera_id"] = cameraID
	}
	if eventType != "" {
		filters["event_type"] = eventType
	}
	if !startTime.IsZero() {
		filters["start_time"] = startTime
	}
	if !endTime.IsZero() {
		filters["end_time"] = endTime
	}
	filters["limit"] = limit

	ctx := c.Request.Context()
	eventMaps, err := g.metaStorage.ListSecurityEvents(ctx, filters)
	if err != nil {
		g.logger.Error("Failed to list events", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to list events",
		})
		return
	}

	events := make([]gin.H, 0, len(eventMaps))
	for _, eventMap := range eventMaps {
		events = append(events, g.securityEventMapToJSON(eventMap))
	}

	c.JSON(http.StatusOK, gin.H{
		"events": events,
		"count":  len(events),
		"total":  len(events),
		"limit":  limit,
		"offset": 0,
	})
}

// securityEventMapToJSON converts a security event map to JSON format
func (g *WebGatewayImpl) securityEventMapToJSON(eventMap map[string]interface{}) gin.H {
	result := gin.H{}
	for k, v := range eventMap {
		result[k] = v
	}
	return result
}

// handleGetEvent handles getting a single event by ID
func (g *WebGatewayImpl) handleGetEvent(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	eventID := c.Param("id")
	ctx := c.Request.Context()

	eventMap, found := g.metaStorage.GetSecurityEvent(ctx, eventID)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Event not found",
		})
		return
	}

	c.JSON(http.StatusOK, g.securityEventMapToJSON(eventMap))
}

// eventToAPIResponse converts a SecurityEvent to API response format
func eventToAPIResponse(event *statemngtypes.SecurityEvent) gin.H {
	response := gin.H{
		"id":            event.ID,
		"camera_id":     event.CameraID,
		"event_type":    event.EventType,
		"timestamp":     event.Timestamp.Format(time.RFC3339),
		"confidence":    event.Confidence,
		"metadata":      event.Metadata,
		"clip_path":     event.ClipPath,
		"snapshot_path": event.SnapshotPath,
	}

	if event.BoundingBox != nil {
		response["bounding_box"] = gin.H{
			"x1":         event.BoundingBox.X1,
			"y1":         event.BoundingBox.Y1,
			"x2":         event.BoundingBox.X2,
			"y2":         event.BoundingBox.Y2,
			"class_id":   event.BoundingBox.ClassID,
			"class_name": event.BoundingBox.ClassName,
			"confidence": event.BoundingBox.Confidence,
		}
	}

	return response
}

// handlePlayClip handles clip playback endpoint
func (g *WebGatewayImpl) handlePlayClip(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "Clip playback is not implemented in the refactored web-gateway",
		"message": "Clips should be served via object storage / VM service",
	})
}

// handleDownloadClip handles clip download endpoint
func (g *WebGatewayImpl) handleDownloadClip(c *gin.Context) {
	_ = fmt.Sprintf // keep fmt imported for potential future use
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "Clip download is not implemented in the refactored web-gateway",
		"message": "Clips should be served via object storage / VM service",
	})
}

// handleGetSnapshot handles snapshot viewing endpoint
func (g *WebGatewayImpl) handleGetSnapshot(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "Snapshot viewing is not implemented in the refactored web-gateway",
		"message": "Snapshots should be served via object storage / VM service",
	})
}

// handleTriggerObstructionEvent manually triggers a camera obstruction security event for testing
func (g *WebGatewayImpl) handleTriggerObstructionEvent(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	cameraID := c.Param("camera_id")
	ctx := c.Request.Context()

	if g.metaStorage != nil {
		_, found := g.metaStorage.GetCamera(c.Request.Context(), cameraID)
		if !found {
			c.JSON(http.StatusNotFound, gin.H{
				"error": fmt.Sprintf("Camera not found: %s", cameraID),
			})
			return
		}
	}

	event := statemngtypes.NewSecurityEvent()
	event.CameraID = cameraID
	event.EventType = statemngtypes.SecurityEventTypeCameraObstructed
	event.Timestamp = time.Now()
	event.Confidence = 1.0
	event.Metadata = map[string]interface{}{
		"severity":         "critical",
		"description":      "Camera view is blocked or obstructed",
		"detection_method": "manual_test",
		"test":             true,
	}

	if g.eventBus != nil {
		g.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventTypeDeviceCaptureFrame,
			Source:    "web-gateway",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"camera_id": cameraID,
				"event_id":  event.ID,
			},
		})
	}

	eventData := map[string]interface{}{
		"id":            event.ID,
		"camera_id":     event.CameraID,
		"event_type":    event.EventType,
		"timestamp":     event.Timestamp,
		"confidence":    event.Confidence,
		"metadata":      event.Metadata,
		"clip_path":     event.ClipPath,
		"snapshot_path": event.SnapshotPath,
	}
	if event.BoundingBox != nil {
		eventData["bounding_box"] = map[string]interface{}{
			"x1":         event.BoundingBox.X1,
			"y1":         event.BoundingBox.Y1,
			"x2":         event.BoundingBox.X2,
			"y2":         event.BoundingBox.Y2,
			"confidence": event.BoundingBox.Confidence,
			"class_id":   event.BoundingBox.ClassID,
			"class_name": event.BoundingBox.ClassName,
		}
	}

	if err := g.metaStorage.SaveSecurityEvent(ctx, event.ID, eventData); err != nil {
		g.logger.Error("Failed to save obstruction security event", zap.Error(err), zap.String("camera_id", cameraID))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to create event: %v", err),
		})
		return
	}

	if g.eventBus != nil {
		g.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("security.event.created"),
			Source:    "web-gateway",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"event_id":  event.ID,
				"camera_id": cameraID,
			},
		})
	}

	g.logger.Info("Camera obstruction security event created", zap.String("camera_id", cameraID), zap.String("event_id", event.ID))

	c.JSON(http.StatusCreated, g.securityEventMapToJSON(eventData))
}
