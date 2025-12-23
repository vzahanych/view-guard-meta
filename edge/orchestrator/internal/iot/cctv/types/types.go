package types

import "time"

// Frame represents a single video frame (moved from @video package)
// This is the standard frame format used throughout the system
type Frame struct {
	Data      []byte    // JPEG-encoded frame data
	Timestamp time.Time // Frame timestamp
	Width     int       // Frame width
	Height    int       // Frame height
	CameraID  string    // Camera ID this frame came from
}

// MJPEGStream represents an active MJPEG stream (moved from @streaming package)
type MJPEGStream struct {
	CameraID     string
	FrameChan    chan []byte
	Done         <-chan struct{} // Channel that's closed when stream is done
	GetLastFrame func() []byte   // Function to get the last captured frame
}

// Camera represents a CCTV camera
type Camera struct {
	ID           string
	Name         string
	Type         CameraType
	Manufacturer string
	Model        string
	Enabled      bool
	Status       CameraStatus
	LastSeen     *time.Time
	DiscoveredAt time.Time

	// Network camera fields
	IPAddress     string
	ONVIFEndpoint string
	RTSPURLs      []string

	// USB camera fields
	DevicePath string

	// Configuration
	Config       CameraConfig
	Capabilities CameraCapabilities
}

// CameraType represents the type of camera
type CameraType string

const (
	CameraTypeRTSP  CameraType = "rtsp"
	CameraTypeONVIF CameraType = "onvif"
	CameraTypeUSB   CameraType = "usb"
)

// CameraStatus represents camera connection status
type CameraStatus string

const (
	CameraStatusUnknown    CameraStatus = "unknown"
	CameraStatusOnline     CameraStatus = "online"
	CameraStatusOffline    CameraStatus = "offline"
	CameraStatusConnecting CameraStatus = "connecting"
	CameraStatusError      CameraStatus = "error"
)

// CameraConfig represents camera configuration
type CameraConfig struct {
	RecordingEnabled bool
	MotionDetection  bool
	Quality          string
	FrameRate        int
	Resolution       string
}

// CameraCapabilities represents camera capabilities
type CameraCapabilities struct {
	HasPTZ          bool
	HasSnapshot     bool
	HasVideoStreams bool
	StreamProfiles  []StreamProfile
}

// StreamProfile represents a video stream profile
type StreamProfile struct {
	Name      string
	Width     int
	Height    int
	FrameRate float64
	RTSPURL   string
	Encoding  string
}

// CameraUpdate represents updates to a camera
type CameraUpdate struct {
	Name    *string
	Enabled *bool
	Config  *CameraConfig
}

// Screenshot represents a captured screenshot
type Screenshot struct {
	ID           string
	CameraID     string
	ObjectKey    string // Object key in object-storage (replaces FilePath)
	ThumbnailKey string // Object key for thumbnail in object-storage
	Label        string
	CustomLabel  string
	Description  string
	Metadata     map[string]interface{}
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CreatedBy    string
}

// ScreenshotFilters contains filters for listing screenshots
type ScreenshotFilters struct {
	CameraID      *string
	Label         *string
	CustomLabel   *string
	Description   *string
	StartTime     *time.Time
	EndTime       *time.Time
	CreatedAfter  time.Time
	CreatedBefore time.Time
	SortBy        string
	SortOrder     string
	Limit         int
	Offset        int
}

// ScreenshotUpdate contains fields to update
type ScreenshotUpdate struct {
	Label       *string
	CustomLabel *string
	Description *string
	Metadata    map[string]interface{}
}

// DatasetStatus represents dataset readiness status for a camera
type DatasetStatus struct {
	LabelCounts           map[string]int
	LabeledSnapshotCount  int
	RequiredSnapshotCount int
	SnapshotRequired      bool
	LastSynced            time.Time
}

// ScreenshotStorageStats holds storage statistics for screenshots
type ScreenshotStorageStats struct {
	TotalScreenshots    int
	TotalSizeBytes      int64
	Cameras             map[string]CameraScreenshotStats
	OldestScreenshotAt  string
	NewestScreenshotAt  string
	OrphanedRecordCount int
	DiskTotalBytes      int64
	DiskUsedBytes       int64
	DiskAvailableBytes  int64
	DiskUsagePercent    float64
	MaxDiskUsagePercent float64
}

// CameraScreenshotStats holds screenshot stats per camera
type CameraScreenshotStats struct {
	CameraID        string
	ScreenshotCount int
	TotalSizeBytes  int64
}

// StorageCleanupOptions controls storage cleanup behavior
type StorageCleanupOptions struct {
	CleanupOrphanedFiles   bool
	CleanupOrphanedRecords bool
	RetentionDays          int
}

// StorageCleanupResult describes the outcome of a storage cleanup operation
type StorageCleanupResult struct {
	OrphanedFilesDeleted   int
	OrphanedRecordsDeleted int
	OldScreenshotsDeleted  int
	FreedBytes             int64
}

// DatasetExportResult represents dataset export details
type DatasetExportResult struct {
	FilePath     string
	SampleCount  int
	ManifestName string
	CreatedAt    time.Time
}

// Clip represents a recorded video clip
type Clip struct {
	ID        string
	CameraID  string
	EventID   string
	ObjectKey string // Object key in object-storage
	Duration  time.Duration
	SizeBytes int64
	CreatedAt time.Time
	Metadata  map[string]interface{}
}

// ClipFilters contains filters for listing clips
type ClipFilters struct {
	CameraID  *string
	EventID   *string
	StartTime *time.Time
	EndTime   *time.Time
}

// CCTVServiceConfig contains configuration for the CCTV service
type CCTVServiceConfig struct {
	Discovery DiscoveryConfig `yaml:"discovery"`
	RTSP      RTSPConfig      `yaml:"rtsp"`
}

// DiscoveryConfig contains camera discovery configuration
type DiscoveryConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Interval     time.Duration `yaml:"interval"`
	USBDevicePath string       `yaml:"usb_device_path"` // Path to scan for USB cameras (default: "/dev")
}

// RTSPConfig contains RTSP client configuration
type RTSPConfig struct {
	Timeout           time.Duration `yaml:"timeout"`
	ReconnectInterval time.Duration `yaml:"reconnect_interval"`
}
