package impl

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// handleHealth handles the health check endpoint
func (g *WebGatewayImpl) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "web-server",
	})
}

// handleStatus handles the system status endpoint
func (g *WebGatewayImpl) handleStatus(c *gin.Context) {
	uptime := time.Since(g.startTime)

	// Service is considered healthy if it's running (no explicit status tracking needed)
	health := "healthy"

	c.JSON(http.StatusOK, gin.H{
		"status":         health,
		"uptime":         uptime.String(),
		"uptime_seconds": int64(uptime.Seconds()),
		"version":        g.version,
		"timestamp":      time.Now().Format(time.RFC3339),
	})
}
