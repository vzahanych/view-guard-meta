package orchestrator

import (
	"os"
)

// EdgeIDProvider provides the edge ID from environment or default
func EdgeIDProvider() string {
	edgeID := "poc-edge-1" // Default for PoC/test environment
	if envEdgeID := os.Getenv("EDGE_ID"); envEdgeID != "" {
		edgeID = envEdgeID
	}
	return edgeID
}
