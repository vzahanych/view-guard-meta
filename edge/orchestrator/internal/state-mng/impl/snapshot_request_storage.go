package impl

import (
	"context"
	"fmt"
	"time"

	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	statetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	"go.uber.org/zap"
)

// SavePendingSnapshotRequest saves a pending snapshot request from VM
func (m *StateManagerImpl) SavePendingSnapshotRequest(ctx context.Context, cameraID string, label string, customLabel string, count int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage not available")
	}

	metaStore, ok := m.metaStorage.(metastorage.MetaDataStore)
	if !ok {
		return fmt.Errorf("meta-storage is not a MetaDataStore")
	}

	request := &statetypes.PendingSnapshotRequest{
		CameraID:    cameraID,
		Label:       label,
		CustomLabel: customLabel,
		Count:       count,
		RequestedAt: time.Now(),
	}

	// Convert to map for meta-storage
	requestData := map[string]interface{}{
		"camera_id":    request.CameraID,
		"label":        request.Label,
		"custom_label": request.CustomLabel,
		"count":        request.Count,
		"requested_at": request.RequestedAt,
	}

	// Save to meta-storage
	err := metaStore.SavePendingSnapshotRequest(ctx, cameraID, requestData)
	if err != nil {
		return fmt.Errorf("failed to save pending snapshot request: %w", err)
	}

	// Also keep in memory for fast access
	m.pendingSnapshotRequests[cameraID] = request

	m.logger.Debug("Pending snapshot request saved", zap.String("camera_id", cameraID), zap.String("label", label), zap.Int32("count", count))
	return nil
}

// GetPendingSnapshotRequest retrieves a pending snapshot request for a camera
func (m *StateManagerImpl) GetPendingSnapshotRequest(ctx context.Context, cameraID string) (*statetypes.PendingSnapshotRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.metaStorage == nil {
		return nil, fmt.Errorf("meta-storage not available")
	}

	// Try memory first
	if request, ok := m.pendingSnapshotRequests[cameraID]; ok {
		// Return a copy to avoid race conditions
		return &statetypes.PendingSnapshotRequest{
			CameraID:    request.CameraID,
			Label:       request.Label,
			CustomLabel: request.CustomLabel,
			Count:       request.Count,
			RequestedAt: request.RequestedAt,
		}, nil
	}

	// Fall back to meta-storage
	metaStore, ok := m.metaStorage.(metastorage.MetaDataStore)
	if !ok {
		return nil, fmt.Errorf("meta-storage is not a MetaDataStore")
	}

	requestData, exists := metaStore.GetPendingSnapshotRequest(ctx, cameraID)
	if !exists {
		return nil, nil
	}

	request := mapToPendingSnapshotRequest(requestData)
	return &request, nil
}

// GetAllPendingSnapshotRequests retrieves all pending snapshot requests
func (m *StateManagerImpl) GetAllPendingSnapshotRequests(ctx context.Context) (map[string]*statetypes.PendingSnapshotRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.metaStorage == nil {
		return nil, fmt.Errorf("meta-storage not available")
	}

	metaStore, ok := m.metaStorage.(metastorage.MetaDataStore)
	if !ok {
		return nil, fmt.Errorf("meta-storage is not a MetaDataStore")
	}

	// Load from meta-storage
	requestDataList, err := metaStore.ListPendingSnapshotRequests(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all pending snapshot requests: %w", err)
	}

	result := make(map[string]*statetypes.PendingSnapshotRequest)
	for _, requestData := range requestDataList {
		cameraID, ok := requestData["camera_id"].(string)
		if !ok {
			continue
		}

		request := mapToPendingSnapshotRequest(requestData)
		result[cameraID] = &request
	}

	// Update memory cache
	m.pendingSnapshotRequests = result

	return result, nil
}

// ClearPendingSnapshotRequest clears a pending snapshot request for a camera
func (m *StateManagerImpl) ClearPendingSnapshotRequest(ctx context.Context, cameraID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage not available")
	}

	metaStore, ok := m.metaStorage.(metastorage.MetaDataStore)
	if !ok {
		return fmt.Errorf("meta-storage is not a MetaDataStore")
	}

	// Remove from meta-storage
	err := metaStore.DeletePendingSnapshotRequest(ctx, cameraID)
	if err != nil {
		return fmt.Errorf("failed to delete pending snapshot request: %w", err)
	}

	// Remove from memory
	delete(m.pendingSnapshotRequests, cameraID)

	m.logger.Debug("Pending snapshot request cleared", zap.String("camera_id", cameraID))
	return nil
}

// mapToPendingSnapshotRequest converts a map from meta-storage to PendingSnapshotRequest
func mapToPendingSnapshotRequest(requestData map[string]interface{}) statetypes.PendingSnapshotRequest {
	request := statetypes.PendingSnapshotRequest{}

	if cameraID, ok := requestData["camera_id"].(string); ok {
		request.CameraID = cameraID
	}
	if label, ok := requestData["label"].(string); ok {
		request.Label = label
	}
	if customLabel, ok := requestData["custom_label"].(string); ok {
		request.CustomLabel = customLabel
	}
	if count, ok := requestData["count"].(float64); ok {
		request.Count = int32(count)
	} else if count, ok := requestData["count"].(int32); ok {
		request.Count = count
	}
	if requestedAt, ok := requestData["requested_at"].(time.Time); ok {
		request.RequestedAt = requestedAt
	} else if requestedAtStr, ok := requestData["requested_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, requestedAtStr); err == nil {
			request.RequestedAt = t
		}
	}

	return request
}
