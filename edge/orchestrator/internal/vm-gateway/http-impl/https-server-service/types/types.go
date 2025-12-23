package types

import "time"

// HTTPServerConfig contains configuration for the HTTPS server
// that accepts connections from the VM over the WireGuard tunnel.
type HTTPServerConfig struct {
	// ListenAddress is the address the HTTPS server listens on, e.g. "10.0.0.2:8443".
	// This should be the Edge's WireGuard IP address.
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

	// WireGuardInterfaceWaitTimeout is the maximum time to wait for WireGuard interface to be ready.
	WireGuardInterfaceWaitTimeout time.Duration `yaml:"wireguard_interface_wait_timeout"`

	// WireGuardInterfaceCheckInterval is the interval between checks for WireGuard interface readiness.
	WireGuardInterfaceCheckInterval time.Duration `yaml:"wireguard_interface_check_interval"`

	// MultipartFormMaxMemory is the maximum memory size for multipart form parsing (used for model deployment).
	MultipartFormMaxMemory int64 `yaml:"multipart_form_max_memory"`
}
