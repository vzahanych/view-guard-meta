package types

import (
	"fmt"
	"time"
)

// IoTServiceConfig contains device-agnostic IoT service configuration.
type IoTServiceConfig struct {
	Discovery    DiscoveryConfig    `yaml:"discovery"`
	Processing   ProcessingConfig   `yaml:"processing"`
	StateMachine StateMachineConfig `yaml:"state_machine"`
	Hooks        HooksConfig        `yaml:"hooks"`
}

// DiscoveryConfig contains device discovery configuration.
type DiscoveryConfig struct {
	AutoDiscover      bool          `yaml:"auto_discover"`
	DiscoveryInterval time.Duration `yaml:"discovery_interval"`
	DiscoveryTimeout  time.Duration `yaml:"discovery_timeout"`
	ParallelDiscovery bool          `yaml:"parallel_discovery"`
	EnabledPlugins    []string      `yaml:"enabled_plugins,omitempty"`
}

// ProcessingConfig contains data processing configuration.
type ProcessingConfig struct {
	Enabled          bool          `yaml:"enabled"`
	ProcessorTimeout time.Duration `yaml:"processor_timeout"`
}

// StateMachineConfig contains state machine configuration.
type StateMachineConfig struct {
	Enabled bool `yaml:"enabled"`
}

// HooksConfig contains lifecycle hook configuration.
type HooksConfig struct {
	Enabled bool `yaml:"enabled"`
}

// Validate validates the IoT service configuration.
func (c *IoTServiceConfig) Validate() error {
	if err := c.Discovery.Validate(); err != nil {
		return fmt.Errorf("discovery config: %w", err)
	}
	if err := c.Processing.Validate(); err != nil {
		return fmt.Errorf("processing config: %w", err)
	}
	if err := c.StateMachine.Validate(); err != nil {
		return fmt.Errorf("state machine config: %w", err)
	}
	if err := c.Hooks.Validate(); err != nil {
		return fmt.Errorf("hooks config: %w", err)
	}
	return nil
}

// Validate validates the discovery configuration.
func (c *DiscoveryConfig) Validate() error {
	if c.AutoDiscover && c.DiscoveryInterval <= 0 {
		return fmt.Errorf("discovery_interval must be > 0 when auto_discover is enabled")
	}
	if c.DiscoveryTimeout < 0 {
		return fmt.Errorf("discovery_timeout must be >= 0")
	}
	return nil
}

// Validate validates the processing configuration.
func (c *ProcessingConfig) Validate() error {
	if c.Enabled && c.ProcessorTimeout <= 0 {
		return fmt.Errorf("processor_timeout must be > 0 when processing is enabled")
	}
	return nil
}

// Validate validates the state machine configuration.
func (c *StateMachineConfig) Validate() error {
	// Add validation rules as needed
	return nil
}

// Validate validates the hooks configuration.
func (c *HooksConfig) Validate() error {
	// Add validation rules as needed
	return nil
}

