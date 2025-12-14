package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	modelcatalog "github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
	modeldeployment "github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-deployment"
	tunnelgateway "github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
	"go.uber.org/zap"
)

// handleEdgeModelsDeploy handles POST /api/edges/{edge_id}/models/deploy?model_id={model_id}
// Triggers model deployment to a specific Edge
func (s *APIServer) handleEdgeModelsDeploy(w http.ResponseWriter, r *http.Request, edgeID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Validate Edge ID
	if edgeID == "" {
		http.Error(w, "Edge ID is required", http.StatusBadRequest)
		return
	}

	// Get model_id from query parameter (as per implementation plan)
	modelID := r.URL.Query().Get("model_id")
	if modelID == "" {
		http.Error(w, "model_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Optional: Get camera_id from query parameter (for camera-specific deployment)
	var cameraID *string
	if cameraIDParam := r.URL.Query().Get("camera_id"); cameraIDParam != "" {
		cameraID = &cameraIDParam
	}

	// Validate Edge is connected via WireGuard tunnel
	if s.edgeAPIServer == nil {
		s.logger.Error("Edge API server not available")
		http.Error(w, "Edge API server not available", http.StatusServiceUnavailable)
		return
	}

	// Check Edge connection status using connection monitor
	connMonitor := s.edgeAPIServer.GetConnectionMonitor()
	if connMonitor != nil {
		stateInfo, exists := connMonitor.GetConnectionState(edgeID)
		if exists && stateInfo != nil {
			state := stateInfo.State
			if state == tunnelgateway.StateDisconnected {
				s.logger.Warn("Edge is disconnected",
					zap.String("edge_id", edgeID),
					zap.String("state", string(state)),
				)
				http.Error(w, fmt.Sprintf("Edge %s is not connected", edgeID), http.StatusServiceUnavailable)
				return
			}
		}
	}

	// Fallback: Check legacy connection map
	conn, exists := s.edgeAPIServer.GetConnection(edgeID)
	if !exists || conn == nil {
		s.logger.Warn("Edge connection not found",
			zap.String("edge_id", edgeID),
		)
		// For PoC, allow deployment even without active connection (will connect on request)
		s.logger.Info("Allowing deployment without active connection (PoC mode)",
			zap.String("edge_id", edgeID),
		)
	} else {
		// Check if connection is recent (within last 5 minutes)
		if time.Since(conn.LastHeartbeat) > 5*time.Minute {
			s.logger.Warn("Edge connection is stale",
				zap.String("edge_id", edgeID),
				zap.Duration("age", time.Since(conn.LastHeartbeat)),
			)
			// Still allow deployment (connection might reconnect)
		}
	}

	// Validate model exists in catalog
	if s.modelCatalog == nil {
		s.logger.Error("Model catalog not available")
		http.Error(w, "Model catalog not available", http.StatusServiceUnavailable)
		return
	}

	modelEntry, err := s.modelCatalog.GetModel(modelID)
	if err != nil {
		s.logger.Warn("Model not found",
			zap.String("model_id", modelID),
			zap.Error(err),
		)
		http.Error(w, fmt.Sprintf("Model %s not found", modelID), http.StatusNotFound)
		return
	}

	// Validate model is ready for deployment
	if modelEntry.Status != modelcatalog.ModelStatusReady && modelEntry.Status != modelcatalog.ModelStatusBaseline {
		s.logger.Warn("Model is not ready for deployment",
			zap.String("model_id", modelID),
			zap.String("status", string(modelEntry.Status)),
		)
		http.Error(w, fmt.Sprintf("Model %s is not ready for deployment (status: %s)", modelID, modelEntry.Status), http.StatusBadRequest)
		return
	}

	// Validate deployment service is available
	if s.modelDeploymentService == nil {
		s.logger.Error("Model deployment service not available")
		http.Error(w, "Model deployment service not available", http.StatusServiceUnavailable)
		return
	}

	// Trigger manual deployment
	ctx := r.Context()
	job, err := s.modelDeploymentService.ManualDeploy(ctx, modelID, edgeID, cameraID)
	if err != nil {
		s.logger.Error("Failed to trigger model deployment",
			zap.String("model_id", modelID),
			zap.String("edge_id", edgeID),
			zap.Error(err),
		)
		http.Error(w, fmt.Sprintf("Failed to deploy model: %v", err), http.StatusInternalServerError)
		return
	}

	// Return deployment job ID and status (202 Accepted)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deployment_id": job.DeploymentID,
		"model_id":      job.ModelID,
		"edge_id":       job.EdgeID,
		"camera_id":     job.CameraID,
		"status":        job.Status,
		"message":       "Deployment job created and started",
	})
}

// handleDeployments handles GET /api/deployments (list deployments with filtering)
func (s *APIServer) handleDeployments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.modelDeploymentOrchestrator == nil {
		s.logger.Error("Model deployment orchestrator not configured")
		http.Error(w, "Model deployment orchestrator not available", http.StatusServiceUnavailable)
		return
	}

	// Parse query parameters for filtering
	filters := &modeldeployment.DeploymentFilters{
		EdgeID:   r.URL.Query().Get("edge_id"),
		ModelID:  r.URL.Query().Get("model_id"),
		CameraID: r.URL.Query().Get("camera_id"),
		Status:   modeldeployment.DeploymentStatus(r.URL.Query().Get("status")),
	}

	// Parse pagination
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		var limit int
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && limit > 0 {
			filters.Limit = limit
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		var offset int
		if _, err := fmt.Sscanf(offsetStr, "%d", &offset); err == nil && offset >= 0 {
			filters.Offset = offset
		}
	}

	// List deployments
	jobs, err := s.modelDeploymentOrchestrator.ListDeploymentJobs(r.Context(), filters)
	if err != nil {
		s.logger.Error("Failed to list deployments", zap.Error(err))
		http.Error(w, fmt.Sprintf("Failed to list deployments: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to API response format
	deployments := make([]map[string]interface{}, len(jobs))
	for i, job := range jobs {
		deployments[i] = map[string]interface{}{
			"deployment_id":           job.DeploymentID,
			"model_id":                job.ModelID,
			"edge_id":                 job.EdgeID,
			"camera_id":               job.CameraID,
			"status":                  string(job.Status),
			"deployment_started_at":   job.DeploymentStartedAt,
			"deployment_completed_at": job.DeploymentCompletedAt,
			"error_message":           job.ErrorMessage,
			"model_file_path":         job.ModelFilePath,
			"deployment_version":      job.DeploymentVersion,
			"created_at":              job.CreatedAt,
			"updated_at":              job.UpdatedAt,
		}
	}

	response := map[string]interface{}{
		"deployments": deployments,
		"total":       len(deployments),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// handleDeploymentByID handles GET /api/deployments/{deployment_id} and related paths
func (s *APIServer) handleDeploymentByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/deployments/{deployment_id} or /api/edges/{edge_id}/deployments or /api/models/{model_id}/deployments
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	parts := strings.Split(path, "/")

	if s.modelDeploymentOrchestrator == nil {
		s.logger.Error("Model deployment orchestrator not configured")
		http.Error(w, "Model deployment orchestrator not available", http.StatusServiceUnavailable)
		return
	}

	// Handle /api/deployments/{deployment_id}
	if len(parts) == 2 && parts[0] == "deployments" {
		deploymentID := parts[1]
		if deploymentID == "" {
			http.Error(w, "deployment_id is required", http.StatusBadRequest)
			return
		}

		job, err := s.modelDeploymentOrchestrator.GetDeploymentJob(r.Context(), deploymentID)
		if err != nil {
			s.logger.Warn("Deployment not found",
				zap.String("deployment_id", deploymentID),
				zap.Error(err),
			)
			http.Error(w, fmt.Sprintf("Deployment not found: %s", deploymentID), http.StatusNotFound)
			return
		}

		response := map[string]interface{}{
			"deployment_id":           job.DeploymentID,
			"model_id":                job.ModelID,
			"edge_id":                 job.EdgeID,
			"camera_id":               job.CameraID,
			"status":                  string(job.Status),
			"deployment_started_at":   job.DeploymentStartedAt,
			"deployment_completed_at": job.DeploymentCompletedAt,
			"error_message":           job.ErrorMessage,
			"model_file_path":         job.ModelFilePath,
			"deployment_version":      job.DeploymentVersion,
			"created_at":              job.CreatedAt,
			"updated_at":              job.UpdatedAt,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			s.logger.Error("Failed to encode response", zap.Error(err))
		}
		return
	}

	// Handle /api/edges/{edge_id}/deployments
	if len(parts) == 3 && parts[0] == "edges" && parts[2] == "deployments" {
		edgeID := parts[1]
		if edgeID == "" {
			http.Error(w, "edge_id is required", http.StatusBadRequest)
			return
		}

		filters := &modeldeployment.DeploymentFilters{
			EdgeID: edgeID,
		}

		// Parse pagination
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			var limit int
			if _, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && limit > 0 {
				filters.Limit = limit
			}
		}
		if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
			var offset int
			if _, err := fmt.Sscanf(offsetStr, "%d", &offset); err == nil && offset >= 0 {
				filters.Offset = offset
			}
		}

		jobs, err := s.modelDeploymentOrchestrator.ListDeploymentJobs(r.Context(), filters)
		if err != nil {
			s.logger.Error("Failed to list deployments for Edge", zap.Error(err))
			http.Error(w, fmt.Sprintf("Failed to list deployments: %v", err), http.StatusInternalServerError)
			return
		}

		deployments := make([]map[string]interface{}, len(jobs))
		for i, job := range jobs {
			deployments[i] = map[string]interface{}{
				"deployment_id":           job.DeploymentID,
				"model_id":                job.ModelID,
				"edge_id":                 job.EdgeID,
				"camera_id":               job.CameraID,
				"status":                  string(job.Status),
				"deployment_started_at":   job.DeploymentStartedAt,
				"deployment_completed_at": job.DeploymentCompletedAt,
				"error_message":           job.ErrorMessage,
				"model_file_path":         job.ModelFilePath,
				"deployment_version":      job.DeploymentVersion,
				"created_at":              job.CreatedAt,
				"updated_at":              job.UpdatedAt,
			}
		}

		response := map[string]interface{}{
			"deployments": deployments,
			"total":       len(deployments),
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			s.logger.Error("Failed to encode response", zap.Error(err))
		}
		return
	}

	// Handle /api/models/{model_id}/deployments
	if len(parts) == 3 && parts[0] == "models" && parts[2] == "deployments" {
		modelID := parts[1]
		if modelID == "" {
			http.Error(w, "model_id is required", http.StatusBadRequest)
			return
		}

		filters := &modeldeployment.DeploymentFilters{
			ModelID: modelID,
		}

		// Parse pagination
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			var limit int
			if _, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && limit > 0 {
				filters.Limit = limit
			}
		}
		if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
			var offset int
			if _, err := fmt.Sscanf(offsetStr, "%d", &offset); err == nil && offset >= 0 {
				filters.Offset = offset
			}
		}

		jobs, err := s.modelDeploymentOrchestrator.ListDeploymentJobs(r.Context(), filters)
		if err != nil {
			s.logger.Error("Failed to list deployments for model", zap.Error(err))
			http.Error(w, fmt.Sprintf("Failed to list deployments: %v", err), http.StatusInternalServerError)
			return
		}

		deployments := make([]map[string]interface{}, len(jobs))
		for i, job := range jobs {
			deployments[i] = map[string]interface{}{
				"deployment_id":           job.DeploymentID,
				"model_id":                job.ModelID,
				"edge_id":                 job.EdgeID,
				"camera_id":               job.CameraID,
				"status":                  string(job.Status),
				"deployment_started_at":   job.DeploymentStartedAt,
				"deployment_completed_at": job.DeploymentCompletedAt,
				"error_message":           job.ErrorMessage,
				"model_file_path":         job.ModelFilePath,
				"deployment_version":      job.DeploymentVersion,
				"created_at":              job.CreatedAt,
				"updated_at":              job.UpdatedAt,
			}
		}

		response := map[string]interface{}{
			"deployments": deployments,
			"total":       len(deployments),
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			s.logger.Error("Failed to encode response", zap.Error(err))
		}
		return
	}

	// Handle POST /api/deployments/{deployment_id}/status (Edge status update)
	if len(parts) == 3 && parts[0] == "deployments" && parts[2] == "status" {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		deploymentID := parts[1]
		if deploymentID == "" {
			http.Error(w, "deployment_id is required", http.StatusBadRequest)
			return
		}

		// Parse request body
		var request struct {
			Status    string  `json:"status"`
			Timestamp string  `json:"timestamp"`
			ModelPath *string `json:"model_path,omitempty"`
			Error     *string `json:"error,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.logger.Error("Failed to decode status update request", zap.Error(err))
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		if request.Status == "" {
			http.Error(w, "status is required", http.StatusBadRequest)
			return
		}

		// Update deployment status
		if s.modelDeploymentOrchestrator == nil {
			s.logger.Error("Model deployment orchestrator not configured")
			http.Error(w, "Model deployment orchestrator not available", http.StatusServiceUnavailable)
			return
		}

		// Update status based on Edge report
		if request.Status == "deployed" {
			modelPath := request.ModelPath
			if err := s.modelDeploymentOrchestrator.CompleteDeployment(r.Context(), deploymentID, modelPath); err != nil {
				s.logger.Error("Failed to complete deployment",
					zap.String("deployment_id", deploymentID),
					zap.Error(err),
				)
				http.Error(w, fmt.Sprintf("Failed to update deployment status: %v", err), http.StatusInternalServerError)
				return
			}
		} else if request.Status == "active" {
			// Edge reports model is loaded and active for inference
			if err := s.modelDeploymentOrchestrator.ActivateDeployment(r.Context(), deploymentID); err != nil {
				s.logger.Error("Failed to activate deployment",
					zap.String("deployment_id", deploymentID),
					zap.Error(err),
				)
				http.Error(w, fmt.Sprintf("Failed to update deployment status: %v", err), http.StatusInternalServerError)
				return
			}
		} else if request.Status == "failed" {
			errorMsg := "Deployment failed on Edge"
			if request.Error != nil {
				errorMsg = *request.Error
			}
			if err := s.modelDeploymentOrchestrator.FailDeployment(r.Context(), deploymentID, errorMsg); err != nil {
				s.logger.Error("Failed to mark deployment as failed",
					zap.String("deployment_id", deploymentID),
					zap.Error(err),
				)
				http.Error(w, fmt.Sprintf("Failed to update deployment status: %v", err), http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, fmt.Sprintf("Invalid status: %s (expected: deployed, active, or failed)", request.Status), http.StatusBadRequest)
			return
		}

		s.logger.Info("Deployment status updated from Edge",
			zap.String("deployment_id", deploymentID),
			zap.String("status", request.Status),
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Deployment status updated",
		}); err != nil {
			s.logger.Error("Failed to encode response", zap.Error(err))
		}
		return
	}

	// Invalid path
	http.Error(w, "Invalid path", http.StatusBadRequest)
}
