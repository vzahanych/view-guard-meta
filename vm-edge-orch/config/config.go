package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration for the User VM API.
type Config struct {
	// Version of the VM Edge Orchestrator configuration schema/application.
	Version string `yaml:"version"`

	// Env describes the deployment environment (e.g. "dev", "staging", "prod").
	Env string `yaml:"env"`

	// Log controls basic logging output format and level.
	Log LogConfig `yaml:"log"`

	// DataDir is the base directory for all VM-local data (edge state DB, models, etc.).
	// If empty, the application will fall back to a sensible default (e.g. "./data").
	DataDir string `yaml:"data_dir"`

	// Wireguard holds configuration for the VM's WireGuard server interface.
	Wireguard WireguardConfig `yaml:"wireguard"`

	// TLS holds paths to TLS certificates/keys used for HTTPS mTLS between VM and Edge.
	TLS TLSConfig `yaml:"tls"`
}

// LogConfig contains logging configuration.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// WireguardConfig contains configuration for the VM's WireGuard server.
type WireguardConfig struct {
	// Interface is the name of the WireGuard interface (e.g. "wg0").
	Interface string `yaml:"interface"`

	// ListenPort is the UDP port WireGuard listens on (e.g. 51820).
	// If zero, a sensible default is used.
	ListenPort int `yaml:"listen_port"`

	// ConfigPath is an optional path to a WireGuard config file (wg-quick style).
	// If set and readable, the server will try to load keys and settings from it.
	ConfigPath string `yaml:"config_path"`
}

// TLSConfig contains paths to TLS certificates and keys for HTTPS mTLS.
type TLSConfig struct {
	CACert     string `yaml:"ca_cert"`
	ServerCert string `yaml:"server_cert"`
	ServerKey  string `yaml:"server_key"`
	ClientCert string `yaml:"client_cert"`
	ClientKey  string `yaml:"client_key"`
}

// Load reads, parses, validates and returns the configuration.
func Load(configPath string) (*Config, error) {

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	return &cfg, nil
}
