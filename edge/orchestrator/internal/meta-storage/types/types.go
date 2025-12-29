package types

import "time"

// This file maintains backward compatibility by keeping existing type definitions.
// New types have been added to their respective files:
//   - config.go: MetaStorageConfig (moved from here)
//   - storage.go: RecordMetadata, StorageQuota, BucketInfo, HealthStatus (new types)
//   - schema.go: SchemaVersion, SchemaMigration (new types)
//   - provider.go: MetaStorageProvider, KeyValue, BoltDBConfig, SQLiteConfig, PostgreSQLConfig (new types)
//   - errors.go: (reserved for future error types)

// StorageEntryMetadata represents metadata for a stored file (clip or snapshot)
type StorageEntryMetadata struct {
	Path      string
	FileType  string // "clip" or "snapshot"
	SizeBytes int64
	CameraID  string
	EventID   string
	CreatedAt time.Time
	ExpiresAt *time.Time
}

// DeployedModelMetadata represents metadata for a deployed model
type DeployedModelMetadata struct {
	ModelID      string
	DeploymentID *string
	ModelPath    string
	MetadataPath string
	DeployedAt   time.Time
	Status       string // 'active', 'inactive', 'failed'
	EdgeID       string
	CameraID     *string
	Version      string
	ModelType    string
	Framework    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// StorageStats contains storage statistics
type StorageStats struct {
	TotalClips       int
	TotalSnapshots   int
	TotalSizeBytes   int64
	DiskUsagePercent float64
	AvailableBytes   int64
}

// ModelFilters contains filters for listing model deployments.
// Updated to use DeviceID instead of CameraID for device-agnostic support.
type ModelFilters struct {
	// EdgeID filters by edge device ID
	EdgeID *string

	// DeviceID filters by device ID (replaces CameraID)
	DeviceID *string

	// DeviceType filters by device type (e.g., "camera", "sensor", "audio_device")
	DeviceType *string

	// Status filters by deployment status
	Status *string

	// ModelType filters by model type
	ModelType *string

	// Framework filters by ML framework
	Framework *string

	// CameraID is kept for backward compatibility (deprecated: use DeviceID instead)
	CameraID *string
}

// CameraMetadata represents metadata for a CCTV camera
type CameraMetadata struct {
	ID            string
	Name          string
	Type          string // "rtsp", "onvif", "usb"
	Manufacturer  string
	Model         string
	Enabled       bool
	Status        string // "unknown", "online", "offline", "connecting", "error"
	LastSeen      *time.Time
	DiscoveredAt  time.Time
	IPAddress     string
	ONVIFEndpoint string
	RTSPURLs      []string
	DevicePath    string
	Config        map[string]interface{} // Camera configuration
	Capabilities  map[string]interface{} // Camera capabilities
	SyncedWithVM  bool                   // Whether camera has been synced with VM
	SyncedAt      *time.Time             // Timestamp of last sync with VM
	VMCameraID    string                 // Camera ID on VM side (if different from Edge ID)
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CameraFilters contains filters for listing cameras
type CameraFilters struct {
	EnabledOnly   *bool   // Filter by enabled status
	Status        *string // Filter by status (e.g., "online", "offline", "discovered")
	SyncedWithVM  *bool   // Filter by VM sync status
	Type          *string // Filter by camera type (e.g., "rtsp", "onvif", "usb")
}

// ScreenshotMetadata represents metadata for a screenshot
type ScreenshotMetadata struct {
	ID           string
	CameraID     string
	ObjectKey    string
	ThumbnailKey string // Object key for thumbnail in object storage
	Label        string
	CustomLabel  string
	Description  string
	Metadata     map[string]interface{}
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ClipMetadata represents metadata for a video clip
type ClipMetadata struct {
	ID        string
	CameraID  string
	EventID   string
	ObjectKey string
	Duration  time.Duration
	SizeBytes int64
	CreatedAt time.Time
	Metadata  map[string]interface{}
}
