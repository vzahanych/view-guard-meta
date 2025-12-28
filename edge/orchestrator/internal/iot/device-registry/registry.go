package deviceregistry

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// deviceRegistryImpl is the default implementation of DeviceRegistry.
// It provides device discovery, registration, and management with integration
// with plugin registry, state machine registry, and lifecycle hooks.
type deviceRegistryImpl struct {
	// Device storage
	devices map[string]types.Device

	// Indexes for efficient lookup
	devicesByType      map[types.DeviceType][]types.Device
	devicesByCapability map[types.DeviceCapability][]types.Device

	// Dependencies
	pluginRegistry types.DevicePluginRegistry
	stateRegistry  types.DeviceStateMachineRegistry
	hookRegistry   types.LifecycleHookRegistry
	storageBackend DeviceStorageBackend // Optional persistence backend

	// Observability
	logger *zap.Logger

	// Thread safety
	mu sync.RWMutex
}

// NewDeviceRegistry creates a new device registry.
// All three registries are required dependencies.
// If logger is nil, a no-op logger will be used.
// If storageBackend is nil, an in-memory storage backend will be used (no persistence).
func NewDeviceRegistry(
	pluginRegistry types.DevicePluginRegistry,
	stateRegistry types.DeviceStateMachineRegistry,
	hookRegistry types.LifecycleHookRegistry,
	logger *zap.Logger,
) types.DeviceRegistry {
	return NewDeviceRegistryWithStorage(pluginRegistry, stateRegistry, hookRegistry, nil, logger)
}

// NewDeviceRegistryWithStorage creates a new device registry with optional persistence.
// If storageBackend is nil, an in-memory storage backend will be used (no persistence).
func NewDeviceRegistryWithStorage(
	pluginRegistry types.DevicePluginRegistry,
	stateRegistry types.DeviceStateMachineRegistry,
	hookRegistry types.LifecycleHookRegistry,
	storageBackend DeviceStorageBackend,
	logger *zap.Logger,
) types.DeviceRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	if storageBackend == nil {
		storageBackend = NewInMemoryStorage(logger)
	}
	return &deviceRegistryImpl{
		devices:             make(map[string]types.Device),
		devicesByType:       make(map[types.DeviceType][]types.Device),
		devicesByCapability: make(map[types.DeviceCapability][]types.Device),
		pluginRegistry:      pluginRegistry,
		stateRegistry:       stateRegistry,
		hookRegistry:        hookRegistry,
		storageBackend:      storageBackend,
		logger:              logger,
	}
}

// DiscoverDevices discovers devices of a specific type.
// It uses the plugin registry to discover devices and executes discovery hooks.
func (r *deviceRegistryImpl) DiscoverDevices(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error) {
	r.logger.Info("Discovering devices",
		zap.String("device_type", string(deviceType)),
	)

	// Discover via plugin registry
	devices, err := r.pluginRegistry.DiscoverDevicesByType(ctx, deviceType)
	if err != nil {
		r.logger.Error("Plugin discovery failed",
			zap.String("device_type", string(deviceType)),
			zap.Error(err),
		)
		return nil, fmt.Errorf("plugin discovery failed: %w", err)
	}

	// Execute discovery hooks for all discovered devices
	hookCtx := &types.DiscoveryHookContext{
		DeviceType:        deviceType,
		DiscoveredDevices: devices,
		Metadata:          make(map[string]interface{}),
	}

	if err := r.hookRegistry.ExecuteDiscoveryHooks(ctx, hookCtx); err != nil {
		r.logger.Warn("Discovery hooks failed",
			zap.String("device_type", string(deviceType)),
			zap.Error(err),
		)
		// Continue despite hook failure - hooks are advisory
	}

	r.logger.Info("Device discovery completed",
		zap.String("device_type", string(deviceType)),
		zap.Int("device_count", len(devices)),
	)

	return devices, nil
}

// DiscoverAllDevices discovers all supported device types.
// It uses the plugin registry to discover all devices and executes discovery hooks.
func (r *deviceRegistryImpl) DiscoverAllDevices(ctx context.Context) ([]types.Device, error) {
	r.logger.Info("Discovering all devices")

	// Discover via plugin registry
	devices, err := r.pluginRegistry.DiscoverDevices(ctx)
	if err != nil {
		r.logger.Error("Plugin discovery failed",
			zap.Error(err),
		)
		return nil, fmt.Errorf("plugin discovery failed: %w", err)
	}

	// Group devices by type for hook execution
	devicesByType := make(map[types.DeviceType][]types.Device)
	for _, device := range devices {
		deviceType := device.GetMetadata().Type
		devicesByType[deviceType] = append(devicesByType[deviceType], device)
	}

	// Execute discovery hooks for each device type
	for deviceType, typeDevices := range devicesByType {
		hookCtx := &types.DiscoveryHookContext{
			DeviceType:        deviceType,
			DiscoveredDevices: typeDevices,
			Metadata:          make(map[string]interface{}),
		}

		if err := r.hookRegistry.ExecuteDiscoveryHooks(ctx, hookCtx); err != nil {
			r.logger.Warn("Discovery hooks failed",
				zap.String("device_type", string(deviceType)),
				zap.Error(err),
			)
			// Continue despite hook failure - hooks are advisory
		}
	}

	r.logger.Info("Device discovery completed",
		zap.Int("device_count", len(devices)),
	)

	return devices, nil
}

// RegisterDevice registers a discovered device.
// It creates a state machine for the device, executes registration hooks,
// and updates internal indexes.
func (r *deviceRegistryImpl) RegisterDevice(ctx context.Context, device types.Device) error {
	if device == nil {
		r.logger.Warn("Attempted to register nil device")
		return fmt.Errorf("%w: device cannot be nil", types.ErrInvalidDevice)
	}

	deviceID := device.GetID()
	if deviceID == "" {
		r.logger.Warn("Attempted to register device with empty ID")
		return fmt.Errorf("%w: device ID cannot be empty", types.ErrInvalidDevice)
	}

	metadata := device.GetMetadata()
	if metadata.Type == types.DeviceTypeUnknown {
		r.logger.Warn("Attempted to register device with unknown type",
			zap.String("device_id", deviceID),
		)
		return fmt.Errorf("%w: device type cannot be unknown", types.ErrInvalidDevice)
	}

	// Check if already registered (under lock)
	r.mu.Lock()
	if _, exists := r.devices[deviceID]; exists {
		r.mu.Unlock()
		r.logger.Warn("Device already registered",
			zap.String("device_id", deviceID),
		)
		return fmt.Errorf("device %s already registered: %w", deviceID, types.ErrDeviceExists)
	}
	r.mu.Unlock()

	// Create state machine for device (outside lock)
	_, err := r.stateRegistry.GetOrCreateStateMachine(ctx, deviceID, metadata.Type)
	if err != nil {
		r.logger.Error("Failed to create state machine",
			zap.String("device_id", deviceID),
			zap.String("device_type", string(metadata.Type)),
			zap.Error(err),
		)
		return fmt.Errorf("failed to create state machine: %w", err)
	}

	// Execute registration hooks (outside lock)
	hookCtx := &types.RegistrationHookContext{
		Device:            device,
		Metadata:          metadata,
		Capabilities:      metadata.Capabilities,
		AdditionalMetadata: make(map[string]interface{}),
	}

	if err := r.hookRegistry.ExecuteRegistrationHooks(ctx, hookCtx); err != nil {
		r.logger.Warn("Registration hooks failed",
			zap.String("device_id", deviceID),
			zap.Error(err),
		)
		// Continue despite hook failure - hooks are advisory
		// But we should clean up state machine if hooks fail critically
		// For now, we continue as hooks are advisory
	}

	// Register device and update indexes (under lock)
	r.mu.Lock()
	r.devices[deviceID] = device
	r.devicesByType[metadata.Type] = append(r.devicesByType[metadata.Type], device)

	// Index by capability
	for cap := range metadata.Capabilities {
		r.devicesByCapability[cap] = append(r.devicesByCapability[cap], device)
	}
	r.mu.Unlock()

	// Persist device to storage backend (outside lock)
	if err := r.storageBackend.SaveDevice(ctx, device); err != nil {
		r.logger.Warn("Failed to persist device to storage",
			zap.String("device_id", deviceID),
			zap.Error(err),
		)
		// Continue despite persistence failure - device is still registered in memory
	}

	r.logger.Info("Device registered successfully",
		zap.String("device_id", deviceID),
		zap.String("device_type", string(metadata.Type)),
		zap.Int("capability_count", metadata.Capabilities.Count()),
	)

	return nil
}

// GetDevice retrieves a device by ID.
func (r *deviceRegistryImpl) GetDevice(ctx context.Context, deviceID string) (types.Device, error) {
	if deviceID == "" {
		r.logger.Warn("Attempted to get device with empty ID")
		return nil, fmt.Errorf("%w: device ID cannot be empty", types.ErrInvalidDevice)
	}

	r.mu.RLock()
	device, exists := r.devices[deviceID]
	r.mu.RUnlock()

	if !exists {
		r.logger.Debug("Device not found",
			zap.String("device_id", deviceID),
		)
		return nil, fmt.Errorf("device %s not found: %w", deviceID, types.ErrDeviceNotFound)
	}

	return device, nil
}

// ListDevices lists all registered devices, optionally filtered by type or capability.
func (r *deviceRegistryImpl) ListDevices(ctx context.Context, filters *types.DeviceFilters) ([]types.Device, error) {
	r.mu.RLock()
	// Copy all devices under lock
	allDevices := make([]types.Device, 0, len(r.devices))
	for _, device := range r.devices {
		allDevices = append(allDevices, device)
	}
	r.mu.RUnlock()

	// Apply filters outside lock
	if filters == nil {
		return allDevices, nil
	}

	result := make([]types.Device, 0)
	for _, device := range allDevices {
		if r.matchesFilters(device, filters) {
			result = append(result, device)
		}
	}

	r.logger.Debug("Listed devices",
		zap.Int("total_devices", len(allDevices)),
		zap.Int("filtered_devices", len(result)),
	)

	return result, nil
}

// UpdateDevice updates device metadata.
// It updates the device's metadata and refreshes indexes if type or capabilities change.
func (r *deviceRegistryImpl) UpdateDevice(ctx context.Context, deviceID string, updates *types.DeviceMetadataUpdate) error {
	if deviceID == "" {
		r.logger.Warn("Attempted to update device with empty ID")
		return fmt.Errorf("%w: device ID cannot be empty", types.ErrInvalidDevice)
	}

	if updates == nil {
		r.logger.Warn("Attempted to update device with nil updates",
			zap.String("device_id", deviceID),
		)
		return fmt.Errorf("%w: updates cannot be nil", types.ErrInvalidDevice)
	}

	// Get device (under lock)
	r.mu.RLock()
	device, exists := r.devices[deviceID]
	if !exists {
		r.mu.RUnlock()
		r.logger.Warn("Device not found for update",
			zap.String("device_id", deviceID),
		)
		return fmt.Errorf("device %s not found: %w", deviceID, types.ErrDeviceNotFound)
	}
	oldMetadata := device.GetMetadata()
	r.mu.RUnlock()

	// Update device metadata (device implementation handles this)
	if err := device.UpdateMetadata(ctx, updates); err != nil {
		r.logger.Error("Failed to update device metadata",
			zap.String("device_id", deviceID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to update device metadata: %w", err)
	}

	// Persist updated device to storage backend (outside lock)
	if err := r.storageBackend.SaveDevice(ctx, device); err != nil {
		r.logger.Warn("Failed to persist device update to storage",
			zap.String("device_id", deviceID),
			zap.Error(err),
		)
		// Continue despite persistence failure - device is still updated in memory
	}

	// Get new metadata
	newMetadata := device.GetMetadata()

	// Check if type or capabilities changed (need to update indexes)
	typeChanged := oldMetadata.Type != newMetadata.Type
	capabilitiesChanged := !capabilitiesEqual(oldMetadata.Capabilities, newMetadata.Capabilities)

	if typeChanged || capabilitiesChanged {
		// Update indexes (under lock)
		r.mu.Lock()

		// Remove from old type index if type changed
		if typeChanged {
			oldDevices := r.devicesByType[oldMetadata.Type]
			for i, d := range oldDevices {
				if d.GetID() == deviceID {
					r.devicesByType[oldMetadata.Type] = append(oldDevices[:i], oldDevices[i+1:]...)
					break
				}
			}
			// Add to new type index
			r.devicesByType[newMetadata.Type] = append(r.devicesByType[newMetadata.Type], device)
		}

		// Update capability index if capabilities changed
		if capabilitiesChanged {
			// Remove from all old capability indexes
			for cap := range oldMetadata.Capabilities {
				devices := r.devicesByCapability[cap]
				for i, d := range devices {
					if d.GetID() == deviceID {
						r.devicesByCapability[cap] = append(devices[:i], devices[i+1:]...)
						break
					}
				}
			}
			// Add to new capability indexes
			for cap := range newMetadata.Capabilities {
				// Check if device is already in this capability index
				found := false
				for _, d := range r.devicesByCapability[cap] {
					if d.GetID() == deviceID {
						found = true
						break
					}
				}
				if !found {
					r.devicesByCapability[cap] = append(r.devicesByCapability[cap], device)
				}
			}
		}

		r.mu.Unlock()
	}

	r.logger.Info("Device updated successfully",
		zap.String("device_id", deviceID),
		zap.Bool("type_changed", typeChanged),
		zap.Bool("capabilities_changed", capabilitiesChanged),
	)

	return nil
}

// DeleteDevice removes a device from the registry.
// It executes teardown hooks, deletes the state machine, and updates indexes.
func (r *deviceRegistryImpl) DeleteDevice(ctx context.Context, deviceID string) error {
	if deviceID == "" {
		r.logger.Warn("Attempted to delete device with empty ID")
		return fmt.Errorf("%w: device ID cannot be empty", types.ErrInvalidDevice)
	}

	// Get device (under lock)
	r.mu.Lock()
	device, exists := r.devices[deviceID]
	if !exists {
		r.mu.Unlock()
		r.logger.Warn("Device not found for deletion",
			zap.String("device_id", deviceID),
		)
		return fmt.Errorf("device %s not found: %w", deviceID, types.ErrDeviceNotFound)
	}

	metadata := device.GetMetadata()
	r.mu.Unlock()

	// Execute teardown hooks (outside lock)
	hookCtx := &types.TeardownHookContext{
		Device: device,
		Reason: "deletion",
		Metadata: make(map[string]interface{}),
	}

	if err := r.hookRegistry.ExecuteTeardownHooks(ctx, hookCtx); err != nil {
		r.logger.Warn("Teardown hooks failed",
			zap.String("device_id", deviceID),
			zap.Error(err),
		)
		// Continue despite hook failure - hooks are advisory
	}

	// Remove state machine (outside lock)
	if err := r.stateRegistry.RemoveStateMachine(deviceID); err != nil {
		r.logger.Warn("Failed to remove state machine",
			zap.String("device_id", deviceID),
			zap.Error(err),
		)
		// Continue despite state machine removal failure
	}

	// Remove from registry and indexes (under lock)
	r.mu.Lock()
	delete(r.devices, deviceID)

	// Remove from type index
	devices := r.devicesByType[metadata.Type]
	for i, d := range devices {
		if d.GetID() == deviceID {
			r.devicesByType[metadata.Type] = append(devices[:i], devices[i+1:]...)
			break
		}
	}

	// Remove from capability indexes
	for cap := range metadata.Capabilities {
		capDevices := r.devicesByCapability[cap]
		for i, d := range capDevices {
			if d.GetID() == deviceID {
				r.devicesByCapability[cap] = append(capDevices[:i], capDevices[i+1:]...)
				break
			}
		}
	}
	r.mu.Unlock()

	// Delete device from storage backend (outside lock)
	if err := r.storageBackend.DeleteDevice(ctx, deviceID); err != nil {
		r.logger.Warn("Failed to delete device from storage",
			zap.String("device_id", deviceID),
			zap.Error(err),
		)
		// Continue despite persistence failure - device is still deleted from memory
	}

	r.logger.Info("Device deleted successfully",
		zap.String("device_id", deviceID),
		zap.String("device_type", string(metadata.Type)),
	)

	return nil
}

// GetDevicesByCapability returns all devices that support a specific capability.
func (r *deviceRegistryImpl) GetDevicesByCapability(ctx context.Context, capability types.DeviceCapability) ([]types.Device, error) {
	r.mu.RLock()
	devices := make([]types.Device, len(r.devicesByCapability[capability]))
	copy(devices, r.devicesByCapability[capability])
	r.mu.RUnlock()

	r.logger.Debug("Listed devices by capability",
		zap.String("capability", string(capability)),
		zap.Int("device_count", len(devices)),
	)

	return devices, nil
}

// GetDevicesByType returns all devices of a specific type.
func (r *deviceRegistryImpl) GetDevicesByType(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error) {
	r.mu.RLock()
	devices := make([]types.Device, len(r.devicesByType[deviceType]))
	copy(devices, r.devicesByType[deviceType])
	r.mu.RUnlock()

	r.logger.Debug("Listed devices by type",
		zap.String("device_type", string(deviceType)),
		zap.Int("device_count", len(devices)),
	)

	return devices, nil
}

// matchesFilters checks if a device matches the given filters.
func (r *deviceRegistryImpl) matchesFilters(device types.Device, filters *types.DeviceFilters) bool {
	metadata := device.GetMetadata()

	// Filter by type
	if filters.Type != nil && *filters.Type != metadata.Type {
		return false
	}

	// Filter by capability
	if filters.Capability != nil && !metadata.Capabilities.Has(*filters.Capability) {
		return false
	}

	// Filter by enabled status
	if filters.Enabled != nil && *filters.Enabled != device.IsEnabled() {
		return false
	}

	// Filter by status
	if filters.Status != nil && *filters.Status != device.GetStatus() {
		return false
	}

	// Filter by zone
	if filters.Zone != nil && *filters.Zone != metadata.Zone {
		return false
	}

	// Filter by tags (device must have all specified tags)
	if len(filters.Tags) > 0 {
		deviceTags := make(map[string]bool)
		for _, tag := range metadata.Tags {
			deviceTags[tag] = true
		}
		for _, filterTag := range filters.Tags {
			if !deviceTags[filterTag] {
				return false
			}
		}
	}

	return true
}

// capabilitiesEqual compares two DeviceCapabilities maps for equality.
func capabilitiesEqual(a, b types.DeviceCapabilities) bool {
	if len(a) != len(b) {
		return false
	}
	for cap := range a {
		if !b.Has(cap) {
			return false
		}
	}
	for cap := range b {
		if !a.Has(cap) {
			return false
		}
	}
	return true
}

