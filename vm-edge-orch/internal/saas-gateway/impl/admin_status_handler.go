package impl

import (
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// handleAdminStatus handles GET /api/admin/status - Get VM application status
func (s *saasGateway) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "operational",
		"service":   s.Name(),
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0", // TODO: Get from build info
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode status response", zap.Error(err))
	}
}
