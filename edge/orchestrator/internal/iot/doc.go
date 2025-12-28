/*
Package iot provides a unified, device-agnostic interface for IoT device management.
It coordinates device discovery, registration, state management, data processing,
and lifecycle hooks across all device types (cameras, sensors, audio devices, etc.).

The IoT service is the core service that manages all IoT devices on the Edge appliance.
It abstracts away device-specific details through the DevicePlugin system, allowing the
system to work with any device type (cameras, sensors, audio devices, etc.) through a
unified interface.

# Architecture

The IoT service follows a clean, device-agnostic architecture similar to vm-gateway:

	┌─────────────────────────────────────────────────────────┐
	│                  IoTService Interface                    │
	│  (Unified API for Device Management)                    │
	│  - Lifecycle management (Start/Stop)                    │
	│  - Device discovery and registration                    │
	│  - State management                                     │
	│  - Data processing                                      │
	│  - Observability (HealthSnapshot)                       │
	└─────────────────────────────────────────────────────────┘
	                          │
	          ┌───────────────┴───────────────┐
	          │                                 │
	┌─────────▼──────────┐         ┌───────────▼──────────┐
	│  Device Registry   │         │   Plugin Registry     │
	│                    │         │                      │
	│ - Device storage   │         │ - Device discovery   │
	│ - Lifecycle mgmt   │         │ - Plugin system      │
	│ - Indexes (type,   │         │ - Type support       │
	│   capability)      │         │                      │
	└────────────────────┘         └──────────────────────┘
	          │                                 │
	          └───────────────┬───────────────┘
	                          │
	          ┌───────────────┴───────────────┐
	          │                                 │
	┌─────────▼──────────┐         ┌───────────▼──────────┐
	│  State Machine     │         │  Processing Pipeline  │
	│                    │         │                      │
	│ - State tracking   │         │ - Data processors    │
	│ - Transitions      │         │ - Pipeline execution │
	│ - Type-specific    │         │ - Priority handling  │
	│   rules            │         │                      │
	└────────────────────┘         └──────────────────────┘
	          │                                 │
	          └───────────────┬───────────────┘
	                          │
	                  ┌───────▼───────┐
	                  │  Lifecycle    │
	                  │     Hooks     │
	                  │               │
	                  │ - Discovery   │
	                  │ - Registration│
	                  │ - Teardown    │
	                  │ - Filtering   │
	                  │ - Priority    │
	                  └───────────────┘

Package Structure:

  - iot/: Top-level package with IoTService interface and provider
  - types/: Shared contracts and types (Device, DevicePlugin, DeviceStateMachine, etc.)
  - plugin-registry/: Device plugin registry implementation
  - device-registry/: Device registry implementation with persistence
  - state-machine/: Device state machine implementation with type-specific transitions
  - processing/: Data processing pipeline implementation
  - hooks/: Lifecycle hooks implementation
  - cctv/: CCTV implementation (example DevicePlugin)

The service coordinates the lifecycle of all components:
  - Plugin registry manages device type plugins (CCTV, sensors, etc.)
  - Device registry manages registered devices, their lifecycle, and persistence
  - State machine tracks device state transitions with type-specific rules
  - Processing pipeline processes device data through registered processors
  - Lifecycle hooks allow custom logic injection at key lifecycle events

# Device-Agnostic Design

The service is designed to be device-agnostic:

  - Device types: Cameras (CCTV), sensors, audio devices, and other IoT devices
  - Device plugins: CCTV (current), sensors (future), audio (future)
  - Capability-based operations: Devices expose capabilities, operations check capabilities

This allows the system to:
  - Work with any device type through the DevicePlugin system
  - Add new device types by implementing DevicePlugin
  - Support device-specific operations while maintaining a unified interface
  - Enable device-agnostic orchestration and management

# Device Plugin System

Devices are discovered and managed through the DevicePlugin system:

  - DevicePlugin: Interface for device type-specific logic
    - GetDeviceType(): Returns the device type this plugin supports
    - GetSupportedCapabilities(): Returns capabilities supported by this device type
    - DiscoverDevices(): Discover devices of this type
    - CreateDevice(): Create a Device instance from metadata
    - ValidateMetadata(): Validate device metadata
  - DevicePluginRegistry: Manages registered plugins and coordinates discovery
  - Device: Generic interface for all devices with capability-based operations

Example plugin implementations:
  - CCTVDevicePlugin: Discovers and manages cameras via CCTVService
  - SensorDevicePlugin: (Future) Discovers and manages sensors
  - AudioDevicePlugin: (Future) Discovers and manages audio devices

# Configuration

The service uses a device-agnostic configuration structure:

	discovery:
	  auto_discover: true
	  discovery_interval: 30s
	  discovery_timeout: 10s
	  parallel_discovery: true

	processing:
	  enabled: true
	  processor_timeout: 5s

	state_machine:
	  enabled: true

	hooks:
	  enabled: true

Configuration is validated on service creation and defaults are provided if not specified.

# State Management

Devices have state machines that track operational state:

	undiscovered → discovered → registered → active → idle → disabled → error

State transitions are managed by DeviceStateMachine:
  - Transitions are validated against device type-specific rules
  - State changes can trigger lifecycle hooks
  - State is tracked per device and can be queried by type
  - State machines are created automatically when devices are registered

# Data Processing

Device data flows through a processing pipeline:

	Device → DataProcessor → DataProcessor → ... → Result

Processors are registered by data type and priority:
  - VideoFrameProcessor: Processes video frames
  - SensorDataProcessor: Processes sensor readings
  - AudioDataProcessor: (Future) Processes audio data
  - EventProcessor: Processes device events

Processors can filter, transform, or analyze data. The pipeline executes processors
in priority order and can short-circuit on errors.

# Lifecycle Hooks

Lifecycle hooks allow custom logic injection at key lifecycle events:

  - Discovery hooks: Executed when devices are discovered
  - Registration hooks: Executed when devices are registered
  - Teardown hooks: Executed when devices are removed

Hooks support:
  - Filtering: Hooks can filter devices during discovery
  - Priority: Hooks execute in priority order
  - Conditional execution: Hooks can be enabled/disabled
  - Error handling: Hook failures are logged but don't block operations

Hooks enable:
  - Custom validation
  - External system integration
  - Metrics collection
  - Logging and auditing

# Lifecycle Management

The service manages its own lifecycle and coordinates subcomponents:

  - Start(): Verifies component readiness and marks service as started
  - Stop(): Performs graceful shutdown of all subcomponents in reverse order
  - HealthSnapshot(): Provides observability into service and subcomponent status

The service follows the vm-gateway pattern:
  - Locking strategy: Copy references under lock, call outside lock
  - Context handling: Contexts flow from callers, not stored in structs
  - Error handling: Sentinel errors for programmatic error handling
  - Observability: Structured logging and health snapshots

# Observability

The service provides observability through:

  - HealthSnapshot(): Returns service status including:
    - Registered device count
    - Plugin status (registered plugins and supported types)
    - State registry size
    - Sub-service status (started/stopped)
  - Structured logging: All operations are logged with context
  - Error tracking: Errors are logged with full context

# Usage

The service is typically created using dependency injection (Fx):

	service, err := IoTServiceProvider(
	    lc,     // fx.Lifecycle
	    config, // *types.IoTServiceConfig (optional, defaults provided)
	    logger, // *zap.Logger
	)

The provider creates all subcomponents internally and manages their lifecycle.
The service is self-contained and creates its dependencies in the correct order.

Basic usage:

	ctx := context.Background()
	
	// Start service
	err := service.Start(ctx)
	if err != nil {
	    log.Fatal(err)
	}
	defer service.Stop(ctx)
	
	// Discover devices
	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
	    log.Fatal(err)
	}
	
	// Register a device
	if len(devices) > 0 {
	    err = service.RegisterDevice(ctx, devices[0])
	    if err != nil {
	        log.Fatal(err)
	    }
	}
	
	// Get health snapshot
	status := service.HealthSnapshot()
	fmt.Printf("Registered devices: %d\n", status.RegisteredDevices)

# Integration Points

The IoT service integrates with:

  - orchestrator: Provides IoTService and CCTVDevicePlugin via Fx
  - state-mng: Uses DeviceStateService for state management integration
  - cctv: CCTVDevicePlugin adapts CCTVService to DevicePlugin interface
  - meta-storage: Device registry uses meta-storage for device persistence

# Recent Refactoring

This package has been refactored to follow vm-gateway patterns:

  - Types extraction: All types moved to iot/types package
  - Service façade: IoTService as single entry point
  - Subpackage extraction: Implementations moved to dedicated packages
    - plugin-registry/: Plugin registry implementation
    - device-registry/: Device registry with persistence
    - state-machine/: State machine with type-specific transitions
    - processing/: Data processing pipeline
    - hooks/: Lifecycle hooks
  - Device-agnostic design: Top-level API works with any device type
  - CCTV integration: CCTV becomes a DevicePlugin implementation
  - Error handling: Sentinel errors and consistent error wrapping
  - Context handling: Contexts flow from callers, not stored in structs
  - Observability: HealthSnapshot() method for debugging
  - Locking strategy: Copy references under lock, call outside lock

# Examples

See the Example* functions in iot_examples_test.go for comprehensive usage examples:
  - ExampleIoTService_Start: Starting the service
  - ExampleIoTService_DiscoverDevices: Discovering devices
  - ExampleIoTService_RegisterDevice: Registering devices
  - ExampleIoTService_ProcessDeviceData: Processing device data
  - ExampleIoTService_HealthSnapshot: Getting health status
*/
package iot

