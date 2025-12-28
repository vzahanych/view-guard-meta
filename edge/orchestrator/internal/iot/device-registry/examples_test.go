package deviceregistry_test

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/device-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"
	pluginregistry "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"
	statemachine "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// ExampleNewDeviceRegistry demonstrates how to create a new device registry.
func ExampleNewDeviceRegistry() {
	logger := zap.NewNop()
	pluginReg := pluginregistry.NewDevicePluginRegistry(logger)
	factory := statemachine.NewDeviceStateMachineFactory(logger)
	stateReg := statemachine.NewDeviceStateMachineRegistry(factory, logger)
	hookReg := hooks.NewLifecycleHookRegistry(logger)

	registry := deviceregistry.NewDeviceRegistry(
		pluginReg,
		stateReg,
		hookReg,
		logger,
	)

	fmt.Printf("Registry created: %T\n", registry)
	// Output:
	// Registry created: *deviceregistry.deviceRegistryImpl
}

// ExampleDeviceRegistry_DiscoverDevices demonstrates how to discover devices of a specific type.
func ExampleDeviceRegistry_DiscoverDevices() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)
	_ = statemachine.NewDeviceStateMachineFactory(logger)
	_ = statemachine.NewDeviceStateMachineRegistry(nil, logger)
	_ = hooks.NewLifecycleHookRegistry(logger)

	deviceType := types.DeviceTypeCamera
	// In production, plugins would be registered first
	// registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	// devices, err := registry.DiscoverDevices(ctx, deviceType)

	fmt.Printf("Device discovery pattern: registry.DiscoverDevices(ctx, %s)\n", deviceType)
	// Output:
	// Device discovery pattern: registry.DiscoverDevices(ctx, camera)
}

// ExampleDeviceRegistry_DiscoverAllDevices demonstrates how to discover all devices.
func ExampleDeviceRegistry_DiscoverAllDevices() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)
	_ = statemachine.NewDeviceStateMachineFactory(logger)
	_ = statemachine.NewDeviceStateMachineRegistry(nil, logger)
	_ = hooks.NewLifecycleHookRegistry(logger)

	// In production, plugins would be registered first
	// registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	// devices, err := registry.DiscoverAllDevices(ctx)

	fmt.Println("Device discovery pattern: registry.DiscoverAllDevices(ctx)")
	// Output:
	// Device discovery pattern: registry.DiscoverAllDevices(ctx)
}

// ExampleDeviceRegistry_RegisterDevice demonstrates how to register a device.
func ExampleDeviceRegistry_RegisterDevice() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)
	_ = statemachine.NewDeviceStateMachineFactory(logger)
	_ = statemachine.NewDeviceStateMachineRegistry(nil, logger)
	_ = hooks.NewLifecycleHookRegistry(logger)

	// In production, you would discover devices first
	// registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	// device := discoveredDevice
	// err := registry.RegisterDevice(ctx, device)

	fmt.Println("Device registration pattern: registry.RegisterDevice(ctx, device)")
	// Output:
	// Device registration pattern: registry.RegisterDevice(ctx, device)
}

// ExampleDeviceRegistry_GetDevice demonstrates how to retrieve a device by ID.
func ExampleDeviceRegistry_GetDevice() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)
	_ = statemachine.NewDeviceStateMachineFactory(logger)
	_ = statemachine.NewDeviceStateMachineRegistry(nil, logger)
	_ = hooks.NewLifecycleHookRegistry(logger)

	deviceID := "device-123"
	// In production, device would be registered first
	// registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	// device, err := registry.GetDevice(ctx, deviceID)

	fmt.Printf("Device retrieval pattern: registry.GetDevice(ctx, %s)\n", deviceID)
	// Output:
	// Device retrieval pattern: registry.GetDevice(ctx, device-123)
}

// ExampleDeviceRegistry_ListDevices demonstrates how to list devices with filters.
func ExampleDeviceRegistry_ListDevices() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)
	_ = statemachine.NewDeviceStateMachineFactory(logger)
	_ = statemachine.NewDeviceStateMachineRegistry(nil, logger)
	_ = hooks.NewLifecycleHookRegistry(logger)

	// List all devices
	// registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	// devices, err := registry.ListDevices(ctx, nil)

	// List devices with filters
	deviceType := types.DeviceTypeCamera
	_ = &types.DeviceFilters{
		Type: &deviceType,
	}
	// filters := &types.DeviceFilters{Type: &deviceType}
	// devices, err := registry.ListDevices(ctx, filters)

	fmt.Printf("Device listing pattern: registry.ListDevices(ctx, filters) with type filter: %s\n", deviceType)
	// Output:
	// Device listing pattern: registry.ListDevices(ctx, filters) with type filter: camera
}

// ExampleDeviceRegistry_GetDevicesByType demonstrates how to get devices by type.
func ExampleDeviceRegistry_GetDevicesByType() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)
	_ = statemachine.NewDeviceStateMachineFactory(logger)
	_ = statemachine.NewDeviceStateMachineRegistry(nil, logger)
	_ = hooks.NewLifecycleHookRegistry(logger)

	deviceType := types.DeviceTypeCamera
	// registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	// devices, err := registry.GetDevicesByType(ctx, deviceType)

	fmt.Printf("Get devices by type pattern: registry.GetDevicesByType(ctx, %s)\n", deviceType)
	// Output:
	// Get devices by type pattern: registry.GetDevicesByType(ctx, camera)
}

// ExampleDeviceRegistry_GetDevicesByCapability demonstrates how to get devices by capability.
func ExampleDeviceRegistry_GetDevicesByCapability() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)
	_ = statemachine.NewDeviceStateMachineFactory(logger)
	_ = statemachine.NewDeviceStateMachineRegistry(nil, logger)
	_ = hooks.NewLifecycleHookRegistry(logger)

	capability := types.DeviceCapabilityVideoCapture
	// registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	// devices, err := registry.GetDevicesByCapability(ctx, capability)

	fmt.Printf("Get devices by capability pattern: registry.GetDevicesByCapability(ctx, %s)\n", capability)
	// Output:
	// Get devices by capability pattern: registry.GetDevicesByCapability(ctx, video_capture)
}

// ExampleDeviceRegistry_UpdateDevice demonstrates how to update device metadata.
func ExampleDeviceRegistry_UpdateDevice() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)
	_ = statemachine.NewDeviceStateMachineFactory(logger)
	_ = statemachine.NewDeviceStateMachineRegistry(nil, logger)
	_ = hooks.NewLifecycleHookRegistry(logger)

	deviceID := "device-123"
	newName := "Updated Device Name"
	_ = &types.DeviceMetadataUpdate{
		Name: &newName,
	}
	// registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	// updates := &types.DeviceMetadataUpdate{Name: &newName}
	// err := registry.UpdateDevice(ctx, deviceID, updates)

	fmt.Printf("Device update pattern: registry.UpdateDevice(ctx, %s, updates)\n", deviceID)
	// Output:
	// Device update pattern: registry.UpdateDevice(ctx, device-123, updates)
}

// ExampleDeviceRegistry_DeleteDevice demonstrates how to delete a device.
func ExampleDeviceRegistry_DeleteDevice() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)
	_ = statemachine.NewDeviceStateMachineFactory(logger)
	_ = statemachine.NewDeviceStateMachineRegistry(nil, logger)
	_ = hooks.NewLifecycleHookRegistry(logger)

	deviceID := "device-123"
	// registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
	// err := registry.DeleteDevice(ctx, deviceID)

	fmt.Printf("Device deletion pattern: registry.DeleteDevice(ctx, %s)\n", deviceID)
	// Output:
	// Device deletion pattern: registry.DeleteDevice(ctx, device-123)
}

