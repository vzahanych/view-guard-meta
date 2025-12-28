package iot

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

func TestIoTServiceProvider(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := &types.IoTServiceConfig{
			Discovery: types.DiscoveryConfig{
				AutoDiscover:      false,
				DiscoveryInterval: 30 * time.Second,
				DiscoveryTimeout:  10 * time.Second,
			},
			Processing: types.ProcessingConfig{
				Enabled:          false,
				ProcessorTimeout: 5 * time.Second,
			},
		}

		logger := zap.NewNop()

		// Create fx app
		app := fx.New(
			fx.Provide(
				func() *types.IoTServiceConfig { return config },
				func() *zap.Logger { return logger },
				IoTServiceProvider,
			),
			fx.Invoke(func(service IoTService) {
				// Verify service is created
				assert.NotNil(t, service)
				assert.Equal(t, "iot-service", service.Name())
			}),
		)

		ctx := context.Background()
		err := app.Start(ctx)
		require.NoError(t, err)
		defer func() {
			err := app.Stop(ctx)
			assert.NoError(t, err)
		}()

		// Verify service is started
		status := app.Err()
		assert.NoError(t, status)
	})

	t.Run("invalid config", func(t *testing.T) {
		config := &types.IoTServiceConfig{
			Discovery: types.DiscoveryConfig{
				AutoDiscover:      true,
				DiscoveryInterval: 0, // Invalid: must be > 0 when auto_discover is enabled
			},
		}

		logger := zap.NewNop()

		// Create fx app - should fail due to invalid config
		// The provider will be called during fx.New() when we try to invoke the service
		app := fx.New(
			fx.Provide(
				func() *types.IoTServiceConfig { return config },
				func() *zap.Logger { return logger },
				IoTServiceProvider,
			),
			fx.Invoke(func(service IoTService) {
				// This will trigger the provider, which should fail validation
			}),
		)

		ctx := context.Background()
		err := app.Start(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid IoT service configuration")
	})

	t.Run("nil config", func(t *testing.T) {
		logger := zap.NewNop()

		// Create fx app with nil config (should use default)
		app := fx.New(
			fx.Provide(
				func() *types.IoTServiceConfig { return nil },
				func() *zap.Logger { return logger },
				IoTServiceProvider,
			),
			fx.Invoke(func(service IoTService) {
				// Verify service is created even with nil config
				assert.NotNil(t, service)
				assert.Equal(t, "iot-service", service.Name())
			}),
		)

		ctx := context.Background()
		err := app.Start(ctx)
		require.NoError(t, err)
		defer func() {
			err := app.Stop(ctx)
			assert.NoError(t, err)
		}()
	})
}

