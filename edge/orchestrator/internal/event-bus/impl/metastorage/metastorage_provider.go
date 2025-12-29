package metastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	metastoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	"go.uber.org/zap"
)

// MetaStorageProvider implements the EventBusProvider interface using meta-storage.
// This provider uses the refactored meta-storage API with structured types.
type MetaStorageProvider struct {
	metaStorage metastorage.MetaDataStore
	logger      *zap.Logger
}

// NewMetaStorageProvider creates a new meta-storage provider for event bus.
func NewMetaStorageProvider(metaStorage metastorage.MetaDataStore, logger *zap.Logger) (*MetaStorageProvider, error) {
	if metaStorage == nil {
		return nil, fmt.Errorf("meta-storage is required")
	}

	return &MetaStorageProvider{
		metaStorage: metaStorage,
		logger:      logger,
	}, nil
}

// PersistEvent persists an event to meta-storage using structured types.
func (p *MetaStorageProvider) PersistEvent(ctx context.Context, event types.EventAny) error {
	// Generate a unique event ID based on timestamp
	eventID := generateEventID(event.Timestamp)

	// Unmarshal event data from json.RawMessage to map for storage
	var dataMap map[string]interface{}
	if len(event.Data) > 0 {
		if err := json.Unmarshal(event.Data, &dataMap); err != nil {
			p.logger.Warn("Failed to unmarshal event data for storage, storing as empty map",
				zap.String("event_id", eventID),
				zap.Error(err))
			dataMap = make(map[string]interface{})
		}
	} else {
		dataMap = make(map[string]interface{})
	}

	// Store source in data map (EventBusEventMetadata doesn't have a Source field)
	dataMap["_source"] = event.Source

	// Store sequence number in data map if present
	if event.SequenceNumber > 0 {
		dataMap["_sequence_number"] = event.SequenceNumber
	}

	// Create structured event metadata
	now := time.Now()
	eventMetadata := metastoragetypes.EventBusEventMetadata{
		EventID:          eventID,
		EventType:        string(event.Type),
		Timestamp:        event.Timestamp,
		Data:             dataMap,
		ProcessingStatus: string(types.EventStatusPending),
		RetryCount:       0,
		LastError:        "",
		NextRetryTime:    nil,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// Store event in meta-storage
	return p.metaStorage.SaveEvent(ctx, eventMetadata)
}

// LoadEvent loads an event by event ID from meta-storage.
func (p *MetaStorageProvider) LoadEvent(ctx context.Context, eventID string) (*types.EventAny, error) {
	eventMetadata, exists := p.metaStorage.GetEvent(ctx, eventID)
	if !exists {
		return nil, nil // Event not found
	}

	// Convert EventBusEventMetadata to EventAny
	event, err := p.metadataToEvent(*eventMetadata)
	if err != nil {
		return nil, fmt.Errorf("failed to convert event metadata: %w", err)
	}

	return &event, nil
}

// ListEvents lists events matching the provided filters.
func (p *MetaStorageProvider) ListEvents(ctx context.Context, filters *types.EventFilters) ([]types.EventAny, error) {
	// Convert EventFilters to EventBusFilters
	eventBusFilters := &metastoragetypes.EventBusFilters{}
	if filters != nil {
		if filters.EventType != nil {
			eventTypeStr := string(*filters.EventType)
			eventBusFilters.EventType = &eventTypeStr
		}
		if filters.ProcessingStatus != nil {
			statusStr := string(*filters.ProcessingStatus)
			eventBusFilters.ProcessingStatus = &statusStr
		}
		if filters.From != nil {
			eventBusFilters.From = filters.From
		}
		if filters.To != nil {
			eventBusFilters.To = filters.To
		}
		if filters.Limit != nil {
			eventBusFilters.Limit = filters.Limit
		}
	}

	// Query events from meta-storage
	eventMetadatas, err := p.metaStorage.ListEvents(ctx, eventBusFilters)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Convert EventBusEventMetadata to EventAny
	events := make([]types.EventAny, 0, len(eventMetadatas))
	for _, eventMetadata := range eventMetadatas {
		event, err := p.metadataToEvent(eventMetadata)
		if err != nil {
			p.logger.Warn("Failed to convert event metadata to Event",
				zap.String("event_id", eventMetadata.EventID),
				zap.Error(err))
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// DeleteEvent deletes an event by event ID.
func (p *MetaStorageProvider) DeleteEvent(ctx context.Context, eventID string) error {
	return p.metaStorage.DeleteEvent(ctx, eventID)
}

// DeleteExpiredEvents deletes events that expired before the specified time.
func (p *MetaStorageProvider) DeleteExpiredEvents(ctx context.Context, beforeTime time.Time) (int, error) {
	// Query events before the specified time
	filters := &metastoragetypes.EventBusFilters{
		To: &beforeTime,
	}

	eventMetadatas, err := p.metaStorage.ListEvents(ctx, filters)
	if err != nil {
		return 0, fmt.Errorf("failed to list expired events: %w", err)
	}

	// Delete events in batches
	deletedCount := 0
	for _, eventMetadata := range eventMetadatas {
		if err := p.metaStorage.DeleteEvent(ctx, eventMetadata.EventID); err != nil {
			p.logger.Warn("Failed to delete expired event",
				zap.String("event_id", eventMetadata.EventID),
				zap.Error(err))
			continue
		}
		deletedCount++
	}

	return deletedCount, nil
}

// GetEventCount returns the total number of events in storage.
func (p *MetaStorageProvider) GetEventCount(ctx context.Context) (int, error) {
	return p.metaStorage.GetEventCount(ctx)
}

// HealthCheck performs a health check on the provider.
func (p *MetaStorageProvider) HealthCheck(ctx context.Context) error {
	// Check meta-storage health via HealthSnapshot
	health := p.metaStorage.HealthSnapshot(ctx)
	if health.Status == metastoragetypes.HealthStatusCorrupted {
		return fmt.Errorf("meta-storage is corrupted")
	}
	return nil
}

// Close closes the provider and releases all resources.
func (p *MetaStorageProvider) Close() error {
	// Meta-storage provider doesn't require explicit closing
	// The meta-storage service manages its own lifecycle
	return nil
}

// metadataToEvent converts EventBusEventMetadata to EventAny.
func (p *MetaStorageProvider) metadataToEvent(eventMetadata metastoragetypes.EventBusEventMetadata) (types.EventAny, error) {
	event := types.EventAny{}

	// Extract type
	event.Type = types.EventType(eventMetadata.EventType)

	// Extract source from data map (stored as _source)
	if source, ok := eventMetadata.Data["_source"].(string); ok {
		event.Source = source
		// Remove _source from data map to avoid exposing it
		delete(eventMetadata.Data, "_source")
	} else {
		return event, fmt.Errorf("missing or invalid source field in event data")
	}

	// Extract timestamp
	event.Timestamp = eventMetadata.Timestamp

	// Extract sequence number from data map if present
	if seqNum, ok := eventMetadata.Data["_sequence_number"].(float64); ok {
		event.SequenceNumber = int64(seqNum)
		delete(eventMetadata.Data, "_sequence_number")
	} else if seqNum, ok := eventMetadata.Data["_sequence_number"].(int64); ok {
		event.SequenceNumber = seqNum
		delete(eventMetadata.Data, "_sequence_number")
	} else if seqNum, ok := eventMetadata.Data["_sequence_number"].(int); ok {
		event.SequenceNumber = int64(seqNum)
		delete(eventMetadata.Data, "_sequence_number")
	}

	// Extract data as JSON (excluding internal fields)
	dataBytes, err := json.Marshal(eventMetadata.Data)
	if err != nil {
		return event, fmt.Errorf("failed to marshal event data: %w", err)
	}
	event.Data = json.RawMessage(dataBytes)

	return event, nil
}

// generateEventID generates a unique event ID based on timestamp.
// Format: <timestamp_nanos>_<micros>
func generateEventID(timestamp time.Time) string {
	nanos := timestamp.UnixNano()
	micros := time.Now().UnixMicro()
	return fmt.Sprintf("%d_%d", nanos, micros)
}

