package impl

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	eventbus "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage"
)

// createEdgeRequest represents the payload for creating a new Edge device.
type createEdgeRequest struct {
	// EdgeID is a client-provided identifier for the Edge (typically a UUID as string).
	EdgeID string `json:"edge_id"`

	// WgPublicKey is the WireGuard public key of the Edge.
	WgPublicKey string `json:"wg_public_key"`

	// Metadata carries optional arbitrary key/value metadata about the Edge.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// updateEdgeRequest represents the payload for updating an existing Edge device.
type updateEdgeRequest struct {
	// WgPublicKey is the WireGuard public key of the Edge (optional update).
	WgPublicKey *string `json:"wg_public_key,omitempty"`

	// Metadata carries optional arbitrary key/value metadata about the Edge.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// handleEdges handles collection-level operations on edges:
//   - GET /api/admin/edges - list all edges
//   - POST /api/admin/edges - create a new edge
func (s *saasGateway) handleEdges(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListEdges(w, r)
	case http.MethodPost:
		s.handleCreateEdge(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleEdgeByID handles individual edge operations:
//   - GET /api/admin/edges/{id} - get a specific edge
//   - PUT /api/admin/edges/{id} - update an edge
//   - DELETE /api/admin/edges/{id} - delete an edge
func (s *saasGateway) handleEdgeByID(w http.ResponseWriter, r *http.Request) {
	// Extract edge ID from path: /api/admin/edges/{id}
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid path: edge ID required", http.StatusBadRequest)
		return
	}
	edgeIDStr := pathParts[3]

	edgeID, err := uuid.Parse(edgeIDStr)
	if err != nil {
		http.Error(w, "Invalid edge ID format", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetEdge(w, r, edgeID)
	case http.MethodPut:
		s.handleUpdateEdge(w, r, edgeID)
	case http.MethodDelete:
		s.handleDeleteEdge(w, r, edgeID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListEdges handles GET /api/admin/edges - list all registered edges.
func (s *saasGateway) handleListEdges(w http.ResponseWriter, r *http.Request) {
	if s.metaStore == nil {
		http.Error(w, "Meta storage not available", http.StatusInternalServerError)
		return
	}

	// For now, return a placeholder response.
	// TODO: Implement GetAllEdges() in MetaDataStore if needed, or iterate over known edges.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"edges":     []interface{}{},
		"count":     0,
		"timestamp": time.Now().Format(time.RFC3339),
	}); err != nil {
		s.logger.Error("Failed to encode list edges response", zap.Error(err))
	}
}

// handleCreateEdge handles POST /api/admin/edges - register a new Edge device.
func (s *saasGateway) handleCreateEdge(w http.ResponseWriter, r *http.Request) {
	if s.metaStore == nil {
		http.Error(w, "Meta storage not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req createEdgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.EdgeID == "" || req.WgPublicKey == "" {
		http.Error(w, "edge_id and wg_public_key are required", http.StatusBadRequest)
		return
	}

	edgeID, err := uuid.Parse(req.EdgeID)
	if err != nil {
		http.Error(w, "invalid edge_id format (must be UUID)", http.StatusBadRequest)
		return
	}

	// Check if edge already exists
	if _, exists := s.metaStore.GetEdge(edgeID); exists {
		http.Error(w, "edge already exists", http.StatusConflict)
		return
	}

	// Create initial edge state
	edgeState := metastorage.EdgeState{
		UUID:        edgeID,
		WGPublicKey: req.WgPublicKey,
		Status:      metastorage.EdgeStatusRegistered,
		Metadata:    req.Metadata,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Register edge in meta-storage
	if err := s.metaStore.RegisterEdge(edgeID, edgeState); err != nil {
		s.logger.Error("Failed to register edge", zap.Error(err), zap.String("edge_id", edgeID.String()))
		http.Error(w, "failed to register edge", http.StatusInternalServerError)
		return
	}

	// Publish edge registration event
	if s.eventBus != nil {
		s.eventBus.Publish(eventbus.Event{
			Type:      "edge.registered",
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"edge_id":       edgeID.String(),
				"wg_public_key": req.WgPublicKey,
			},
		})
	}

	resp := map[string]interface{}{
		"status":    "created",
		"edge_id":   edgeID.String(),
		"timestamp": time.Now().Format(time.RFC3339),
	}

	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("Failed to encode create edge response", zap.Error(err))
	}
}

// handleGetEdge handles GET /api/admin/edges/{id} - get a specific edge.
func (s *saasGateway) handleGetEdge(w http.ResponseWriter, r *http.Request, edgeID uuid.UUID) {
	if s.metaStore == nil {
		http.Error(w, "Meta storage not available", http.StatusInternalServerError)
		return
	}

	edgeState, exists := s.metaStore.GetEdge(edgeID)
	if !exists {
		http.Error(w, "edge not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(edgeState); err != nil {
		s.logger.Error("Failed to encode get edge response", zap.Error(err))
	}
}

// handleUpdateEdge handles PUT /api/admin/edges/{id} - update an existing edge.
func (s *saasGateway) handleUpdateEdge(w http.ResponseWriter, r *http.Request, edgeID uuid.UUID) {
	if s.metaStore == nil {
		http.Error(w, "Meta storage not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req updateEdgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Update edge state
	updatedState, err := s.metaStore.UpdateEdge(edgeID, func(current metastorage.EdgeState) metastorage.EdgeState {
		if req.WgPublicKey != nil {
			current.WGPublicKey = *req.WgPublicKey
		}
		if req.Metadata != nil {
			// Merge metadata
			if current.Metadata == nil {
				current.Metadata = make(map[string]string)
			}
			for k, v := range req.Metadata {
				current.Metadata[k] = v
			}
		}
		return current
	})

	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "edge not found", http.StatusNotFound)
			return
		}
		s.logger.Error("Failed to update edge", zap.Error(err), zap.String("edge_id", edgeID.String()))
		http.Error(w, "failed to update edge", http.StatusInternalServerError)
		return
	}

	// Publish edge update event
	if s.eventBus != nil {
		s.eventBus.Publish(eventbus.Event{
			Type:      "edge.updated",
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"edge_id": edgeID.String(),
			},
		})
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(updatedState); err != nil {
		s.logger.Error("Failed to encode update edge response", zap.Error(err))
	}
}

// handleDeleteEdge handles DELETE /api/admin/edges/{id} - unregister an edge.
func (s *saasGateway) handleDeleteEdge(w http.ResponseWriter, r *http.Request, edgeID uuid.UUID) {
	if s.metaStore == nil {
		http.Error(w, "Meta storage not available", http.StatusInternalServerError)
		return
	}

	// Check if edge exists
	if _, exists := s.metaStore.GetEdge(edgeID); !exists {
		http.Error(w, "edge not found", http.StatusNotFound)
		return
	}

	// Unregister edge
	if err := s.metaStore.UnregisterEdge(edgeID); err != nil {
		s.logger.Error("Failed to unregister edge", zap.Error(err), zap.String("edge_id", edgeID.String()))
		http.Error(w, "failed to unregister edge", http.StatusInternalServerError)
		return
	}

	// Publish edge unregistration event
	if s.eventBus != nil {
		s.eventBus.Publish(eventbus.Event{
			Type:      "edge.unregistered",
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"edge_id": edgeID.String(),
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "deleted",
		"edge_id":   edgeID.String(),
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
