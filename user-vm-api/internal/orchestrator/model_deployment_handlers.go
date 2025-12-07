package orchestrator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	modeldeployment "github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-deployment"
	"go.uber.org/zap"
)

// handleEdgeModelsDeploy handles POST /api/edges/{edge_id}/models/deploy
func (s *APIServer) handleEdgeModelsDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/edges/{edge_id}/models/deploy
	path := strings.TrimPrefix(r.URL.Path, "/api/edges/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[1] != "models" || parts[2] != "deploy" {
		http.Error(w, "Invalid path. Expected: /api/edges/{edge_id}/models/deploy", http.StatusBadRequest)
		return
	}

	edgeID := parts[0]
	if edgeID == "" {
		http.Error(w, "edge_id is required", http.StatusBadRequest)
		return
	}

	// Verify Edge is connected via WireGuard tunnel
	if s.edgeAPIServer != nil {
		conn, exists := s.edgeAPIServer.GetConnection(edgeID)
		if !exists || conn == nil {
			s.logger.Warn("Edge not connected",
				zap.String("edge_id", edgeID),
			)
			http.Error(w, "Edge is not connected via WireGuard tunnel", http.StatusServiceUnavailable)
			return
		}
	} else {
		s.logger.Error("Edge API server not configured")
		http.Error(w, "Edge API server not available", http.StatusServiceUnavailable)
		return
	}

	// Parse request body
	var request struct {
		ModelID  string  `json:"model_id"`
		CameraID *string `json:"camera_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.logger.Error("Failed to decode request", zap.Error(err))
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if request.ModelID == "" {
		http.Error(w, "model_id is required", http.StatusBadRequest)
		return
	}

	// Verify model exists
	if s.modelCatalog != nil {
		_, err := s.modelCatalog.GetModel(request.ModelID)
		if err != nil {
			s.logger.Warn("Model not found",
				zap.String("model_id", request.ModelID),
				zap.Error(err),
			)
			http.Error(w, fmt.Sprintf("Model not found: %s", request.ModelID), http.StatusNotFound)
			return
		}
	}

	// Check if model deployment service is available
	if s.modelDeploymentService == nil {
		s.logger.Error("Model deployment service not configured")
		http.Error(w, "Model deployment service not available", http.StatusServiceUnavailable)
		return
	}

	// Trigger model deployment
	job, err := s.modelDeploymentService.ManualDeploy(r.Context(), request.ModelID, edgeID, request.CameraID)
	if err != nil {
		s.logger.Error("Failed to deploy model",
			zap.String("edge_id", edgeID),
			zap.String("model_id", request.ModelID),
			zap.Error(err),
		)
		http.Error(w, fmt.Sprintf("Failed to deploy model: %v", err), http.StatusInternalServerError)
		return
	}

	// Return deployment job ID
	// Type assertion to ensure we're using the correct type
	var _ *modeldeployment.DeploymentJob = job
	response := map[string]interface{}{
		"deployment_id": job.DeploymentID,
		"model_id":     job.ModelID,
		"edge_id":      job.EdgeID,
		"status":       string(job.Status),
		"created_at":    job.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode response", zap.Error(err))
	}
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
			"deployment_id":          job.DeploymentID,
			"model_id":               job.ModelID,
			"edge_id":                job.EdgeID,
			"camera_id":              job.CameraID,
			"status":                 string(job.Status),
			"deployment_started_at":  job.DeploymentStartedAt,
			"deployment_completed_at": job.DeploymentCompletedAt,
			"error_message":          job.ErrorMessage,
			"model_file_path":        job.ModelFilePath,
			"deployment_version":     job.DeploymentVersion,
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
			"deployment_id":          job.DeploymentID,
			"model_id":               job.ModelID,
			"edge_id":                job.EdgeID,
			"camera_id":              job.CameraID,
			"status":                 string(job.Status),
			"deployment_started_at":  job.DeploymentStartedAt,
			"deployment_completed_at": job.DeploymentCompletedAt,
			"error_message":          job.ErrorMessage,
			"model_file_path":        job.ModelFilePath,
			"deployment_version":     job.DeploymentVersion,
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
				"deployment_id":          job.DeploymentID,
				"model_id":               job.ModelID,
				"edge_id":                job.EdgeID,
				"camera_id":              job.CameraID,
				"status":                 string(job.Status),
				"deployment_started_at":  job.DeploymentStartedAt,
				"deployment_completed_at": job.DeploymentCompletedAt,
				"error_message":          job.ErrorMessage,
				"model_file_path":        job.ModelFilePath,
				"deployment_version":     job.DeploymentVersion,
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
				"deployment_id":          job.DeploymentID,
				"model_id":               job.ModelID,
				"edge_id":                job.EdgeID,
				"camera_id":              job.CameraID,
				"status":                 string(job.Status),
				"deployment_started_at":  job.DeploymentStartedAt,
				"deployment_completed_at": job.DeploymentCompletedAt,
				"error_message":          job.ErrorMessage,
				"model_file_path":        job.ModelFilePath,
				"deployment_version":     job.DeploymentVersion,
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
			Status     string  `json:"status"`
			Timestamp  string  `json:"timestamp"`
			ModelPath  *string `json:"model_path,omitempty"`
			Error      *string `json:"error,omitempty"`
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
		if request.Status == "deployed" || request.Status == "active" {
			modelPath := request.ModelPath
			if err := s.modelDeploymentOrchestrator.CompleteDeployment(r.Context(), deploymentID, modelPath); err != nil {
				s.logger.Error("Failed to complete deployment",
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

