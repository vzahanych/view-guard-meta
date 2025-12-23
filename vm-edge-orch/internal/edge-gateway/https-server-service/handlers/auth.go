package handlers

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventbus "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage"
	edgeproto "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// AuthHandler handles Edge device authentication requests.
type AuthHandler struct {
	metaStore metastorage.MetaDataStore
	eventBus  eventbus.EventBus
	logger    *zap.Logger
}

// NewAuthHandler creates a new authentication handler.
func NewAuthHandler(metaStore metastorage.MetaDataStore, eventBus eventbus.EventBus, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		metaStore: metaStore,
		eventBus:  eventBus,
		logger:    logger,
	}
}

// HandleAuthenticate handles POST /api/v1/auth/authenticate - Edge authentication request.
func (h *AuthHandler) HandleAuthenticate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, edgeproto.AuthErrorCode_AUTH_ERROR_INVALID_REQUEST, fmt.Sprintf("failed to read request body: %v", err))
		return
	}

	// Decode authentication request from JSON (protojson supports JSON encoding)
	req := &edgeproto.AuthRequest{}
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
	if err := unmarshaler.Unmarshal(body, req); err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, edgeproto.AuthErrorCode_AUTH_ERROR_INVALID_REQUEST, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Validate required fields
	if req.EdgeId == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, edgeproto.AuthErrorCode_AUTH_ERROR_INVALID_REQUEST, "edge_id is required")
		return
	}

	// Parse edge ID as UUID
	edgeID, err := uuid.Parse(req.EdgeId)
	if err != nil {
		h.sendErrorResponse(w, http.StatusBadRequest, edgeproto.AuthErrorCode_AUTH_ERROR_INVALID_REQUEST, fmt.Sprintf("invalid edge_id format: %v", err))
		return
	}

	if req.WgPublicKey == "" {
		h.sendErrorResponse(w, http.StatusBadRequest, edgeproto.AuthErrorCode_AUTH_ERROR_INVALID_REQUEST, "wg_public_key is required")
		return
	}

	// Check if edge exists in meta-storage
	edgeState, exists := h.metaStore.GetEdge(edgeID)
	if !exists {
		if h.logger != nil {
			h.logger.Warn("Edge authentication failed: edge not found",
				zap.String("edge_id", edgeID.String()))
		}
		h.sendErrorResponse(w, http.StatusUnauthorized, edgeproto.AuthErrorCode_AUTH_ERROR_EDGE_NOT_FOUND, "edge not found")
		return
	}

	// Verify WireGuard public key matches registered key
	if edgeState.WGPublicKey != req.WgPublicKey {
		if h.logger != nil {
			h.logger.Warn("Edge authentication failed: invalid WireGuard key",
				zap.String("edge_id", edgeID.String()),
				zap.String("expected_key", edgeState.WGPublicKey),
				zap.String("provided_key", req.WgPublicKey))
		}
		h.sendErrorResponse(w, http.StatusUnauthorized, edgeproto.AuthErrorCode_AUTH_ERROR_INVALID_WG_KEY, "invalid WireGuard public key")
		return
	}

	// Check if edge is already authenticated
	if edgeState.Status == metastorage.EdgeStatusAuthenticated {
		if h.logger != nil {
			h.logger.Info("Edge already authenticated, returning success",
				zap.String("edge_id", edgeID.String()))
		}
		// Return success even if already authenticated (idempotent)
		h.sendSuccessResponse(w, edgeID, []string{"start_heartbeat", "sync_iot_devices"})
		return
	}

	// Update edge state to authenticated
	updatedState, err := h.metaStore.UpdateEdge(edgeID, func(current metastorage.EdgeState) metastorage.EdgeState {
		current.Status = metastorage.EdgeStatusAuthenticated
		// Store edge version if provided
		if req.EdgeVersion != "" {
			if current.Metadata == nil {
				current.Metadata = make(map[string]string)
			}
			current.Metadata["edge_version"] = req.EdgeVersion
		}
		// Store OS info if provided
		if req.DeviceInfo != nil {
			if current.Metadata == nil {
				current.Metadata = make(map[string]string)
			}
			if req.DeviceInfo.Os != "" {
				current.Metadata["os"] = req.DeviceInfo.Os
			}
			if req.DeviceInfo.OsVersion != "" {
				current.Metadata["os_version"] = req.DeviceInfo.OsVersion
			}
			if req.DeviceInfo.SerialNumber != "" {
				current.Metadata["serial_number"] = req.DeviceInfo.SerialNumber
			}
		}
		// Store capabilities if provided
		if len(req.Capabilities) > 0 {
			if current.Metadata == nil {
				current.Metadata = make(map[string]string)
			}
			// Store capabilities as comma-separated string (or could use JSON)
			capabilitiesStr := ""
			for i, cap := range req.Capabilities {
				if i > 0 {
					capabilitiesStr += ","
				}
				capabilitiesStr += cap
			}
			current.Metadata["capabilities"] = capabilitiesStr
		}
		return current
	})

	if err != nil {
		if h.logger != nil {
			h.logger.Error("Failed to update edge state after authentication",
				zap.Error(err),
				zap.String("edge_id", edgeID.String()))
		}
		h.sendErrorResponse(w, http.StatusInternalServerError, edgeproto.AuthErrorCode_AUTH_ERROR_INTERNAL_SERVER_ERROR, "failed to update edge state")
		return
	}

	// Publish edge authenticated event
	if h.eventBus != nil {
		h.eventBus.Publish(eventbus.Event{
			Type:      "edge.authenticated",
			Source:    "https-server",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"edge_id":      edgeID.String(),
				"edge_version": req.EdgeVersion,
				"capabilities": req.Capabilities,
			},
		})
	}

	if h.logger != nil {
		h.logger.Info("Edge authenticated successfully",
			zap.String("edge_id", edgeID.String()),
			zap.String("edge_version", req.EdgeVersion),
			zap.String("status", string(updatedState.Status)))
	}

	// Return success response with next steps
	nextSteps := []string{"start_heartbeat", "sync_iot_devices"}
	h.sendSuccessResponse(w, edgeID, nextSteps)
}

// sendSuccessResponse sends a successful authentication response.
func (h *AuthHandler) sendSuccessResponse(w http.ResponseWriter, edgeID uuid.UUID, nextSteps []string) {
	resp := &edgeproto.AuthResponse{
		Success:         true,
		EdgeId:          edgeID.String(),
		Message:         "Edge authenticated successfully",
		ServerTimestamp: timestamppb.Now(),
		NextSteps:       nextSteps,
		ErrorCode:       edgeproto.AuthErrorCode_AUTH_ERROR_UNSPECIFIED,
	}

	w.WriteHeader(http.StatusOK)
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   false, // Use JSON field names (camelCase)
	}
	if data, err := marshaler.Marshal(resp); err != nil {
		if h.logger != nil {
			h.logger.Error("Failed to marshal authentication response", zap.Error(err))
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	} else {
		w.Write(data)
	}
}

// sendErrorResponse sends an error authentication response.
func (h *AuthHandler) sendErrorResponse(w http.ResponseWriter, statusCode int, errorCode edgeproto.AuthErrorCode, message string) {
	resp := &edgeproto.AuthResponse{
		Success:         false,
		EdgeId:          "", // Empty on error
		Message:         message,
		ErrorCode:       errorCode,
		ServerTimestamp: timestamppb.Now(),
	}

	w.WriteHeader(statusCode)
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   false, // Use JSON field names (camelCase)
	}
	if data, err := marshaler.Marshal(resp); err != nil {
		if h.logger != nil {
			h.logger.Error("Failed to marshal error response", zap.Error(err))
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	} else {
		w.Write(data)
	}
}
