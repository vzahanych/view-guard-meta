package pluginregistry_test

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// ExampleNewDevicePluginRegistry demonstrates how to create a new device plugin registry.
func ExampleNewDevicePluginRegistry() {
	logger := zap.NewNop()
	registry := pluginregistry.NewDevicePluginRegistry(logger)

	fmt.Printf("Registry created: %T\n", registry)
	// Output:
	// Registry created: *pluginregistry.devicePluginRegistryImpl
}

// ExampleDevicePluginRegistry_RegisterPlugin demonstrates how to register a plugin.
func ExampleDevicePluginRegistry_RegisterPlugin() {
	// Create registry
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)

	// In production, you would create a real plugin implementation
	// For example: plugin := cctv.NewCCTVDevicePlugin(...)
	// Here we show the pattern:
	// plugin := &myPlugin{deviceType: types.DeviceTypeCamera}
	// err := registry.RegisterPlugin(plugin)

	fmt.Println("Plugin registration pattern: registry.RegisterPlugin(plugin)")
	// Output:
	// Plugin registration pattern: registry.RegisterPlugin(plugin)
}

// ExampleDevicePluginRegistry_DiscoverDevices demonstrates how to discover devices using all registered plugins.
func ExampleDevicePluginRegistry_DiscoverDevices() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)

	// In production, plugins would be registered first
	// devices, err := registry.DiscoverDevices(ctx)

	fmt.Println("Device discovery pattern: registry.DiscoverDevices(ctx)")
	// Output:
	// Device discovery pattern: registry.DiscoverDevices(ctx)
}

// ExampleDevicePluginRegistry_DiscoverDevicesByType demonstrates how to discover devices of a specific type.
func ExampleDevicePluginRegistry_DiscoverDevicesByType() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)

	deviceType := types.DeviceTypeCamera
	// In production, plugins would be registered first
	// devices, err := registry.DiscoverDevicesByType(ctx, deviceType)

	fmt.Printf("Device discovery by type pattern: registry.DiscoverDevicesByType(ctx, %s)\n", deviceType)
	// Output:
	// Device discovery by type pattern: registry.DiscoverDevicesByType(ctx, camera)
}

// ExampleDevicePluginRegistry_CreateDevice demonstrates how to create a device from metadata.
func ExampleDevicePluginRegistry_CreateDevice() {
	_ = context.Background()
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)

	metadata := types.DeviceMetadata{
		ID:   "device-123",
		Type: types.DeviceTypeCamera,
	}

	// In production, plugins would be registered first
	// device, err := registry.CreateDevice(ctx, metadata)

	fmt.Printf("Device creation pattern: registry.CreateDevice(ctx, metadata) for device %s\n", metadata.ID)
	// Output:
	// Device creation pattern: registry.CreateDevice(ctx, metadata) for device device-123
}

// ExampleDevicePluginRegistry_GetSupportedDeviceTypes demonstrates how to get all supported device types.
func ExampleDevicePluginRegistry_GetSupportedDeviceTypes() {
	logger := zap.NewNop()
	_ = pluginregistry.NewDevicePluginRegistry(logger)

	// In production, plugins would be registered first
	// deviceTypes := registry.GetSupportedDeviceTypes()

	fmt.Println("Get supported device types pattern: registry.GetSupportedDeviceTypes()")
	// Output:
	// Get supported device types pattern: registry.GetSupportedDeviceTypes()
}

// ExampleNewPluginManager demonstrates how to create a plugin manager.
func ExampleNewPluginManager() {
	logger := zap.NewNop()
	registry := pluginregistry.NewDevicePluginRegistry(logger)
	manager := pluginregistry.NewPluginManager(registry, logger)

	fmt.Printf("Plugin manager created: %T\n", manager)
	// Output:
	// Plugin manager created: *pluginregistry.PluginManager
}

// ExamplePluginManager_DiscoverAllDevices demonstrates how to use plugin manager to discover devices.
func ExamplePluginManager_DiscoverAllDevices() {
	_ = context.Background()
	logger := zap.NewNop()
	registry := pluginregistry.NewDevicePluginRegistry(logger)
	_ = pluginregistry.NewPluginManager(registry, logger)

	// In production, plugins would be registered first
	// devices, err := manager.DiscoverAllDevices(ctx)

	fmt.Println("Plugin manager device discovery pattern: manager.DiscoverAllDevices(ctx)")
	// Output:
	// Plugin manager device discovery pattern: manager.DiscoverAllDevices(ctx)
}

