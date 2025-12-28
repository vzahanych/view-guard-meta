package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIoTServiceConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *IoTServiceConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &IoTServiceConfig{
				Discovery: DiscoveryConfig{
					AutoDiscover:      true,
					DiscoveryInterval: 30 * time.Second,
					DiscoveryTimeout:  10 * time.Second,
				},
				Processing: ProcessingConfig{
					Enabled:          true,
					ProcessorTimeout: 5 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid discovery interval",
			config: &IoTServiceConfig{
				Discovery: DiscoveryConfig{
					AutoDiscover:      true,
					DiscoveryInterval: 0,
				},
			},
			wantErr: true,
			errMsg:  "discovery_interval must be > 0",
		},
		{
			name: "invalid processor timeout",
				config: &IoTServiceConfig{
				Processing: ProcessingConfig{
					Enabled:          true,
					ProcessorTimeout: 0,
				},
			},
			wantErr: true,
			errMsg:  "processor_timeout must be > 0",
		},
		{
			name: "negative discovery timeout",
			config: &IoTServiceConfig{
				Discovery: DiscoveryConfig{
					DiscoveryTimeout: -1 * time.Second,
				},
			},
			wantErr: true,
			errMsg:  "discovery_timeout must be >= 0",
		},
		{
			name: "auto_discover false with zero interval is valid",
			config: &IoTServiceConfig{
				Discovery: DiscoveryConfig{
					AutoDiscover:      false,
					DiscoveryInterval: 0,
				},
			},
			wantErr: false,
		},
		{
			name: "processing disabled with zero timeout is valid",
			config: &IoTServiceConfig{
				Processing: ProcessingConfig{
					Enabled:          false,
					ProcessorTimeout: 0,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDiscoveryConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  DiscoveryConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: DiscoveryConfig{
				AutoDiscover:      true,
				DiscoveryInterval: 30 * time.Second,
				DiscoveryTimeout:  10 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "invalid interval when auto_discover is true",
			config: DiscoveryConfig{
				AutoDiscover:      true,
				DiscoveryInterval: 0,
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			config: DiscoveryConfig{
				DiscoveryTimeout: -1 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProcessingConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ProcessingConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ProcessingConfig{
				Enabled:          true,
				ProcessorTimeout: 5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "invalid timeout when enabled is true",
			config: ProcessingConfig{
				Enabled:          true,
				ProcessorTimeout: 0,
			},
			wantErr: true,
		},
		{
			name: "disabled with zero timeout is valid",
			config: ProcessingConfig{
				Enabled:          false,
				ProcessorTimeout: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

