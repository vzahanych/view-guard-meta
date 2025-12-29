package types

import "time"

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

// DataType represents the type of data stored.
type DataType string

const (
	// DataTypeVideoClip represents a video clip
	DataTypeVideoClip DataType = "video_clip"
	// DataTypeVideoFrame represents a single video frame
	DataTypeVideoFrame DataType = "video_frame"
	// DataTypeImage represents an image
	DataTypeImage DataType = "image"
	// DataTypeSensorReading represents a sensor reading
	DataTypeSensorReading DataType = "sensor_reading"
	// DataTypeAudioSample represents an audio sample
	DataTypeAudioSample DataType = "audio_sample"
	// DataTypeModelArtifact represents a model artifact
	DataTypeModelArtifact DataType = "model_artifact"

	// DataTypeSecurityEventAttachment represents a security event attachment
	DataTypeSecurityEventAttachment DataType = "security_event_attachment"
)

// String returns the string representation of DataType
func (dt DataType) String() string {
	return string(dt)
}

// ObjectMetadata represents metadata about a stored object.
// This is used by the provider interface to return object information.
type ObjectMetadata struct {
	// Key is the object key/path
	Key string

	// Size is the object size in bytes
	Size int64

	// ContentType is the MIME type of the object
	ContentType string

	// Hash is the SHA-256 hash of the object content
	Hash string

	// CreatedAt is when the object was created
	CreatedAt time.Time

	// DeviceID is the device ID that created this object
	DeviceID DeviceID

	// DeviceType is the type of device (camera, sensor, audio_device, etc.)
	DeviceType DeviceType

	// Metadata contains additional metadata (key-value pairs)
	Metadata map[string]string

	// UploadCompletedAt is when the upload was completed (for dataset retention)
	UploadCompletedAt *time.Time

	// VMAckAt is when the VM acknowledged the event (for event retention)
	VMAckAt *time.Time
}

// DataUnit represents a data unit (image, video clip, sensor reading, etc.).
// This is the device-agnostic representation of stored data.
type DataUnit struct {
	// DeviceID is the device that generated this data unit
	DeviceID DeviceID

	// DeviceType is the type of device that generated this data unit
	DeviceType DeviceType

	// DataType is the type of data (image, video_clip, sensor_reading, etc.)
	DataType DataType

	// Key is the object storage key for the data unit
	Key string

	// Size is the size of the data unit in bytes
	Size int64

	// ContentType is the MIME type
	ContentType string

	// Hash is the SHA-256 hash of the content
	Hash string

	// CreatedAt is when the data unit was created
	CreatedAt time.Time

	// Metadata contains additional metadata
	Metadata map[string]interface{}
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

// RetentionPolicy represents a retention policy for objects.
type RetentionPolicy struct {
	// RetentionDays is the retention period in days
	RetentionDays int

	// CleanupSchedule is the cleanup schedule (e.g., "0 0 * * *" for daily at midnight)
	CleanupSchedule string
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

// StorageHealth represents the health status of the object storage service.
type StorageHealth struct {
	// Status is the overall health status
	Status HealthStatus

	// Quota contains quota information
	Quota *StorageQuota

	// IntegrityErrors is the count of integrity failures
	IntegrityErrors int

	// LastHealthCheck is when the last health check was performed
	LastHealthCheck time.Time

	// ProviderHealth is the provider-specific health status string
	ProviderHealth string

	// ObjectCounts is a map of data type to object count
	ObjectCounts map[string]int64

	// TotalSizeMB is the total storage size in megabytes
	TotalSizeMB int64

	// LastCleanupTime is when the last cleanup was performed
	LastCleanupTime time.Time

	// CleanupStats contains cleanup statistics
	CleanupStats *CleanupStats

	// ProviderStatus contains provider-specific status details
	ProviderStatus map[string]interface{}
}

// CleanupStats contains statistics about cleanup operations.
type CleanupStats struct {
	// ObjectsDeleted is the number of objects deleted in the last cleanup
	ObjectsDeleted int64

	// SpaceFreedBytes is the approximate space freed in bytes
	SpaceFreedBytes int64

	// DataTypesProcessed is the number of data types processed (for object storage)
	DataTypesProcessed int

	// Duration is how long the cleanup took
	Duration time.Duration
}

