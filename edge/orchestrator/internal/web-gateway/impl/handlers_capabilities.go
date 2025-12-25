package impl

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleGetCapabilities handles the GET /api/capabilities endpoint
// Returns Edge capabilities received from VM
func (g *WebGatewayImpl) handleGetCapabilities(c *gin.Context) {
	if g.metaStorage == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Meta storage not available",
		})
		return
	}

	capabilities, found := g.metaStorage.GetEdgeCapabilities(c.Request.Context())
	if !found {
		c.JSON(http.StatusOK, gin.H{
			"capabilities": make(map[string]interface{}),
			"message":      "No capabilities received yet",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"capabilities": capabilities,
	})
}

