package types

import "context"

// LifecycleHookType represents the type of lifecycle hook
type LifecycleHookType string

const (
	// HookTypeDiscovery represents device discovery hooks
	HookTypeDiscovery LifecycleHookType = "discovery"

	// HookTypeRegistration represents device registration hooks
	HookTypeRegistration LifecycleHookType = "registration"

	// HookTypeDataCollection represents data collection hooks
	HookTypeDataCollection LifecycleHookType = "data_collection"

	// HookTypeTeardown represents device teardown hooks
	HookTypeTeardown LifecycleHookType = "teardown"
)

// DiscoveryHookContext provides context for discovery hooks
type DiscoveryHookContext struct {
	// DeviceType is the type of device being discovered
	DeviceType DeviceType `json:"device_type"`

	// Plugin is the plugin performing discovery
	Plugin DevicePlugin `json:"plugin,omitempty"`

	// DiscoveredDevices are devices that were discovered
	DiscoveredDevices []Device `json:"discovered_devices"`

	// Metadata contains additional discovery metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DiscoveryHook is called during device discovery
// Hooks can:
//   - Filter discovered devices
//   - Add additional metadata
//   - Perform device identification/validation
//   - Log discovery events
type DiscoveryHook func(ctx context.Context, hookCtx *DiscoveryHookContext) error

// RegistrationHookContext provides context for registration hooks
type RegistrationHookContext struct {
	// Device is the device being registered
	Device Device `json:"device"`

	// Metadata is the device metadata
	Metadata DeviceMetadata `json:"metadata"`

	// Registry is the device registry (if available)
	Registry DeviceRegistry `json:"registry,omitempty"`

	// Capabilities are the device capabilities
	Capabilities DeviceCapabilities `json:"capabilities"`

	// Metadata contains additional registration metadata
	AdditionalMetadata map[string]interface{} `json:"additional_metadata,omitempty"`
}

// RegistrationHook is called during device registration
// Hooks can:
//   - Validate device before registration
//   - Initialize device-specific resources
//   - Report capabilities to external systems
//   - Set up device monitoring
//   - Configure device settings
type RegistrationHook func(ctx context.Context, hookCtx *RegistrationHookContext) error

// DataCollectionHookContext provides context for data collection hooks
type DataCollectionHookContext struct {
	// Device is the device collecting data
	Device Device `json:"device"`

	// DataType is the type of data being collected
	DataType DeviceDataType `json:"data_type"`

	// Data is the collected data (may be nil for pre-capture hooks)
	Data *DeviceData `json:"data,omitempty"`

	// Operation is the data collection operation (capture, stream_start, stream_stop, poll)
	Operation string `json:"operation"`

	// Metadata contains additional operation metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DataCollectionHook is called during data collection operations
// Hooks can:
//   - Pre-process data before capture
//   - Post-process data after capture
//   - Monitor data collection operations
//   - Route data to external systems
//   - Validate data quality
//   - Handle data collection errors
type DataCollectionHook func(ctx context.Context, hookCtx *DataCollectionHookContext) error

// TeardownHookContext provides context for teardown hooks
type TeardownHookContext struct {
	// Device is the device being torn down
	Device Device `json:"device"`

	// Reason is the reason for teardown (shutdown, error, disconnect, etc.)
	Reason string `json:"reason"`

	// Metadata contains additional teardown metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TeardownHook is called during device teardown
// Hooks can:
//   - Clean up device-specific resources
//   - Release device connections
//   - Save device state
//   - Notify external systems
//   - Log teardown events
type TeardownHook func(ctx context.Context, hookCtx *TeardownHookContext) error

// LifecycleHook represents a registered lifecycle hook
type LifecycleHook struct {
	// ID is a unique identifier for the hook
	ID string `json:"id"`

	// Type is the type of hook
	Type LifecycleHookType `json:"type"`

	// Name is a human-readable name for the hook
	Name string `json:"name"`

	// Description describes what the hook does
	Description string `json:"description,omitempty"`

	// Priority determines hook execution order (lower = earlier)
	Priority int `json:"priority"`

	// DiscoveryHook is the discovery hook function (if Type is HookTypeDiscovery)
	DiscoveryHook DiscoveryHook `json:"-"`

	// RegistrationHook is the registration hook function (if Type is HookTypeRegistration)
	RegistrationHook RegistrationHook `json:"-"`

	// DataCollectionHook is the data collection hook function (if Type is HookTypeDataCollection)
	DataCollectionHook DataCollectionHook `json:"-"`

	// TeardownHook is the teardown hook function (if Type is HookTypeTeardown)
	TeardownHook TeardownHook `json:"-"`

	// Enabled indicates if the hook is enabled
	Enabled bool `json:"enabled"`

	// DeviceTypeFilter filters hooks by device type (nil = all types)
	DeviceTypeFilter *DeviceType `json:"device_type_filter,omitempty"`

	// CapabilityFilter filters hooks by device capability (nil = all capabilities)
	CapabilityFilter *DeviceCapability `json:"capability_filter,omitempty"`
}

// LifecycleHookRegistry is an interface for managing device lifecycle hooks
//
//go:generate go run go.uber.org/mock/mockgen -destination=../mocks/mock_lifecycle_hook_registry.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types LifecycleHookRegistry
type LifecycleHookRegistry interface {
	// RegisterHook registers a lifecycle hook
	RegisterHook(hook *LifecycleHook) error

	// UnregisterHook unregisters a lifecycle hook by ID
	UnregisterHook(hookID string) error

	// GetHook retrieves a hook by ID
	GetHook(hookID string) (*LifecycleHook, error)

	// ListHooks returns all registered hooks, optionally filtered by type
	ListHooks(hookType *LifecycleHookType) []*LifecycleHook

	// ExecuteDiscoveryHooks executes all discovery hooks
	ExecuteDiscoveryHooks(ctx context.Context, hookCtx *DiscoveryHookContext) error

	// ExecuteRegistrationHooks executes all registration hooks
	ExecuteRegistrationHooks(ctx context.Context, hookCtx *RegistrationHookContext) error

	// ExecuteDataCollectionHooks executes all data collection hooks
	ExecuteDataCollectionHooks(ctx context.Context, hookCtx *DataCollectionHookContext) error

	// ExecuteTeardownHooks executes all teardown hooks
	ExecuteTeardownHooks(ctx context.Context, hookCtx *TeardownHookContext) error
}
