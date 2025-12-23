package types

import "time"

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

// ModelFilters contains filters for listing models
type ModelFilters struct {
	EdgeID   *string
	CameraID *string
	Status   *string
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
	CreatedAt     time.Time
	UpdatedAt     time.Time
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

// MetaStorageConfig contains configuration for the meta-storage service
type MetaStorageConfig struct {
	Provider  string `yaml:"provider"`
	Endpoint  string `yaml:"endpoint"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
	DataDir   string `yaml:"data_dir"` // Data directory for local storage (used by bbolt)
}
