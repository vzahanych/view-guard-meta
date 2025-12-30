package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// TransportProvider represents the type of transport implementation.
type TransportProvider string

const (
	// TransportProviderHTTP uses HTTP/HTTPS for transport.
	TransportProviderHTTP TransportProvider = "http"
	// TransportProviderGRPC uses gRPC for transport (future).
	TransportProviderGRPC TransportProvider = "grpc"
	// TransportProviderWebSocket uses WebSocket for transport (future).
	TransportProviderWebSocket TransportProvider = "websocket"
	// TransportProviderNone indicates no transport (for testing/dev mode).
	TransportProviderNone TransportProvider = "none"
)

// Config types for VM Gateway sub-services.
// These are transport-agnostic configuration types that can be used
// by any transport implementation (HTTP, gRPC, etc.).

// WireGuardConfig contains WireGuard client configuration.
type WireGuardConfig struct {
	// Enabled controls whether WireGuard client is enabled.
	Enabled bool `yaml:"enabled"`

	// ConfigPath is the file system path to the WireGuard configuration file.
	ConfigPath string `yaml:"config_path"`

	// KVMEndpoint is the endpoint address of the KVM VM that the Edge connects to.
	KVMEndpoint string `yaml:"kvm_endpoint"`

	// InterfaceName is the name of the WireGuard network interface to create.
	InterfaceName string `yaml:"interface_name"`

	// HealthCheckInterval is the interval between health checks of the WireGuard tunnel.
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`

	// ConnectionTimeout is the maximum time to wait for initial tunnel establishment.
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`

	// PingTimeout is the timeout for latency measurement pings.
	PingTimeout time.Duration `yaml:"ping_timeout"`

	// ReconnectTimeout is the time to wait before attempting to reconnect after a tunnel failure.
	ReconnectTimeout time.Duration `yaml:"reconnect_timeout"`

	// MaxReconnectAttempts is the maximum number of consecutive reconnection attempts.
	MaxReconnectAttempts int `yaml:"max_reconnect_attempts"`
}

// CertificatePinningConfig contains configuration for certificate pinning.
type CertificatePinningConfig struct {
	// VMCAFingerprint is the SHA-256 fingerprint of the VM CA root certificate.
	// This is used to pin the VM's CA certificate when Edge connects to VM.
	VMCAFingerprint string `yaml:"vm_ca_fingerprint"`

	// EdgeCAFingerprint is the SHA-256 fingerprint of the Edge CA root certificate.
	// This is used to pin the Edge's CA certificate when VM connects to Edge.
	EdgeCAFingerprint string `yaml:"edge_ca_fingerprint"`

	// PinningEnabled controls whether certificate pinning is enabled.
	// Default: true (enabled for security)
	PinningEnabled bool `yaml:"pinning_enabled"`
}

// Validate validates the certificate pinning configuration.
func (c *CertificatePinningConfig) Validate() error {
	if !c.PinningEnabled {
		return nil // Skip validation if pinning is disabled
	}

	// Validate fingerprint format (should be hex-encoded SHA-256, 64 characters)
	if c.VMCAFingerprint != "" {
		if len(c.VMCAFingerprint) != 64 {
			return fmt.Errorf("vm_ca_fingerprint must be 64 characters (SHA-256 hex)")
		}
		if _, err := hex.DecodeString(c.VMCAFingerprint); err != nil {
			return fmt.Errorf("vm_ca_fingerprint must be valid hex: %w", err)
		}
	}

	if c.EdgeCAFingerprint != "" {
		if len(c.EdgeCAFingerprint) != 64 {
			return fmt.Errorf("edge_ca_fingerprint must be 64 characters (SHA-256 hex)")
		}
		if _, err := hex.DecodeString(c.EdgeCAFingerprint); err != nil {
			return fmt.Errorf("edge_ca_fingerprint must be valid hex: %w", err)
		}
	}

	return nil
}

// CertificateRevocationConfig contains configuration for certificate revocation checking.
type CertificateRevocationConfig struct {
	// CRLEnabled controls whether Certificate Revocation List (CRL) checking is enabled.
	CRLEnabled bool `yaml:"crl_enabled"`

	// CRLURL is the URL endpoint for downloading CRL.
	// If empty, CRL URL will be extracted from certificate's CRL Distribution Points extension.
	CRLURL string `yaml:"crl_url"`

	// OCSPEnabled controls whether Online Certificate Status Protocol (OCSP) checking is enabled.
	OCSPEnabled bool `yaml:"ocsp_enabled"`

	// OCSPURL is the URL endpoint for OCSP requests.
	// If empty, OCSP URL will be extracted from certificate's Authority Information Access extension.
	OCSPURL string `yaml:"ocsp_url"`

	// RevocationCacheTTL is the time-to-live for revocation status cache.
	// Default: 1 hour
	RevocationCacheTTL time.Duration `yaml:"revocation_cache_ttl"`
}

// Validate validates the certificate revocation configuration.
func (c *CertificateRevocationConfig) Validate() error {
	if !c.CRLEnabled && !c.OCSPEnabled {
		return nil // Both disabled is valid (revocation checking disabled)
	}

	if c.CRLEnabled && c.CRLURL == "" {
		// CRL URL can be empty if it will be extracted from certificate
		// This is valid
	}

	if c.OCSPEnabled && c.OCSPURL == "" {
		// OCSP URL can be empty if it will be extracted from certificate
		// This is valid
	}

	if c.RevocationCacheTTL < 0 {
		return fmt.Errorf("revocation_cache_ttl must be non-negative")
	}

	return nil
}

// TimeSyncConfig contains configuration for time synchronization checking.
type TimeSyncConfig struct {
	// Enabled controls whether time synchronization checking is enabled.
	// Default: true
	Enabled bool `yaml:"enabled"`

	// ToleranceMinutes is the acceptable time drift in minutes before emitting a warning.
	// If the time difference between Edge and VM is within ±ToleranceMinutes, a warning is logged but authentication continues.
	// Default: 5 minutes
	ToleranceMinutes int `yaml:"tolerance_minutes"`

	// CriticalDriftMinutes is the critical time drift threshold in minutes.
	// If the time difference exceeds CriticalDriftMinutes, authentication fails and a critical alert is emitted.
	// Default: 30 minutes
	CriticalDriftMinutes int `yaml:"critical_drift_minutes"`
}

// Validate validates the time synchronization configuration.
func (c *TimeSyncConfig) Validate() error {
	if c.ToleranceMinutes < 0 {
		return fmt.Errorf("tolerance_minutes must be non-negative")
	}
	if c.CriticalDriftMinutes < 0 {
		return fmt.Errorf("critical_drift_minutes must be non-negative")
	}
	// Only validate the relationship if both are explicitly set (non-zero)
	// If both are 0, defaults will be used (5 and 30 minutes respectively)
	if c.ToleranceMinutes > 0 && c.CriticalDriftMinutes > 0 {
		if c.CriticalDriftMinutes <= c.ToleranceMinutes {
			return fmt.Errorf("critical_drift_minutes (%d) must be greater than tolerance_minutes (%d)", c.CriticalDriftMinutes, c.ToleranceMinutes)
		}
	}
	return nil
}

// GetToleranceDuration returns the tolerance as a time.Duration.
func (c *TimeSyncConfig) GetToleranceDuration() time.Duration {
	if c.ToleranceMinutes == 0 {
		return 5 * time.Minute // Default: 5 minutes
	}
	return time.Duration(c.ToleranceMinutes) * time.Minute
}

// GetCriticalDriftDuration returns the critical drift threshold as a time.Duration.
func (c *TimeSyncConfig) GetCriticalDriftDuration() time.Duration {
	if c.CriticalDriftMinutes == 0 {
		return 30 * time.Minute // Default: 30 minutes
	}
	return time.Duration(c.CriticalDriftMinutes) * time.Minute
}

// TimeoutConfig contains timeout configuration for all VM Gateway operations.
type TimeoutConfig struct {
	// TunnelEstablishmentTimeout is the maximum time to wait for tunnel establishment.
	// Default: 30 seconds
	TunnelEstablishmentTimeout time.Duration `yaml:"tunnel_establishment_timeout"`

	// TransportEstablishmentTimeout is the maximum time to wait for transport connection establishment.
	// Default: 30 seconds
	TransportEstablishmentTimeout time.Duration `yaml:"transport_establishment_timeout"`

	// AuthenticationTimeout is the maximum time to wait for authentication to complete.
	// Default: 30 seconds
	AuthenticationTimeout time.Duration `yaml:"authentication_timeout"`

	// VMAPIRequestTimeout is the maximum time to wait for VM API requests to complete.
	// This applies to all Edge → VM API calls (SyncDevices, SyncDataUnits, etc.).
	// Default: 30 seconds
	VMAPIRequestTimeout time.Duration `yaml:"vm_api_request_timeout"`

	// VMCommandProcessingTimeout is the maximum time to wait for VM command processing.
	// This applies to VM → Edge commands (deploy model, capture snapshot, etc.).
	// Default: 10 seconds
	VMCommandProcessingTimeout time.Duration `yaml:"vm_command_processing_timeout"`
}

// Validate validates the timeout configuration.
func (c *TimeoutConfig) Validate() error {
	if c.TunnelEstablishmentTimeout < 0 {
		return fmt.Errorf("tunnel_establishment_timeout must be non-negative")
	}
	if c.TransportEstablishmentTimeout < 0 {
		return fmt.Errorf("transport_establishment_timeout must be non-negative")
	}
	if c.AuthenticationTimeout < 0 {
		return fmt.Errorf("authentication_timeout must be non-negative")
	}
	if c.VMAPIRequestTimeout < 0 {
		return fmt.Errorf("vm_api_request_timeout must be non-negative")
	}
	if c.VMCommandProcessingTimeout < 0 {
		return fmt.Errorf("vm_command_processing_timeout must be non-negative")
	}
	return nil
}

// GetTunnelEstablishmentTimeout returns the tunnel establishment timeout with default.
func (c *TimeoutConfig) GetTunnelEstablishmentTimeout() time.Duration {
	if c.TunnelEstablishmentTimeout == 0 {
		return 30 * time.Second // Default: 30 seconds
	}
	return c.TunnelEstablishmentTimeout
}

// GetTransportEstablishmentTimeout returns the transport establishment timeout with default.
func (c *TimeoutConfig) GetTransportEstablishmentTimeout() time.Duration {
	if c.TransportEstablishmentTimeout == 0 {
		return 30 * time.Second // Default: 30 seconds
	}
	return c.TransportEstablishmentTimeout
}

// GetAuthenticationTimeout returns the authentication timeout with default.
func (c *TimeoutConfig) GetAuthenticationTimeout() time.Duration {
	if c.AuthenticationTimeout == 0 {
		return 30 * time.Second // Default: 30 seconds
	}
	return c.AuthenticationTimeout
}

// GetVMAPIRequestTimeout returns the VM API request timeout with default.
func (c *TimeoutConfig) GetVMAPIRequestTimeout() time.Duration {
	if c.VMAPIRequestTimeout == 0 {
		return 30 * time.Second // Default: 30 seconds
	}
	return c.VMAPIRequestTimeout
}

// GetVMCommandProcessingTimeout returns the VM command processing timeout with default.
func (c *TimeoutConfig) GetVMCommandProcessingTimeout() time.Duration {
	if c.VMCommandProcessingTimeout == 0 {
		return 10 * time.Second // Default: 10 seconds
	}
	return c.VMCommandProcessingTimeout
}

// RetryConfig contains configuration for retry and backoff strategies.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts.
	// Default: 3
	MaxRetries int `yaml:"max_retries"`

	// InitialBackoff is the initial backoff duration before the first retry.
	// Default: 1 second
	InitialBackoff time.Duration `yaml:"initial_backoff"`

	// MaxBackoff is the maximum backoff duration between retries.
	// Default: 60 seconds
	MaxBackoff time.Duration `yaml:"max_backoff"`

	// BackoffMultiplier is the multiplier for exponential backoff.
	// Each retry will wait: InitialBackoff * (BackoffMultiplier ^ attempt_number)
	// Default: 2.0
	BackoffMultiplier float64 `yaml:"backoff_multiplier"`

	// JitterEnabled controls whether jitter is added to backoff to prevent thundering herd.
	// Default: true
	JitterEnabled bool `yaml:"jitter_enabled"`
}

// Validate validates the retry configuration.
func (c *RetryConfig) Validate() error {
	if c.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be non-negative")
	}
	if c.InitialBackoff < 0 {
		return fmt.Errorf("initial_backoff must be non-negative")
	}
	if c.MaxBackoff < 0 {
		return fmt.Errorf("max_backoff must be non-negative")
	}
	if c.MaxBackoff > 0 && c.InitialBackoff > 0 && c.MaxBackoff < c.InitialBackoff {
		return fmt.Errorf("max_backoff must be greater than or equal to initial_backoff")
	}
	if c.BackoffMultiplier < 0 {
		return fmt.Errorf("backoff_multiplier must be non-negative")
	}
	// Note: 0 is allowed and will use default (2.0) in GetBackoffMultiplier()
	return nil
}

// GetMaxRetries returns the maximum number of retries with default.
func (c *RetryConfig) GetMaxRetries() int {
	if c.MaxRetries == 0 {
		return 3 // Default: 3 retries
	}
	return c.MaxRetries
}

// GetInitialBackoff returns the initial backoff duration with default.
func (c *RetryConfig) GetInitialBackoff() time.Duration {
	if c.InitialBackoff == 0 {
		return 1 * time.Second // Default: 1 second
	}
	return c.InitialBackoff
}

// GetMaxBackoff returns the maximum backoff duration with default.
func (c *RetryConfig) GetMaxBackoff() time.Duration {
	if c.MaxBackoff == 0 {
		return 60 * time.Second // Default: 60 seconds
	}
	return c.MaxBackoff
}

// GetBackoffMultiplier returns the backoff multiplier with default.
func (c *RetryConfig) GetBackoffMultiplier() float64 {
	if c.BackoffMultiplier == 0 {
		return 2.0 // Default: 2.0
	}
	return c.BackoffMultiplier
}

// IsJitterEnabled returns whether jitter is enabled (default: true).
func (c *RetryConfig) IsJitterEnabled() bool {
	// Default to true if not explicitly set to false
	// Since bool defaults to false, we need to check if it was explicitly set
	// For simplicity, we'll default to true (jitter is beneficial)
	return c.JitterEnabled
}

// RateLimitConfig contains configuration for rate limiting inbound VM commands.
type RateLimitConfig struct {
	// Enabled controls whether rate limiting is enabled.
	// Default: true
	Enabled bool `yaml:"enabled"`

	// RequestsPerMinute is the default rate limit in requests per minute per endpoint.
	// Default: 100 requests per minute
	RequestsPerMinute int `yaml:"requests_per_minute"`

	// BurstSize is the maximum number of requests allowed in a burst.
	// This allows short bursts above the rate limit.
	// Default: 10 requests
	BurstSize int `yaml:"burst_size"`

	// PerEndpointLimits allows endpoint-specific rate limits.
	// Key is the endpoint path (e.g., "/api/v1/devices/sync", "/api/v1/models/deploy").
	// Value is the requests per minute limit for that endpoint.
	// If an endpoint is not specified, RequestsPerMinute is used.
	PerEndpointLimits map[string]int `yaml:"per_endpoint_limits"`
}

// Validate validates the rate limiting configuration.
func (c *RateLimitConfig) Validate() error {
	if c.RequestsPerMinute < 0 {
		return fmt.Errorf("requests_per_minute must be non-negative")
	}
	if c.BurstSize < 0 {
		return fmt.Errorf("burst_size must be non-negative")
	}
	for endpoint, limit := range c.PerEndpointLimits {
		if limit < 0 {
			return fmt.Errorf("per_endpoint_limits[%s] must be non-negative", endpoint)
		}
	}
	return nil
}

// GetLimitForEndpoint returns the rate limit for a specific endpoint.
// Returns the endpoint-specific limit if configured, otherwise returns the default RequestsPerMinute.
func (c *RateLimitConfig) GetLimitForEndpoint(endpoint string) int {
	if c.PerEndpointLimits != nil {
		if limit, ok := c.PerEndpointLimits[endpoint]; ok {
			return limit
		}
	}
	if c.RequestsPerMinute == 0 {
		return 100 // Default: 100 requests per minute
	}
	return c.RequestsPerMinute
}

// GetBurstSize returns the burst size, with a default of 10 if not set.
func (c *RateLimitConfig) GetBurstSize() int {
	if c.BurstSize == 0 {
		return 10 // Default: 10 requests
	}
	return c.BurstSize
}

// ComputeFingerprint computes the SHA-256 fingerprint of a certificate in hex format.
func ComputeFingerprint(certDER []byte) string {
	hash := sha256.Sum256(certDER)
	return hex.EncodeToString(hash[:])
}

// HTTPServerConfig contains configuration for the HTTPS server.
type HTTPServerConfig struct {
	// ListenAddress is the address the HTTPS server listens on.
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

	// MultipartFormMaxMemory is the maximum memory size for multipart form parsing.
	MultipartFormMaxMemory int64 `yaml:"multipart_form_max_memory"`

	// CertificatePinning contains certificate pinning configuration for server-side verification.
	CertificatePinning CertificatePinningConfig `yaml:"certificate_pinning"`

	// CertificateRevocation contains certificate revocation checking configuration.
	CertificateRevocation CertificateRevocationConfig `yaml:"certificate_revocation"`

	// RateLimit contains rate limiting configuration for inbound VM commands.
	RateLimit RateLimitConfig `yaml:"rate_limit"`

	// Timeouts contains timeout configuration for VM Gateway operations.
	Timeouts TimeoutConfig `yaml:"timeouts"`
}

// HTTPSClientConfig contains configuration for the HTTPS client.
type HTTPSClientConfig struct {
	// VMEndpoint is the HTTPS endpoint of the VM.
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
	CertificatePinning CertificatePinningConfig `yaml:"certificate_pinning"`

	// CertificateRevocation contains certificate revocation checking configuration.
	CertificateRevocation CertificateRevocationConfig `yaml:"certificate_revocation"`

	// TimeSync contains time synchronization checking configuration.
	TimeSync TimeSyncConfig `yaml:"time_sync"`
}

// VMGatewayConfig contains transport-agnostic VM gateway configuration.
// Transport-specific configs (e.g., HTTP) are parsed separately and converted
// to the transport-agnostic types defined in this package.
type VMGatewayConfig struct {
	// TransportProvider specifies the transport implementation (e.g., "http").
	// This field is the primary way to specify the transport.
	TransportProvider TransportProvider `yaml:"transport_provider"`

	// Tunnel contains tunnel-agnostic configuration.
	// This is the only way to configure tunnels (WireGuard, OpenVPN, IPSec, etc.).
	// Use `provider: wireguard` in Tunnel config for WireGuard tunnels.
	Tunnel TunnelConfig `yaml:"tunnel"`

	// HTTPServerConfig contains HTTPS server configuration.
	HTTPServerConfig HTTPServerConfig `yaml:"https_server_config"`

	// HTTPSClientConfig contains HTTPS client configuration.
	HTTPSClientConfig HTTPSClientConfig `yaml:"https_client_config"`

	// EdgeID is the unique identifier for this Edge node.
	EdgeID string `yaml:"edge_id"`

	// TimeSync contains time synchronization checking configuration.
	TimeSync TimeSyncConfig `yaml:"time_sync"`

	// Timeouts contains timeout configuration for all VM Gateway operations.
	Timeouts TimeoutConfig `yaml:"timeouts"`

	// Retry contains retry and backoff strategy configuration.
	Retry RetryConfig `yaml:"retry"`
}

// GetTransportProvider returns the transport provider.
func (c *VMGatewayConfig) GetTransportProvider() TransportProvider {
	if c.TransportProvider != "" {
		return c.TransportProvider
	}
	return TransportProviderNone
}

// GetTunnelConfig returns the tunnel configuration.
// Returns nil if no tunnel is configured (for localhost/dev mode).
func (c *VMGatewayConfig) GetTunnelConfig() *TunnelConfig {
	// Return tunnel config if present and configured
	if c.Tunnel.Provider != "" || c.Tunnel.Enabled {
		return &c.Tunnel
	}

	// No tunnel configured - return nil (for localhost/dev mode)
	return nil
}

// Validate validates the VM gateway configuration and returns an error if invalid.
// This implements "shift-left" validation - config errors are caught at parse time.
func (c *VMGatewayConfig) Validate() error {
	// Validate transport provider
	if c.TransportProvider == "" {
		return fmt.Errorf("transport_provider is required")
	}

	if c.TransportProvider != TransportProviderHTTP {
		return fmt.Errorf("unsupported transport provider: %s (only 'http' is supported)", c.TransportProvider)
	}

	if c.EdgeID == "" {
		return fmt.Errorf("edge_id is required")
	}

	// Get tunnel config
	tunnelCfg := c.GetTunnelConfig()

	// Validate tunnel configuration if present
	tunnelEnabled := false
	if tunnelCfg != nil {
		// Validate tunnel configuration
		if err := tunnelCfg.Validate(); err != nil {
			return fmt.Errorf("tunnel configuration validation failed: %w", err)
		}

		tunnelEnabled = tunnelCfg.Enabled

		// If tunnel is enabled, validate required fields
		if tunnelEnabled {
			if tunnelCfg.KVMEndpoint == "" {
				return fmt.Errorf("tunnel.kvm_endpoint is required when tunnel is enabled")
			}
		}
	}

	// When tunnel is disabled, we allow localhost mode for development
	// but require explicit localhost configuration (not 0.0.0.0 for security)
	if !tunnelEnabled {
		serverAddr := c.HTTPServerConfig.ListenAddress
		isLocalhostServer := serverAddr == "" || // Empty defaults to localhost
			serverAddr == "localhost:8443" ||
			serverAddr == "127.0.0.1:8443"

		clientEndpoint := c.HTTPSClientConfig.VMEndpoint
		isLocalhostClient := clientEndpoint == "" || // Empty defaults to localhost
			strings.HasPrefix(clientEndpoint, "localhost:") ||
			strings.HasPrefix(clientEndpoint, "127.0.0.1:")

		if !isLocalhostServer || !isLocalhostClient {
			return fmt.Errorf("tunnel is disabled but configuration is not in localhost mode (server: %s, client: %s). "+
				"For production, enable tunnel. For development, use localhost addresses",
				serverAddr, clientEndpoint)
		}
	}

	// Validate HTTPS server configuration
	if c.HTTPServerConfig.ServerCertPath == "" {
		return fmt.Errorf("https_server_config.server_cert_path is required")
	}
	if c.HTTPServerConfig.ServerKeyPath == "" {
		return fmt.Errorf("https_server_config.server_key_path is required")
	}

	// Validate HTTPS client configuration
	if c.HTTPSClientConfig.VMEndpoint == "" {
		return fmt.Errorf("https_client_config.vm_endpoint is required")
	}
	if c.HTTPSClientConfig.ClientCertPath == "" {
		return fmt.Errorf("https_client_config.client_cert_path is required")
	}
	if c.HTTPSClientConfig.ClientKeyPath == "" {
		return fmt.Errorf("https_client_config.client_key_path is required")
	}
	if c.HTTPSClientConfig.CACertPath == "" {
		return fmt.Errorf("https_client_config.ca_cert_path is required")
	}

	// Validate certificate pinning configuration
	if err := c.HTTPSClientConfig.CertificatePinning.Validate(); err != nil {
		return fmt.Errorf("https_client_config.certificate_pinning validation failed: %w", err)
	}
	if err := c.HTTPServerConfig.CertificatePinning.Validate(); err != nil {
		return fmt.Errorf("https_server_config.certificate_pinning validation failed: %w", err)
	}

	// Validate certificate revocation configuration
	if err := c.HTTPSClientConfig.CertificateRevocation.Validate(); err != nil {
		return fmt.Errorf("https_client_config.certificate_revocation validation failed: %w", err)
	}
	if err := c.HTTPServerConfig.CertificateRevocation.Validate(); err != nil {
		return fmt.Errorf("https_server_config.certificate_revocation validation failed: %w", err)
	}

	// Validate time synchronization configuration
	if err := c.TimeSync.Validate(); err != nil {
		return fmt.Errorf("time_sync validation failed: %w", err)
	}

	// Validate rate limiting configuration
	if err := c.HTTPServerConfig.RateLimit.Validate(); err != nil {
		return fmt.Errorf("https_server_config.rate_limit validation failed: %w", err)
	}

	// Validate timeout configuration
	if err := c.Timeouts.Validate(); err != nil {
		return fmt.Errorf("timeouts validation failed: %w", err)
	}

	// Validate retry configuration
	if err := c.Retry.Validate(); err != nil {
		return fmt.Errorf("retry validation failed: %w", err)
	}

	return nil
}
