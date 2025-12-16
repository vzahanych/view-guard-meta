package impl

import (
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// handleHealth handles GET /health - Health check endpoint
func (s *saasGateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "healthy",
		"service":   s.Name(),
		"timestamp": time.Now().Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode health response", zap.Error(err))
	}
}
