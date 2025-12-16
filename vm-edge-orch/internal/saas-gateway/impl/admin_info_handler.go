package impl

import (
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// handleAdminInfo handles GET /api/admin/info - Get VM application information
func (s *saasGateway) handleAdminInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"service":   s.Name(),
		"timestamp": time.Now().Format(time.RFC3339),
		"info": map[string]interface{}{
			"description": "SaaS Gateway - Admin interface for VM application",
			"endpoints": []string{
				"/health",
				"/api/admin/status",
				"/api/admin/info",
			},
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode info response", zap.Error(err))
	}
}
