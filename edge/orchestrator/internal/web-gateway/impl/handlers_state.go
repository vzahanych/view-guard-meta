package impl

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// EdgeStateResponse represents the edge application state for API responses
type EdgeStateResponse struct {
	Status               string    `json:"status"`
	WireGuardConnected   bool      `json:"wireguard_connected"`   // WireGuard tunnel connection status
	VMHTTPConnected      bool      `json:"vm_http_connected"`    // VM HTTP client connection status (Edge → VM)
	VMAuthenticated      bool      `json:"vm_authenticated"`
	CamerasEnabled       int       `json:"cameras_enabled"`
	AIProcessingActive   bool      `json:"ai_processing_active"`
	MetaStorageHealth    string    `json:"meta_storage_health"`
	ObjectStorageHealth  string    `json:"object_storage_health"`
	StorageHealth        string    `json:"storage_health"` // Deprecated: use meta_storage_health and object_storage_health
	LastUpdated          time.Time `json:"last_updated"`
}

// handleGetState handles the GET /api/state endpoint to get current edge application state
func (g *WebGatewayImpl) handleGetState(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "meta storage not available",
		})
		return
	}

	// Get current state from meta-storage
	stateMap, found := g.metaStorage.GetCurrentEdgeState(c.Request.Context())
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "edge state not found",
		})
		return
	}

	// Convert map to EdgeStateResponse
	var edgeState EdgeStateResponse
	if err := convertStateMap(stateMap, &edgeState); err != nil {
		g.logger.Warn("failed to convert state", zap.Error(err), zap.Any("state", stateMap))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to convert state",
		})
		return
	}

	// Check meta-storage health
	metaStorageHealth := "unknown"
	if g.metaStorage != nil {
		_, err := g.metaStorage.GetStorageStats(c.Request.Context())
		if err != nil {
			metaStorageHealth = "unhealthy"
			g.logger.Warn("meta-storage health check failed", zap.Error(err))
		} else {
			metaStorageHealth = "healthy"
		}
	} else {
		metaStorageHealth = "unavailable"
	}
	edgeState.MetaStorageHealth = metaStorageHealth

	// Check object-storage health
	objectStorageHealth := "unknown"
	if g.objectStorage != nil {
		// Object-storage is available and was initialized successfully
		// If Start() had failed during initialization, the service wouldn't be available
		// Note: This is a basic check - a full connectivity test would require
		// a HealthCheck() method in the ObjectStorageService interface
		objectStorageHealth = "healthy"
	} else {
		objectStorageHealth = "unavailable"
	}
	edgeState.ObjectStorageHealth = objectStorageHealth

	// Set legacy storage_health field for backward compatibility
	// It reflects the overall storage health (both must be healthy)
	if metaStorageHealth == "healthy" && objectStorageHealth == "healthy" {
		edgeState.StorageHealth = "healthy"
	} else if metaStorageHealth == "unavailable" || objectStorageHealth == "unavailable" {
		edgeState.StorageHealth = "unavailable"
	} else {
		edgeState.StorageHealth = "unhealthy"
	}

	// Check WireGuard tunnel connection status
	wireGuardConnected := false
	if g.vmGateway != nil {
		wireGuardConnected = g.vmGateway.IsConnected()
	}
	edgeState.WireGuardConnected = wireGuardConnected

	// Check VM HTTP client connection status
	vmHTTPConnected := false
	if g.vmGateway != nil {
		vmHTTPConnected = g.vmGateway.IsHTTPConnected()
	}
	edgeState.VMHTTPConnected = vmHTTPConnected

	c.JSON(http.StatusOK, edgeState)
}

// convertStateMap converts the state map from meta-storage to API response format
func convertStateMap(from map[string]interface{}, to *EdgeStateResponse) error {
	// Use JSON as an intermediate format for type conversion
	data, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, to)
}

