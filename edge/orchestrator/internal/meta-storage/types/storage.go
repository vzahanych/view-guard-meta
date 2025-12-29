package types

import "time"

// RecordMetadata represents metadata about a stored record.
// This is used for tracking record lifecycle and versioning.
type RecordMetadata struct {
	// Key is the unique key for the record
	Key string

	// Bucket is the bucket/namespace where the record is stored
	Bucket string

	// CreatedAt is when the record was created
	CreatedAt time.Time

	// UpdatedAt is when the record was last updated
	UpdatedAt time.Time

	// Version is the record version (for CAS operations)
	Version int
}

// StorageQuota represents storage quota information.
// This is used for quota tracking and enforcement.
type StorageQuota struct {
	// Used is the current storage usage in bytes
	Used int64

	// Limit is the maximum storage limit in bytes
	Limit int64

	// WarningThreshold is the warning threshold percentage (e.g., 80)
	WarningThreshold int

	// FullThreshold is the full threshold percentage (e.g., 95)
	FullThreshold int
}

// BucketInfo represents information about a storage bucket.
type BucketInfo struct {
	// Name is the bucket name
	Name string

	// RecordCount is the number of records in the bucket
	RecordCount int64

	// SizeBytes is the total size of records in the bucket in bytes
	SizeBytes int64
}

// HealthStatus represents the health status of the storage service.
type HealthStatus int

const (
	// HealthStatusHealthy indicates the storage service is healthy
	HealthStatusHealthy HealthStatus = iota

	// HealthStatusWarning indicates the storage service is in a warning state (e.g., quota 80-90%)
	HealthStatusWarning

	// HealthStatusFull indicates the storage service is full (quota >95%)
	HealthStatusFull

	// HealthStatusCorrupted indicates the storage service has detected corruption
	HealthStatusCorrupted
)

// String returns the string representation of HealthStatus
func (s HealthStatus) String() string {
	switch s {
	case HealthStatusHealthy:
		return "healthy"
	case HealthStatusWarning:
		return "warning"
	case HealthStatusFull:
		return "full"
	case HealthStatusCorrupted:
		return "corrupted"
	default:
		return "unknown"
	}
}

// CleanupStats contains statistics about cleanup operations.
// This is used in StorageHealth to report cleanup statistics.
type CleanupStats struct {
	// RecordsDeleted is the number of records deleted in the last cleanup
	RecordsDeleted int64

	// SpaceFreedBytes is the approximate space freed in bytes
	SpaceFreedBytes int64

	// BucketsProcessed is the number of buckets processed
	BucketsProcessed int

	// Duration is how long the cleanup took
	Duration time.Duration
}

// StorageHealth represents the health status of the storage service.
// This follows the vm-gateway pattern for health snapshots.
type StorageHealth struct {
	// Status is the overall health status
	Status HealthStatus

	// Quota is the current quota status (nil if quota management is disabled)
	Quota *StorageQuota

	// IntegrityErrors is the count of integrity failures detected
	IntegrityErrors int

	// LastHealthCheck is when the last health check was performed
	LastHealthCheck time.Time

	// ProviderHealth is the provider-specific health status string
	// Values: "healthy", "degraded", "unhealthy"
	ProviderHealth string

	// BucketCounts is a map of bucket names to record counts
	BucketCounts map[string]int

	// TotalRecords is the total number of records across all buckets
	TotalRecords int64

	// DatabaseSizeMB is the size of the database file in megabytes
	DatabaseSizeMB float64

	// LastCleanupTime is when the last retention cleanup was performed
	LastCleanupTime time.Time

	// CleanupStats contains statistics from the last cleanup operation (nil if no cleanup has been performed)
	CleanupStats *CleanupStats

	// SchemaVersion is the current schema version
	SchemaVersion int

	// ProviderStatus contains provider-specific status details (e.g., connection count, transaction stats)
	ProviderStatus map[string]interface{}
}

// DeviceID is a type alias for device identifiers.
// This replaces CameraID to support device-agnostic architecture.
type DeviceID string

// DeviceType represents the type of device.
type DeviceType string

const (
	// DeviceTypeCamera represents a camera device
	DeviceTypeCamera DeviceType = "camera"
	// DeviceTypeSensor represents a sensor device
	DeviceTypeSensor DeviceType = "sensor"
	// DeviceTypeAudioDevice represents an audio device
	DeviceTypeAudioDevice DeviceType = "audio_device"
	// DeviceTypeOther represents other types of IoT devices
	DeviceTypeOther DeviceType = "other"
)

// String returns the string representation of DeviceType
func (dt DeviceType) String() string {
	return string(dt)
}

// DeviceMetadata represents metadata for a device (replaces CameraMetadata).
// This is device-agnostic and supports cameras, sensors, audio devices, and other IoT devices.
type DeviceMetadata struct {
	// ID is the unique device identifier
	ID DeviceID

	// Name is the human-readable device name
	Name string

	// DeviceType is the type of device (camera, sensor, audio_device, etc.)
	DeviceType DeviceType

	// Manufacturer is the device manufacturer
	Manufacturer string

	// Model is the device model
	Model string

	// Enabled indicates whether the device is enabled
	Enabled bool

	// Status is the current device status (unknown, online, offline, connecting, error)
	Status string

	// LastSeen is when the device was last seen/contacted
	LastSeen *time.Time

	// DiscoveredAt is when the device was discovered
	DiscoveredAt time.Time

	// Device-specific fields (for cameras: IPAddress, ONVIFEndpoint, RTSPURLs, etc.)
	IPAddress     string
	ONVIFEndpoint string
	RTSPURLs      []string
	DevicePath    string

	// Config contains device-specific configuration
	Config map[string]interface{}

	// Capabilities contains device capabilities
	Capabilities map[string]interface{}

	// SyncedWithVM indicates whether the device has been synced with VM
	SyncedWithVM bool

	// SyncedAt is the timestamp of last sync with VM
	SyncedAt *time.Time

	// VMDeviceID is the device ID on VM side (if different from Edge ID)
	VMDeviceID string

	// CreatedAt is when the device metadata was created
	CreatedAt time.Time

	// UpdatedAt is when the device metadata was last updated
	UpdatedAt time.Time
}

// DeviceFilters contains filters for listing devices (replaces CameraFilters).
type DeviceFilters struct {
	// EnabledOnly filters by enabled status
	EnabledOnly *bool

	// Status filters by device status (e.g., "online", "offline", "discovered")
	Status *string

	// SyncedWithVM filters by VM sync status
	SyncedWithVM *bool

	// DeviceType filters by device type (e.g., "camera", "sensor", "audio_device")
	DeviceType *DeviceType
}

// DataUnitMetadata represents metadata for a data unit (replaces ScreenshotMetadata).
// This is device-agnostic and supports images, sensor readings, audio samples, etc.
type DataUnitMetadata struct {
	// ID is the unique data unit identifier
	ID string

	// DeviceID is the device that generated this data unit
	DeviceID DeviceID

	// DeviceType is the type of device that generated this data unit
	DeviceType DeviceType

	// DataType is the type of data (image, sensor_reading, audio_sample, etc.)
	DataType string

	// ObjectKey is the object storage key for the data unit
	ObjectKey string

	// ThumbnailKey is the object storage key for the thumbnail (if applicable)
	ThumbnailKey string

	// Label is the data unit label
	Label string

	// CustomLabel is a custom label for the data unit
	CustomLabel string

	// Description is a description of the data unit
	Description string

	// Metadata contains additional metadata
	Metadata map[string]interface{}

	// CreatedAt is when the data unit was created
	CreatedAt time.Time

	// UpdatedAt is when the data unit was last updated
	UpdatedAt time.Time
}

// DataUnitFilters contains filters for querying data units.
type DataUnitFilters struct {
	// DeviceID filters by device ID
	DeviceID *DeviceID

	// DeviceType filters by device type (e.g., "camera", "sensor", "audio_device")
	DeviceType *DeviceType

	// DataType filters by data type (e.g., "image", "sensor_reading", "audio_sample")
	DataType *string

	// Label filters by label
	Label *string

	// CustomLabel filters by custom label
	CustomLabel *string

	// CreatedAfter filters by creation time (after this time)
	CreatedAfter *time.Time

	// CreatedBefore filters by creation time (before this time)
	CreatedBefore *time.Time

	// Limit limits the number of results
	Limit *int
}

// VideoClipMetadata represents metadata for a video clip (replaces ClipMetadata).
// This is device-agnostic and part of the data units system.
type VideoClipMetadata struct {
	// ID is the unique clip identifier
	ID string

	// DeviceID is the device that generated this clip
	DeviceID DeviceID

	// EventID is the associated event ID
	EventID string

	// ObjectKey is the object storage key for the clip
	ObjectKey string

	// Duration is the clip duration
	Duration time.Duration

	// SizeBytes is the clip size in bytes
	SizeBytes int64

	// CreatedAt is when the clip was created
	CreatedAt time.Time

	// Metadata contains additional metadata
	Metadata map[string]interface{}
}

// ModelDeploymentMetadata represents metadata for a deployed model (replaces DeployedModelMetadata).
// This is device-agnostic and supports models deployed to any device type.
type ModelDeploymentMetadata struct {
	// ModelID is the unique model identifier
	ModelID string

	// DeploymentID is the unique deployment identifier
	DeploymentID string

	// DeviceID is the device where the model is deployed (replaces CameraID)
	DeviceID DeviceID

	// DeviceType is the type of device (camera, sensor, etc.)
	DeviceType DeviceType

	// ModelPath is the path to the model file
	ModelPath string

	// MetadataPath is the path to the model metadata file
	MetadataPath string

	// ManifestPath is the path to the model manifest file (new field)
	ManifestPath string

	// DeployedAt is when the model was deployed
	DeployedAt time.Time

	// Status is the deployment status (active, inactive, failed)
	Status string

	// EdgeID is the edge device identifier
	EdgeID string

	// Version is the model version
	Version string

	// ModelType is the type of model (e.g., "yolo", "classification")
	ModelType string

	// Framework is the ML framework (e.g., "openvino", "tensorflow")
	Framework string

	// VerificationResults contains verification results (signature, hash, compatibility checks)
	VerificationResults map[string]interface{}

	// CreatedAt is when the deployment record was created
	CreatedAt time.Time

	// UpdatedAt is when the deployment record was last updated
	UpdatedAt time.Time
}

// Note: ModelFilters is defined in types.go and updated there to use DeviceID instead of CameraID.
// ModelDeploymentMetadata is the new device-agnostic type that replaces DeployedModelMetadata.

// Note: Existing types (StorageEntryMetadata, DeployedModelMetadata, StorageStats, ModelFilters,
// CameraMetadata, CameraFilters, ScreenshotMetadata, ClipMetadata) are kept in types.go
// for backward compatibility. New device-agnostic types are defined above.

// SecurityEventMetadata represents structured metadata for a security event.
// This replaces the previous use of map[string]interface{} for security events.
type SecurityEventMetadata struct {
	// EventID is the unique security event identifier
	EventID string

	// DeviceID is the device where the event occurred
	DeviceID DeviceID

	// DeviceType is the type of device (camera, sensor, etc.)
	DeviceType DeviceType

	// EventType is the type of event (e.g., "intrusion", "motion", "line_crossing")
	EventType string

	// Timestamp is when the event occurred
	Timestamp time.Time

	// Confidence is the model's confidence in the event (0.0 - 1.0)
	Confidence float64

	// ModelID is the ID of the model that detected the event
	ModelID string

	// ModelVersion is the version of the model that detected the event
	ModelVersion string

	// Status is the delivery status (pending_delivery, delivery_failed, delivered)
	Status string

	// DeliveryAttempts is the number of delivery attempts made
	DeliveryAttempts int

	// LastDeliveryAttempt is when the last delivery attempt was made
	LastDeliveryAttempt time.Time

	// VMACKTimestamp is when the VM acknowledged the event
	VMACKTimestamp time.Time

	// AttachmentRefs are object storage keys for any attachments (images, clips, etc.)
	AttachmentRefs []string

	// Metadata contains additional metadata about the event
	Metadata map[string]interface{}
}

// SecurityEventFilters contains filters for querying security events.
type SecurityEventFilters struct {
	// DeviceID filters by device ID
	DeviceID *DeviceID

	// DeviceType filters by device type
	DeviceType *DeviceType

	// EventType filters by event type
	EventType *string

	// Status filters by delivery status
	Status *string

	// ModelID filters by model ID
	ModelID *string

	// ModelVersion filters by model version
	ModelVersion *string

	// From filters by events occurring at or after this time
	From *time.Time

	// To filters by events occurring at or before this time
	To *time.Time

	// Limit limits the number of results
	Limit *int
}

// EventBusEventMetadata represents metadata for an event bus event.
// This replaces the map[string]interface{} type with a structured type.
type EventBusEventMetadata struct {
	// EventID is the unique event identifier
	EventID string

	// EventType is the type of event
	EventType string

	// Timestamp is when the event occurred
	Timestamp time.Time

	// Data contains the event payload
	Data map[string]interface{}

	// ProcessingStatus is the current processing status
	// Values: pending, processing, completed, failed, dead_letter
	ProcessingStatus string

	// RetryCount is the number of retry attempts
	RetryCount int

	// LastError is the last error message (if any)
	LastError string

	// NextRetryTime is when the event should be retried (if applicable)
	NextRetryTime *time.Time

	// CreatedAt is when the event was created
	CreatedAt time.Time

	// UpdatedAt is when the event was last updated
	UpdatedAt time.Time
}

// EventBusFilters contains filters for querying event bus events.
type EventBusFilters struct {
	// EventType filters by event type
	EventType *string

	// ProcessingStatus filters by processing status
	ProcessingStatus *string

	// From filters by events occurring at or after this time
	From *time.Time

	// To filters by events occurring at or before this time
	To *time.Time

	// Limit limits the number of results
	Limit *int
}

// PendingDataUnitRequest represents a pending data unit capture request (replaces pending snapshot request).
// This is device-agnostic and supports images, sensor readings, audio samples, etc.
type PendingDataUnitRequest struct {
	// DeviceID is the device that should generate the data unit (replaces CameraID)
	DeviceID DeviceID

	// DeviceType is the type of device (camera, sensor, audio_device, etc.)
	DeviceType DeviceType

	// DataType is the type of data to capture (image, sensor_reading, audio_sample, etc.)
	DataType string

	// Label is the label for the data unit
	Label string

	// CustomLabel is a custom label for the data unit
	CustomLabel string

	// Count is the number of data units to capture
	Count int32

	// RequestedAt is when the request was made
	RequestedAt time.Time

	// Status is the request status (pending, in_progress, completed, failed)
	Status string
}

