package web

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// handleHealth handles the health check endpoint
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "web-server",
	})
}

// handleStatus handles the system status endpoint
func (s *Server) handleStatus(c *gin.Context) {
	uptime := time.Since(s.startTime)

	// Determine overall health status
	health := "healthy"
	if s.GetStatus().GetStatus() != service.StatusRunning {
		health = "unhealthy"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         health,
		"uptime":         uptime.String(),
		"uptime_seconds": int64(uptime.Seconds()),
		"version":        s.version,
		"timestamp":      time.Now().Format(time.RFC3339),
	})
}

// handleListCameras handles listing all cameras
func (s *Server) handleListCameras(c *gin.Context) {
	if s.cameraMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Camera manager not available",
		})
		return
	}

	// Parse query parameter for enabled only
	enabledOnly := c.Query("enabled") == "true"

	cameras := s.cameraMgr.ListCameras(enabledOnly)

	// Convert to API response format
	response := make([]gin.H, 0, len(cameras))
	for _, cam := range cameras {
		response = append(response, s.cameraToJSON(cam))
	}

	c.JSON(http.StatusOK, gin.H{
		"cameras": response,
		"count":   len(response),
	})
}

// handleGetCamera handles getting a single camera by ID
func (s *Server) handleGetCamera(c *gin.Context) {
	if s.cameraMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Camera manager not available",
		})
		return
	}

	cameraID := c.Param("id")
	camera, err := s.cameraMgr.GetCamera(cameraID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, s.cameraToJSON(camera))
}

// handleAddCamera handles adding a new camera
func (s *Server) handleAddCamera(c *gin.Context) {
	if s.cameraMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Camera manager not available",
		})
		return
	}

	var req struct {
		ID            string   `json:"id" binding:"required"`
		Name          string   `json:"name" binding:"required"`
		Type          string   `json:"type" binding:"required,oneof=rtsp onvif usb"`
		RTSPURLs      []string `json:"rtsp_urls,omitempty"`
		DevicePath    string   `json:"device_path,omitempty"`
		IPAddress     string   `json:"ip_address,omitempty"`
		ONVIFEndpoint string   `json:"onvif_endpoint,omitempty"`
		Manufacturer  string   `json:"manufacturer,omitempty"`
		Model         string   `json:"model,omitempty"`
		Enabled       bool     `json:"enabled"`
		Config        struct {
			RecordingEnabled bool   `json:"recording_enabled"`
			MotionDetection  bool   `json:"motion_detection"`
			Quality          string `json:"quality,omitempty"`
			FrameRate        int    `json:"frame_rate,omitempty"`
			Resolution       string `json:"resolution,omitempty"`
		} `json:"config,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}

	// Validate camera type specific fields
	if req.Type == "rtsp" && len(req.RTSPURLs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "RTSP cameras require rtsp_urls",
		})
		return
	}
	if req.Type == "usb" && req.DevicePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "USB cameras require device_path",
		})
		return
	}

	// Create camera object
	camera := &camera.Camera{
		ID:            req.ID,
		Name:          req.Name,
		Type:          camera.CameraType(req.Type),
		Manufacturer:  req.Manufacturer,
		Model:         req.Model,
		Enabled:       req.Enabled,
		Status:        camera.CameraStatusOffline,
		IPAddress:     req.IPAddress,
		ONVIFEndpoint: req.ONVIFEndpoint,
		RTSPURLs:      req.RTSPURLs,
		DevicePath:    req.DevicePath,
		Config: camera.CameraConfig{
			RecordingEnabled: req.Config.RecordingEnabled,
			MotionDetection:  req.Config.MotionDetection,
			Quality:          req.Config.Quality,
			FrameRate:        req.Config.FrameRate,
			Resolution:       req.Config.Resolution,
		},
	}

	if err := s.cameraMgr.AddCamera(c.Request.Context(), camera); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, s.cameraToJSON(camera))
}

// handleUpdateCamera handles updating an existing camera
func (s *Server) handleUpdateCamera(c *gin.Context) {
	if s.cameraMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Camera manager not available",
		})
		return
	}

	cameraID := c.Param("id")

	var req struct {
		Name       string   `json:"name,omitempty"`
		RTSPURLs   []string `json:"rtsp_urls,omitempty"`
		DevicePath string   `json:"device_path,omitempty"`
		Enabled    *bool    `json:"enabled,omitempty"`
		Config     struct {
			RecordingEnabled *bool  `json:"recording_enabled,omitempty"`
			MotionDetection  *bool  `json:"motion_detection,omitempty"`
			Quality          string `json:"quality,omitempty"`
			FrameRate        int    `json:"frame_rate,omitempty"`
			Resolution       string `json:"resolution,omitempty"`
		} `json:"config,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}

	// Get existing camera
	existing, err := s.cameraMgr.GetCamera(cameraID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Create update object
	updates := &camera.Camera{
		ID:         cameraID,
		Name:       req.Name,
		Enabled:    existing.Enabled,
		RTSPURLs:   existing.RTSPURLs,
		DevicePath: existing.DevicePath,
		Config:     existing.Config,
	}

	if req.Name != "" {
		updates.Name = req.Name
	}
	if req.Enabled != nil {
		updates.Enabled = *req.Enabled
	}
	if len(req.RTSPURLs) > 0 {
		updates.RTSPURLs = req.RTSPURLs
	}
	if req.DevicePath != "" {
		updates.DevicePath = req.DevicePath
	}
	if req.Config.RecordingEnabled != nil {
		updates.Config.RecordingEnabled = *req.Config.RecordingEnabled
	}
	if req.Config.MotionDetection != nil {
		updates.Config.MotionDetection = *req.Config.MotionDetection
	}
	if req.Config.Quality != "" {
		updates.Config.Quality = req.Config.Quality
	}
	if req.Config.FrameRate > 0 {
		updates.Config.FrameRate = req.Config.FrameRate
	}
	if req.Config.Resolution != "" {
		updates.Config.Resolution = req.Config.Resolution
	}

	if err := s.cameraMgr.UpdateCamera(c.Request.Context(), cameraID, updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Get updated camera
	updated, _ := s.cameraMgr.GetCamera(cameraID)
	c.JSON(http.StatusOK, s.cameraToJSON(updated))
}

// handleDeleteCamera handles deleting a camera
func (s *Server) handleDeleteCamera(c *gin.Context) {
	if s.cameraMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Camera manager not available",
		})
		return
	}

	cameraID := c.Param("id")
	if err := s.cameraMgr.DeleteCamera(c.Request.Context(), cameraID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Camera deleted",
		"id":      cameraID,
	})
}

// handleDiscoverCameras handles camera discovery
func (s *Server) handleDiscoverCameras(c *gin.Context) {
	if s.cameraMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Camera manager not available",
		})
		return
	}

	// Trigger discovery
	s.cameraMgr.TriggerDiscovery()

	// Get discovered cameras
	discovered := s.cameraMgr.GetDiscoveredCameras()

	// Convert to JSON format
	response := make([]gin.H, 0, len(discovered))
	for _, cam := range discovered {
		var lastSeen interface{}
		if !cam.LastSeen.IsZero() {
			lastSeen = cam.LastSeen.Format(time.RFC3339)
		}
		response = append(response, gin.H{
			"id":             cam.ID,
			"manufacturer":   cam.Manufacturer,
			"model":          cam.Model,
			"ip_address":     cam.IPAddress,
			"onvif_endpoint": cam.ONVIFEndpoint,
			"rtsp_urls":      cam.RTSPURLs,
			"last_seen":      lastSeen,
			"discovered_at":  cam.DiscoveredAt.Format(time.RFC3339),
			"capabilities": gin.H{
				"has_ptz":           cam.Capabilities.HasPTZ,
				"has_snapshot":      cam.Capabilities.HasSnapshot,
				"has_video_streams": cam.Capabilities.HasVideoStreams,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"discovered": response,
		"count":      len(response),
	})
}

// handleTestCamera handles testing camera connection
func (s *Server) handleTestCamera(c *gin.Context) {
	if s.cameraMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Camera manager not available",
		})
		return
	}

	cameraID := c.Param("id")
	cam, err := s.cameraMgr.GetCamera(cameraID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Test connection based on camera type
	var success bool
	var message string

	switch cam.Type {
	case camera.CameraTypeRTSP, camera.CameraTypeONVIF:
		// For RTSP/ONVIF, we could try to connect briefly
		// For now, just check if camera is online
		status, _ := s.cameraMgr.GetCameraStatus(cameraID)
		success = status == camera.CameraStatusOnline
		if success {
			message = "Camera is online and connected"
		} else {
			message = "Camera is offline or not connected"
		}
	case camera.CameraTypeUSB:
		// For USB, check if device path exists
		if cam.DevicePath != "" {
			// Check if device exists (simplified - would need actual file check)
			status, _ := s.cameraMgr.GetCameraStatus(cameraID)
			success = status == camera.CameraStatusOnline
			if success {
				message = "USB camera device is accessible"
			} else {
				message = "USB camera device is not accessible"
			}
		} else {
			success = false
			message = "USB camera device path not configured"
		}
	default:
		success = false
		message = "Unknown camera type"
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   success,
		"message":   message,
		"camera_id": cameraID,
		"status":    cam.Status,
	})
}

// cameraToJSON converts a Camera to JSON format
func (s *Server) cameraToJSON(cam *camera.Camera) gin.H {
	var lastSeen interface{}
	if cam.LastSeen != nil {
		lastSeen = cam.LastSeen.Format(time.RFC3339)
	}

	var datasetStatus interface{}
	if cam.DatasetStatus != nil {
		var lastSynced interface{}
		if !cam.DatasetStatus.LastSynced.IsZero() {
			lastSynced = cam.DatasetStatus.LastSynced.Format(time.RFC3339)
		}

		// Convert label counts map to JSON-compatible format
		labelCountsMap := make(map[string]int)
		for label, count := range cam.DatasetStatus.LabelCounts {
			labelCountsMap[label] = count
		}

		datasetStatus = gin.H{
			"label_counts":            labelCountsMap,
			"labeled_snapshot_count":  cam.DatasetStatus.LabeledSnapshotCount,
			"required_snapshot_count": cam.DatasetStatus.RequiredSnapshotCount,
			"snapshot_required":       cam.DatasetStatus.SnapshotRequired,
			"last_synced":             lastSynced,
		}
	}

	return gin.H{
		"id":             cam.ID,
		"name":           cam.Name,
		"type":           string(cam.Type),
		"manufacturer":   cam.Manufacturer,
		"model":          cam.Model,
		"enabled":        cam.Enabled,
		"status":         string(cam.Status),
		"last_seen":      lastSeen,
		"discovered_at":  cam.DiscoveredAt.Format(time.RFC3339),
		"ip_address":     cam.IPAddress,
		"onvif_endpoint": cam.ONVIFEndpoint,
		"rtsp_urls":      cam.RTSPURLs,
		"device_path":    cam.DevicePath,
		"config": gin.H{
			"recording_enabled": cam.Config.RecordingEnabled,
			"motion_detection":  cam.Config.MotionDetection,
			"quality":           cam.Config.Quality,
			"frame_rate":        cam.Config.FrameRate,
			"resolution":        cam.Config.Resolution,
		},
		"capabilities": gin.H{
			"has_ptz":           cam.Capabilities.HasPTZ,
			"has_snapshot":      cam.Capabilities.HasSnapshot,
			"has_video_streams": cam.Capabilities.HasVideoStreams,
		},
		"dataset_status": datasetStatus,
	}
}

// handleListEvents handles listing events with filtering and pagination
func (s *Server) handleListEvents(c *gin.Context) {
	if s.stateMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "State manager not available",
		})
		return
	}

	// Parse query parameters
	opts := state.ListEventsOptions{}

	// Camera filter
	if cameraID := c.Query("camera_id"); cameraID != "" {
		opts.CameraID = cameraID
	}

	// Event type filter
	if eventType := c.Query("event_type"); eventType != "" {
		opts.EventType = eventType
	}

	// Date range filters
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if startTime, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			opts.StartTime = startTime
		}
	}

	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if endTime, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			opts.EndTime = endTime
		}
	}

	// Pagination
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			opts.Limit = limit
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			opts.Offset = offset
		}
	}

	// Order by
	if orderBy := c.Query("order_by"); orderBy != "" {
		opts.OrderBy = orderBy
	}

	// Query events
	ctx := c.Request.Context()
	eventStates, totalCount, err := s.stateMgr.ListEvents(ctx, opts)
	if err != nil {
		s.logger.Error("Failed to list events", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to list events",
		})
		return
	}

	// Convert EventState to Event and then to API response
	apiEvents := make([]gin.H, 0, len(eventStates))
	for _, es := range eventStates {
		event := events.FromEventState(es)
		apiEvents = append(apiEvents, eventToAPIResponse(event))
	}

	c.JSON(http.StatusOK, gin.H{
		"events": apiEvents,
		"count":  len(apiEvents),
		"total":  totalCount,
		"limit":  opts.Limit,
		"offset": opts.Offset,
	})
}

// handleGetEvent handles getting a single event by ID
func (s *Server) handleGetEvent(c *gin.Context) {
	if s.stateMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "State manager not available",
		})
		return
	}

	eventID := c.Param("id")
	ctx := c.Request.Context()

	eventState, err := s.stateMgr.GetEventByID(ctx, eventID)
	if err != nil {
		s.logger.Error("Failed to get event", err, "event_id", eventID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get event",
		})
		return
	}

	if eventState == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Event not found",
		})
		return
	}

	event := events.FromEventState(*eventState)
	c.JSON(http.StatusOK, eventToAPIResponse(event))
}

// eventToAPIResponse converts an Event to API response format
func eventToAPIResponse(event *events.Event) gin.H {
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

	// Add bounding box if present
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
func (s *Server) handlePlayClip(c *gin.Context) {
	if s.stateMgr == nil || s.storageSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage service not available",
		})
		return
	}

	eventID := c.Param("id")
	ctx := c.Request.Context()

	// Get event to find clip path
	eventState, err := s.stateMgr.GetEventByID(ctx, eventID)
	if err != nil || eventState == nil || eventState.ClipPath == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Clip not found",
		})
		return
	}

	// Resolve clip path (may be relative to clips directory)
	clipPath := eventState.ClipPath
	if !filepath.IsAbs(clipPath) {
		// If path doesn't start with /, it's relative to clips directory
		if !filepath.HasPrefix(clipPath, string(filepath.Separator)) {
			clipPath = filepath.Join(s.storageSvc.GetClipsDir(), clipPath)
		} else {
			// Path starts with / but isn't absolute, treat as relative to clips dir
			clipPath = filepath.Join(s.storageSvc.GetClipsDir(), clipPath[1:])
		}
	}

	// Check if file exists
	if _, err := os.Stat(clipPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Clip file not found",
		})
		return
	}

	// Serve video file
	c.File(clipPath)
}

// handleDownloadClip handles clip download endpoint
func (s *Server) handleDownloadClip(c *gin.Context) {
	if s.stateMgr == nil || s.storageSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage service not available",
		})
		return
	}

	eventID := c.Param("id")
	ctx := c.Request.Context()

	// Get event to find clip path
	eventState, err := s.stateMgr.GetEventByID(ctx, eventID)
	if err != nil || eventState == nil || eventState.ClipPath == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Clip not found",
		})
		return
	}

	// Resolve clip path
	clipPath := eventState.ClipPath
	if !filepath.IsAbs(clipPath) {
		clipPath = filepath.Join(s.storageSvc.GetClipsDir(), clipPath)
	}

	// Check if file exists
	if _, err := os.Stat(clipPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Clip file not found",
		})
		return
	}

	// Set download headers
	filename := filepath.Base(clipPath)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "video/mp4")

	// Serve file
	c.File(clipPath)
}

// handleGetSnapshot handles snapshot viewing endpoint
func (s *Server) handleGetSnapshot(c *gin.Context) {
	if s.stateMgr == nil || s.storageSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Storage service not available",
		})
		return
	}

	eventID := c.Param("id")
	ctx := c.Request.Context()

	// Get event to find snapshot path
	eventState, err := s.stateMgr.GetEventByID(ctx, eventID)
	if err != nil || eventState == nil || eventState.SnapshotPath == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Snapshot not found",
		})
		return
	}

	// Resolve snapshot path
	snapshotPath := eventState.SnapshotPath
	if !filepath.IsAbs(snapshotPath) {
		// If path doesn't start with /, it's relative to snapshots directory
		if !filepath.HasPrefix(snapshotPath, string(filepath.Separator)) {
			snapshotPath = filepath.Join(s.storageSvc.GetSnapshotsDir(), snapshotPath)
		} else {
			// Path starts with / but isn't absolute, treat as relative to snapshots dir
			snapshotPath = filepath.Join(s.storageSvc.GetSnapshotsDir(), snapshotPath[1:])
		}
	}

	// Check if file exists
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Snapshot file not found",
		})
		return
	}

	// Serve image file
	c.File(snapshotPath)
}

// handleTriggerObstructionEvent manually triggers a camera obstruction event for testing
func (s *Server) handleTriggerObstructionEvent(c *gin.Context) {
	if s.stateMgr == nil || s.eventStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Event storage not available",
		})
		return
	}

	cameraID := c.Param("camera_id")
	ctx := c.Request.Context()

	// Verify camera exists
	if s.cameraMgr != nil {
		_, err := s.cameraMgr.GetCamera(cameraID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": fmt.Sprintf("Camera not found: %s", cameraID),
			})
			return
		}
	}

	// Create obstruction event
	event := events.NewEvent()
	event.CameraID = cameraID
	event.EventType = events.EventTypeCameraObstructed
	event.Timestamp = time.Now()
	event.Confidence = 1.0 // Critical event - high confidence
	event.Metadata = map[string]interface{}{
		"severity":         "critical",
		"description":      "Camera view is blocked or obstructed",
		"detection_method": "manual_test",
		"test":             true,
	}

	// Capture snapshot for the event
	if s.streamingSvc != nil {
		frameData, err := s.streamingSvc.GetFrame(cameraID)
		if err == nil && len(frameData) > 0 {
			// Save snapshot
			snapshotPath := fmt.Sprintf("obstruction-%s-%d.jpg", cameraID, time.Now().Unix())
			if s.storageSvc != nil {
				snapshotsDir := s.storageSvc.GetSnapshotsDir()
				fullPath := filepath.Join(snapshotsDir, snapshotPath)
				if err := os.WriteFile(fullPath, frameData, 0644); err == nil {
					event.SnapshotPath = snapshotPath
					event.Metadata["snapshot_path"] = snapshotPath
				}
			}
		}
	}

	// Save event
	if err := s.eventStorage.SaveEvent(ctx, event); err != nil {
		s.logger.Error("Failed to save obstruction event", err, "camera_id", cameraID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to create event: %v", err),
		})
		return
	}

	// Also enqueue if queue is available
	if s.eventQueue != nil {
		_ = s.eventQueue.Enqueue(ctx, event, 10) // High priority for critical events
	}

	s.logger.Info("Camera obstruction event created", "camera_id", cameraID, "event_id", event.ID)

	c.JSON(http.StatusCreated, eventToAPIResponse(event))
}

// handleCameraSnapshot handles camera snapshot capture endpoint
func (s *Server) handleCameraSnapshot(c *gin.Context) {
	if s.streamingSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Streaming service not available",
		})
		return
	}

	cameraID := c.Param("id")

	// Capture snapshot using streaming service
	frameData, err := s.streamingSvc.GetFrame(cameraID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to capture snapshot: %v", err),
		})
		return
	}

	// Set content type and serve JPEG data
	c.Data(http.StatusOK, "image/jpeg", frameData)
}

// handleGetConfig handles getting configuration
func (s *Server) handleGetConfig(c *gin.Context) {
	if s.configSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Configuration service not available",
		})
		return
	}

	// Get section parameter (optional)
	section := c.Query("section")

	// Get current configuration
	cfg := s.configSvc.Get()

	// Return full config or specific section
	if section != "" {
		sectionConfig := s.getConfigSection(cfg, section)
		if sectionConfig == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Invalid section: %s", section),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"section": section,
			"config":  sectionConfig,
		})
	} else {
		// Return full config (sanitize sensitive fields)
		sanitizedConfig := s.sanitizeConfig(cfg)
		c.JSON(http.StatusOK, gin.H{
			"config": sanitizedConfig,
		})
	}
}

// handleUpdateConfig handles updating configuration
func (s *Server) handleUpdateConfig(c *gin.Context) {
	if s.configSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Configuration service not available",
		})
		return
	}

	// Get section parameter (optional)
	section := c.Query("section")

	// Get current configuration
	currentCfg := s.configSvc.Get()

	// Create a copy for modification
	newCfg := *currentCfg

	if section != "" {
		// Partial update: update only the specified section
		var sectionData map[string]interface{}
		if err := c.ShouldBindJSON(&sectionData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Invalid request body: %v", err),
			})
			return
		}

		// Update the section
		if err := s.updateConfigSection(&newCfg, section, sectionData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Failed to update section: %v", err),
			})
			return
		}
	} else {
		// Full update: replace entire configuration
		var updateData map[string]interface{}
		if err := c.ShouldBindJSON(&updateData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Invalid request body: %v", err),
			})
			return
		}

		// Merge update data into current config
		if err := s.mergeConfig(&newCfg, updateData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Failed to merge configuration: %v", err),
			})
			return
		}
	}

	// Update configuration (validates and saves)
	ctx := c.Request.Context()
	if err := s.configSvc.Update(ctx, &newCfg); err != nil {
		s.logger.Error("Failed to update configuration", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Failed to update configuration: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "updated",
		"message": "Configuration updated successfully",
	})
}

// getConfigSection returns a specific configuration section
func (s *Server) getConfigSection(cfg *config.Config, section string) interface{} {
	switch strings.ToLower(section) {
	case "cameras":
		return cfg.Edge.Cameras
	case "ai":
		return cfg.Edge.AI
	case "storage":
		return cfg.Edge.Storage
	case "wireguard":
		return cfg.Edge.WireGuard
	case "telemetry":
		return cfg.Edge.Telemetry
	case "encryption":
		// Sanitize encryption config (don't expose user secret)
		encCfg := cfg.Edge.Encryption
		encCfg.UserSecret = "" // Never expose user secret
		return encCfg
	case "web":
		return cfg.Edge.Web
	case "events":
		return cfg.Edge.Events
	default:
		return nil
	}
}

// updateConfigSection updates a specific configuration section
func (s *Server) updateConfigSection(cfg *config.Config, section string, data map[string]interface{}) error {
	// Normalize keys to match struct field names (CamelCase)
	normalizedData := s.normalizeKeys(data)

	// Manually update fields for type safety (since structs have YAML tags, not JSON tags)
	switch strings.ToLower(section) {
	case "ai":
		return s.updateAIConfig(&cfg.Edge.AI, normalizedData)
	case "cameras":
		return s.updateCamerasConfig(&cfg.Edge.Cameras, normalizedData)
	case "storage":
		return s.updateStorageConfig(&cfg.Edge.Storage, normalizedData)
	case "wireguard":
		return s.updateWireGuardConfig(&cfg.Edge.WireGuard, normalizedData)
	case "telemetry":
		return s.updateTelemetryConfig(&cfg.Edge.Telemetry, normalizedData)
	case "encryption":
		// Don't allow updating user secret via API (security)
		if userSecret, ok := data["user_secret"].(string); ok && userSecret != "" {
			return fmt.Errorf("user_secret cannot be updated via API")
		}
		if userSecret, ok := data["UserSecret"].(string); ok && userSecret != "" {
			return fmt.Errorf("user_secret cannot be updated via API")
		}
		return s.updateEncryptionConfig(&cfg.Edge.Encryption, normalizedData)
	case "web":
		return s.updateWebConfig(&cfg.Edge.Web, normalizedData)
	case "events":
		return s.updateEventsConfig(&cfg.Edge.Events, normalizedData)
	default:
		return fmt.Errorf("unknown section: %s", section)
	}
}

// Helper functions to update config sections with type safety
func (s *Server) updateAIConfig(aiCfg *config.AIConfig, data map[string]interface{}) error {
	if val, ok := data["ServiceURL"].(string); ok {
		aiCfg.ServiceURL = val
	}
	if val, ok := data["ConfidenceThreshold"].(float64); ok {
		aiCfg.ConfidenceThreshold = val
	}
	if val, ok := data["InferenceInterval"].(string); ok {
		if duration, err := time.ParseDuration(val); err == nil {
			aiCfg.InferenceInterval = duration
		}
	}
	if val, ok := data["EnabledClasses"].([]interface{}); ok {
		classes := make([]string, 0, len(val))
		for _, v := range val {
			if str, ok := v.(string); ok {
				classes = append(classes, str)
			}
		}
		aiCfg.EnabledClasses = classes
	}
	if val, ok := data["LocalInferenceEnabled"].(bool); ok {
		aiCfg.LocalInferenceEnabled = val
	}
	if val, ok := data["BaselineLabel"].(string); ok {
		aiCfg.BaselineLabel = val
	}
	if val, ok := data["AnomalyThreshold"].(float64); ok {
		aiCfg.AnomalyThreshold = val
	}
	if val, ok := data["LocalModelPath"].(string); ok {
		aiCfg.LocalModelPath = val
	}
	if val, ok := data["ClipDuration"].(string); ok {
		if duration, err := time.ParseDuration(val); err == nil {
			aiCfg.ClipDuration = duration
		}
	}
	if val, ok := data["PreEventDuration"].(string); ok {
		if duration, err := time.ParseDuration(val); err == nil {
			aiCfg.PreEventDuration = duration
		}
	}
	if val, ok := data["DatasetExportDir"].(string); ok {
		aiCfg.DatasetExportDir = val
	}
	return nil
}

func (s *Server) updateCamerasConfig(camCfg *config.CamerasConfig, data map[string]interface{}) error {
	if discovery, ok := data["Discovery"].(map[string]interface{}); ok {
		if val, ok := discovery["Enabled"].(bool); ok {
			camCfg.Discovery.Enabled = val
		}
		if val, ok := discovery["Interval"].(string); ok {
			if duration, err := time.ParseDuration(val); err == nil {
				camCfg.Discovery.Interval = duration
			}
		}
	}
	if rtsp, ok := data["RTSP"].(map[string]interface{}); ok {
		if val, ok := rtsp["Timeout"].(string); ok {
			if duration, err := time.ParseDuration(val); err == nil {
				camCfg.RTSP.Timeout = duration
			}
		}
		if val, ok := rtsp["ReconnectInterval"].(string); ok {
			if duration, err := time.ParseDuration(val); err == nil {
				camCfg.RTSP.ReconnectInterval = duration
			}
		}
	}
	return nil
}

func (s *Server) updateStorageConfig(storageCfg *config.StorageConfig, data map[string]interface{}) error {
	if val, ok := data["ClipsDir"].(string); ok {
		storageCfg.ClipsDir = val
	}
	if val, ok := data["SnapshotsDir"].(string); ok {
		storageCfg.SnapshotsDir = val
	}
	if val, ok := data["RetentionDays"].(float64); ok {
		storageCfg.RetentionDays = int(val)
	}
	if val, ok := data["MaxDiskUsagePercent"].(float64); ok {
		storageCfg.MaxDiskUsagePercent = val
	}
	return nil
}

func (s *Server) updateWireGuardConfig(wgCfg *config.WireGuardConfig, data map[string]interface{}) error {
	if val, ok := data["Enabled"].(bool); ok {
		wgCfg.Enabled = val
	}
	if val, ok := data["ConfigPath"].(string); ok {
		wgCfg.ConfigPath = val
	}
	if val, ok := data["KVMEndpoint"].(string); ok {
		wgCfg.KVMEndpoint = val
	}
	return nil
}

func (s *Server) updateTelemetryConfig(telemetryCfg *config.TelemetryConfig, data map[string]interface{}) error {
	if val, ok := data["Enabled"].(bool); ok {
		telemetryCfg.Enabled = val
	}
	if val, ok := data["Interval"].(string); ok {
		if duration, err := time.ParseDuration(val); err == nil {
			telemetryCfg.Interval = duration
		}
	}
	return nil
}

func (s *Server) updateEncryptionConfig(encCfg *config.EncryptionConfig, data map[string]interface{}) error {
	if val, ok := data["Enabled"].(bool); ok {
		encCfg.Enabled = val
	}
	if val, ok := data["Salt"].(string); ok {
		encCfg.Salt = val
	}
	if val, ok := data["SaltPath"].(string); ok {
		encCfg.SaltPath = val
	}
	// UserSecret is explicitly not allowed to be updated via API
	return nil
}

func (s *Server) updateWebConfig(webCfg *config.WebConfig, data map[string]interface{}) error {
	if val, ok := data["Enabled"].(bool); ok {
		webCfg.Enabled = val
	}
	if val, ok := data["Host"].(string); ok {
		webCfg.Host = val
	}
	if val, ok := data["Port"].(float64); ok {
		webCfg.Port = int(val)
	}
	return nil
}

func (s *Server) updateEventsConfig(eventsCfg *config.EventsConfig, data map[string]interface{}) error {
	if val, ok := data["QueueSize"].(float64); ok {
		eventsCfg.QueueSize = int(val)
	}
	if val, ok := data["BatchSize"].(float64); ok {
		eventsCfg.BatchSize = int(val)
	}
	if val, ok := data["TransmissionInterval"].(string); ok {
		if duration, err := time.ParseDuration(val); err == nil {
			eventsCfg.TransmissionInterval = duration
		}
	}
	return nil
}

// normalizeKeys converts snake_case keys to struct field names (CamelCase)
func (s *Server) normalizeKeys(data map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{})

	// Map of common snake_case to struct field names
	keyMap := map[string]string{
		"service_url":            "ServiceURL",
		"confidence_threshold":   "ConfidenceThreshold",
		"inference_interval":     "InferenceInterval",
		"enabled_classes":        "EnabledClasses",
		"clips_dir":              "ClipsDir",
		"snapshots_dir":          "SnapshotsDir",
		"retention_days":         "RetentionDays",
		"max_disk_usage_percent": "MaxDiskUsagePercent",
		"queue_size":             "QueueSize",
		"batch_size":             "BatchSize",
		"transmission_interval":  "TransmissionInterval",
		"user_secret":            "UserSecret",
		"salt":                   "Salt",
		"salt_path":              "SaltPath",
		"config_path":            "ConfigPath",
		"kvm_endpoint":           "KVMEndpoint",
		"reconnect_interval":     "ReconnectInterval",
		"timeout":                "Timeout",
		"interval":               "Interval",
		"enabled":                "Enabled",
		"host":                   "Host",
		"port":                   "Port",
		"discovery":              "Discovery",
		"rtsp":                   "RTSP",
	}

	for k, v := range data {
		// Check if key needs normalization
		lowerKey := strings.ToLower(k)
		if normalizedKey, ok := keyMap[lowerKey]; ok {
			// Handle nested maps (like discovery, rtsp)
			if nestedMap, ok := v.(map[string]interface{}); ok {
				normalized[normalizedKey] = s.normalizeKeys(nestedMap)
			} else {
				normalized[normalizedKey] = v
			}
		} else {
			// Keep original key if it's already CamelCase or unknown
			normalized[k] = v
		}
	}

	return normalized
}

// mergeConfig merges update data into the configuration
func (s *Server) mergeConfig(cfg *config.Config, updateData map[string]interface{}) error {
	// Handle both "edge" (lowercase) and "Edge" (capitalized) keys
	var edgeData map[string]interface{}
	if ed, ok := updateData["edge"].(map[string]interface{}); ok {
		edgeData = ed
	} else if ed, ok := updateData["Edge"].(map[string]interface{}); ok {
		edgeData = ed
	} else {
		// If no edge key, assume the updateData itself is edge data
		edgeData = updateData
	}

	// Update each section if present (handle both lowercase and capitalized keys)
	if camerasData, ok := getMapValue(edgeData, "cameras", "Cameras"); ok {
		if err := s.updateConfigSection(cfg, "cameras", camerasData); err != nil {
			return fmt.Errorf("failed to update cameras section: %w", err)
		}
	}
	if aiData, ok := getMapValue(edgeData, "ai", "AI"); ok {
		if err := s.updateConfigSection(cfg, "ai", aiData); err != nil {
			return fmt.Errorf("failed to update ai section: %w", err)
		}
	}
	if storageData, ok := getMapValue(edgeData, "storage", "Storage"); ok {
		if err := s.updateConfigSection(cfg, "storage", storageData); err != nil {
			return fmt.Errorf("failed to update storage section: %w", err)
		}
	}
	if wireguardData, ok := getMapValue(edgeData, "wireguard", "WireGuard"); ok {
		if err := s.updateConfigSection(cfg, "wireguard", wireguardData); err != nil {
			return fmt.Errorf("failed to update wireguard section: %w", err)
		}
	}
	if telemetryData, ok := getMapValue(edgeData, "telemetry", "Telemetry"); ok {
		if err := s.updateConfigSection(cfg, "telemetry", telemetryData); err != nil {
			return fmt.Errorf("failed to update telemetry section: %w", err)
		}
	}
	if encryptionData, ok := getMapValue(edgeData, "encryption", "Encryption"); ok {
		if err := s.updateConfigSection(cfg, "encryption", encryptionData); err != nil {
			return fmt.Errorf("failed to update encryption section: %w", err)
		}
	}
	if webData, ok := getMapValue(edgeData, "web", "Web"); ok {
		if err := s.updateConfigSection(cfg, "web", webData); err != nil {
			return fmt.Errorf("failed to update web section: %w", err)
		}
	}
	if eventsData, ok := getMapValue(edgeData, "events", "Events"); ok {
		if err := s.updateConfigSection(cfg, "events", eventsData); err != nil {
			return fmt.Errorf("failed to update events section: %w", err)
		}
	}

	return nil
}

// getMapValue tries to get a value from a map using multiple possible keys
func getMapValue(m map[string]interface{}, keys ...string) (map[string]interface{}, bool) {
	for _, key := range keys {
		if val, ok := m[key].(map[string]interface{}); ok {
			return val, true
		}
	}
	return nil, false
}

// sanitizeConfig removes sensitive information from config before returning
func (s *Server) sanitizeConfig(cfg *config.Config) *config.Config {
	sanitized := *cfg
	// Don't expose user secret
	sanitized.Edge.Encryption.UserSecret = ""
	return &sanitized
}

// handleMetrics handles the system metrics endpoint
func (s *Server) handleMetrics(c *gin.Context) {
	if s.telemetryCollector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Telemetry collector not available",
		})
		return
	}

	// Get last collected metrics
	lastMetrics := s.telemetryCollector.GetLastMetrics()
	if lastMetrics == nil {
		// Try to collect metrics now
		ctx := c.Request.Context()
		metrics, err := s.telemetryCollector.Collect(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to collect metrics: " + err.Error(),
			})
			return
		}
		lastMetrics = metrics
	}

	// Convert to JSON format
	// Note: This assumes lastMetrics is *edge.TelemetryData
	// We'll need to convert it properly
	response := s.telemetryDataToJSON(lastMetrics)
	if response == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to convert metrics",
		})
		return
	}

	// Extract system metrics
	systemMetrics, ok := response["system"].(map[string]interface{})
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"system": map[string]interface{}{},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"system":    systemMetrics,
		"timestamp": response["timestamp"],
	})
}

// handleAppMetrics handles the application metrics endpoint
func (s *Server) handleAppMetrics(c *gin.Context) {
	if s.telemetryCollector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Telemetry collector not available",
		})
		return
	}

	// Get last collected metrics
	lastMetrics := s.telemetryCollector.GetLastMetrics()
	if lastMetrics == nil {
		// Try to collect metrics now
		ctx := c.Request.Context()
		metrics, err := s.telemetryCollector.Collect(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to collect metrics: " + err.Error(),
			})
			return
		}
		lastMetrics = metrics
	}

	// Convert to JSON format
	response := s.telemetryDataToJSON(lastMetrics)
	if response == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to convert metrics",
		})
		return
	}

	// Extract application metrics
	appMetrics, ok := response["application"].(map[string]interface{})
	if !ok {
		appMetrics = map[string]interface{}{}
	}

	// Add camera count if available
	if s.cameraMgr != nil {
		cameras := s.cameraMgr.ListCameras(false)
		appMetrics["total_cameras"] = len(cameras)

		enabledCount := 0
		onlineCount := 0
		for _, cam := range cameras {
			if cam.Enabled {
				enabledCount++
			}
			if cam.Status == camera.CameraStatusOnline {
				onlineCount++
			}
		}
		appMetrics["enabled_cameras"] = enabledCount
		appMetrics["online_cameras"] = onlineCount
	}

	c.JSON(http.StatusOK, gin.H{
		"application": appMetrics,
		"timestamp":   response["timestamp"],
	})
}

// handleTelemetry handles the telemetry data endpoint
func (s *Server) handleTelemetry(c *gin.Context) {
	if s.telemetryCollector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Telemetry collector not available",
		})
		return
	}

	// Get last collected metrics
	lastMetrics := s.telemetryCollector.GetLastMetrics()
	if lastMetrics == nil {
		// Try to collect metrics now
		ctx := c.Request.Context()
		metrics, err := s.telemetryCollector.Collect(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to collect metrics: " + err.Error(),
			})
			return
		}
		lastMetrics = metrics
	}

	// Convert to JSON format
	response := s.telemetryDataToJSON(lastMetrics)
	if response == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to convert metrics",
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// telemetryDataToJSON converts telemetry data to JSON-compatible format
func (s *Server) telemetryDataToJSON(data interface{}) map[string]interface{} {
	telemetryData, ok := data.(*edge.TelemetryData)
	if !ok || telemetryData == nil {
		return nil
	}

	result := make(map[string]interface{})
	if telemetryData.Timestamp != 0 {
		result["timestamp"] = time.Unix(0, telemetryData.Timestamp).Format(time.RFC3339)
	} else {
		result["timestamp"] = time.Now().Format(time.RFC3339)
	}
	result["edge_id"] = telemetryData.EdgeId

	// System metrics
	if telemetryData.System != nil {
		result["system"] = map[string]interface{}{
			"cpu_usage_percent":  telemetryData.System.CpuUsagePercent,
			"memory_used_bytes":  telemetryData.System.MemoryUsedBytes,
			"memory_total_bytes": telemetryData.System.MemoryTotalBytes,
			"disk_used_bytes":    telemetryData.System.DiskUsedBytes,
			"disk_total_bytes":   telemetryData.System.DiskTotalBytes,
			"disk_usage_percent": telemetryData.System.DiskUsagePercent,
		}
	}

	// Application metrics
	if telemetryData.Application != nil {
		result["application"] = map[string]interface{}{
			"event_queue_length":       telemetryData.Application.EventQueueLength,
			"active_cameras":           telemetryData.Application.ActiveCameras,
			"ai_inference_time_ms":     telemetryData.Application.AiInferenceTimeMs,
			"storage_clips_count":      telemetryData.Application.StorageClipsCount,
			"storage_clips_size_bytes": telemetryData.Application.StorageClipsSizeBytes,
		}
	}

	// Camera statuses
	if len(telemetryData.Cameras) > 0 {
		cameras := make([]map[string]interface{}, 0, len(telemetryData.Cameras))
		for _, cam := range telemetryData.Cameras {
			cameras = append(cameras, map[string]interface{}{
				"camera_id":      cam.CameraId,
				"online":         cam.Online,
				"last_seen":      cam.LastSeen,
				"status_message": cam.StatusMessage,
			})
		}
		result["cameras"] = cameras
	}

	return result
}

// handleMJPEGStream handles MJPEG streaming endpoint
func (s *Server) handleMJPEGStream(c *gin.Context) {
	cameraID := c.Param("id")

	if s.streamingSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Streaming service not available",
		})
		return
	}

	// Start stream
	stream, err := s.streamingSvc.StartStream(cameraID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Failed to start stream: %v", err),
		})
		return
	}

	// Set headers for MJPEG stream
	c.Header("Content-Type", "multipart/x-mixed-replace; boundary=--frame")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Pragma", "no-cache")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering if behind proxy

	// Get flusher for explicit flushing
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Streaming not supported",
		})
		return
	}

	// Stream frames
	c.Stream(func(w io.Writer) bool {
		select {
		case frame, ok := <-stream.FrameChan:
			if !ok {
				return false // Stream closed
			}
			// Write frame boundary and data
			fmt.Fprintf(w, "--frame\r\n")
			fmt.Fprintf(w, "Content-Type: image/jpeg\r\n")
			fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(frame))
			w.Write(frame)
			fmt.Fprintf(w, "\r\n")
			// Flush immediately after each frame
			flusher.Flush()
			return true
		case <-stream.Done():
			return false
		case <-c.Request.Context().Done():
			return false
		}
	})

	// Stop stream when client disconnects
	s.streamingSvc.StopStream(cameraID)
}

// handleSingleFrame handles single frame JPEG endpoint
func (s *Server) handleSingleFrame(c *gin.Context) {
	cameraID := c.Param("id")

	if s.streamingSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Streaming service not available",
		})
		return
	}

	// Get frame
	frame, err := s.streamingSvc.GetFrame(cameraID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Failed to capture frame: %v", err),
		})
		return
	}

	// Return JPEG
	c.Data(http.StatusOK, "image/jpeg", frame)
}

// handleListScreenshots handles listing labeled screenshots
func (s *Server) handleListScreenshots(c *gin.Context) {
	if s.screenshotSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
		})
		return
	}

	// Parse query parameters
	filters := &screenshots.ScreenshotFilters{}
	if cameraID := c.Query("camera_id"); cameraID != "" {
		filters.CameraID = cameraID
	}
	if label := c.Query("label"); label != "" {
		filters.Label = screenshots.Label(label)
	}
	if customLabel := c.Query("custom_label"); customLabel != "" {
		filters.CustomLabel = customLabel
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filters.Limit = limit
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filters.Offset = offset
		}
	}

	screenshotsList, err := s.screenshotSvc.ListScreenshots(c.Request.Context(), filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to list screenshots: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"screenshots": screenshotsList,
		"count":       len(screenshotsList),
	})
}

// handleGetScreenshot handles getting a single screenshot
func (s *Server) handleGetScreenshot(c *gin.Context) {
	if s.screenshotSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
		})
		return
	}

	id := c.Param("id")
	screenshot, err := s.screenshotSvc.GetScreenshot(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Screenshot not found: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, screenshot)
}

// handleGetScreenshotImage handles getting the image file for a screenshot
func (s *Server) handleGetScreenshotImage(c *gin.Context) {
	if s.screenshotSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
		})
		return
	}

	id := c.Param("id")
	imageData, err := s.screenshotSvc.GetScreenshotImage(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Screenshot image not found: %v", err),
		})
		return
	}

	c.Data(http.StatusOK, "image/jpeg", imageData)
}

// handleSaveScreenshot handles saving a labeled screenshot
func (s *Server) handleSaveScreenshot(c *gin.Context) {
	if s.screenshotSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
		})
		return
	}

	var req struct {
		CameraID    string                 `json:"camera_id" binding:"required"`
		Label       string                 `json:"label" binding:"required"`
		CustomLabel string                 `json:"custom_label,omitempty"`
		Description string                 `json:"description,omitempty"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
		ImageData   string                 `json:"image_data" binding:"required"` // Base64 encoded
		CreatedBy   string                 `json:"created_by,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Decode base64 image data
	imageData, err := decodeBase64Image(req.ImageData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid image data: %v", err),
		})
		return
	}

	// Create screenshot object
	screenshot := &screenshots.Screenshot{
		CameraID:    req.CameraID,
		Label:       screenshots.Label(req.Label),
		CustomLabel: req.CustomLabel,
		Description: req.Description,
		Metadata:    req.Metadata,
		CreatedBy:   req.CreatedBy,
	}

	if err := s.screenshotSvc.SaveScreenshot(c.Request.Context(), screenshot, imageData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to save screenshot: %v", err),
		})
		return
	}

	c.JSON(http.StatusCreated, screenshot)
}

// handleUpdateScreenshot handles updating a screenshot's label/metadata
func (s *Server) handleUpdateScreenshot(c *gin.Context) {
	if s.screenshotSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	updates := &screenshots.ScreenshotUpdate{}
	if req.Label != nil {
		label := screenshots.Label(*req.Label)
		updates.Label = &label
	}
	updates.CustomLabel = req.CustomLabel
	updates.Description = req.Description
	updates.Metadata = req.Metadata

	if err := s.screenshotSvc.UpdateScreenshot(c.Request.Context(), id, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to update screenshot: %v", err),
		})
		return
	}

	// Return updated screenshot
	screenshot, err := s.screenshotSvc.GetScreenshot(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to get updated screenshot: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, screenshot)
}

// handleDeleteScreenshot handles deleting a screenshot
func (s *Server) handleDeleteScreenshot(c *gin.Context) {
	if s.screenshotSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
		})
		return
	}

	id := c.Param("id")
	if err := s.screenshotSvc.DeleteScreenshot(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to delete screenshot: %v", err),
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// handleExportScreenshots handles exporting labeled screenshots as a dataset
func (s *Server) handleExportScreenshots(c *gin.Context) {
	if s.screenshotSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Screenshot service not available",
		})
		return
	}

	var req struct {
		CameraID        string `json:"camera_id"`
		Label           string `json:"label"`
		CustomLabel     string `json:"custom_label"`
		IncludeMetadata bool   `json:"include_metadata"`
	}

	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	filters := &screenshots.ScreenshotFilters{
		CameraID:    req.CameraID,
		CustomLabel: req.CustomLabel,
	}
	if req.Label != "" {
		filters.Label = screenshots.Label(req.Label)
	}

	result, err := s.screenshotSvc.ExportDataset(c.Request.Context(), filters, req.IncludeMetadata)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to export dataset: %v", err),
		})
		return
	}

	c.Header("Content-Type", "application/zip")
	c.FileAttachment(result.FilePath, filepath.Base(result.FilePath))
}

// decodeBase64Image decodes a base64-encoded image string
func decodeBase64Image(base64Str string) ([]byte, error) {
	// Remove data URL prefix if present (e.g., "data:image/jpeg;base64,")
	parts := strings.Split(base64Str, ",")
	if len(parts) == 2 {
		base64Str = parts[1]
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	return decoded, nil
}

// handleReminderTelemetry handles telemetry for reminder acknowledgments and completions
func (s *Server) handleReminderTelemetry(c *gin.Context) {
	var req struct {
		CameraID  string `json:"camera_id" binding:"required"`
		Action    string `json:"action" binding:"required,oneof=acknowledged completed"`
		Timestamp string `json:"timestamp"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}

	// Log reminder interaction for ops visibility
	s.logger.Info("Reminder telemetry received",
		"camera_id", req.CameraID,
		"action", req.Action,
		"timestamp", req.Timestamp,
	)

	// TODO: Forward to telemetry collector for transmission to VM
	// For now, we just log it locally
	// In the future, this could be sent via telemetry collector to VM

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Reminder telemetry recorded",
	})
}
