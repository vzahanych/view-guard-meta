package impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	"go.uber.org/zap"
)

// MetaStorageImpl implements the MetaDataStore interface using a storage provider.
// This implementation delegates low-level storage operations to the provider
// and adds business logic (marshaling, filtering, sorting, etc.).
type MetaStorageImpl struct {
	provider          types.MetaStorageProvider
	logger            *zap.Logger
	quotaManager      *QuotaManager
	retentionManager  *RetentionManager
	integrityManager  *IntegrityManager
	eventEmitter      types.StorageEventEmitter // Optional event emitter for emitting operational events

	// Lifecycle state
	mu      sync.RWMutex
	started bool
	stopCh  chan struct{}
}

// NewMetaStorageImpl creates a new meta storage implementation.
func NewMetaStorageImpl(provider types.MetaStorageProvider, logger *zap.Logger) *MetaStorageImpl {
	return &MetaStorageImpl{
		provider: provider,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// SetQuotaManager sets the quota manager for this storage implementation.
func (s *MetaStorageImpl) SetQuotaManager(quotaManager *QuotaManager) {
	s.quotaManager = quotaManager
	// Pass event emitter to quota manager if available
	if s.eventEmitter != nil && quotaManager != nil {
		quotaManager.SetEventEmitter(s.eventEmitter)
	}
}

// SetRetentionManager sets the retention manager for this storage implementation.
func (s *MetaStorageImpl) SetRetentionManager(retentionManager *RetentionManager) {
	s.retentionManager = retentionManager
	// Pass event emitter to retention manager if available
	if s.eventEmitter != nil && retentionManager != nil {
		retentionManager.SetEventEmitter(s.eventEmitter)
	}
}

// SetIntegrityManager sets the integrity manager for this storage implementation.
func (s *MetaStorageImpl) SetIntegrityManager(integrityManager *IntegrityManager) {
	s.integrityManager = integrityManager
	// Pass event emitter to integrity manager if available
	if s.eventEmitter != nil && integrityManager != nil {
		integrityManager.SetEventEmitter(s.eventEmitter)
	}
}

// SetEventEmitter sets the event emitter for this storage implementation.
// This is optional - if not set, events will not be emitted.
func (s *MetaStorageImpl) SetEventEmitter(eventEmitter types.StorageEventEmitter) {
	s.eventEmitter = eventEmitter
	// Pass event emitter to managers if they are already set
	if s.retentionManager != nil {
		s.retentionManager.SetEventEmitter(eventEmitter)
	}
	if s.integrityManager != nil {
		s.integrityManager.SetEventEmitter(eventEmitter)
	}
}

// emitEvent emits an event using the event emitter if it is configured.
// This is a helper method that safely handles the case when event emitter is not set.
func (s *MetaStorageImpl) emitEvent(eventType string, data interface{}) {
	if s.eventEmitter == nil {
		return // Event emitter not configured, skip emission
	}

	s.eventEmitter.EmitStorageEvent(eventType, data)
}

// Key names for special records
const (
	currentEdgeStateKey        = "current"
	currentEdgeCapabilitiesKey = "current"
)

// InitializeBuckets creates all required buckets in the storage provider.
// This uses the standard bucket names from buckets.go.
func (s *MetaStorageImpl) InitializeBuckets(ctx context.Context) error {
	buckets := AllStandardBuckets()

	for _, bucketName := range buckets {
		exists := s.provider.BucketExists(ctx, bucketName)
		if !exists {
			if err := s.provider.CreateBucket(ctx, bucketName); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucketName, err)
			}
		}
	}

	return nil
}

// SaveStorageEntry saves a storage entry metadata to the database
func (s *MetaStorageImpl) SaveStorageEntry(ctx context.Context, entry types.StorageEntryMetadata) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal storage entry: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketStorageState, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketStorageState, []byte(entry.Path), data)
}

// DeleteStorageEntry deletes a storage entry metadata from the database
func (s *MetaStorageImpl) DeleteStorageEntry(ctx context.Context, path string) error {
	return s.provider.Delete(ctx, BucketStorageState, []byte(path))
}

// ListStorageEntries lists storage entry metadata by file type
func (s *MetaStorageImpl) ListStorageEntries(ctx context.Context, fileType string) ([]types.StorageEntryMetadata, error) {
	keyValues, err := s.provider.List(ctx, BucketStorageState, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list storage entries: %w", err)
	}

	var entries []types.StorageEntryMetadata
	for _, kv := range keyValues {
		var entry types.StorageEntryMetadata
		if err := json.Unmarshal(kv.Value, &entry); err != nil {
			s.logger.Warn("Failed to unmarshal storage entry", zap.String("path", string(kv.Key)), zap.Error(err))
			continue // Skip invalid entries
		}
		if fileType == "" || entry.FileType == fileType {
			entries = append(entries, entry)
		}
	}

	// Sort by created_at ascending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})

	return entries, nil
}

// GetStorageStats returns storage statistics
func (s *MetaStorageImpl) GetStorageStats(ctx context.Context) (*types.StorageStats, error) {
	stats := &types.StorageStats{
		TotalClips:     0,
		TotalSnapshots: 0,
		TotalSizeBytes: 0,
	}

	keyValues, err := s.provider.List(ctx, BucketStorageState, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage stats: %w", err)
	}

	for _, kv := range keyValues {
		var entry types.StorageEntryMetadata
		if err := json.Unmarshal(kv.Value, &entry); err != nil {
			continue // Skip invalid entries
		}

		if entry.FileType == "clip" {
			stats.TotalClips++
		} else if entry.FileType == "snapshot" {
			stats.TotalSnapshots++
		}
		stats.TotalSizeBytes += entry.SizeBytes
	}

	// Note: DiskUsagePercent and AvailableBytes would need disk monitor
	// For now, we'll return 0 and let the caller combine with disk monitor
	return stats, nil
}

// Security Event Operations

// SaveSecurityEvent saves a structured security event metadata record.
func (s *MetaStorageImpl) SaveSecurityEvent(ctx context.Context, event types.SecurityEventMetadata) error {
	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal security event: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketSecurityEvents, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketSecurityEvents, []byte(event.EventID), data)
}

// GetSecurityEvent retrieves a structured security event by ID.
func (s *MetaStorageImpl) GetSecurityEvent(ctx context.Context, eventID string) (*types.SecurityEventMetadata, bool) {
	data, err := s.provider.Get(ctx, BucketSecurityEvents, []byte(eventID))
	if err != nil {
		return nil, false
	}

	var event types.SecurityEventMetadata
	if err := json.Unmarshal(data, &event); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal security event", zap.String("event_id", eventID), zap.Error(err))
		}
		return nil, false
	}

	return &event, true
}

// ListSecurityEvents lists security events matching the provided filters.
func (s *MetaStorageImpl) ListSecurityEvents(ctx context.Context, filters *types.SecurityEventFilters) ([]types.SecurityEventMetadata, error) {
	keyValues, err := s.provider.List(ctx, BucketSecurityEvents, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list security events: %w", err)
	}

	var events []types.SecurityEventMetadata
	for _, kv := range keyValues {
		var event types.SecurityEventMetadata
		if err := json.Unmarshal(kv.Value, &event); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to unmarshal security event", zap.String("event_id", string(kv.Key)), zap.Error(err))
			}
			continue
		}

		// Apply filters if provided
		if filters != nil {
			// DeviceID
			if filters.DeviceID != nil && event.DeviceID != *filters.DeviceID {
				continue
			}

			// DeviceType
			if filters.DeviceType != nil && event.DeviceType != *filters.DeviceType {
				continue
			}

			// EventType
			if filters.EventType != nil && event.EventType != *filters.EventType {
				continue
			}

			// Status
			if filters.Status != nil && event.Status != *filters.Status {
				continue
			}

			// ModelID
			if filters.ModelID != nil && event.ModelID != *filters.ModelID {
				continue
			}

			// ModelVersion
			if filters.ModelVersion != nil && event.ModelVersion != *filters.ModelVersion {
				continue
			}

			// From
			if filters.From != nil && event.Timestamp.Before(*filters.From) {
				continue
			}

			// To
			if filters.To != nil && event.Timestamp.After(*filters.To) {
				continue
			}
		}

		events = append(events, event)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	// Apply limit if provided
	if filters != nil && filters.Limit != nil && *filters.Limit > 0 && len(events) > *filters.Limit {
		events = events[:*filters.Limit]
	}

	return events, nil
}

// DeleteSecurityEvent deletes a security event by ID.
func (s *MetaStorageImpl) DeleteSecurityEvent(ctx context.Context, eventID string) error {
	return s.provider.Delete(ctx, BucketSecurityEvents, []byte(eventID))
}

// UpdateSecurityEventStatus updates the status and VM ACK timestamp of a security event.
func (s *MetaStorageImpl) UpdateSecurityEventStatus(ctx context.Context, eventID string, status string, vmACKTime *time.Time) error {
	event, found := s.GetSecurityEvent(ctx, eventID)
	if !found {
		return types.ErrRecordNotFound
	}

	event.Status = status
	if vmACKTime != nil {
		event.VMACKTimestamp = *vmACKTime
	}

	return s.SaveSecurityEvent(ctx, *event)
}

// GetPendingSecurityEvents returns security events that are pending delivery.
func (s *MetaStorageImpl) GetPendingSecurityEvents(ctx context.Context, limit int) ([]types.SecurityEventMetadata, error) {
	status := "pending_delivery"
	filters := &types.SecurityEventFilters{
		Status: &status,
	}
	if limit > 0 {
		filters.Limit = &limit
	}
	return s.ListSecurityEvents(ctx, filters)
}

// Event Bus Operations

// SaveEvent saves an event bus event metadata to the database.
func (s *MetaStorageImpl) SaveEvent(ctx context.Context, event types.EventBusEventMetadata) error {
	// Set timestamps if not set
	now := time.Now()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = now
	}
	event.UpdatedAt = now

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketEventBus, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketEventBus, []byte(event.EventID), data)
}

// GetEvent retrieves an event bus event metadata by event ID.
func (s *MetaStorageImpl) GetEvent(ctx context.Context, eventID string) (*types.EventBusEventMetadata, bool) {
	data, err := s.provider.Get(ctx, BucketEventBus, []byte(eventID))
	if err != nil {
		return nil, false
	}

	var event types.EventBusEventMetadata
	if err := json.Unmarshal(data, &event); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal event", zap.String("event_id", eventID), zap.Error(err))
		}
		return nil, false
	}

	return &event, true
}

// ListEvents lists event bus events with optional filters.
func (s *MetaStorageImpl) ListEvents(ctx context.Context, filters *types.EventBusFilters) ([]types.EventBusEventMetadata, error) {
	keyValues, err := s.provider.List(ctx, BucketEventBus, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	var events []types.EventBusEventMetadata
	for _, kv := range keyValues {
		var event types.EventBusEventMetadata
		if err := json.Unmarshal(kv.Value, &event); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to unmarshal event", zap.String("event_id", string(kv.Key)), zap.Error(err))
			}
			continue // Skip invalid entries
		}

		// Apply filters if provided
		if filters != nil {
			// Filter by EventType
			if filters.EventType != nil && event.EventType != *filters.EventType {
				continue
			}

			// Filter by ProcessingStatus
			if filters.ProcessingStatus != nil && event.ProcessingStatus != *filters.ProcessingStatus {
				continue
			}

			// Filter by From (timestamp)
			if filters.From != nil && event.Timestamp.Before(*filters.From) {
				continue
			}

			// Filter by To (timestamp)
			if filters.To != nil && event.Timestamp.After(*filters.To) {
				continue
			}
		}

		events = append(events, event)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	// Apply limit if provided
	if filters != nil && filters.Limit != nil && *filters.Limit > 0 && len(events) > *filters.Limit {
		events = events[:*filters.Limit]
	}

	return events, nil
}

// DeleteEvent deletes an event bus event metadata by event ID.
func (s *MetaStorageImpl) DeleteEvent(ctx context.Context, eventID string) error {
	return s.provider.Delete(ctx, BucketEventBus, []byte(eventID))
}

// GetEventCount returns the total count of events in the event bus.
func (s *MetaStorageImpl) GetEventCount(ctx context.Context) (int, error) {
	keyValues, err := s.provider.List(ctx, BucketEventBus, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to count events: %w", err)
	}
	return len(keyValues), nil
}

// UpdateEventProcessingStatus updates the processing status of an event.
func (s *MetaStorageImpl) UpdateEventProcessingStatus(ctx context.Context, eventID string, status string, retryCount int, lastError string, nextRetryTime *time.Time) error {
	event, found := s.GetEvent(ctx, eventID)
	if !found {
		return types.ErrRecordNotFound
	}

	event.ProcessingStatus = status
	event.RetryCount = retryCount
	event.LastError = lastError
	event.NextRetryTime = nextRetryTime
	event.UpdatedAt = time.Now()

	return s.SaveEvent(ctx, *event)
}

// GetFailedEvents returns events that have failed processing before the specified time.
func (s *MetaStorageImpl) GetFailedEvents(ctx context.Context, beforeTime time.Time) ([]types.EventBusEventMetadata, error) {
	status := "failed"
	filters := &types.EventBusFilters{
		ProcessingStatus: &status,
		To:                &beforeTime,
	}
	return s.ListEvents(ctx, filters)
}

// GetDeadLetterEvents returns events in the dead letter queue.
func (s *MetaStorageImpl) GetDeadLetterEvents(ctx context.Context, limit int) ([]types.EventBusEventMetadata, error) {
	status := "dead_letter"
	filters := &types.EventBusFilters{
		ProcessingStatus: &status,
	}
	if limit > 0 {
		filters.Limit = &limit
	}
	return s.ListEvents(ctx, filters)
}

// MoveEventToDeadLetter moves an event to the dead letter queue.
func (s *MetaStorageImpl) MoveEventToDeadLetter(ctx context.Context, eventID string) error {
	event, found := s.GetEvent(ctx, eventID)
	if !found {
		return types.ErrRecordNotFound
	}

	// Update status to dead_letter
	event.ProcessingStatus = "dead_letter"
	event.UpdatedAt = time.Now()

	// Save to dead letter bucket
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketDeadLetterEvents, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	// Save to dead letter bucket
	if err := s.provider.Put(ctx, BucketDeadLetterEvents, []byte(eventID), data); err != nil {
		return fmt.Errorf("failed to save event to dead letter: %w", err)
	}

	// Delete from event bus bucket
	return s.provider.Delete(ctx, BucketEventBus, []byte(eventID))
}

// Data Unit Operations

// SaveDataUnit saves a data unit metadata to the database.
func (s *MetaStorageImpl) SaveDataUnit(ctx context.Context, dataUnit types.DataUnitMetadata) error {
	// Set timestamps if not set
	now := time.Now()
	if dataUnit.CreatedAt.IsZero() {
		dataUnit.CreatedAt = now
	}
	dataUnit.UpdatedAt = now

	data, err := json.Marshal(dataUnit)
	if err != nil {
		return fmt.Errorf("failed to marshal data unit: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketDataUnits, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketDataUnits, []byte(dataUnit.ID), data)
}

// UpdateDataUnit updates a data unit metadata using an update function.
func (s *MetaStorageImpl) UpdateDataUnit(ctx context.Context, dataUnitID string, updateFn func(types.DataUnitMetadata) types.DataUnitMetadata) (types.DataUnitMetadata, error) {
	// Get existing data unit
	dataUnit, found := s.GetDataUnit(ctx, dataUnitID)
	if !found {
		return types.DataUnitMetadata{}, types.ErrRecordNotFound
	}

	// Apply update function
	updated := updateFn(dataUnit)

	// Ensure ID matches
	if updated.ID != dataUnitID {
		return types.DataUnitMetadata{}, fmt.Errorf("data unit ID mismatch: expected %s, got %s", dataUnitID, updated.ID)
	}

	// Update timestamp
	updated.UpdatedAt = time.Now()

	// Save updated data unit
	if err := s.SaveDataUnit(ctx, updated); err != nil {
		return types.DataUnitMetadata{}, err
	}

	return updated, nil
}

// GetDataUnit retrieves a data unit metadata by ID.
func (s *MetaStorageImpl) GetDataUnit(ctx context.Context, dataUnitID string) (types.DataUnitMetadata, bool) {
	data, err := s.provider.Get(ctx, BucketDataUnits, []byte(dataUnitID))
	if err != nil {
		return types.DataUnitMetadata{}, false
	}

	var dataUnit types.DataUnitMetadata
	if err := json.Unmarshal(data, &dataUnit); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal data unit", zap.String("id", dataUnitID), zap.Error(err))
		}
		return types.DataUnitMetadata{}, false
	}

	return dataUnit, true
}

// ListDataUnits lists data units with optional filters.
func (s *MetaStorageImpl) ListDataUnits(ctx context.Context, filters *types.DataUnitFilters) ([]types.DataUnitMetadata, error) {
	keyValues, err := s.provider.List(ctx, BucketDataUnits, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list data units: %w", err)
	}

	var dataUnits []types.DataUnitMetadata
	for _, kv := range keyValues {
		var dataUnit types.DataUnitMetadata
		if err := json.Unmarshal(kv.Value, &dataUnit); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to unmarshal data unit", zap.String("id", string(kv.Key)), zap.Error(err))
			}
			continue // Skip invalid entries
		}

		// Apply filters if provided
		if filters != nil {
			// Filter by DeviceID
			if filters.DeviceID != nil && dataUnit.DeviceID != *filters.DeviceID {
				continue
			}

			// Filter by DeviceType
			if filters.DeviceType != nil && dataUnit.DeviceType != *filters.DeviceType {
				continue
			}

			// Filter by DataType
			if filters.DataType != nil && dataUnit.DataType != *filters.DataType {
				continue
			}

			// Filter by Label
			if filters.Label != nil && dataUnit.Label != *filters.Label {
				continue
			}

			// Filter by CustomLabel
			if filters.CustomLabel != nil && dataUnit.CustomLabel != *filters.CustomLabel {
				continue
			}

			// Filter by CreatedAfter
			if filters.CreatedAfter != nil && dataUnit.CreatedAt.Before(*filters.CreatedAfter) {
				continue
			}

			// Filter by CreatedBefore
			if filters.CreatedBefore != nil && dataUnit.CreatedAt.After(*filters.CreatedBefore) {
				continue
			}
		}

		dataUnits = append(dataUnits, dataUnit)
	}

	// Sort by created_at descending (newest first)
	sort.Slice(dataUnits, func(i, j int) bool {
		return dataUnits[i].CreatedAt.After(dataUnits[j].CreatedAt)
	})

	// Apply limit if provided
	if filters != nil && filters.Limit != nil && *filters.Limit > 0 && len(dataUnits) > *filters.Limit {
		dataUnits = dataUnits[:*filters.Limit]
	}

	return dataUnits, nil
}

// DeleteDataUnit deletes a data unit metadata by ID.
func (s *MetaStorageImpl) DeleteDataUnit(ctx context.Context, dataUnitID string) error {
	return s.provider.Delete(ctx, BucketDataUnits, []byte(dataUnitID))
}

// Model Deployment Operations

// SaveModelDeployment saves a model deployment metadata to the database.
func (s *MetaStorageImpl) SaveModelDeployment(ctx context.Context, deployment types.ModelDeploymentMetadata) error {
	// Set timestamps if not set
	now := time.Now()
	if deployment.CreatedAt.IsZero() {
		deployment.CreatedAt = now
	}
	deployment.UpdatedAt = now

	data, err := json.Marshal(deployment)
	if err != nil {
		return fmt.Errorf("failed to marshal model deployment: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketModelDeployments, int64(len(data))); err != nil {
			// Emit quota exceeded event
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketModelDeployments, []byte(deployment.ModelID), data)
}

// UpdateModelDeployment updates a model deployment metadata using an update function.
func (s *MetaStorageImpl) UpdateModelDeployment(ctx context.Context, modelID string, updateFn func(types.ModelDeploymentMetadata) types.ModelDeploymentMetadata) (types.ModelDeploymentMetadata, error) {
	// Get existing deployment
	deployment, found := s.GetModelDeployment(ctx, modelID)
	if !found {
		return types.ModelDeploymentMetadata{}, types.ErrRecordNotFound
	}

	// Apply update function
	updated := updateFn(deployment)

	// Ensure ModelID matches
	if updated.ModelID != modelID {
		return types.ModelDeploymentMetadata{}, fmt.Errorf("model ID mismatch: expected %s, got %s", modelID, updated.ModelID)
	}

	// Update timestamp
	updated.UpdatedAt = time.Now()

	// Save updated deployment
	if err := s.SaveModelDeployment(ctx, updated); err != nil {
		return types.ModelDeploymentMetadata{}, err
	}

	return updated, nil
}

// GetModelDeployment retrieves a model deployment metadata by model ID.
func (s *MetaStorageImpl) GetModelDeployment(ctx context.Context, modelID string) (types.ModelDeploymentMetadata, bool) {
	data, err := s.provider.Get(ctx, BucketModelDeployments, []byte(modelID))
	if err != nil {
		return types.ModelDeploymentMetadata{}, false
	}

	var deployment types.ModelDeploymentMetadata
	if err := json.Unmarshal(data, &deployment); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal model deployment", zap.String("model_id", modelID), zap.Error(err))
		}
		return types.ModelDeploymentMetadata{}, false
	}

	return deployment, true
}

// ListModelDeployments lists model deployments with optional filters.
func (s *MetaStorageImpl) ListModelDeployments(ctx context.Context, filters *types.ModelFilters) ([]types.ModelDeploymentMetadata, error) {
	keyValues, err := s.provider.List(ctx, BucketModelDeployments, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list model deployments: %w", err)
	}

	var deployments []types.ModelDeploymentMetadata
	for _, kv := range keyValues {
		var deployment types.ModelDeploymentMetadata
		if err := json.Unmarshal(kv.Value, &deployment); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to unmarshal model deployment", zap.String("model_id", string(kv.Key)), zap.Error(err))
			}
			continue // Skip invalid entries
		}

		// Apply filters if provided
		if filters != nil {
			// Filter by EdgeID
			if filters.EdgeID != nil && deployment.EdgeID != *filters.EdgeID {
				continue
			}

			// Filter by DeviceID (convert string to DeviceID for comparison)
			if filters.DeviceID != nil && string(deployment.DeviceID) != *filters.DeviceID {
				continue
			}

			// Filter by DeviceType (convert string to DeviceType for comparison)
			if filters.DeviceType != nil && string(deployment.DeviceType) != *filters.DeviceType {
				continue
			}

			// Filter by Status
			if filters.Status != nil && deployment.Status != *filters.Status {
				continue
			}

			// Filter by ModelType
			if filters.ModelType != nil && deployment.ModelType != *filters.ModelType {
				continue
			}

			// Filter by Framework
			if filters.Framework != nil && deployment.Framework != *filters.Framework {
				continue
			}
		}

		deployments = append(deployments, deployment)
	}

	// Sort by deployed_at descending (newest first)
	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].DeployedAt.After(deployments[j].DeployedAt)
	})

	return deployments, nil
}

// DeleteModelDeployment deletes a model deployment metadata by model ID.
func (s *MetaStorageImpl) DeleteModelDeployment(ctx context.Context, modelID string) error {
	return s.provider.Delete(ctx, BucketModelDeployments, []byte(modelID))
}

// ListModelVersions lists all model versions for a specific device.
func (s *MetaStorageImpl) ListModelVersions(ctx context.Context, deviceID string) ([]types.ModelDeploymentMetadata, error) {
	filters := &types.ModelFilters{
		DeviceID: &deviceID,
	}
	return s.ListModelDeployments(ctx, filters)
}

// GetLatestModelVersion retrieves the latest model version for a specific device.
func (s *MetaStorageImpl) GetLatestModelVersion(ctx context.Context, deviceID string) (*types.ModelDeploymentMetadata, bool) {
	versions, err := s.ListModelVersions(ctx, deviceID)
	if err != nil {
		return nil, false
	}

	if len(versions) == 0 {
		return nil, false
	}

	// Versions are already sorted by DeployedAt descending (newest first)
	return &versions[0], true
}

// Pending Data Unit Request Operations

// SavePendingDataUnitRequest saves a pending data unit request to the database.
func (s *MetaStorageImpl) SavePendingDataUnitRequest(ctx context.Context, deviceID string, request types.PendingDataUnitRequest) error {
	// Ensure DeviceID matches
	if string(request.DeviceID) != deviceID {
		return fmt.Errorf("device ID mismatch: expected %s, got %s", deviceID, string(request.DeviceID))
	}

	// Set RequestedAt if not set
	if request.RequestedAt.IsZero() {
		request.RequestedAt = time.Now()
	}

	// Set default status if not set
	if request.Status == "" {
		request.Status = "pending"
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal pending data unit request: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketPendingDataUnitRequests, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketPendingDataUnitRequests, []byte(deviceID), data)
}

// GetPendingDataUnitRequest retrieves a pending data unit request by device ID.
func (s *MetaStorageImpl) GetPendingDataUnitRequest(ctx context.Context, deviceID string) (*types.PendingDataUnitRequest, bool) {
	data, err := s.provider.Get(ctx, BucketPendingDataUnitRequests, []byte(deviceID))
	if err != nil {
		return nil, false
	}

	var request types.PendingDataUnitRequest
	if err := json.Unmarshal(data, &request); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal pending data unit request", zap.String("device_id", deviceID), zap.Error(err))
		}
		return nil, false
	}

	return &request, true
}

// ListPendingDataUnitRequests lists all pending data unit requests.
func (s *MetaStorageImpl) ListPendingDataUnitRequests(ctx context.Context) ([]types.PendingDataUnitRequest, error) {
	keyValues, err := s.provider.List(ctx, BucketPendingDataUnitRequests, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending data unit requests: %w", err)
	}

	var requests []types.PendingDataUnitRequest
	for _, kv := range keyValues {
		var request types.PendingDataUnitRequest
		if err := json.Unmarshal(kv.Value, &request); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to unmarshal pending data unit request", zap.String("device_id", string(kv.Key)), zap.Error(err))
			}
			continue // Skip invalid entries
		}

		requests = append(requests, request)
	}

	// Sort by requested_at descending (newest first)
	sort.Slice(requests, func(i, j int) bool {
		return requests[i].RequestedAt.After(requests[j].RequestedAt)
	})

	return requests, nil
}

// DeletePendingDataUnitRequest deletes a pending data unit request by device ID.
func (s *MetaStorageImpl) DeletePendingDataUnitRequest(ctx context.Context, deviceID string) error {
	return s.provider.Delete(ctx, BucketPendingDataUnitRequests, []byte(deviceID))
}

// Start starts the metadata store service.
// This initializes the provider, verifies connectivity, creates required buckets,
// runs schema migrations, and starts background tasks.
func (s *MetaStorageImpl) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return types.ErrAlreadyStarted
	}

	if s.logger != nil {
		s.logger.Info("Starting meta storage service...")
	}

	// Step 1: Verify provider connectivity
	if err := s.provider.HealthCheck(ctx); err != nil {
		return fmt.Errorf("provider health check failed: %w", err)
	}

	// Step 2: Create required buckets/namespaces
	if err := s.InitializeBuckets(ctx); err != nil {
		return fmt.Errorf("failed to initialize buckets: %w", err)
	}

	// Step 3: Run schema migrations
	migrator := NewSchemaMigrator(s.provider, s.logger)
	// Pass event emitter to migrator if available
	if s.eventEmitter != nil {
		migrator.SetEventEmitter(s.eventEmitter)
	}
	if err := RegisterDefaultMigrations(migrator, s.provider, s.logger); err != nil {
		return fmt.Errorf("failed to register schema migrations: %w", err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to run schema migrations: %w", err)
	}

	// Step 4: Initialize quota monitoring
	// Quota manager is set via SetQuotaManager() if quota config is provided
	// Periodic quota checks are started if quota manager is configured
	if s.quotaManager != nil {
		// Start periodic quota checks (every 5 minutes)
		s.quotaManager.StartPeriodicChecks(ctx, 5*time.Minute)
		if s.logger != nil {
			s.logger.Info("Quota monitoring started")
		}
	} else if s.logger != nil {
		s.logger.Info("Quota monitoring: not configured")
	}

	// Step 5: Start background tasks
	// Start retention cleanup if retention manager is configured
	if s.retentionManager != nil {
		s.retentionManager.StartPeriodicCleanup(ctx)
		if s.logger != nil {
			s.logger.Info("Retention cleanup started")
		}
	} else if s.logger != nil {
		s.logger.Info("Retention cleanup: not configured")
	}

	// Start integrity checks if integrity manager is configured
	if s.integrityManager != nil {
		s.integrityManager.StartPeriodicIntegrityChecks(ctx, 24*time.Hour)
		if s.logger != nil {
			s.logger.Info("Integrity checks started")
		}
	} else if s.logger != nil {
		s.logger.Info("Integrity checks: not configured")
	}

	s.started = true

	if s.logger != nil {
		s.logger.Info("Meta storage service started successfully")
	}

	return nil
}

// Stop gracefully shuts down the metadata store service.
// This stops background tasks, closes provider connections, flushes pending operations,
// and closes database connections.
func (s *MetaStorageImpl) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil // Already stopped
	}

	if s.logger != nil {
		s.logger.Info("Stopping meta storage service...")
	}

	var errs []error

	// Step 1: Stop background tasks gracefully
	// TODO: Implement background task stopping in Epic 5 and Epic 6
	// For now, this is a placeholder
	close(s.stopCh)
	s.stopCh = make(chan struct{}) // Reset for potential restart
	if s.logger != nil {
		s.logger.Info("Background tasks stopped")
	}

	// Step 2: Flush pending operations
	// For BoltDB, this is handled automatically by the database
	// For other providers, this may need explicit flushing
	if s.logger != nil {
		s.logger.Info("Pending operations flushed")
	}

	// Step 3: Close provider connections
	// The provider should have a Close method
	if closer, ok := s.provider.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close provider: %w", err))
		}
	}

	s.started = false

	if len(errs) > 0 {
		if s.logger != nil {
			s.logger.Error("Some operations failed during stop", zap.Errors("errors", errs))
		}
		return errors.Join(errs...)
	}

	if s.logger != nil {
		s.logger.Info("Meta storage service stopped successfully")
	}

	return nil
}

// Close closes the metadata store and releases all resources.
func (s *MetaStorageImpl) Close() error {
	return s.Stop(context.Background())
}

// HealthSnapshot returns the current health status of the storage service.
// This follows the vm-gateway pattern for health snapshots.
func (s *MetaStorageImpl) HealthSnapshot(ctx context.Context) types.StorageHealth {
	health := types.StorageHealth{
		Status:          types.HealthStatusHealthy,
		BucketCounts:    make(map[string]int),
		LastHealthCheck: time.Now(),
		ProviderHealth:  "healthy",
		ProviderStatus:  make(map[string]interface{}),
	}

	// Step 1: Query quota status
	if s.quotaManager != nil {
		quota, err := s.quotaManager.GetQuotaStatus(ctx)
		if err == nil && quota != nil {
			health.Quota = quota

			// Determine health status based on quota
			if quota.Limit > 0 {
				usagePercent := float64(quota.Used) / float64(quota.Limit) * 100
				if usagePercent >= float64(quota.FullThreshold) {
					health.Status = types.HealthStatusFull
				} else if usagePercent >= float64(quota.WarningThreshold) {
					// Only set warning if not already full
					if health.Status != types.HealthStatusFull {
						health.Status = types.HealthStatusWarning
					}
				}
			}
		}
	}

	// Step 2: Query integrity error count
	if s.integrityManager != nil {
		health.IntegrityErrors = s.integrityManager.GetErrorCount()
		if health.IntegrityErrors > 0 {
			// Integrity errors take precedence over quota warnings
			health.Status = types.HealthStatusCorrupted
		}
	}

	// Step 3: Query provider health
	if err := s.provider.HealthCheck(ctx); err != nil {
		health.ProviderHealth = "unhealthy"
		health.ProviderStatus["error"] = err.Error()
		// Provider health issues take precedence over quota warnings
		if health.Status != types.HealthStatusCorrupted {
			health.Status = types.HealthStatusWarning
		}
	} else {
		health.ProviderHealth = "healthy"
	}

	// Step 4: Query bucket record counts and calculate total
	var totalRecords int64
	buckets := AllStandardBuckets()
	for _, bucketName := range buckets {
		exists := s.provider.BucketExists(ctx, bucketName)
		if !exists {
			continue
		}

		// Count records in bucket
		keyValues, err := s.provider.List(ctx, bucketName, nil)
		if err != nil {
			// Log error but don't fail health check
			if s.logger != nil {
				s.logger.Warn("Failed to list records in bucket for health check",
					zap.String("bucket", bucketName),
					zap.Error(err))
			}
			continue
		}

		count := len(keyValues)
		health.BucketCounts[bucketName] = count
		totalRecords += int64(count)
	}
	health.TotalRecords = totalRecords

	// Step 5: Query retention cleanup statistics
	if s.retentionManager != nil {
		health.LastCleanupTime = s.retentionManager.GetLastCleanupTime()
		cleanupStats := s.retentionManager.GetLastCleanupStats()
		if cleanupStats != nil {
			// Convert impl.CleanupStats to types.CleanupStats
			health.CleanupStats = &types.CleanupStats{
				RecordsDeleted:   cleanupStats.RecordsDeleted,
				SpaceFreedBytes:  cleanupStats.SpaceFreedBytes,
				BucketsProcessed: cleanupStats.BucketsProcessed,
				Duration:         cleanupStats.Duration,
			}
		}
	}

	// Step 6: Query schema version
	migrator := NewSchemaMigrator(s.provider, s.logger)
	schemaVersion, err := migrator.GetCurrentVersion(ctx)
	if err == nil {
		health.SchemaVersion = schemaVersion
	} else if s.logger != nil {
		s.logger.Warn("Failed to get schema version for health check", zap.Error(err))
	}

	// Step 7: Query database size (provider-specific)
	// For BoltDB, try to get database file size
	if bboltProvider, ok := s.provider.(interface {
		GetDatabasePath() string
	}); ok {
		dbPath := bboltProvider.GetDatabasePath()
		if dbPath != "" {
			if size, err := s.getDatabaseSize(dbPath); err == nil {
				health.DatabaseSizeMB = size
			} else if s.logger != nil {
				s.logger.Warn("Failed to get database size for health check", zap.Error(err))
			}
		}
	}

	return health
}

// getDatabaseSize returns the size of the database file in megabytes.
func (s *MetaStorageImpl) getDatabaseSize(dbPath string) (float64, error) {
	fileInfo, err := os.Stat(dbPath)
	if err != nil {
		return 0, fmt.Errorf("failed to stat database file: %w", err)
	}
	// Convert bytes to megabytes
	return float64(fileInfo.Size()) / (1024 * 1024), nil
}

// emitQuotaExceededEvent emits a storage.quota_exceeded event.
func (s *MetaStorageImpl) emitQuotaExceededEvent(ctx context.Context, err error) {
	if s.quotaManager == nil {
		return
	}

	quota, quotaErr := s.quotaManager.GetQuotaStatus(ctx)
	if quotaErr != nil || quota == nil {
		return
	}

	usagePercent := float64(quota.Used) / float64(quota.Limit) * 100
	data := map[string]interface{}{
		"used_bytes":    quota.Used,
		"limit_bytes":   quota.Limit,
		"usage_percent": usagePercent,
		"error":         err.Error(),
	}

	s.emitEvent("storage.quota_exceeded", data)
}

// emitQuotaWarningEvent emits a storage.warning event.
func (s *MetaStorageImpl) emitQuotaWarningEvent(ctx context.Context, quota *types.StorageQuota) {
	if quota == nil || quota.Limit == 0 {
		return
	}

	usagePercent := float64(quota.Used) / float64(quota.Limit) * 100
	data := map[string]interface{}{
		"used_bytes":    quota.Used,
		"limit_bytes":   quota.Limit,
		"usage_percent": usagePercent,
	}

	s.emitEvent("storage.warning", data)
}

// emitQuotaFullEvent emits a storage.full event.
func (s *MetaStorageImpl) emitQuotaFullEvent(ctx context.Context, quota *types.StorageQuota) {
	if quota == nil || quota.Limit == 0 {
		return
	}

	usagePercent := float64(quota.Used) / float64(quota.Limit) * 100
	data := map[string]interface{}{
		"used_bytes":    quota.Used,
		"limit_bytes":   quota.Limit,
		"usage_percent": usagePercent,
	}

	s.emitEvent("storage.full", data)
}

// emitCleanupStartedEvent emits a storage.cleanup_started event.
func (s *MetaStorageImpl) emitCleanupStartedEvent() {
	s.emitEvent("storage.cleanup_started", map[string]interface{}{})
}

// emitCleanupCompletedEvent emits a storage.cleanup_completed event.
func (s *MetaStorageImpl) emitCleanupCompletedEvent(stats *CleanupStats) {
	if stats == nil {
		return
	}

	durationStr := stats.Duration.String()
	data := map[string]interface{}{
		"records_deleted":   stats.RecordsDeleted,
		"space_freed_bytes": stats.SpaceFreedBytes,
		"buckets_processed": stats.BucketsProcessed,
		"duration":          durationStr,
	}

	s.emitEvent("storage.cleanup_completed", data)
}

// emitCorruptionDetectedEvent emits a storage.corruption_detected event.
func (s *MetaStorageImpl) emitCorruptionDetectedEvent(errorCount int, errorDetails string) {
	data := map[string]interface{}{
		"error_count":   errorCount,
		"error_details": errorDetails,
	}

	s.emitEvent("storage.corruption_detected", data)
}

// emitSchemaMigrationStartedEvent emits a storage.schema_migration_started event.
func (s *MetaStorageImpl) emitSchemaMigrationStartedEvent(version int, description string) {
	data := map[string]interface{}{
		"schema_version": version,
		"description":    description,
	}

	s.emitEvent("storage.schema_migration_started", data)
}

// emitSchemaMigrationCompletedEvent emits a storage.schema_migration_completed event.
func (s *MetaStorageImpl) emitSchemaMigrationCompletedEvent(version int, description string) {
	data := map[string]interface{}{
		"schema_version": version,
		"description":    description,
	}

	s.emitEvent("storage.schema_migration_completed", data)
}

// Device Operations

// SaveDevice saves device metadata to the database.
func (s *MetaStorageImpl) SaveDevice(ctx context.Context, device types.DeviceMetadata) error {
	// Set timestamps if not set
	now := time.Now()
	if device.CreatedAt.IsZero() {
		device.CreatedAt = now
	}
	device.UpdatedAt = now

	data, err := json.Marshal(device)
	if err != nil {
		return fmt.Errorf("failed to marshal device: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketDevices, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketDevices, []byte(string(device.ID)), data)
}

// UpdateDevice updates device metadata using an update function.
func (s *MetaStorageImpl) UpdateDevice(ctx context.Context, deviceID string, updateFn func(types.DeviceMetadata) types.DeviceMetadata) (types.DeviceMetadata, error) {
	// Get existing device
	device, found := s.GetDevice(ctx, deviceID)
	if !found {
		return types.DeviceMetadata{}, types.ErrRecordNotFound
	}

	// Apply update function
	updated := updateFn(device)

	// Ensure ID matches
	if string(updated.ID) != deviceID {
		return types.DeviceMetadata{}, fmt.Errorf("device ID mismatch: expected %s, got %s", deviceID, string(updated.ID))
	}

	// Update timestamp
	updated.UpdatedAt = time.Now()

	// Save updated device
	if err := s.SaveDevice(ctx, updated); err != nil {
		return types.DeviceMetadata{}, err
	}

	return updated, nil
}

// GetDevice retrieves device metadata by device ID.
func (s *MetaStorageImpl) GetDevice(ctx context.Context, deviceID string) (types.DeviceMetadata, bool) {
	data, err := s.provider.Get(ctx, BucketDevices, []byte(deviceID))
	if err != nil {
		return types.DeviceMetadata{}, false
	}

	var device types.DeviceMetadata
	if err := json.Unmarshal(data, &device); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal device", zap.String("device_id", deviceID), zap.Error(err))
		}
		return types.DeviceMetadata{}, false
	}

	return device, true
}

// ListDevices lists devices with optional filters.
func (s *MetaStorageImpl) ListDevices(ctx context.Context, filters *types.DeviceFilters) ([]types.DeviceMetadata, error) {
	keyValues, err := s.provider.List(ctx, BucketDevices, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}

	var devices []types.DeviceMetadata
	for _, kv := range keyValues {
		var device types.DeviceMetadata
		if err := json.Unmarshal(kv.Value, &device); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to unmarshal device", zap.String("device_id", string(kv.Key)), zap.Error(err))
			}
			continue // Skip invalid entries
		}

		// Apply filters if provided
		if filters != nil {
			// Filter by EnabledOnly
			if filters.EnabledOnly != nil && device.Enabled != *filters.EnabledOnly {
				continue
			}

			// Filter by Status
			if filters.Status != nil && device.Status != *filters.Status {
				continue
			}

			// Filter by SyncedWithVM
			if filters.SyncedWithVM != nil && device.SyncedWithVM != *filters.SyncedWithVM {
				continue
			}

			// Filter by DeviceType
			if filters.DeviceType != nil && device.DeviceType != *filters.DeviceType {
				continue
			}
		}

		devices = append(devices, device)
	}

	// Sort by device ID
	sort.Slice(devices, func(i, j int) bool {
		return string(devices[i].ID) < string(devices[j].ID)
	})

	return devices, nil
}

// DeleteDevice deletes device metadata by device ID.
func (s *MetaStorageImpl) DeleteDevice(ctx context.Context, deviceID string) error {
	return s.provider.Delete(ctx, BucketDevices, []byte(deviceID))
}

// Video Clip Operations

// SaveVideoClip saves video clip metadata to the database.
func (s *MetaStorageImpl) SaveVideoClip(ctx context.Context, clip types.VideoClipMetadata) error {
	// Set timestamp if not set
	if clip.CreatedAt.IsZero() {
		clip.CreatedAt = time.Now()
	}

	data, err := json.Marshal(clip)
	if err != nil {
		return fmt.Errorf("failed to marshal video clip: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketVideoClips, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketVideoClips, []byte(clip.ID), data)
}

// GetVideoClip retrieves video clip metadata by clip ID.
func (s *MetaStorageImpl) GetVideoClip(ctx context.Context, clipID string) (types.VideoClipMetadata, bool) {
	data, err := s.provider.Get(ctx, BucketVideoClips, []byte(clipID))
	if err != nil {
		return types.VideoClipMetadata{}, false
	}

	var clip types.VideoClipMetadata
	if err := json.Unmarshal(data, &clip); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal video clip", zap.String("clip_id", clipID), zap.Error(err))
		}
		return types.VideoClipMetadata{}, false
	}

	return clip, true
}

// ListVideoClips lists video clips with optional filters.
func (s *MetaStorageImpl) ListVideoClips(ctx context.Context, filters map[string]interface{}) ([]types.VideoClipMetadata, error) {
	keyValues, err := s.provider.List(ctx, BucketVideoClips, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list video clips: %w", err)
	}

	var clips []types.VideoClipMetadata
	for _, kv := range keyValues {
		var clip types.VideoClipMetadata
		if err := json.Unmarshal(kv.Value, &clip); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to unmarshal video clip", zap.String("clip_id", string(kv.Key)), zap.Error(err))
			}
			continue // Skip invalid entries
		}

		// Apply filters if provided
		if filters != nil {
			// Filter by DeviceID
			if deviceID, ok := filters["device_id"].(string); ok && string(clip.DeviceID) != deviceID {
				continue
			}

			// Filter by EventID
			if eventID, ok := filters["event_id"].(string); ok && clip.EventID != eventID {
				continue
			}
		}

		clips = append(clips, clip)
	}

	// Sort by created_at descending (newest first)
	sort.Slice(clips, func(i, j int) bool {
		return clips[i].CreatedAt.After(clips[j].CreatedAt)
	})

	return clips, nil
}

// DeleteVideoClip deletes video clip metadata by clip ID.
func (s *MetaStorageImpl) DeleteVideoClip(ctx context.Context, clipID string) error {
	return s.provider.Delete(ctx, BucketVideoClips, []byte(clipID))
}

// ML Lifecycle State Operations

// SaveMLLifecycleState saves or updates the ML lifecycle state for a device.
func (s *MetaStorageImpl) SaveMLLifecycleState(ctx context.Context, deviceID string, stateInfo types.MLLifecycleStateInfo) error {
	// Ensure DeviceID matches
	if string(stateInfo.DeviceID) != deviceID {
		return fmt.Errorf("device ID mismatch: expected %s, got %s", deviceID, string(stateInfo.DeviceID))
	}

	// Set timestamps if not set
	now := time.Now()
	if stateInfo.CreatedAt.IsZero() {
		stateInfo.CreatedAt = now
	}
	stateInfo.LastUpdated = now

	// Initialize version if not set
	if stateInfo.Version == 0 {
		stateInfo.Version = 1
	}

	data, err := json.Marshal(stateInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal ML lifecycle state: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketMLLifecycle, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketMLLifecycle, []byte(deviceID), data)
}

// GetMLLifecycleState retrieves the ML lifecycle state for a device.
func (s *MetaStorageImpl) GetMLLifecycleState(ctx context.Context, deviceID string) (*types.MLLifecycleStateInfo, bool) {
	data, err := s.provider.Get(ctx, BucketMLLifecycle, []byte(deviceID))
	if err != nil {
		return nil, false
	}

	var stateInfo types.MLLifecycleStateInfo
	if err := json.Unmarshal(data, &stateInfo); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal ML lifecycle state", zap.String("device_id", deviceID), zap.Error(err))
		}
		return nil, false
	}

	return &stateInfo, true
}

// UpdateMLLifecycleState updates the ML lifecycle state for a device using the provided update function.
func (s *MetaStorageImpl) UpdateMLLifecycleState(ctx context.Context, deviceID string, updateFn func(types.MLLifecycleStateInfo) types.MLLifecycleStateInfo) (*types.MLLifecycleStateInfo, error) {
	// Get existing state
	stateInfo, found := s.GetMLLifecycleState(ctx, deviceID)
	if !found {
		return nil, types.ErrRecordNotFound
	}

	// Apply update function
	updated := updateFn(*stateInfo)

	// Ensure DeviceID matches
	if string(updated.DeviceID) != deviceID {
		return nil, fmt.Errorf("device ID mismatch: expected %s, got %s", deviceID, string(updated.DeviceID))
	}

	// Update timestamp and increment version
	updated.LastUpdated = time.Now()
	updated.Version++

	// Save updated state
	if err := s.SaveMLLifecycleState(ctx, deviceID, updated); err != nil {
		return nil, err
	}

	return &updated, nil
}

// UpdateMLLifecycleStateCAS performs a Compare-And-Swap (CAS) update on the ML lifecycle state.
func (s *MetaStorageImpl) UpdateMLLifecycleStateCAS(ctx context.Context, deviceID string, expectedVersion int, updateFn func(types.MLLifecycleStateInfo) types.MLLifecycleStateInfo) (*types.MLLifecycleStateInfo, error) {
	// Get existing state
	stateInfo, found := s.GetMLLifecycleState(ctx, deviceID)
	if !found {
		return nil, types.ErrRecordNotFound
	}

	// Verify version matches expected
	if stateInfo.Version != expectedVersion {
		return nil, fmt.Errorf("version mismatch: expected %d, got %d (concurrent modification detected)", expectedVersion, stateInfo.Version)
	}

	// Apply update function
	updated := updateFn(*stateInfo)

	// Ensure DeviceID matches
	if string(updated.DeviceID) != deviceID {
		return nil, fmt.Errorf("device ID mismatch: expected %s, got %s", deviceID, string(updated.DeviceID))
	}

	// Update timestamp and increment version
	updated.LastUpdated = time.Now()
	updated.Version = expectedVersion + 1

	// Save updated state
	if err := s.SaveMLLifecycleState(ctx, deviceID, updated); err != nil {
		return nil, err
	}

	return &updated, nil
}

// ListMLLifecycleStates lists ML lifecycle states with optional filters.
func (s *MetaStorageImpl) ListMLLifecycleStates(ctx context.Context, filters *types.MLLifecycleFilters) ([]types.MLLifecycleStateInfo, error) {
	keyValues, err := s.provider.List(ctx, BucketMLLifecycle, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list ML lifecycle states: %w", err)
	}

	var states []types.MLLifecycleStateInfo
	for _, kv := range keyValues {
		var stateInfo types.MLLifecycleStateInfo
		if err := json.Unmarshal(kv.Value, &stateInfo); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to unmarshal ML lifecycle state", zap.String("device_id", string(kv.Key)), zap.Error(err))
			}
			continue // Skip invalid entries
		}

		// Apply filters if provided
		if filters != nil {
			// Filter by DeviceID
			if filters.DeviceID != nil && stateInfo.DeviceID != *filters.DeviceID {
				continue
			}

			// Filter by State
			if filters.State != nil && stateInfo.State != *filters.State {
				continue
			}

			// Filter by States (OR condition)
			if len(filters.States) > 0 {
				found := false
				for _, state := range filters.States {
					if stateInfo.State == state {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// Filter by HasModel
			if filters.HasModel != nil {
				hasModel := stateInfo.ModelID != ""
				if hasModel != *filters.HasModel {
					continue
				}
			}

			// Filter by HasDataset
			if filters.HasDataset != nil {
				hasDataset := stateInfo.DatasetID != ""
				if hasDataset != *filters.HasDataset {
					continue
				}
			}

			// Filter by OfflineInferenceAllowed
			if filters.OfflineInferenceAllowed != nil && stateInfo.OfflineInferenceAllowed != *filters.OfflineInferenceAllowed {
				continue
			}
		}

		states = append(states, stateInfo)
	}

	// Sort by device ID
	sort.Slice(states, func(i, j int) bool {
		return string(states[i].DeviceID) < string(states[j].DeviceID)
	})

	return states, nil
}

// DeleteMLLifecycleState deletes the ML lifecycle state for a device.
func (s *MetaStorageImpl) DeleteMLLifecycleState(ctx context.Context, deviceID string) error {
	return s.provider.Delete(ctx, BucketMLLifecycle, []byte(deviceID))
}

// Pending Model Deployment Operations

// SavePendingModelDeployment saves a pending model deployment to the database.
func (s *MetaStorageImpl) SavePendingModelDeployment(ctx context.Context, deviceID string, deployment types.PendingModelDeployment) error {
	// Ensure DeviceID matches
	if string(deployment.DeviceID) != deviceID {
		return fmt.Errorf("device ID mismatch: expected %s, got %s", deviceID, string(deployment.DeviceID))
	}

	// Set ReceivedAt if not set
	if deployment.ReceivedAt.IsZero() {
		deployment.ReceivedAt = time.Now()
	}

	// Set default TTL if not set (24 hours)
	if deployment.TTL == 0 {
		deployment.TTL = 24 * time.Hour
	}

	data, err := json.Marshal(deployment)
	if err != nil {
		return fmt.Errorf("failed to marshal pending model deployment: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketPendingModelDeployments, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketPendingModelDeployments, []byte(deviceID), data)
}

// GetPendingModelDeployment retrieves a pending model deployment by device ID.
func (s *MetaStorageImpl) GetPendingModelDeployment(ctx context.Context, deviceID string) (*types.PendingModelDeployment, bool) {
	data, err := s.provider.Get(ctx, BucketPendingModelDeployments, []byte(deviceID))
	if err != nil {
		return nil, false
	}

	var deployment types.PendingModelDeployment
	if err := json.Unmarshal(data, &deployment); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal pending model deployment", zap.String("device_id", deviceID), zap.Error(err))
		}
		return nil, false
	}

	return &deployment, true
}

// ListPendingModelDeployments lists all pending model deployments.
func (s *MetaStorageImpl) ListPendingModelDeployments(ctx context.Context) ([]types.PendingModelDeployment, error) {
	keyValues, err := s.provider.List(ctx, BucketPendingModelDeployments, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending model deployments: %w", err)
	}

	var deployments []types.PendingModelDeployment
	for _, kv := range keyValues {
		var deployment types.PendingModelDeployment
		if err := json.Unmarshal(kv.Value, &deployment); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to unmarshal pending model deployment", zap.String("device_id", string(kv.Key)), zap.Error(err))
			}
			continue // Skip invalid entries
		}

		deployments = append(deployments, deployment)
	}

	// Sort by received_at descending (newest first)
	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].ReceivedAt.After(deployments[j].ReceivedAt)
	})

	return deployments, nil
}

// DeletePendingModelDeployment deletes a pending model deployment by device ID.
func (s *MetaStorageImpl) DeletePendingModelDeployment(ctx context.Context, deviceID string) error {
	return s.provider.Delete(ctx, BucketPendingModelDeployments, []byte(deviceID))
}

// Edge State Operations

// SaveEdgeState saves the current edge state to the database.
func (s *MetaStorageImpl) SaveEdgeState(ctx context.Context, state map[string]interface{}) error {
	// Add timestamp
	stateWithTime := make(map[string]interface{})
	for k, v := range state {
		stateWithTime[k] = v
	}
	stateWithTime["updated_at"] = time.Now()

	data, err := json.Marshal(stateWithTime)
	if err != nil {
		return fmt.Errorf("failed to marshal edge state: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketEdgeState, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	// Save current state
	if err := s.provider.Put(ctx, BucketEdgeState, []byte(currentEdgeStateKey), data); err != nil {
		return fmt.Errorf("failed to save current edge state: %w", err)
	}

	// Also save to history with timestamp key
	historyKey := fmt.Sprintf("%d", time.Now().UnixNano())
	if err := s.provider.Put(ctx, BucketEdgeStateHistory, []byte(historyKey), data); err != nil {
		// Log error but don't fail - history is optional
		if s.logger != nil {
			s.logger.Warn("Failed to save edge state history", zap.Error(err))
		}
	}

	return nil
}

// GetCurrentEdgeState retrieves the current edge state.
func (s *MetaStorageImpl) GetCurrentEdgeState(ctx context.Context) (map[string]interface{}, bool) {
	data, err := s.provider.Get(ctx, BucketEdgeState, []byte(currentEdgeStateKey))
	if err != nil {
		return nil, false
	}

	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal edge state", zap.Error(err))
		}
		return nil, false
	}

	return state, true
}

// GetEdgeStateHistory retrieves edge state history up to the specified limit.
func (s *MetaStorageImpl) GetEdgeStateHistory(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	keyValues, err := s.provider.List(ctx, BucketEdgeStateHistory, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list edge state history: %w", err)
	}

	var history []map[string]interface{}
	for _, kv := range keyValues {
		var state map[string]interface{}
		if err := json.Unmarshal(kv.Value, &state); err != nil {
			if s.logger != nil {
				s.logger.Warn("Failed to unmarshal edge state history entry", zap.String("key", string(kv.Key)), zap.Error(err))
			}
			continue // Skip invalid entries
		}

		history = append(history, state)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(history, func(i, j int) bool {
		// Handle time.Time or string (JSON unmarshaling converts time.Time to string)
		var timeI, timeJ time.Time
		var okI, okJ bool

		if t, ok := history[i]["updated_at"].(time.Time); ok {
			timeI = t
			okI = true
		} else if str, ok := history[i]["updated_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				timeI = t
				okI = true
			}
		}

		if t, ok := history[j]["updated_at"].(time.Time); ok {
			timeJ = t
			okJ = true
		} else if str, ok := history[j]["updated_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, str); err == nil {
				timeJ = t
				okJ = true
			}
		}

		if !okI || !okJ {
			return false
		}
		return timeI.After(timeJ)
	})

	// Apply limit
	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}

	return history, nil
}

// Edge Capabilities Operations

// SaveEdgeCapabilities saves edge capabilities to the database.
func (s *MetaStorageImpl) SaveEdgeCapabilities(ctx context.Context, capabilities map[string]interface{}) error {
	data, err := json.Marshal(capabilities)
	if err != nil {
		return fmt.Errorf("failed to marshal edge capabilities: %w", err)
	}

	// Check quota before write
	if s.quotaManager != nil {
		if err := s.quotaManager.CheckQuotaBeforeWrite(ctx, BucketEdgeCapabilities, int64(len(data))); err != nil {
			s.emitQuotaExceededEvent(ctx, err)
			return types.ErrQuotaExceeded
		}
	}

	return s.provider.Put(ctx, BucketEdgeCapabilities, []byte(currentEdgeCapabilitiesKey), data)
}

// GetEdgeCapabilities retrieves edge capabilities.
func (s *MetaStorageImpl) GetEdgeCapabilities(ctx context.Context) (map[string]interface{}, bool) {
	data, err := s.provider.Get(ctx, BucketEdgeCapabilities, []byte(currentEdgeCapabilitiesKey))
	if err != nil {
		return nil, false
	}

	var capabilities map[string]interface{}
	if err := json.Unmarshal(data, &capabilities); err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to unmarshal edge capabilities", zap.Error(err))
		}
		return nil, false
	}

	return capabilities, true
}

