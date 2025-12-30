package types

import (
	"time"

	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// HTTPSClientConfig contains configuration for the HTTPS client
// that communicates with the VM over the tunnel (WireGuard, OpenVPN, IPSec, etc.).
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

	// CertificatePinning contains certificate pinning configuration for client-side verification.
	CertificatePinning vmgatewaytypes.CertificatePinningConfig `yaml:"certificate_pinning"`

	// CertificateRevocation contains certificate revocation checking configuration.
	CertificateRevocation vmgatewaytypes.CertificateRevocationConfig `yaml:"certificate_revocation"`

	// TimeSync contains time synchronization checking configuration.
	TimeSync vmgatewaytypes.TimeSyncConfig `yaml:"time_sync"`
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

// DeviceCapability describes a single device's dataset readiness state
// for capability synchronization with the VM.
// This is device-agnostic and supports cameras, sensors, audio devices, etc.
type DeviceCapability struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // Device type (e.g., "camera", "sensor", etc.)
	Enabled  bool   `json:"enabled"`
	Status   string `json:"status"`
	// LabelCounts holds per-label data unit counts (for video devices: snapshots, for sensors: readings, etc.).
	LabelCounts map[string]uint32 `json:"label_counts"`
	// LabeledDataUnitCount is the total number of labeled data units.
	// For video devices, this represents labeled snapshots/images.
	// For sensors, this represents labeled readings.
	LabeledDataUnitCount uint32 `json:"labeled_data_unit_count"`
	// RequiredDataUnitCount is the number of data units required for readiness.
	RequiredDataUnitCount uint32 `json:"required_data_unit_count"`
	// DataUnitRequired indicates whether more data units are needed.
	DataUnitRequired bool `json:"data_unit_required"`
}

// SyncCapabilitiesRequest contains the payload for syncing device capabilities to the VM.
type SyncCapabilitiesRequest struct {
	SyncedAt int64                `json:"synced_at"`
	Devices  []*DeviceCapability `json:"devices"` // Device-agnostic: supports cameras, sensors, etc.
}

// SyncCapabilitiesResponse represents the VM response for capability sync.
type SyncCapabilitiesResponse struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// DeviceInfo represents basic device information for sync.
// This is device-agnostic and supports cameras, sensors, and other IoT devices.
type DeviceInfo struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`   // Device type (e.g., "camera", "motion_sensor", etc.)
	Source   string `json:"source,omitempty"` // Discovery source (e.g., "onvif", "usb", "rtsp", etc.)
	Enabled  bool   `json:"enabled,omitempty"`
}

// SyncDevicesRequest contains the payload for syncing discovered devices to the VM.
// VM decides which devices should be enabled based on configuration and policies.
type SyncDevicesRequest struct {
	EdgeID  string        `json:"edge_id"`
	Devices []*DeviceInfo `json:"devices"` // Device-agnostic: supports cameras, sensors, etc.
}

// EnabledDevice represents a device that VM has decided to enable.
type EnabledDevice struct {
	DeviceID string `json:"device_id"`
	Enabled  bool   `json:"enabled"`
}

// SyncDevicesResponse represents the VM response for device sync.
type SyncDevicesResponse struct {
	Success        bool             `json:"success"`
	ErrorMessage   string           `json:"error_message,omitempty"`
	EnabledDevices []*EnabledDevice `json:"enabled_devices,omitempty"`
}

// DataUnitInfo represents a labeled data unit for model training.
// This is device-agnostic and can represent:
//   - Screenshots/images from cameras (video devices)
//   - Sensor readings from sensors
//   - Audio samples from audio devices
//   - Any other labeled data unit for training
type DataUnitInfo struct {
	DataUnitID    string                 `json:"data_unit_id"`              // Unique identifier for this data unit
	DeviceID      string                 `json:"device_id"`                 // Device that produced this data unit
	ObjectKey     string                 `json:"object_key"`                // Path to data in object storage
	RawData       string                 `json:"raw_data,omitempty"`        // Base64 encoded raw data (image, sensor reading, audio, etc.)
	RawDataFormat string                 `json:"raw_data_format,omitempty"` // Data format (e.g., "jpeg", "png", "json", "wav", etc.)
	Label         string                 `json:"label"`                     // Label for training
	CustomLabel   string                 `json:"custom_label,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     int64                  `json:"created_at"` // Unix timestamp
}

// SyncDataUnitsRequest contains the payload for syncing labeled data units to the VM for model training.
// This is device-agnostic and supports all IoT device types (cameras, sensors, audio devices, etc.).
type SyncDataUnitsRequest struct {
	EdgeID    string          `json:"edge_id"`
	DeviceID  string          `json:"device_id"`  // Device that produced the data units
	DataUnits []*DataUnitInfo `json:"data_units"` // Labeled data units for training
}

// SyncDataUnitsResponse represents the VM response for data unit sync.
type SyncDataUnitsResponse struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	Message      string `json:"message,omitempty"`
}

// AuditLogEntry represents a single audit log entry for sync to VM.
// Note: VM only knows about EdgeID, not individual users, so UserID is not included.
// Note: Edge devices are typically behind NAT, so IPAddress (internal IP) is not meaningful to VM.
type AuditLogEntry struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Timestamp    int64                  `json:"timestamp"` // Unix timestamp
	EdgeID       string                 `json:"edge_id"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	Result       string                 `json:"result"`
	Error        string                 `json:"error,omitempty"`
	PreviousHash string                 `json:"previous_hash,omitempty"`
	Hash         string                 `json:"hash"`
	Data         map[string]interface{} `json:"data"`             // Entry-specific data
	Format       string                 `json:"format,omitempty"` // "json" or "cef"
	CEF          string                 `json:"cef,omitempty"`    // CEF formatted entry if format is "cef"
}

// SyncAuditLogsRequest contains the payload for syncing audit logs to the VM.
type SyncAuditLogsRequest struct {
	EdgeID     string           `json:"edge_id"`
	StartTime  int64            `json:"start_time"`  // Unix timestamp of first entry
	EndTime    int64            `json:"end_time"`    // Unix timestamp of last entry
	EntryCount int              `json:"entry_count"` // Number of entries in this batch
	Entries    []*AuditLogEntry `json:"entries"`     // Audit log entries
	Format     string           `json:"format"`      // "json" or "cef"
}

// SyncAuditLogsResponse represents the VM response for audit log sync.
type SyncAuditLogsResponse struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	SyncedCount  int    `json:"synced_count,omitempty"` // Number of entries successfully synced
}
