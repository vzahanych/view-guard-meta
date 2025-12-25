package types

import "time"

// HTTPSClientConfig contains configuration for the HTTPS client
// that communicates with the VM over the WireGuard tunnel.
// It focuses on connection and TLS settings.
type HTTPSClientConfig struct {
	// VMEndpoint is the HTTPS endpoint of the VM, e.g. "10.0.0.1:8443".
	VMEndpoint string `yaml:"vm_endpoint"`

	// ClientCertPath is the path to the client certificate used for mTLS.
	ClientCertPath string `yaml:"client_cert_path"`

	// ClientKeyPath is the path to the client private key used for mTLS.
	ClientKeyPath string `yaml:"client_key_path"`

	// CACertPath is the path to the CA certificate used to verify the VM certificate.
	CACertPath string `yaml:"ca_cert_path"`

	// Timeout is the HTTP request timeout.
	Timeout time.Duration `yaml:"timeout"`
}

// GetConfigResponse represents the response from the VM GetConfig HTTP endpoint.
// It deliberately mirrors the minimal information we need on the Edge side
// without depending on the gRPC/proto definition.
type GetConfigResponse struct {
	// Success indicates whether configuration retrieval succeeded.
	Success bool `json:"success"`

	// ConfigJSON contains the VM configuration as a JSON string.
	ConfigJSON string `json:"config_json"`

	// ErrorMessage is populated when Success is false.
	ErrorMessage string `json:"error_message,omitempty"`
}

// CameraCapability describes a single camera's dataset readiness state
// for capability synchronization with the VM.
type CameraCapability struct {
	CameraID string `json:"camera_id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Enabled  bool   `json:"enabled"`
	Status   string `json:"status"`
	// LabelCounts holds per-label snapshot counts.
	LabelCounts map[string]uint32 `json:"label_counts"`
	// LabeledSnapshotCount is the total number of labeled snapshots.
	LabeledSnapshotCount uint32 `json:"labeled_snapshot_count"`
	// RequiredSnapshotCount is the number of snapshots required for readiness.
	RequiredSnapshotCount uint32 `json:"required_snapshot_count"`
	// SnapshotRequired indicates whether more snapshots are needed.
	SnapshotRequired bool `json:"snapshot_required"`
}

// SyncCapabilitiesRequest contains the payload for syncing capabilities to the VM.
type SyncCapabilitiesRequest struct {
	SyncedAt int64               `json:"synced_at"`
	Cameras  []*CameraCapability `json:"cameras"`
}

// SyncCapabilitiesResponse represents the VM response for capability sync.
type SyncCapabilitiesResponse struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// CameraInfo represents basic camera information for sync
type CameraInfo struct {
	CameraID string `json:"camera_id"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Source   string `json:"source,omitempty"`
	Enabled  bool   `json:"enabled,omitempty"`
}

// SyncCamerasRequest contains the payload for syncing discovered cameras to the VM.
type SyncCamerasRequest struct {
	EdgeID  string        `json:"edge_id"`
	Cameras []*CameraInfo `json:"cameras"`
}

// EnabledCamera represents a camera that VM has decided to enable
type EnabledCamera struct {
	CameraID string `json:"camera_id"`
	Enabled  bool   `json:"enabled"`
}

// SyncCamerasResponse represents the VM response for camera sync.
type SyncCamerasResponse struct {
	Success         bool             `json:"success"`
	ErrorMessage    string           `json:"error_message,omitempty"`
	EnabledCameras  []*EnabledCamera `json:"enabled_cameras,omitempty"`
}

// ScreenshotInfo represents screenshot metadata for sync to VM
type ScreenshotInfo struct {
	ScreenshotID string            `json:"screenshot_id"`
	CameraID     string            `json:"camera_id"`
	ObjectKey    string            `json:"object_key"` // Path to image in object storage
	ImageData    string            `json:"image_data,omitempty"` // Base64 encoded image data
	ImageFormat  string            `json:"image_format,omitempty"` // Image format (e.g., "jpeg", "png")
	Label        string            `json:"label"`
	CustomLabel  string            `json:"custom_label,omitempty"`
	Description  string            `json:"description,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    int64             `json:"created_at"` // Unix timestamp
}

// SyncScreenshotsRequest contains the payload for syncing labeled screenshots to the VM for model training.
type SyncScreenshotsRequest struct {
	EdgeID      string          `json:"edge_id"`
	CameraID    string          `json:"camera_id"`
	Screenshots []*ScreenshotInfo `json:"screenshots"`
}

// SyncScreenshotsResponse represents the VM response for screenshot sync.
type SyncScreenshotsResponse struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	Message      string `json:"message,omitempty"`
}
