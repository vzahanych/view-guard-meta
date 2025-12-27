package types

import "time"

// ModelDeploymentMetadata represents the metadata for a model deployment request.
// This is a typed structure that replaces the previous map[string]interface{} approach.
type ModelDeploymentMetadata struct {
	DeploymentID      string                 `json:"deployment_id"`
	ModelID           string                 `json:"model_id"`
	Version           string                 `json:"version"`
	ModelType         string                 `json:"model_type"`
	DeviceID          string                 `json:"device_id"` // Device ID (device-agnostic)
	Framework         string                 `json:"framework"`
	TrainingDatasetID string                 `json:"training_dataset_id,omitempty"`
	TrainingDate      string                 `json:"training_date,omitempty"`
	InputShape        []int                  `json:"input_shape,omitempty"`
	Preprocessing     map[string]interface{} `json:"preprocessing,omitempty"`
	TotalSize         uint64                 `json:"total_size"`
}

// HTTPServerConfig contains configuration for the HTTPS server
// that accepts connections from the VM over the tunnel (WireGuard, OpenVPN, IPSec, etc.).
type HTTPServerConfig struct {
	// ListenAddress is the address the HTTPS server listens on, e.g. "10.0.0.2:8443".
	// This should be the Edge's tunnel IP address.
	ListenAddress string `yaml:"listen_address"`

	// ServerCertPath is the path to the server certificate used for TLS.
	ServerCertPath string `yaml:"server_cert_path"`

	// ServerKeyPath is the path to the server private key used for TLS.
	ServerKeyPath string `yaml:"server_key_path"`

	// CACertPath is the path to the CA certificate used to verify client certificates (for mTLS).
	CACertPath string `yaml:"ca_cert_path"`

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration `yaml:"read_timeout"`

	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// IdleTimeout is the maximum amount of time to wait for the next request when keep-alives are enabled.
	IdleTimeout time.Duration `yaml:"idle_timeout"`

	// TunnelInterfaceWaitTimeout is the maximum time to wait for tunnel interface to be ready.
	// This is provider-agnostic and works with WireGuard, OpenVPN, IPSec, etc.
	TunnelInterfaceWaitTimeout time.Duration `yaml:"tunnel_interface_wait_timeout"`

	// TunnelInterfaceCheckInterval is the interval between checks for tunnel interface readiness.
	// This is provider-agnostic and works with WireGuard, OpenVPN, IPSec, etc.
	TunnelInterfaceCheckInterval time.Duration `yaml:"tunnel_interface_check_interval"`

	// MultipartFormMaxMemory is the maximum memory size for multipart form parsing (used for model deployment).
	MultipartFormMaxMemory int64 `yaml:"multipart_form_max_memory"`
}
