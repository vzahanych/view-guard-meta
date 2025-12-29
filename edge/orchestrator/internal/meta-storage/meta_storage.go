package metastorage

import (
	"context"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/impl"
	implbbolt "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/impl/bbolt"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Re-export errors from types package for convenience
var (
	ErrNotInitialized       = types.ErrNotInitialized
	ErrAlreadyStarted       = types.ErrAlreadyStarted
	ErrQuotaExceeded        = types.ErrQuotaExceeded
	ErrRecordNotFound       = types.ErrRecordNotFound
	ErrCorruptionDetected   = types.ErrCorruptionDetected
	ErrInvalidSchemaVersion = types.ErrInvalidSchemaVersion
)

// Re-export types for convenience
// These types are defined in the types package but re-exported here for easier access
type StorageEntryMetadata = types.StorageEntryMetadata
type StorageStats = types.StorageStats
type ModelFilters = types.ModelFilters

// Model deployment types
type ModelDeploymentMetadata = types.ModelDeploymentMetadata

// Event bus types (new)
type EventBusEventMetadata = types.EventBusEventMetadata
type EventBusFilters = types.EventBusFilters

// Pending data unit request type (new)
type PendingDataUnitRequest = types.PendingDataUnitRequest

// Device-agnostic types (new)
type DeviceID = types.DeviceID
type DeviceType = types.DeviceType
type DeviceMetadata = types.DeviceMetadata
type DeviceFilters = types.DeviceFilters
type DataUnitMetadata = types.DataUnitMetadata
type VideoClipMetadata = types.VideoClipMetadata

// ML lifecycle types
type MLLifecycleState = types.MLLifecycleState
type MLLifecycleStateInfo = types.MLLifecycleStateInfo
type MLLifecycleFilters = types.MLLifecycleFilters
type PendingModelDeployment = types.PendingModelDeployment

// Health monitoring types
type HealthStatus = types.HealthStatus
type StorageHealth = types.StorageHealth
type StorageQuota = types.StorageQuota

// MetaDataStore defines operations for managing metadata about stored objects.
// It provides a unified interface for all metadata storage operations, abstracting away
// the details of the underlying storage provider (BoltDB, SQLite, PostgreSQL, etc.).
//
//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_meta_storage.go -package=mocks
type MetaDataStore interface {
	// Storage entry metadata (clips and snapshots)

	// SaveStorageEntry saves metadata about a storage entry (clip or snapshot).
	// The entry must have a valid Path field that uniquely identifies the storage entry.
	//
	// Returns an error if:
	//   - The entry is invalid (missing required fields)
	//   - Storage quota is exceeded (ErrQuotaExceeded)
	//   - The provider operation fails
	SaveStorageEntry(ctx context.Context, entry StorageEntryMetadata) error

	// DeleteStorageEntry deletes metadata for a storage entry by path.
	// If the entry does not exist, this is a no-op and returns no error.
	//
	// Returns an error if:
	//   - The provider operation fails
	DeleteStorageEntry(ctx context.Context, path string) error

	// ListStorageEntries lists all storage entries of the specified file type.
	// If fileType is empty, all entries are returned.
	//
	// Returns:
	//   - A slice of StorageEntryMetadata entries, sorted by path
	//   - An error if the provider operation fails
	ListStorageEntries(ctx context.Context, fileType string) ([]StorageEntryMetadata, error)

	// GetStorageStats returns statistics about stored entries.
	//
	// Returns:
	//   - StorageStats containing total size, entry counts, etc.
	//   - An error if the provider operation fails
	GetStorageStats(ctx context.Context) (*StorageStats, error)

	// Model deployment metadata (device-agnostic, replaces deployed model methods)

	// SaveModelDeployment saves metadata about a deployed ML model.
	// The deployment must have a valid ModelID that uniquely identifies the model.
	//
	// Returns an error if:
	//   - The deployment is invalid (missing required fields)
	//   - Storage quota is exceeded (ErrQuotaExceeded)
	//   - The provider operation fails
	SaveModelDeployment(ctx context.Context, deployment types.ModelDeploymentMetadata) error

	// UpdateModelDeployment updates an existing model deployment using the provided update function.
	// The update function receives the current deployment and returns the updated deployment.
	//
	// Note: This method is not thread-safe for concurrent updates. For concurrent-safe updates,
	// use a version-based locking mechanism or ensure external synchronization.
	//
	// Returns:
	//   - The updated ModelDeploymentMetadata
	//   - An error if:
	//     - The model deployment does not exist (ErrRecordNotFound)
	//     - Storage quota is exceeded (ErrQuotaExceeded)
	//     - The provider operation fails
	UpdateModelDeployment(ctx context.Context, modelID string, updateFn func(types.ModelDeploymentMetadata) types.ModelDeploymentMetadata) (types.ModelDeploymentMetadata, error)

	// GetModelDeployment retrieves a model deployment by model ID.
	//
	// Returns:
	//   - The ModelDeploymentMetadata if found
	//   - false if the model deployment does not exist
	GetModelDeployment(ctx context.Context, modelID string) (types.ModelDeploymentMetadata, bool)

	// ListModelDeployments lists model deployments matching the provided filters.
	// If filters is nil, all deployments are returned.
	//
	// Filter fields:
	//   - DeviceID: Filter by device ID
	//   - DeviceType: Filter by device type
	//   - ModelType: Filter by model type
	//   - Framework: Filter by ML framework
	//
	// Returns:
	//   - A slice of ModelDeploymentMetadata entries, sorted by deployment time
	//   - An error if the provider operation fails
	ListModelDeployments(ctx context.Context, filters *types.ModelFilters) ([]types.ModelDeploymentMetadata, error)

	// DeleteModelDeployment deletes a model deployment by model ID.
	// If the deployment does not exist, this is a no-op and returns no error.
	//
	// Returns an error if:
	//   - The provider operation fails
	DeleteModelDeployment(ctx context.Context, modelID string) error

	// ListModelVersions lists all model versions deployed for a specific device.
	//
	// Returns:
	//   - A slice of ModelDeploymentMetadata entries for the device, sorted by version
	//   - An error if the provider operation fails
	ListModelVersions(ctx context.Context, deviceID string) ([]types.ModelDeploymentMetadata, error)

	// GetLatestModelVersion retrieves the latest model version for a specific device.
	//
	// Returns:
	//   - The latest ModelDeploymentMetadata if found
	//   - false if no model is deployed for the device
	GetLatestModelVersion(ctx context.Context, deviceID string) (*types.ModelDeploymentMetadata, bool)

	// Device metadata (device-agnostic)

	// SaveDevice saves metadata about a device (camera, sensor, audio device, etc.).
	// The device must have a valid DeviceID that uniquely identifies the device.
	//
	// Returns an error if:
	//   - The device is invalid (missing required fields)
	//   - Storage quota is exceeded (ErrQuotaExceeded)
	//   - The provider operation fails
	SaveDevice(ctx context.Context, device types.DeviceMetadata) error

	// UpdateDevice updates an existing device using the provided update function.
	// The update function receives the current device and returns the updated device.
	//
	// Returns:
	//   - The updated DeviceMetadata
	//   - An error if:
	//     - The device does not exist (ErrRecordNotFound)
	//     - Storage quota is exceeded (ErrQuotaExceeded)
	//     - The provider operation fails
	UpdateDevice(ctx context.Context, deviceID string, updateFn func(types.DeviceMetadata) types.DeviceMetadata) (types.DeviceMetadata, error)

	// GetDevice retrieves a device by device ID.
	//
	// Returns:
	//   - The DeviceMetadata if found
	//   - false if the device does not exist
	GetDevice(ctx context.Context, deviceID string) (types.DeviceMetadata, bool)

	// ListDevices lists devices matching the provided filters.
	// If filters is nil, all devices are returned.
	//
	// Filter fields:
	//   - DeviceType: Filter by device type (camera, sensor, audio, etc.)
	//   - Location: Filter by location
	//
	// Returns:
	//   - A slice of DeviceMetadata entries, sorted by device ID
	//   - An error if the provider operation fails
	ListDevices(ctx context.Context, filters *types.DeviceFilters) ([]types.DeviceMetadata, error)

	// DeleteDevice deletes a device by device ID.
	// If the device does not exist, this is a no-op and returns no error.
	//
	// Returns an error if:
	//   - The provider operation fails
	DeleteDevice(ctx context.Context, deviceID string) error

	// Data unit metadata (device-agnostic, replaces screenshot-specific methods)

	// SaveDataUnit saves metadata about a data unit (screenshot, sensor reading, audio sample, etc.).
	// The data unit must have a valid ID that uniquely identifies the data unit.
	//
	// Returns an error if:
	//   - The data unit is invalid (missing required fields)
	//   - Storage quota is exceeded (ErrQuotaExceeded)
	//   - The provider operation fails
	SaveDataUnit(ctx context.Context, dataUnit types.DataUnitMetadata) error

	// UpdateDataUnit updates an existing data unit using the provided update function.
	// The update function receives the current data unit and returns the updated data unit.
	//
	// Returns:
	//   - The updated DataUnitMetadata
	//   - An error if:
	//     - The data unit does not exist (ErrRecordNotFound)
	//     - Storage quota is exceeded (ErrQuotaExceeded)
	//     - The provider operation fails
	UpdateDataUnit(ctx context.Context, dataUnitID string, updateFn func(types.DataUnitMetadata) types.DataUnitMetadata) (types.DataUnitMetadata, error)

	// GetDataUnit retrieves a data unit by data unit ID.
	//
	// Returns:
	//   - The DataUnitMetadata if found
	//   - false if the data unit does not exist
	GetDataUnit(ctx context.Context, dataUnitID string) (types.DataUnitMetadata, bool)

	// ListDataUnits lists data units matching the provided filters.
	// If filters is nil, all data units are returned.
	//
	// Filter fields:
	//   - DeviceID: Filter by device ID
	//   - DeviceType: Filter by device type
	//   - DataType: Filter by data type (image, sensor, audio, etc.)
	//   - Label: Filter by label (motion, person, etc.)
	//   - StartTime: Filter by start time (inclusive)
	//   - EndTime: Filter by end time (inclusive)
	//
	// Returns:
	//   - A slice of DataUnitMetadata entries, sorted by creation time (newest first)
	//   - An error if the provider operation fails
	ListDataUnits(ctx context.Context, filters *types.DataUnitFilters) ([]types.DataUnitMetadata, error)

	// DeleteDataUnit deletes a data unit by data unit ID.
	// If the data unit does not exist, this is a no-op and returns no error.
	//
	// Returns an error if:
	//   - The provider operation fails
	DeleteDataUnit(ctx context.Context, dataUnitID string) error

	// Video clip metadata (device-agnostic)
	SaveVideoClip(ctx context.Context, clip types.VideoClipMetadata) error
	GetVideoClip(ctx context.Context, clipID string) (types.VideoClipMetadata, bool)
	ListVideoClips(ctx context.Context, filters map[string]interface{}) ([]types.VideoClipMetadata, error)
	DeleteVideoClip(ctx context.Context, clipID string) error

	// Security event metadata (structured, device-agnostic)

	// SaveSecurityEvent saves metadata about a security event.
	// The event must have a valid EventID that uniquely identifies the event.
	//
	// Returns an error if:
	//   - The event is invalid (missing required fields)
	//   - Storage quota is exceeded (ErrQuotaExceeded)
	//   - The provider operation fails
	SaveSecurityEvent(ctx context.Context, event types.SecurityEventMetadata) error

	// GetSecurityEvent retrieves a security event by event ID.
	//
	// Returns:
	//   - A pointer to SecurityEventMetadata if found
	//   - false if the event does not exist
	GetSecurityEvent(ctx context.Context, eventID string) (*types.SecurityEventMetadata, bool)

	// ListSecurityEvents lists security events matching the provided filters.
	// If filters is nil, all events are returned.
	//
	// Filter fields:
	//   - DeviceID: Filter by device ID
	//   - EventType: Filter by event type
	//   - Status: Filter by status (pending, acknowledged, etc.)
	//   - StartTime: Filter by start time (inclusive)
	//   - EndTime: Filter by end time (inclusive)
	//
	// Returns:
	//   - A slice of SecurityEventMetadata entries, sorted by event time (newest first)
	//   - An error if the provider operation fails
	ListSecurityEvents(ctx context.Context, filters *types.SecurityEventFilters) ([]types.SecurityEventMetadata, error)

	// DeleteSecurityEvent deletes a security event by event ID.
	// If the event does not exist, this is a no-op and returns no error.
	//
	// Returns an error if:
	//   - The provider operation fails
	DeleteSecurityEvent(ctx context.Context, eventID string) error

	// UpdateSecurityEventStatus updates the status and VM acknowledgment time of a security event.
	// This is typically called when the VM acknowledges an event.
	//
	// Parameters:
	//   - eventID: The unique identifier of the event
	//   - status: The new status (e.g., "acknowledged", "resolved")
	//   - vmACKTime: The time when the VM acknowledged the event (nil if not acknowledged)
	//
	// Returns an error if:
	//   - The event does not exist (ErrRecordNotFound)
	//   - The provider operation fails
	UpdateSecurityEventStatus(ctx context.Context, eventID string, status string, vmACKTime *time.Time) error

	// GetPendingSecurityEvents retrieves security events that are pending acknowledgment.
	// Events are returned sorted by event time (oldest first) up to the specified limit.
	//
	// Returns:
	//   - A slice of SecurityEventMetadata entries with status "pending"
	//   - An error if the provider operation fails
	GetPendingSecurityEvents(ctx context.Context, limit int) ([]types.SecurityEventMetadata, error)

	// Pending data unit request metadata (device-agnostic)
	SavePendingDataUnitRequest(ctx context.Context, deviceID string, request types.PendingDataUnitRequest) error
	GetPendingDataUnitRequest(ctx context.Context, deviceID string) (*types.PendingDataUnitRequest, bool)
	ListPendingDataUnitRequests(ctx context.Context) ([]types.PendingDataUnitRequest, error)
	DeletePendingDataUnitRequest(ctx context.Context, deviceID string) error

	// Edge state metadata (current state and history)
	SaveEdgeState(ctx context.Context, state map[string]interface{}) error
	GetCurrentEdgeState(ctx context.Context) (map[string]interface{}, bool)
	GetEdgeStateHistory(ctx context.Context, limit int) ([]map[string]interface{}, error)

	// Edge capabilities metadata (capabilities sent by VM)
	SaveEdgeCapabilities(ctx context.Context, capabilities map[string]interface{}) error
	GetEdgeCapabilities(ctx context.Context) (map[string]interface{}, bool)

	// Event bus metadata (for event bus persistence) - structured types

	// SaveEvent saves metadata about an event bus event.
	// The event must have a valid EventID that uniquely identifies the event.
	//
	// Returns an error if:
	//   - The event is invalid (missing required fields)
	//   - Storage quota is exceeded (ErrQuotaExceeded)
	//   - The provider operation fails
	SaveEvent(ctx context.Context, event types.EventBusEventMetadata) error

	// GetEvent retrieves an event bus event by event ID.
	//
	// Returns:
	//   - A pointer to EventBusEventMetadata if found
	//   - false if the event does not exist
	GetEvent(ctx context.Context, eventID string) (*types.EventBusEventMetadata, bool)

	// ListEvents lists event bus events matching the provided filters.
	// If filters is nil, all events are returned.
	//
	// Filter fields:
	//   - EventType: Filter by event type
	//   - Status: Filter by processing status (pending, processing, failed, etc.)
	//   - StartTime: Filter by start time (inclusive)
	//   - EndTime: Filter by end time (inclusive)
	//
	// Returns:
	//   - A slice of EventBusEventMetadata entries, sorted by event time (newest first)
	//   - An error if the provider operation fails
	ListEvents(ctx context.Context, filters *types.EventBusFilters) ([]types.EventBusEventMetadata, error)

	// DeleteEvent deletes an event bus event by event ID.
	// If the event does not exist, this is a no-op and returns no error.
	//
	// Returns an error if:
	//   - The provider operation fails
	DeleteEvent(ctx context.Context, eventID string) error

	// GetEventCount returns the total number of events in the event bus.
	//
	// Returns:
	//   - The total count of events
	//   - An error if the provider operation fails
	GetEventCount(ctx context.Context) (int, error)

	// Event processing status and retry tracking

	// UpdateEventProcessingStatus updates the processing status, retry count, and error information for an event.
	// This is typically called by the event processor to track event processing state.
	//
	// Parameters:
	//   - eventID: The unique identifier of the event
	//   - status: The new processing status (pending, processing, completed, failed, etc.)
	//   - retryCount: The number of retry attempts
	//   - lastError: The last error message (empty string if no error)
	//   - nextRetryTime: The time when the event should be retried (nil if no retry scheduled)
	//
	// Returns an error if:
	//   - The event does not exist (ErrRecordNotFound)
	//   - The provider operation fails
	UpdateEventProcessingStatus(ctx context.Context, eventID string, status string, retryCount int, lastError string, nextRetryTime *time.Time) error

	// GetFailedEvents retrieves events that have failed processing before the specified time.
	// This is used to identify events that need retry or manual intervention.
	//
	// Parameters:
	//   - beforeTime: Only return events that failed before this time
	//
	// Returns:
	//   - A slice of EventBusEventMetadata entries with status "failed", sorted by failure time
	//   - An error if the provider operation fails
	GetFailedEvents(ctx context.Context, beforeTime time.Time) ([]types.EventBusEventMetadata, error)

	// GetDeadLetterEvents retrieves events that have been moved to the dead letter queue.
	// Events are returned sorted by event time (oldest first) up to the specified limit.
	//
	// Parameters:
	//   - limit: Maximum number of events to return
	//
	// Returns:
	//   - A slice of EventBusEventMetadata entries from the dead letter queue
	//   - An error if the provider operation fails
	GetDeadLetterEvents(ctx context.Context, limit int) ([]types.EventBusEventMetadata, error)

	// MoveEventToDeadLetter moves an event to the dead letter queue.
	// This is typically called when an event has failed processing multiple times and should not be retried.
	//
	// Returns an error if:
	//   - The event does not exist (ErrRecordNotFound)
	//   - The provider operation fails
	MoveEventToDeadLetter(ctx context.Context, eventID string) error

	// ML lifecycle state operations

	// SaveMLLifecycleState saves or updates the ML lifecycle state for a device.
	// The state must have a valid DeviceID that uniquely identifies the device.
	// If a state already exists for the device, it will be overwritten.
	//
	// Note: For concurrent-safe updates, use UpdateMLLifecycleStateCAS instead.
	//
	// Returns an error if:
	//   - The state is invalid (missing required fields)
	//   - Storage quota is exceeded (ErrQuotaExceeded)
	//   - The provider operation fails
	SaveMLLifecycleState(ctx context.Context, deviceID string, stateInfo types.MLLifecycleStateInfo) error

	// GetMLLifecycleState retrieves the ML lifecycle state for a device.
	//
	// Returns:
	//   - A pointer to MLLifecycleStateInfo if found
	//   - false if no state exists for the device
	GetMLLifecycleState(ctx context.Context, deviceID string) (*types.MLLifecycleStateInfo, bool)

	// UpdateMLLifecycleState updates the ML lifecycle state for a device using the provided update function.
	// The update function receives the current state and returns the updated state.
	//
	// Note: This method is not thread-safe for concurrent updates. For concurrent-safe updates,
	// use UpdateMLLifecycleStateCAS instead.
	//
	// Returns:
	//   - A pointer to the updated MLLifecycleStateInfo
	//   - An error if:
	//     - The state does not exist for the device (ErrRecordNotFound)
	//     - Storage quota is exceeded (ErrQuotaExceeded)
	//     - The provider operation fails
	UpdateMLLifecycleState(ctx context.Context, deviceID string, updateFn func(types.MLLifecycleStateInfo) types.MLLifecycleStateInfo) (*types.MLLifecycleStateInfo, error)

	// UpdateMLLifecycleStateCAS performs a Compare-And-Swap (CAS) update on the ML lifecycle state.
	// This method is thread-safe and ensures atomic state transitions in concurrent environments.
	//
	// CAS Operation:
	//   1. Reads the current state for the device
	//   2. Verifies that the current version matches expectedVersion
	//   3. If version matches, applies the update function and increments the version
	//   4. If version does not match, returns an error (indicating concurrent modification)
	//
	// This is critical for atomic state transitions in ML lifecycle management.
	// All state transitions should use this method to prevent race conditions.
	//
	// Parameters:
	//   - deviceID: The unique identifier of the device
	//   - expectedVersion: The version number that must match the current state version
	//   - updateFn: Function that receives the current state and returns the updated state
	//
	// Returns:
	//   - A pointer to the updated MLLifecycleStateInfo (with incremented version)
	//   - An error if:
	//     - The state does not exist for the device (ErrRecordNotFound)
	//     - The version does not match expectedVersion (concurrent modification detected)
	//     - Storage quota is exceeded (ErrQuotaExceeded)
	//     - The provider operation fails
	//
	// Example usage:
	//   state, found := store.GetMLLifecycleState(ctx, "device-001")
	//   if !found {
	//       // Handle not found
	//   }
	//   updated, err := store.UpdateMLLifecycleStateCAS(ctx, "device-001", state.Version, func(s types.MLLifecycleStateInfo) types.MLLifecycleStateInfo {
	//       s.State = types.MLLifecycleStateDeploying
	//       s.UpdatedAt = time.Now()
	//       return s
	//   })
	//   if err != nil {
	//       // Handle error (may be version conflict - retry with new version)
	//   }
	UpdateMLLifecycleStateCAS(ctx context.Context, deviceID string, expectedVersion int, updateFn func(types.MLLifecycleStateInfo) types.MLLifecycleStateInfo) (*types.MLLifecycleStateInfo, error)

	// ListMLLifecycleStates lists ML lifecycle states matching the provided filters.
	// If filters is nil, all states are returned.
	//
	// Filter fields:
	//   - DeviceID: Filter by device ID
	//   - DeviceType: Filter by device type
	//   - State: Filter by lifecycle state (unassigned, active, deploying, etc.)
	//   - ModelID: Filter by model ID
	//
	// Returns:
	//   - A slice of MLLifecycleStateInfo entries, sorted by device ID
	//   - An error if the provider operation fails
	ListMLLifecycleStates(ctx context.Context, filters *types.MLLifecycleFilters) ([]types.MLLifecycleStateInfo, error)

	// DeleteMLLifecycleState deletes the ML lifecycle state for a device.
	// If the state does not exist, this is a no-op and returns no error.
	//
	// Returns an error if:
	//   - The provider operation fails
	DeleteMLLifecycleState(ctx context.Context, deviceID string) error

	// Pending model deployment operations
	SavePendingModelDeployment(ctx context.Context, deviceID string, deployment types.PendingModelDeployment) error
	GetPendingModelDeployment(ctx context.Context, deviceID string) (*types.PendingModelDeployment, bool)
	ListPendingModelDeployments(ctx context.Context) ([]types.PendingModelDeployment, error)
	DeletePendingModelDeployment(ctx context.Context, deviceID string) error

	// Health monitoring

	// HealthSnapshot returns the current health status of the storage service.
	// This follows the vm-gateway pattern for health snapshots.
	//
	// The snapshot includes:
	//   - Overall status: healthy, warning, full, corrupted
	//   - Quota information: usage, limits, thresholds
	//   - Integrity errors: count of detected errors
	//   - Provider health: provider-specific status
	//   - Bucket counts: record counts per bucket
	//   - Total records: total number of records
	//   - Database size: database file size in MB
	//   - Last cleanup time: when retention cleanup last ran
	//   - Cleanup statistics: records deleted, space freed
	//   - Schema version: current schema version
	//
	// This method is safe to call frequently and does not perform expensive operations.
	// It aggregates data from quota, retention, integrity managers, and the provider.
	//
	// Returns:
	//   - StorageHealth containing comprehensive health information
	HealthSnapshot(ctx context.Context) types.StorageHealth

	// Lifecycle methods

	// Start starts the metadata store service.
	// This method performs the following operations:
	//   1. Initializes the storage provider (opens database connections)
	//   2. Creates required buckets/namespaces
	//   3. Runs schema migrations (if any pending)
	//   4. Starts background tasks (quota monitoring, retention cleanup, integrity checks)
	//
	// This method should be called after all dependencies are configured (e.g., event emitter).
	// If called multiple times, returns ErrAlreadyStarted.
	//
	// Returns an error if:
	//   - The service is already started (ErrAlreadyStarted)
	//   - Provider initialization fails
	//   - Bucket creation fails
	//   - Schema migration fails
	//   - Background task startup fails
	Start(ctx context.Context) error

	// Stop gracefully shuts down the metadata store service.
	// This method performs the following operations:
	//   1. Stops background tasks (quota monitoring, retention cleanup, integrity checks)
	//   2. Flushes pending operations
	//   3. Closes provider connections
	//   4. Closes database connections
	//
	// This method should be called during service shutdown.
	// It is safe to call multiple times (idempotent).
	//
	// Returns an error if:
	//   - Background task shutdown fails
	//   - Provider shutdown fails
	Stop(ctx context.Context) error

	// Close closes the metadata store and releases all resources (e.g., database connections).
	// This is a legacy method for backward compatibility. Prefer using Stop() instead.
	//
	// After Close, all methods should return errors.
	// It is safe to call multiple times (idempotent).
	//
	// Returns an error if:
	//   - Resource cleanup fails
	Close() error
}

// NewMetaDataStore creates a new metadata store instance.
// This factory function should typically not be called directly;
// use MetaStorageProvider instead for proper dependency injection.
//
// The store uses provider-specific implementations (BoltDB, SQLite, PostgreSQL, etc.)
// based on the configuration. This allows switching between storage providers
// without changing application code.
func NewMetaDataStore(ctx context.Context, config *types.MetaStorageConfig, logger *zap.Logger) (MetaDataStore, error) {
	switch config.Provider {
	case "bbolt":
		// Create BoltDB provider
		bboltConfig := &types.BoltDBConfig{
			DataDir:      config.DataDir,
			DatabaseFile: "meta.db",
			FileMode:     0600,
			Timeout:      1, // 1 second
			NoSync:       false,
		}
		// Override with BoltDB-specific config if provided
		if config.BoltDB != nil {
			if config.BoltDB.DataDir != "" {
				bboltConfig.DataDir = config.BoltDB.DataDir
			}
			if config.BoltDB.DatabaseFile != "" {
				bboltConfig.DatabaseFile = config.BoltDB.DatabaseFile
			}
			if config.BoltDB.FileMode != 0 {
				bboltConfig.FileMode = config.BoltDB.FileMode
			}
			if config.BoltDB.Timeout != 0 {
				bboltConfig.Timeout = config.BoltDB.Timeout
			}
			bboltConfig.NoSync = config.BoltDB.NoSync
		}

		provider, err := implbbolt.NewBoltDBProvider(ctx, bboltConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create BoltDB provider: %w", err)
		}

		// Create main implementation
		storeImpl := impl.NewMetaStorageImpl(provider, logger)

		// Initialize quota manager if quota config is provided
		if config.Quota != nil {
			// Get database path for quota tracking
			databasePath := ""
			if config.DataDir != "" {
				databasePath = fmt.Sprintf("%s/%s", config.DataDir, bboltConfig.DatabaseFile)
			}
			quotaManager := impl.NewQuotaManager(provider, config.Quota, logger, databasePath)
			storeImpl.SetQuotaManager(quotaManager)
		}

		// Initialize retention manager if retention config is provided
		if config.Retention != nil {
			retentionManager := impl.NewRetentionManager(provider, config.Retention, logger)
			storeImpl.SetRetentionManager(retentionManager)
		}

		// Initialize integrity manager (always enabled for production reliability)
		integrityManager := impl.NewIntegrityManager(provider, logger)
		storeImpl.SetIntegrityManager(integrityManager)

		// Initialize buckets
		if err := storeImpl.InitializeBuckets(ctx); err != nil {
			_ = provider.Close()
			return nil, fmt.Errorf("failed to initialize buckets: %w", err)
		}

		return storeImpl, nil
	default:
		return nil, fmt.Errorf("unsupported meta-storage provider: %s", config.Provider)
	}
}

// MetaStorageProvider creates the meta storage service with fx lifecycle management.
//
// The storage service manages metadata about stored objects, devices, models, events, and system state.
// It provides a unified interface for all metadata storage operations, abstracting away
// the details of the underlying storage provider.
//
// Architecture decision (Section 1.0): Service-owned lifecycle.
//   - Fx manages only the storage service lifecycle.
//   - Service Start/Stop is the single place that initializes/closes storage providers.
//   - Storage providers do not register their own fx.Lifecycle hooks.
//
// Fail-fast behavior: This provider will return an error (not nil) if:
//   - Configuration is invalid or unsupported
//   - Storage service creation fails
//   - Required dependencies are missing
//
// The application will not start if storage service creation fails, ensuring production reliability.
func MetaStorageProvider(lc fx.Lifecycle, cfg *types.MetaStorageConfig, logger *zap.Logger) (MetaDataStore, error) {
	store, err := NewMetaDataStore(context.Background(), cfg, logger)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Start the storage service
			if err := store.Start(ctx); err != nil {
				logger.Error("Failed to start meta storage", zap.Error(err))
				return err
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// Stop the storage service gracefully
			if err := store.Stop(ctx); err != nil {
				logger.Error("Failed to stop meta storage", zap.Error(err))
				return err
			}
			return nil
		},
	})

	return store, nil
}
