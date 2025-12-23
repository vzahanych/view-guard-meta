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
