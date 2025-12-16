package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration for the User VM API.
type Config struct {
	// Log controls basic logging output format and level.
	Log LogConfig `yaml:"log"`

	// DataDir is the base directory for all VM-local data (edge state DB, models, etc.).
	// If empty, the application will fall back to a sensible default (e.g. "./data").
	DataDir string `yaml:"data_dir"`
}

// LogConfig contains logging configuration.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
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
