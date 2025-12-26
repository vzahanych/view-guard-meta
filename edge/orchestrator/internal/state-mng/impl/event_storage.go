package impl

import (
	"context"
	"fmt"
	"sort"
	"time"

	aigwtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	"go.uber.org/zap"
)

// saveEvent saves an event to the database
func (m *StateManagerImpl) saveEvent(ctx context.Context, event EventState) error {
	m.operationalMu.Lock()
	defer m.operationalMu.Unlock()

	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage not available")
	}

	// Ensure metadata is initialized
	if event.Metadata == nil {
		event.Metadata = make(map[string]interface{})
	}

	// Convert EventState to map for meta-storage
	eventData := map[string]interface{}{
		"id":            event.ID,
		"camera_id":     event.CameraID,
		"event_type":    event.EventType,
		"timestamp":     event.Timestamp,
		"metadata":      event.Metadata,
		"clip_path":     event.ClipPath,
		"snapshot_path": event.SnapshotPath,
		"transmitted":   event.Transmitted,
	}

	// Save event using meta-storage
	metaStore, ok := m.metaStorage.(metastorage.MetaDataStore)
	if !ok {
		return fmt.Errorf("meta-storage is not a MetaDataStore")
	}

	err := metaStore.SaveSecurityEvent(ctx, event.ID, eventData)
	if err != nil {
		return fmt.Errorf("failed to save event: %w", err)
	}

	// If not transmitted, add to queue (queue is also stored in meta-storage)
	if !event.Transmitted {
		queueKey := "queue_" + event.ID
		queueEntryData := map[string]interface{}{
			"event_id":    event.ID,
			"priority":    0,
			"retry_count": 0,
			"created_at":  time.Now(),
		}
		// Save queue entry using meta-storage (we'll use a special prefix)
		// For now, we'll store queue entries as security events with a special ID
		err = metaStore.SaveSecurityEvent(ctx, queueKey, queueEntryData)
		if err != nil {
			return fmt.Errorf("failed to add event to queue: %w", err)
		}
	}

	return nil
}

// markEventTransmitted marks an event as transmitted
func (m *StateManagerImpl) markEventTransmitted(ctx context.Context, eventID string) error {
	m.operationalMu.Lock()
	defer m.operationalMu.Unlock()

	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage not available")
	}

	metaStore, ok := m.metaStorage.(metastorage.MetaDataStore)
	if !ok {
		return fmt.Errorf("meta-storage is not a MetaDataStore")
	}

	// Get event
	eventData, exists := metaStore.GetSecurityEvent(ctx, eventID)
	if !exists {
		return fmt.Errorf("event %s not found", eventID)
	}

	// Update event
	eventData["transmitted"] = true

	// Save updated event
	err := metaStore.SaveSecurityEvent(ctx, eventID, eventData)
	if err != nil {
		return fmt.Errorf("failed to update event: %w", err)
	}

	// Remove from queue (queue entry has key "queue_" + eventID)
	queueKey := "queue_" + eventID
	metaStore.DeleteSecurityEvent(ctx, queueKey)

	return nil
}

// getPendingEvents retrieves pending events from the queue
func (m *StateManagerImpl) getPendingEvents(ctx context.Context, limit int) ([]EventState, error) {
	m.operationalMu.RLock()
	defer m.operationalMu.RUnlock()

	if m.metaStorage == nil {
		return nil, fmt.Errorf("meta-storage not available")
	}

	metaStore, ok := m.metaStorage.(metastorage.MetaDataStore)
	if !ok {
		return nil, fmt.Errorf("meta-storage is not a MetaDataStore")
	}

	if limit <= 0 {
		limit = 100
	}

	// Get all security events (including queue entries)
	filters := map[string]interface{}{
		"transmitted": false,
	}
	allEvents, err := metaStore.ListSecurityEvents(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list security events: %w", err)
	}

	// Separate actual events from queue entries
	var queueEntries []queueEntry
	var eventMap = make(map[string]EventState)

	for _, eventData := range allEvents {
		eventID, ok := eventData["id"].(string)
		if !ok {
			continue
		}

		// Check if this is a queue entry (starts with "queue_")
		if len(eventID) > 6 && eventID[:6] == "queue_" {
			// This is a queue entry
			entry := queueEntry{}
			if eventIDVal, ok := eventData["event_id"].(string); ok {
				entry.EventID = eventIDVal
			}
			if priorityVal, ok := eventData["priority"].(float64); ok {
				entry.Priority = int(priorityVal)
			}
			if retryVal, ok := eventData["retry_count"].(float64); ok {
				entry.RetryCount = int(retryVal)
			}
			if createdAtVal, ok := eventData["created_at"].(time.Time); ok {
				entry.CreatedAt = createdAtVal
			} else if createdAtStr, ok := eventData["created_at"].(string); ok {
				if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
					entry.CreatedAt = t
				}
			}
			queueEntries = append(queueEntries, entry)
		} else {
			// This is an actual event
			eventState := mapToEventState(eventData)
			eventMap[eventID] = eventState
		}
	}

	// Sort queue entries by priority (desc) and created_at (asc)
	sort.Slice(queueEntries, func(i, j int) bool {
		if queueEntries[i].Priority != queueEntries[j].Priority {
			return queueEntries[i].Priority > queueEntries[j].Priority
		}
		return queueEntries[i].CreatedAt.Before(queueEntries[j].CreatedAt)
	})

	// Build result from queue entries, only including non-transmitted events
	var events []EventState
	for _, qe := range queueEntries {
		if len(events) >= limit {
			break
		}

		if event, exists := eventMap[qe.EventID]; exists {
			// Only include non-transmitted events
			if !event.Transmitted {
				events = append(events, event)
			}
		}
	}

	return events, nil
}

// getEventByID retrieves a single event by ID
func (m *StateManagerImpl) getEventByID(ctx context.Context, eventID string) (*EventState, error) {
	m.operationalMu.RLock()
	defer m.operationalMu.RUnlock()

	if m.metaStorage == nil {
		return nil, fmt.Errorf("meta-storage not available")
	}

	metaStore, ok := m.metaStorage.(metastorage.MetaDataStore)
	if !ok {
		return nil, fmt.Errorf("meta-storage is not a MetaDataStore")
	}

	eventData, exists := metaStore.GetSecurityEvent(ctx, eventID)
	if !exists {
		return nil, nil
	}

	event := mapToEventState(eventData)
	return &event, nil
}

// SaveSecurityEvent saves a security event to storage
func (m *StateManagerImpl) SaveSecurityEvent(ctx context.Context, event *types.SecurityEvent) error {
	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage not available")
	}

	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Convert to EventState
	eventState := securityEventToEventState(event)

	// Save to database
	err := m.saveEvent(ctx, eventState)
	if err != nil {
		return fmt.Errorf("failed to save security event: %w", err)
	}

	m.logger.Debug("Security event saved",
		zap.String("event_id", event.ID),
		zap.String("camera_id", event.CameraID),
		zap.String("event_type", event.EventType),
		zap.Float64("confidence", event.Confidence),
	)

	return nil
}

// EnqueueSecurityEvent enqueues a security event for transmission to VM
func (m *StateManagerImpl) EnqueueSecurityEvent(ctx context.Context, event *types.SecurityEvent, priority int) error {
	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage not available")
	}

	if event == nil {
		return fmt.Errorf("event is nil")
	}

	m.operationalMu.RLock()
	maxSize := m.maxQueueSize
	m.operationalMu.RUnlock()

	// Check queue size limit
	if maxSize > 0 {
		pending, err := m.getPendingEvents(ctx, maxSize+1)
		if err != nil {
			return fmt.Errorf("failed to check queue size: %w", err)
		}
		if len(pending) >= maxSize {
			return fmt.Errorf("security event queue is full: %d/%d", len(pending), maxSize)
		}
	}

	// Save event (this automatically adds it to the queue)
	err := m.SaveSecurityEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to enqueue security event: %w", err)
	}

	// Update priority if specified
	if priority > 0 {
		// TODO: Update queue entry priority
		// For now, priority is set to 0 in saveEvent
	}

	m.logger.Debug("Security event enqueued",
		zap.String("event_id", event.ID),
		zap.Int("priority", priority),
	)
	return nil
}

// GetSecurityEvent retrieves a security event by ID
func (m *StateManagerImpl) GetSecurityEvent(ctx context.Context, eventID string) (*types.SecurityEvent, error) {
	if m.metaStorage == nil {
		return nil, fmt.Errorf("meta-storage not available")
	}

	// Try to get event directly by ID first
	eventState, err := m.getEventByID(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}
	if eventState != nil {
		return eventStateToSecurityEvent(*eventState), nil
	}

	// If not found, search in pending events
	pending, err := m.getPendingEvents(ctx, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	for _, es := range pending {
		if es.ID == eventID {
			return eventStateToSecurityEvent(es), nil
		}
	}

	return nil, fmt.Errorf("security event not found: %s", eventID)
}

// ListSecurityEvents retrieves security events with optional filters
func (m *StateManagerImpl) ListSecurityEvents(ctx context.Context, cameraID string, eventType string, startTime, endTime time.Time, limit int) ([]*types.SecurityEvent, error) {
	if m.metaStorage == nil {
		return nil, fmt.Errorf("meta-storage not available")
	}

	// Build filters for meta-storage
	filters := make(map[string]interface{})
	if cameraID != "" {
		filters["camera_id"] = cameraID
	}
	if eventType != "" {
		filters["event_type"] = eventType
	}
	if !startTime.IsZero() {
		filters["start_time"] = startTime
	}
	if !endTime.IsZero() {
		filters["end_time"] = endTime
	}

	metaStore, ok := m.metaStorage.(metastorage.MetaDataStore)
	if !ok {
		return nil, fmt.Errorf("meta-storage is not a MetaDataStore")
	}

	// Get events from meta-storage
	eventDataList, err := metaStore.ListSecurityEvents(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list security events: %w", err)
	}

	// Convert to SecurityEvent and filter out queue entries
	var events []*types.SecurityEvent
	for _, eventData := range eventDataList {
		eventID, ok := eventData["id"].(string)
		if !ok {
			continue
		}

		// Skip queue entries
		if len(eventID) > 6 && eventID[:6] == "queue_" {
			continue
		}

		eventState := mapToEventState(eventData)
		events = append(events, eventStateToSecurityEvent(eventState))

		// Apply limit
		if limit > 0 && len(events) >= limit {
			break
		}
	}

	return events, nil
}

// GetPendingSecurityEvents returns pending (untransmitted) security events
func (m *StateManagerImpl) GetPendingSecurityEvents(ctx context.Context, limit int) ([]*types.SecurityEvent, error) {
	if m.metaStorage == nil {
		return nil, fmt.Errorf("meta-storage not available")
	}

	pending, err := m.getPendingEvents(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending security events: %w", err)
	}

	events := make([]*types.SecurityEvent, 0, len(pending))
	for _, es := range pending {
		events = append(events, eventStateToSecurityEvent(es))
	}

	return events, nil
}

// Helper functions to convert between SecurityEvent and EventState

// securityEventToEventState converts a SecurityEvent to EventState for storage
func securityEventToEventState(e *types.SecurityEvent) EventState {
	// Build metadata JSON
	metadata := make(map[string]interface{})

	// Copy existing metadata
	for k, v := range e.Metadata {
		metadata[k] = v
	}

	// Add detection-specific metadata
	metadata["confidence"] = e.Confidence
	if e.BoundingBox != nil {
		metadata["bounding_box"] = map[string]interface{}{
			"x1":         e.BoundingBox.X1,
			"y1":         e.BoundingBox.Y1,
			"x2":         e.BoundingBox.X2,
			"y2":         e.BoundingBox.Y2,
			"class_id":   e.BoundingBox.ClassID,
			"class_name": e.BoundingBox.ClassName,
		}
	}
	metadata["frame_width"] = 0  // Will be set by generator if available
	metadata["frame_height"] = 0 // Will be set by generator if available

	return EventState{
		ID:           e.ID,
		CameraID:     e.CameraID,
		EventType:    e.EventType,
		Timestamp:    e.Timestamp,
		Metadata:     metadata,
		ClipPath:     e.ClipPath,
		SnapshotPath: e.SnapshotPath,
		Transmitted:  false,
	}
}

// eventStateToSecurityEvent creates a SecurityEvent from EventState
func eventStateToSecurityEvent(es EventState) *types.SecurityEvent {
	event := &types.SecurityEvent{
		ID:           es.ID,
		CameraID:     es.CameraID,
		EventType:    es.EventType,
		Timestamp:    es.Timestamp,
		ClipPath:     es.ClipPath,
		SnapshotPath: es.SnapshotPath,
		Metadata:     make(map[string]interface{}),
	}

	// Extract metadata
	if es.Metadata != nil {
		// Copy metadata
		for k, v := range es.Metadata {
			event.Metadata[k] = v
		}

		// Extract confidence
		if conf, ok := es.Metadata["confidence"].(float64); ok {
			event.Confidence = conf
		}

		// Extract bounding box
		if bboxMap, ok := es.Metadata["bounding_box"].(map[string]interface{}); ok {
			bbox := &aigwtypes.BoundingBox{}
			if x1, ok := bboxMap["x1"].(float64); ok {
				bbox.X1 = x1
			}
			if y1, ok := bboxMap["y1"].(float64); ok {
				bbox.Y1 = y1
			}
			if x2, ok := bboxMap["x2"].(float64); ok {
				bbox.X2 = x2
			}
			if y2, ok := bboxMap["y2"].(float64); ok {
				bbox.Y2 = y2
			}
			if classID, ok := bboxMap["class_id"].(float64); ok {
				bbox.ClassID = int(classID)
			}
			if className, ok := bboxMap["class_name"].(string); ok {
				bbox.ClassName = className
			}
			if conf, ok := bboxMap["confidence"].(float64); ok {
				bbox.Confidence = conf
			}
			event.BoundingBox = bbox
		}
	}

	return event
}

// mapToEventState converts a map from meta-storage to EventState
func mapToEventState(eventData map[string]interface{}) EventState {
	event := EventState{
		Metadata: make(map[string]interface{}),
	}

	if id, ok := eventData["id"].(string); ok {
		event.ID = id
	}
	if cameraID, ok := eventData["camera_id"].(string); ok {
		event.CameraID = cameraID
	}
	if eventType, ok := eventData["event_type"].(string); ok {
		event.EventType = eventType
	}
	if timestamp, ok := eventData["timestamp"].(time.Time); ok {
		event.Timestamp = timestamp
	} else if timestampStr, ok := eventData["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			event.Timestamp = t
		}
	}
	if clipPath, ok := eventData["clip_path"].(string); ok {
		event.ClipPath = clipPath
	}
	if snapshotPath, ok := eventData["snapshot_path"].(string); ok {
		event.SnapshotPath = snapshotPath
	}
	if transmitted, ok := eventData["transmitted"].(bool); ok {
		event.Transmitted = transmitted
	}
	if metadata, ok := eventData["metadata"].(map[string]interface{}); ok {
		event.Metadata = metadata
	}

	return event
}

// syncPendingSecurityEvents syncs pending security events to VM when connection is restored
// This is called after authentication to ensure events queued during disconnection are transmitted
func (m *StateManagerImpl) syncPendingSecurityEvents(ctx context.Context) {
	if m.vmGateway == nil {
		m.logger.Debug("VM gateway not available, skipping security event sync")
		return
	}

	// Check if connection is ready
	if !m.vmGateway.IsHTTPConnected() {
		m.logger.Debug("HTTP connection not ready, skipping security event sync")
		return
	}

	// Get pending events (batch of 100 at a time)
	pending, err := m.GetPendingSecurityEvents(ctx, 100)
	if err != nil {
		m.logger.Warn("Failed to get pending security events for sync",
			zap.Error(err),
		)
		return
	}

	if len(pending) == 0 {
		m.logger.Debug("No pending security events to sync")
		return
	}

	m.logger.Info("Syncing pending security events to VM",
		zap.Int("count", len(pending)),
	)

	// Mark events as transmitted (they will be synced by the normal event transmission flow)
	// The actual transmission to VM should be handled by the event transmission system
	for _, event := range pending {
		if err := m.markEventTransmitted(ctx, event.ID); err != nil {
			m.logger.Warn("Failed to mark event as transmitted",
				zap.String("event_id", event.ID),
				zap.Error(err),
			)
		}
	}

	m.logger.Info("Pending security events marked for transmission",
		zap.Int("count", len(pending)),
	)
}
