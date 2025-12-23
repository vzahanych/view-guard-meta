package impl

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
)

// handleListCameras handles listing all cameras
func (g *WebGatewayImpl) handleListCameras(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	enabledOnly := c.Query("enabled") == "true"
	statusFilter := c.Query("status")
	syncedFilter := c.Query("synced") // "true" or "false"
	typeFilter := c.Query("type")

	filters := &metastorage.CameraFilters{}
	if enabledOnly {
		enabled := true
		filters.EnabledOnly = &enabled
	}
	if statusFilter != "" {
		filters.Status = &statusFilter
	}
	if syncedFilter != "" {
		synced := syncedFilter == "true"
		filters.SyncedWithVM = &synced
	}
	if typeFilter != "" {
		filters.Type = &typeFilter
	}

	cameraMetas, err := g.metaStorage.ListCameras(c.Request.Context(), filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]gin.H, 0, len(cameraMetas))
	for _, camMeta := range cameraMetas {
		response = append(response, g.cameraMetaToJSON(camMeta))
	}

	c.JSON(http.StatusOK, gin.H{
		"cameras": response,
		"count":   len(response),
	})
}

// handleGetCamera handles getting a single camera by ID
func (g *WebGatewayImpl) handleGetCamera(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	cameraID := c.Param("id")
	cameraMeta, found := g.metaStorage.GetCamera(c.Request.Context(), cameraID)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Camera not found",
		})
		return
	}

	c.JSON(http.StatusOK, g.cameraMetaToJSON(cameraMeta))
}

// handleAddCamera handles adding a new camera
func (g *WebGatewayImpl) handleAddCamera(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
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

	ctx := c.Request.Context()
	_, found := g.metaStorage.GetCamera(ctx, req.ID)
	if found {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Camera already exists",
		})
		return
	}

	now := time.Now()
	cameraMeta := metastorage.CameraMetadata{
		ID:            req.ID,
		Name:          req.Name,
		Type:          req.Type,
		Manufacturer:  req.Manufacturer,
		Model:         req.Model,
		Enabled:       req.Enabled,
		Status:        "offline",
		IPAddress:     req.IPAddress,
		ONVIFEndpoint: req.ONVIFEndpoint,
		RTSPURLs:      req.RTSPURLs,
		DevicePath:    req.DevicePath,
		Config: map[string]interface{}{
			"recording_enabled": req.Config.RecordingEnabled,
			"motion_detection":  req.Config.MotionDetection,
			"quality":           req.Config.Quality,
			"frame_rate":        req.Config.FrameRate,
			"resolution":        req.Config.Resolution,
		},
		Capabilities: make(map[string]interface{}),
		DiscoveredAt: now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := g.metaStorage.SaveCamera(ctx, cameraMeta); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if g.eventBus != nil {
		g.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("camera.registered"),
			Source:    "web-gateway",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"camera_id": req.ID,
			},
		})
	}

	c.JSON(http.StatusCreated, g.cameraMetaToJSON(cameraMeta))
}

// handleUpdateCamera handles updating an existing camera
func (g *WebGatewayImpl) handleUpdateCamera(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
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

	if len(req.RTSPURLs) > 0 || req.DevicePath != "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Updating rtsp_urls or device_path is not supported",
		})
		return
	}

	ctx := c.Request.Context()

	existing, found := g.metaStorage.GetCamera(ctx, cameraID)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Camera not found",
		})
		return
	}

	updated := existing
	updated.UpdatedAt = time.Now()

	if req.Name != "" {
		updated.Name = req.Name
	}
	if req.Enabled != nil {
		updated.Enabled = *req.Enabled
	}

	if updated.Config == nil {
		updated.Config = make(map[string]interface{})
	}
	if req.Config.RecordingEnabled != nil {
		updated.Config["recording_enabled"] = *req.Config.RecordingEnabled
	}
	if req.Config.MotionDetection != nil {
		updated.Config["motion_detection"] = *req.Config.MotionDetection
	}
	if req.Config.Quality != "" {
		updated.Config["quality"] = req.Config.Quality
	}
	if req.Config.FrameRate > 0 {
		updated.Config["frame_rate"] = req.Config.FrameRate
	}
	if req.Config.Resolution != "" {
		updated.Config["resolution"] = req.Config.Resolution
	}

	updatedMeta, err := g.metaStorage.UpdateCamera(ctx, cameraID, func(cam metastorage.CameraMetadata) metastorage.CameraMetadata {
		return updated
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	if g.eventBus != nil {
		g.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("camera.updated"),
			Source:    "web-gateway",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"camera_id": cameraID,
			},
		})
	}

	c.JSON(http.StatusOK, g.cameraMetaToJSON(updatedMeta))
}

// handleDeleteCamera handles deleting a camera
func (g *WebGatewayImpl) handleDeleteCamera(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	cameraID := c.Param("id")
	ctx := c.Request.Context()

	if err := g.metaStorage.DeleteCamera(ctx, cameraID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	if g.eventBus != nil {
		g.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("camera.deleted"),
			Source:    "web-gateway",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"camera_id": cameraID,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Camera deleted",
		"id":      cameraID,
	})
}

// handleDiscoverCameras handles camera discovery (currently returns empty list; discovery is event-driven)
func (g *WebGatewayImpl) handleDiscoverCameras(c *gin.Context) {
	if g.eventBus != nil {
		g.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("camera.discovery.requested"),
			Source:    "web-gateway",
			Timestamp: time.Now(),
			Data:      map[string]interface{}{},
		})
	}

	response := make([]gin.H, 0)
	c.JSON(http.StatusOK, gin.H{
		"discovered": response,
		"count":      len(response),
	})
}

// handleTestCamera handles testing camera connection
func (g *WebGatewayImpl) handleTestCamera(c *gin.Context) {
	if g.cctvService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Camera manager not available",
		})
		return
	}

	cameraID := c.Param("id")
	cam, err := g.cctvService.GetCamera(c.Request.Context(), cameraID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	var success bool
	var message string

	switch cam.Type {
	case cctvtypes.CameraTypeRTSP, cctvtypes.CameraTypeONVIF:
		camInfo, _ := g.cctvService.GetCamera(c.Request.Context(), cameraID)
		var status cctvtypes.CameraStatus
		if camInfo != nil {
			status = camInfo.Status
		}
		success = status == cctvtypes.CameraStatusOnline
		if success {
			message = "Camera is online and connected"
		} else {
			message = "Camera is offline or not connected"
		}
	case cctvtypes.CameraTypeUSB:
		if cam.DevicePath != "" {
			camInfo, _ := g.cctvService.GetCamera(c.Request.Context(), cameraID)
			var status cctvtypes.CameraStatus
			if camInfo != nil {
				status = camInfo.Status
			}
			success = status == cctvtypes.CameraStatusOnline
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

// cameraMetaToJSON converts CameraMetadata to JSON format
func (g *WebGatewayImpl) cameraMetaToJSON(camMeta metastorage.CameraMetadata) gin.H {
	config := camMeta.Config
	if config == nil {
		config = make(map[string]interface{})
	}
	capabilities := camMeta.Capabilities
	if capabilities == nil {
		capabilities = make(map[string]interface{})
	}

	recordingEnabled := true
	if rec, ok := config["recording_enabled"].(bool); ok {
		recordingEnabled = rec
	}
	motionDetection := true
	if md, ok := config["motion_detection"].(bool); ok {
		motionDetection = md
	}
	quality := "medium"
	if q, ok := config["quality"].(string); ok {
		quality = q
	}
	frameRate := 15
	if fr, ok := config["frame_rate"].(float64); ok {
		frameRate = int(fr)
	}

	hasPTZ := false
	if ptz, ok := capabilities["has_ptz"].(bool); ok {
		hasPTZ = ptz
	}
	hasSnapshot := false
	if snap, ok := capabilities["has_snapshot"].(bool); ok {
		hasSnapshot = snap
	}
	hasVideoStreams := false
	if vs, ok := capabilities["has_video_streams"].(bool); ok {
		hasVideoStreams = vs
	}

	return gin.H{
		"id":             camMeta.ID,
		"name":           camMeta.Name,
		"type":           camMeta.Type,
		"manufacturer":   camMeta.Manufacturer,
		"model":          camMeta.Model,
		"enabled":        camMeta.Enabled,
		"status":         camMeta.Status,
		"last_seen":      camMeta.LastSeen,
		"discovered_at":  camMeta.DiscoveredAt,
		"ip_address":     camMeta.IPAddress,
		"onvif_endpoint": camMeta.ONVIFEndpoint,
		"rtsp_urls":      camMeta.RTSPURLs,
		"device_path":    camMeta.DevicePath,
		"config": gin.H{
			"recording_enabled": recordingEnabled,
			"motion_detection":  motionDetection,
			"quality":           quality,
			"frame_rate":        frameRate,
		},
		"capabilities": gin.H{
			"has_ptz":           hasPTZ,
			"has_snapshot":      hasSnapshot,
			"has_video_streams": hasVideoStreams,
		},
	}
}

// cameraToJSON converts a Camera to JSON format (legacy - kept for backward compatibility)
func (g *WebGatewayImpl) cameraToJSON(cam *cctvtypes.Camera) gin.H {
	var lastSeen interface{}
	if cam.LastSeen != nil {
		lastSeen = cam.LastSeen.Format(time.RFC3339)
	}

	var datasetStatus interface{} = nil

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


