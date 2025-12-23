package impl

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/config"
	"go.uber.org/zap"
)

// handleGetConfig handles getting configuration (read-only)
func (g *WebGatewayImpl) handleGetConfig(c *gin.Context) {
	// if g.cfg == nil {
	// 	c.JSON(http.StatusServiceUnavailable, gin.H{
	// 		"error": "Configuration not available",
	// 	})
	// 	return
	// }

	// section := c.Query("section")

	// if section != "" {
	// 	sectionConfig := g.getConfigSection(g.cfg, section)
	// 	if sectionConfig == nil {
	// 		c.JSON(http.StatusBadRequest, gin.H{
	// 			"error": "Invalid section: " + section,
	// 		})
	// 		return
	// 	}
	// 	c.JSON(http.StatusOK, gin.H{
	// 		"section": section,
	// 		"config":  sectionConfig,
	// 	})
	// } else {
	// 	sanitizedConfig := g.sanitizeConfig(g.cfg)
	// 	c.JSON(http.StatusOK, gin.H{
	// 		"config": sanitizedConfig,
	// 	})
	// }
}

// handleUpdateConfig handles updating configuration (not supported in simplified config)
func (g *WebGatewayImpl) handleUpdateConfig(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Configuration updates are not supported. Please modify the configuration file and restart the service.",
	})
}

// getConfigSection returns a specific configuration section
func (g *WebGatewayImpl) getConfigSection(cfg *config.Config, section string) interface{} {
	switch strings.ToLower(section) {
	case "event_bus":
		return cfg.EventBus
	case "ai":
		return cfg.AI
	case "meta_storage":
		return cfg.MetaStorage
	case "object_storage":
		return cfg.ObjectStorage
	case "vm_gateway":
		return cfg.VMGateway
	case "cctv":
		return cfg.CCTV
	case "telemetry":
		return cfg.Telemetry
	case "log":
		return gin.H{
			"log_level":  cfg.LogLevel,
			"log_format": cfg.LogFormat,
		}
	case "environment":
		return cfg.Environment
	default:
		return nil
	}
}

// sanitizeConfig removes sensitive information from config before returning
func (g *WebGatewayImpl) sanitizeConfig(cfg *config.Config) *config.Config {
	sanitized := *cfg
	sanitized.MetaStorage.SecretKey = ""
	sanitized.ObjectStorage.SecretKey = ""
	return &sanitized
}

// handleMetrics handles the system metrics endpoint
func (g *WebGatewayImpl) handleMetrics(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"timestamp": time.Now().UnixNano(),
		"system":    map[string]interface{}{},
		"note":      "Telemetry is now handled by OpenTelemetry. Use OTLP endpoint for metrics.",
	})
}

// handleAppMetrics handles the application metrics endpoint
func (g *WebGatewayImpl) handleAppMetrics(c *gin.Context) {
	appMetrics := map[string]interface{}{}

	if g.metaStorage != nil {
		cameras, err := g.metaStorage.ListCameras(c.Request.Context(), false)
		if err == nil {
			appMetrics["total_cameras"] = len(cameras)

			enabledCount := 0
			onlineCount := 0
			for _, cam := range cameras {
				if cam.Enabled {
					enabledCount++
				}
				if cam.Status == "online" {
					onlineCount++
				}
			}
			appMetrics["enabled_cameras"] = enabledCount
			appMetrics["online_cameras"] = onlineCount
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"application": appMetrics,
		"timestamp":   time.Now().UnixNano(),
		"note":        "Telemetry is now handled by OpenTelemetry. Use OTLP endpoint for metrics.",
	})
}

// handleTelemetry handles the telemetry data endpoint
func (g *WebGatewayImpl) handleTelemetry(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"timestamp":   time.Now().UnixNano(),
		"edge_id":     "",
		"system":      map[string]interface{}{},
		"application": map[string]interface{}{},
		"cameras":     []interface{}{},
		"note":        "Telemetry is now handled by OpenTelemetry. Use OTLP endpoint for metrics.",
	})
}

// handleReminderTelemetry handles telemetry for reminder acknowledgments and completions
func (g *WebGatewayImpl) handleReminderTelemetry(c *gin.Context) {
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

	g.logger.Info("Reminder telemetry received",
		zap.String("camera_id", req.CameraID),
		zap.String("action", req.Action),
		zap.String("timestamp", req.Timestamp),
	)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Reminder telemetry recorded",
	})
}
