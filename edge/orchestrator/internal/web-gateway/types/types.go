package types


type WebGatewayConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Host    string `yaml:"host"`
	// Authentication configuration
	Auth AuthConfig `yaml:"auth"`
}

// AuthConfig defines authentication settings for the web gateway
type AuthConfig struct {
	// Enabled controls whether authentication is required for API endpoints
	Enabled bool `yaml:"enabled"`
	// APIKey is the API key required for authenticated requests (Bearer token)
	// If empty and auth is enabled, a default key will be generated (not recommended for production)
	APIKey string `yaml:"api_key"`
	// PublicEndpoints lists endpoints that don't require authentication even when auth is enabled
	// Default: /api/health, /api/status
	PublicEndpoints []string `yaml:"public_endpoints,omitempty"`
}