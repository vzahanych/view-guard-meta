package iot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/processing"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// iotServiceImpl implements the IoTService interface.
// It composes all subcomponents (plugin-registry, device-registry, state-machine, processing, hooks)
// and delegates operations to them following the vm-gateway pattern.
type iotServiceImpl struct {
	// Subcomponents
	pluginRegistry    types.DevicePluginRegistry
	deviceRegistry    types.DeviceRegistry
	stateRegistry     types.DeviceStateMachineRegistry
	processingService *processing.DataProcessingService
	hookRegistry      types.LifecycleHookRegistry

	// Configuration
	config *types.IoTServiceConfig

	// Observability
	logger *zap.Logger

	// State
	mu      sync.RWMutex
	started bool
}

// NewIoTService creates a new IoT service instance.
// This factory function should typically not be called directly;
// use IoTServiceProvider instead for proper dependency injection.
//
// All subcomponents are required dependencies. They should be created using their respective
// constructors from subpackages (e.g., pluginregistry.NewDevicePluginRegistry).
func NewIoTService(
	pluginRegistry types.DevicePluginRegistry,
	deviceRegistry types.DeviceRegistry,
	stateRegistry types.DeviceStateMachineRegistry,
	processingService *processing.DataProcessingService,
	hookRegistry types.LifecycleHookRegistry,
	config *types.IoTServiceConfig,
	logger *zap.Logger,
) IoTService {
	if config == nil {
		config = &types.IoTServiceConfig{} // Default config
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &iotServiceImpl{
		pluginRegistry:    pluginRegistry,
		deviceRegistry:    deviceRegistry,
		stateRegistry:     stateRegistry,
		processingService: processingService,
		hookRegistry:      hookRegistry,
		config:            config,
		logger:            logger,
		started:           false,
	}
}

// Name returns the service name for identification and logging.
func (s *iotServiceImpl) Name() string {
	return "iot-service"
}

// Start starts all underlying services (plugin registry, device registry, etc.).
// Locking strategy: Copy service references under lock, call them outside lock to avoid deadlocks.
//
// Note: Most IoT subcomponents are stateless and don't have Start() methods.
// This method verifies component readiness and logs component status.
// Future components that require startup will be started here.
func (s *iotServiceImpl) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		s.logger.Warn("IoT Service already started")
		return types.ErrAlreadyStarted
	}

	// Copy service references under lock
	pluginReg := s.pluginRegistry
	deviceReg := s.deviceRegistry
	stateReg := s.stateRegistry
	processingSvc := s.processingService
	hookReg := s.hookRegistry
	s.mu.Unlock()

	s.logger.Info("Starting IoT Service (all components)")

	// Verify and log component readiness (call outside lock to avoid deadlocks)
	// Note: Most components are stateless and don't have Start() methods.
	// This is for future extensibility when components require startup.

	// Plugin registry (stateless, no Start needed)
	if pluginReg != nil {
		plugins := pluginReg.ListPlugins()
		s.logger.Info("Plugin registry ready",
			zap.Int("registered_plugins", len(plugins)))
	} else {
		s.logger.Warn("Plugin registry is nil")
	}

	// Device registry (stateless, no Start needed)
	if deviceReg != nil {
		s.logger.Info("Device registry ready")
	} else {
		s.logger.Warn("Device registry is nil")
	}

	// State registry (stateless, no Start needed)
	if stateReg != nil {
		s.logger.Info("State registry ready")
	} else {
		s.logger.Warn("State registry is nil")
	}

	// Processing service (may have Start in future)
	if processingSvc != nil {
		s.logger.Info("Processing service ready")
	} else {
		s.logger.Warn("Processing service is nil")
	}

	// Hook registry (stateless, no Start needed)
	if hookReg != nil {
		hooks := hookReg.ListHooks(nil)
		s.logger.Info("Hook registry ready",
			zap.Int("registered_hooks", len(hooks)))
	} else {
		s.logger.Warn("Hook registry is nil")
	}

	// Mark as started under lock
	s.mu.Lock()
	s.started = true
	s.mu.Unlock()

	// Log final status
	pluginCount := 0
	hookCount := 0
	if pluginReg != nil {
		pluginCount = len(pluginReg.ListPlugins())
	}
	if hookReg != nil {
		hookCount = len(hookReg.ListHooks(nil))
	}

	s.logger.Info("IoT Service started successfully",
		zap.Int("registered_plugins", pluginCount),
		zap.Int("registered_hooks", hookCount),
	)

	return nil
}

// Stop gracefully shuts down all underlying services.
// Locking strategy: Copy service references under lock, call them outside lock to avoid deadlocks.
//
// Note: Most IoT subcomponents are stateless and don't have Stop() methods.
// This method logs component shutdown and marks the service as stopped.
// Future components that require shutdown will be stopped here in reverse order.
func (s *iotServiceImpl) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil // Already stopped
	}

	// Copy service references under lock (reverse order for stopping)
	hookReg := s.hookRegistry
	processingSvc := s.processingService
	stateReg := s.stateRegistry
	deviceReg := s.deviceRegistry
	pluginReg := s.pluginRegistry
	s.mu.Unlock()

	s.logger.Info("Stopping IoT Service (all components)")

	// Stop components in reverse order (call outside lock to avoid deadlocks)
	// Note: Most components are stateless and don't have Stop() methods.
	// This is for future extensibility when components require shutdown.

	// Hook registry (stateless, no Stop needed)
	if hookReg != nil {
		s.logger.Info("Stopping hook registry...")
		// Future: if hookReg has Stop(), call it here
	}

	// Processing service (may have Stop in future)
	if processingSvc != nil {
		s.logger.Info("Stopping processing service...")
		// Future: if processingSvc has Stop(), call it here
	}

	// State registry (stateless, no Stop needed)
	if stateReg != nil {
		s.logger.Info("Stopping state registry...")
		// Future: if stateReg has Stop(), call it here
	}

	// Device registry (stateless, no Stop needed)
	if deviceReg != nil {
		s.logger.Info("Stopping device registry...")
		// Future: if deviceReg has Stop(), call it here
	}

	// Plugin registry (stateless, no Stop needed)
	if pluginReg != nil {
		s.logger.Info("Stopping plugin registry...")
		// Future: if pluginReg has Stop(), call it here
	}

	// Mark as stopped under lock
	s.mu.Lock()
	s.started = false
	s.mu.Unlock()

	s.logger.Info("IoT Service stopped successfully")
	return nil
}

// HealthSnapshot returns a comprehensive health snapshot of the service.
// Locking strategy: Copy service references under lock, call them outside lock to avoid deadlocks.
func (s *iotServiceImpl) HealthSnapshot() IoTServiceStatus {
	s.mu.RLock()
	started := s.started

	// Copy references under lock
	pluginReg := s.pluginRegistry
	deviceReg := s.deviceRegistry
	stateReg := s.stateRegistry
	processingS := s.processingService
	s.mu.RUnlock()

	// Build status outside lock (call sub-services outside lock to avoid deadlocks)
	status := IoTServiceStatus{
		Timestamp:        time.Now(),
		RegisteredDevices: 0,
		DevicesByType:    make(map[types.DeviceType]int),
		PluginStatus:     make(map[types.DeviceType]PluginStatus),
		ProcessingStatus: ProcessingStatus{Enabled: false},
		StateRegistrySize: 0,
		SubServices:      make(map[string]ServiceStatus),
	}

	// Get device counts
	if deviceReg != nil {
		devices, err := deviceReg.ListDevices(context.Background(), nil)
		if err == nil {
			status.RegisteredDevices = len(devices)

			// Count by type
			status.DevicesByType = make(map[types.DeviceType]int)
			for _, device := range devices {
				deviceType := device.GetMetadata().Type
				status.DevicesByType[deviceType]++
			}
		} else {
			s.logger.Debug("Failed to get device list for health snapshot",
				zap.Error(err))
		}
	}

	// Get plugin status
	if pluginReg != nil {
		plugins := pluginReg.ListPlugins()
		status.PluginStatus = make(map[types.DeviceType]PluginStatus)
		for _, plugin := range plugins {
			deviceType := plugin.GetDeviceType()
			status.PluginStatus[deviceType] = PluginStatus{
				Registered:   true,
				Capabilities: plugin.GetSupportedCapabilities(),
			}
		}
	}

	// Get state registry size
	if stateReg != nil {
		allStates := stateReg.GetAllStateMachines()
		status.StateRegistrySize = len(allStates)
	}

	// Get processing status
	if processingS != nil {
		processors := processingS.ListProcessors(nil)
		status.ProcessingStatus = ProcessingStatus{
			Enabled:             true,
			RegisteredProcessors: len(processors),
		}
	}

	// Sub-service status
	status.SubServices["plugin-registry"] = ServiceStatus{Name: "plugin-registry", Started: started}
	status.SubServices["device-registry"] = ServiceStatus{Name: "device-registry", Started: started}
	status.SubServices["state-registry"] = ServiceStatus{Name: "state-registry", Started: started}
	status.SubServices["processing"] = ServiceStatus{Name: "processing", Started: started}
	status.SubServices["hooks"] = ServiceStatus{Name: "hooks", Started: started}

	return status
}

// DiscoverDevices discovers devices using all registered plugins.
// Delegates to deviceRegistry.DiscoverAllDevices which coordinates discovery
// across all registered plugins and executes lifecycle hooks.
func (s *iotServiceImpl) DiscoverDevices(ctx context.Context) ([]types.Device, error) {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		s.logger.Warn("IoT Service not started")
		return nil, types.ErrNotStarted
	}
	deviceReg := s.deviceRegistry
	s.mu.RUnlock()

	if deviceReg == nil {
		s.logger.Error("Device registry not initialized")
		return nil, types.ErrNotInitialized
	}

	s.logger.Debug("Discovering all devices")
	devices, err := deviceReg.DiscoverAllDevices(ctx)
	if err != nil {
		s.logger.Error("Device discovery failed", zap.Error(err))
		return nil, fmt.Errorf("device discovery failed: %w", err)
	}

	s.logger.Info("Device discovery completed",
		zap.Int("discovered_count", len(devices)))

	return devices, nil
}

// DiscoverDevicesByType discovers devices of a specific type using the appropriate plugin.
// Delegates to deviceRegistry.DiscoverDevices which coordinates discovery
// for the specified device type and executes lifecycle hooks.
func (s *iotServiceImpl) DiscoverDevicesByType(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error) {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		s.logger.Warn("IoT Service not started", zap.String("device_type", string(deviceType)))
		return nil, types.ErrNotStarted
	}
	deviceReg := s.deviceRegistry
	s.mu.RUnlock()

	if deviceReg == nil {
		s.logger.Error("Device registry not initialized", zap.String("device_type", string(deviceType)))
		return nil, types.ErrNotInitialized
	}

	s.logger.Debug("Discovering devices by type",
		zap.String("device_type", string(deviceType)))
	devices, err := deviceReg.DiscoverDevices(ctx, deviceType)
	if err != nil {
		s.logger.Error("Device discovery failed",
			zap.String("device_type", string(deviceType)),
			zap.Error(err))
		return nil, fmt.Errorf("device discovery failed for type %s: %w", deviceType, err)
	}

	s.logger.Info("Device discovery completed",
		zap.String("device_type", string(deviceType)),
		zap.Int("discovered_count", len(devices)))

	return devices, nil
}

// RegisterDevice registers a discovered device.
// Delegates to deviceRegistry which handles registration, state machine creation, and lifecycle hooks.
func (s *iotServiceImpl) RegisterDevice(ctx context.Context, device types.Device) error {
	// Check for nil device first (before checking started state)
	if device == nil {
		s.logger.Warn("Attempted to register nil device")
		return types.ErrInvalidDevice
	}

	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		s.logger.Warn("IoT Service not started", zap.String("device_id", device.GetID()))
		return types.ErrNotStarted
	}
	deviceReg := s.deviceRegistry
	s.mu.RUnlock()

	if deviceReg == nil {
		s.logger.Error("Device registry not initialized", zap.String("device_id", device.GetID()))
		return types.ErrNotInitialized
	}

	deviceID := device.GetID()
	deviceType := device.GetMetadata().Type
	s.logger.Info("Registering device",
		zap.String("device_id", deviceID),
		zap.String("device_type", string(deviceType)))

	err := deviceReg.RegisterDevice(ctx, device)
	if err != nil {
		s.logger.Error("Device registration failed",
			zap.String("device_id", deviceID),
			zap.String("device_type", string(deviceType)),
			zap.Error(err))
		return fmt.Errorf("device registration failed: %w", err)
	}

	s.logger.Info("Device registered successfully",
		zap.String("device_id", deviceID),
		zap.String("device_type", string(deviceType)))

	return nil
}

// GetDevice retrieves a device by ID.
// Delegates to deviceRegistry for device retrieval.
// Note: GetDevice does not require service to be started (read-only operation).
func (s *iotServiceImpl) GetDevice(ctx context.Context, deviceID string) (types.Device, error) {
	s.mu.RLock()
	deviceReg := s.deviceRegistry
	s.mu.RUnlock()

	if deviceReg == nil {
		s.logger.Error("Device registry not initialized", zap.String("device_id", deviceID))
		return nil, types.ErrNotInitialized
	}

	s.logger.Debug("Getting device", zap.String("device_id", deviceID))
	device, err := deviceReg.GetDevice(ctx, deviceID)
	if err != nil {
		s.logger.Debug("Device not found", zap.String("device_id", deviceID), zap.Error(err))
		return nil, fmt.Errorf("get device: %w", err)
	}

	return device, nil
}

// ListDevices lists all registered devices, optionally filtered by type or capability.
// Delegates to deviceRegistry for device listing with optional filters.
// Note: ListDevices does not require service to be started (read-only operation).
func (s *iotServiceImpl) ListDevices(ctx context.Context, filters *types.DeviceFilters) ([]types.Device, error) {
	s.mu.RLock()
	deviceReg := s.deviceRegistry
	s.mu.RUnlock()

	if deviceReg == nil {
		s.logger.Error("Device registry not initialized")
		return nil, types.ErrNotInitialized
	}

	s.logger.Debug("Listing devices", zap.Any("filters", filters))
	devices, err := deviceReg.ListDevices(ctx, filters)
	if err != nil {
		s.logger.Error("List devices failed", zap.Error(err))
		return nil, fmt.Errorf("list devices failed: %w", err)
	}

	s.logger.Debug("Listed devices",
		zap.Int("device_count", len(devices)),
		zap.Any("filters", filters))

	return devices, nil
}

// UpdateDevice updates device metadata.
// Delegates to deviceRegistry which handles metadata updates and persistence.
func (s *iotServiceImpl) UpdateDevice(ctx context.Context, deviceID string, updates *types.DeviceMetadataUpdate) error {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		s.logger.Warn("IoT Service not started", zap.String("device_id", deviceID))
		return types.ErrNotStarted
	}
	deviceReg := s.deviceRegistry
	s.mu.RUnlock()

	if deviceReg == nil {
		s.logger.Error("Device registry not initialized", zap.String("device_id", deviceID))
		return types.ErrNotInitialized
	}

	s.logger.Info("Updating device", zap.String("device_id", deviceID))
	err := deviceReg.UpdateDevice(ctx, deviceID, updates)
	if err != nil {
		s.logger.Error("Device update failed",
			zap.String("device_id", deviceID),
			zap.Error(err))
		return fmt.Errorf("device update failed: %w", err)
	}

	s.logger.Info("Device updated successfully", zap.String("device_id", deviceID))
	return nil
}

// DeleteDevice removes a device from the registry.
// Delegates to deviceRegistry which handles device deletion, state machine removal, and lifecycle hooks.
func (s *iotServiceImpl) DeleteDevice(ctx context.Context, deviceID string) error {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		s.logger.Warn("IoT Service not started", zap.String("device_id", deviceID))
		return types.ErrNotStarted
	}
	deviceReg := s.deviceRegistry
	s.mu.RUnlock()

	if deviceReg == nil {
		s.logger.Error("Device registry not initialized", zap.String("device_id", deviceID))
		return types.ErrNotInitialized
	}

	s.logger.Info("Deleting device", zap.String("device_id", deviceID))
	err := deviceReg.DeleteDevice(ctx, deviceID)
	if err != nil {
		s.logger.Error("Device deletion failed",
			zap.String("device_id", deviceID),
			zap.Error(err))
		return fmt.Errorf("device deletion failed: %w", err)
	}

	s.logger.Info("Device deleted successfully", zap.String("device_id", deviceID))
	return nil
}

// GetDevicesByCapability returns all devices that support a specific capability.
// Delegates to deviceRegistry for capability-based device querying.
// Note: GetDevicesByCapability does not require service to be started (read-only operation).
func (s *iotServiceImpl) GetDevicesByCapability(ctx context.Context, capability types.DeviceCapability) ([]types.Device, error) {
	s.mu.RLock()
	deviceReg := s.deviceRegistry
	s.mu.RUnlock()

	if deviceReg == nil {
		s.logger.Error("Device registry not initialized", zap.String("capability", string(capability)))
		return nil, types.ErrNotInitialized
	}

	s.logger.Debug("Getting devices by capability", zap.String("capability", string(capability)))
	devices, err := deviceReg.GetDevicesByCapability(ctx, capability)
	if err != nil {
		s.logger.Error("Get devices by capability failed",
			zap.String("capability", string(capability)),
			zap.Error(err))
		return nil, fmt.Errorf("get devices by capability failed: %w", err)
	}

	s.logger.Debug("Got devices by capability",
		zap.String("capability", string(capability)),
		zap.Int("device_count", len(devices)))

	return devices, nil
}

// GetDevicesByType returns all devices of a specific type.
// Delegates to deviceRegistry for type-based device querying.
// Note: GetDevicesByType does not require service to be started (read-only operation).
func (s *iotServiceImpl) GetDevicesByType(ctx context.Context, deviceType types.DeviceType) ([]types.Device, error) {
	s.mu.RLock()
	deviceReg := s.deviceRegistry
	s.mu.RUnlock()

	if deviceReg == nil {
		s.logger.Error("Device registry not initialized", zap.String("device_type", string(deviceType)))
		return nil, types.ErrNotInitialized
	}

	s.logger.Debug("Getting devices by type", zap.String("device_type", string(deviceType)))
	devices, err := deviceReg.GetDevicesByType(ctx, deviceType)
	if err != nil {
		s.logger.Error("Get devices by type failed",
			zap.String("device_type", string(deviceType)),
			zap.Error(err))
		return nil, fmt.Errorf("get devices by type failed: %w", err)
	}

	s.logger.Debug("Got devices by type",
		zap.String("device_type", string(deviceType)),
		zap.Int("device_count", len(devices)))

	return devices, nil
}

// GetStateMachine retrieves a state machine for a device.
// Delegates to stateRegistry for state machine retrieval.
// Note: GetStateMachine does not require service to be started (read-only operation).
func (s *iotServiceImpl) GetStateMachine(ctx context.Context, deviceID string) (types.DeviceStateMachine, error) {
	s.mu.RLock()
	stateReg := s.stateRegistry
	s.mu.RUnlock()

	if stateReg == nil {
		s.logger.Error("State registry not initialized", zap.String("device_id", deviceID))
		return nil, types.ErrNotInitialized
	}

	s.logger.Debug("Getting state machine", zap.String("device_id", deviceID))
	stateMachine, err := stateReg.GetStateMachine(deviceID)
	if err != nil {
		s.logger.Debug("State machine not found", zap.String("device_id", deviceID), zap.Error(err))
		return nil, fmt.Errorf("get state machine failed: %w", err)
	}

	return stateMachine, nil
}

// GetStateMachinesByType returns all state machines for a specific device type.
// Delegates to stateRegistry for type-based state machine querying.
// Note: GetStateMachinesByType does not require service to be started (read-only operation).
func (s *iotServiceImpl) GetStateMachinesByType(ctx context.Context, deviceType types.DeviceType) ([]types.DeviceStateMachine, error) {
	s.mu.RLock()
	stateReg := s.stateRegistry
	s.mu.RUnlock()

	if stateReg == nil {
		s.logger.Error("State registry not initialized", zap.String("device_type", string(deviceType)))
		return nil, types.ErrNotInitialized
	}

	s.logger.Debug("Getting state machines by type", zap.String("device_type", string(deviceType)))
	stateMachines := stateReg.GetStateMachinesByType(deviceType)

	s.logger.Debug("Got state machines by type",
		zap.String("device_type", string(deviceType)),
		zap.Int("count", len(stateMachines)))

	return stateMachines, nil
}

// ProcessDeviceData processes data from a device through the processing pipeline.
// Delegates to processingService which applies registered processors to the data.
// Note: ProcessDeviceData requires service to be started (write operation).
func (s *iotServiceImpl) ProcessDeviceData(ctx context.Context, device types.Device, data *types.DeviceData) (*types.DataProcessingContext, error) {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		s.logger.Warn("IoT Service not started", zap.String("device_id", device.GetID()))
		return nil, types.ErrNotStarted
	}
	processingS := s.processingService
	s.mu.RUnlock()

	if processingS == nil {
		s.logger.Error("Processing service not initialized", zap.String("device_id", device.GetID()))
		return nil, types.ErrNotInitialized
	}

	s.logger.Debug("Processing device data",
		zap.String("device_id", device.GetID()),
		zap.String("data_type", string(data.DataType)))

	result, err := processingS.ProcessDeviceData(ctx, device, data)
	if err != nil {
		s.logger.Error("Data processing failed",
			zap.String("device_id", device.GetID()),
			zap.String("data_type", string(data.DataType)),
			zap.Error(err))
		return nil, fmt.Errorf("data processing failed: %w", err)
	}

	s.logger.Debug("Data processing completed",
		zap.String("device_id", device.GetID()),
		zap.String("data_type", string(data.DataType)),
		zap.Int("processors_applied", len(result.ProcessorsApplied)))

	return result, nil
}

// RegisterPlugin registers a device plugin for a specific device type.
// Delegates to pluginRegistry which manages plugin registration and validation.
// Note: RegisterPlugin requires service to be started (write operation).
func (s *iotServiceImpl) RegisterPlugin(ctx context.Context, plugin types.DevicePlugin) error {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		s.logger.Warn("IoT Service not started",
			zap.String("device_type", string(plugin.GetDeviceType())))
		return types.ErrNotStarted
	}
	pluginReg := s.pluginRegistry
	s.mu.RUnlock()

	if pluginReg == nil {
		s.logger.Error("Plugin registry not initialized",
			zap.String("device_type", string(plugin.GetDeviceType())))
		return types.ErrNotInitialized
	}

	deviceType := plugin.GetDeviceType()
	s.logger.Info("Registering device plugin",
		zap.String("device_type", string(deviceType)))

	err := pluginReg.RegisterPlugin(plugin)
	if err != nil {
		s.logger.Error("Plugin registration failed",
			zap.String("device_type", string(deviceType)),
			zap.Error(err))
		return fmt.Errorf("plugin registration failed: %w", err)
	}

	s.logger.Info("Plugin registered successfully",
		zap.String("device_type", string(deviceType)))

	return nil
}

// GetSupportedDeviceTypes returns all device types that have registered plugins.
// Delegates to pluginRegistry for device type querying.
// Note: GetSupportedDeviceTypes does not require service to be started (read-only operation).
func (s *iotServiceImpl) GetSupportedDeviceTypes(ctx context.Context) ([]types.DeviceType, error) {
	s.mu.RLock()
	pluginReg := s.pluginRegistry
	s.mu.RUnlock()

	if pluginReg == nil {
		s.logger.Error("Plugin registry not initialized")
		return nil, types.ErrNotInitialized
	}

	s.logger.Debug("Getting supported device types")
	deviceTypes := pluginReg.GetSupportedDeviceTypes()

	s.logger.Debug("Got supported device types",
		zap.Int("count", len(deviceTypes)))

	return deviceTypes, nil
}

