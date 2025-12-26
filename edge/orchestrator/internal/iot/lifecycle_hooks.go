package iot

import (
	"context"
	"fmt"
	"sync"
)

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
//go:generate go run go.uber.org/mock/mockgen -destination=mocks/mock_lifecycle_hook_registry.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot LifecycleHookRegistry
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

// lifecycleHookRegistryImpl is the default implementation of LifecycleHookRegistry
type lifecycleHookRegistryImpl struct {
	// hooks maps hook ID to hook
	hooks map[string]*LifecycleHook

	// hooksByType maps hook type to list of hooks (for efficient execution)
	hooksByType map[LifecycleHookType][]*LifecycleHook

	// mu protects the hooks maps
	mu sync.RWMutex
}

// NewLifecycleHookRegistry creates a new lifecycle hook registry
func NewLifecycleHookRegistry() LifecycleHookRegistry {
	return &lifecycleHookRegistryImpl{
		hooks:       make(map[string]*LifecycleHook),
		hooksByType: make(map[LifecycleHookType][]*LifecycleHook),
	}
}

// RegisterHook registers a lifecycle hook
func (r *lifecycleHookRegistryImpl) RegisterHook(hook *LifecycleHook) error {
	if hook == nil {
		return fmt.Errorf("hook cannot be nil")
	}

	if hook.ID == "" {
		return fmt.Errorf("hook ID cannot be empty")
	}

	if hook.Name == "" {
		return fmt.Errorf("hook name cannot be empty")
	}

	// Validate hook type and function
	switch hook.Type {
	case HookTypeDiscovery:
		if hook.DiscoveryHook == nil {
			return fmt.Errorf("discovery hook function cannot be nil")
		}
	case HookTypeRegistration:
		if hook.RegistrationHook == nil {
			return fmt.Errorf("registration hook function cannot be nil")
		}
	case HookTypeDataCollection:
		if hook.DataCollectionHook == nil {
			return fmt.Errorf("data collection hook function cannot be nil")
		}
	case HookTypeTeardown:
		if hook.TeardownHook == nil {
			return fmt.Errorf("teardown hook function cannot be nil")
		}
	default:
		return fmt.Errorf("unknown hook type: %s", hook.Type)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if hook is already registered
	if _, exists := r.hooks[hook.ID]; exists {
		return fmt.Errorf("hook with ID %s is already registered", hook.ID)
	}

	// Set default priority if not set
	if hook.Priority == 0 {
		hook.Priority = 100 // Default priority
	}

	// Set enabled to true by default
	if !hook.Enabled {
		hook.Enabled = true
	}

	// Register hook
	r.hooks[hook.ID] = hook

	// Add to type index
	r.hooksByType[hook.Type] = append(r.hooksByType[hook.Type], hook)

	// Sort hooks by priority (insertion sort for simplicity)
	r.sortHooksByPriority(hook.Type)

	return nil
}

// UnregisterHook unregisters a lifecycle hook by ID
func (r *lifecycleHookRegistryImpl) UnregisterHook(hookID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	hook, exists := r.hooks[hookID]
	if !exists {
		return fmt.Errorf("hook with ID %s is not registered", hookID)
	}

	// Remove from hooks map
	delete(r.hooks, hookID)

	// Remove from type index
	hooks := r.hooksByType[hook.Type]
	for i, h := range hooks {
		if h.ID == hookID {
			r.hooksByType[hook.Type] = append(hooks[:i], hooks[i+1:]...)
			break
		}
	}

	return nil
}

// GetHook retrieves a hook by ID
func (r *lifecycleHookRegistryImpl) GetHook(hookID string) (*LifecycleHook, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hook, exists := r.hooks[hookID]
	if !exists {
		return nil, fmt.Errorf("hook with ID %s is not registered", hookID)
	}

	return hook, nil
}

// ListHooks returns all registered hooks, optionally filtered by type
func (r *lifecycleHookRegistryImpl) ListHooks(hookType *LifecycleHookType) []*LifecycleHook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if hookType != nil {
		hooks := r.hooksByType[*hookType]
		result := make([]*LifecycleHook, len(hooks))
		copy(result, hooks)
		return result
	}

	// Return all hooks
	result := make([]*LifecycleHook, 0, len(r.hooks))
	for _, hook := range r.hooks {
		result = append(result, hook)
	}
	return result
}

// ExecuteDiscoveryHooks executes all discovery hooks
func (r *lifecycleHookRegistryImpl) ExecuteDiscoveryHooks(ctx context.Context, hookCtx *DiscoveryHookContext) error {
	return r.executeHooks(ctx, HookTypeDiscovery, func(hook *LifecycleHook) error {
		// Check filters
		if !r.hookMatchesFilters(hook, hookCtx.DeviceType, nil) {
			return nil // Skip this hook
		}

		if !hook.Enabled {
			return nil // Skip disabled hooks
		}

		return hook.DiscoveryHook(ctx, hookCtx)
	})
}

// ExecuteRegistrationHooks executes all registration hooks
func (r *lifecycleHookRegistryImpl) ExecuteRegistrationHooks(ctx context.Context, hookCtx *RegistrationHookContext) error {
	return r.executeHooks(ctx, HookTypeRegistration, func(hook *LifecycleHook) error {
		// Check filters
		if !r.hookMatchesFilters(hook, hookCtx.Metadata.Type, &hookCtx.Capabilities) {
			return nil // Skip this hook
		}

		if !hook.Enabled {
			return nil // Skip disabled hooks
		}

		return hook.RegistrationHook(ctx, hookCtx)
	})
}

// ExecuteDataCollectionHooks executes all data collection hooks
func (r *lifecycleHookRegistryImpl) ExecuteDataCollectionHooks(ctx context.Context, hookCtx *DataCollectionHookContext) error {
	deviceMetadata := hookCtx.Device.GetMetadata()
	return r.executeHooks(ctx, HookTypeDataCollection, func(hook *LifecycleHook) error {
		// Check filters
		if !r.hookMatchesFilters(hook, deviceMetadata.Type, &deviceMetadata.Capabilities) {
			return nil // Skip this hook
		}

		if !hook.Enabled {
			return nil // Skip disabled hooks
		}

		return hook.DataCollectionHook(ctx, hookCtx)
	})
}

// ExecuteTeardownHooks executes all teardown hooks
func (r *lifecycleHookRegistryImpl) ExecuteTeardownHooks(ctx context.Context, hookCtx *TeardownHookContext) error {
	deviceMetadata := hookCtx.Device.GetMetadata()
	return r.executeHooks(ctx, HookTypeTeardown, func(hook *LifecycleHook) error {
		// Check filters
		if !r.hookMatchesFilters(hook, deviceMetadata.Type, &deviceMetadata.Capabilities) {
			return nil // Skip this hook
		}

		if !hook.Enabled {
			return nil // Skip disabled hooks
		}

		return hook.TeardownHook(ctx, hookCtx)
	})
}

// executeHooks executes hooks of a specific type
func (r *lifecycleHookRegistryImpl) executeHooks(ctx context.Context, hookType LifecycleHookType, execute func(*LifecycleHook) error) error {
	r.mu.RLock()
	hooks := make([]*LifecycleHook, len(r.hooksByType[hookType]))
	copy(hooks, r.hooksByType[hookType])
	r.mu.RUnlock()

	// Execute hooks in priority order (already sorted)
	var firstError error
	for _, hook := range hooks {
		if err := execute(hook); err != nil {
			if firstError == nil {
				firstError = err
			}
			// Continue executing other hooks even if one fails
			// This allows multiple hooks to run and collect all errors
		}
	}

	return firstError
}

// hookMatchesFilters checks if a hook matches the device type and capability filters
func (r *lifecycleHookRegistryImpl) hookMatchesFilters(hook *LifecycleHook, deviceType DeviceType, capabilities *DeviceCapabilities) bool {
	// Check device type filter
	if hook.DeviceTypeFilter != nil && *hook.DeviceTypeFilter != deviceType {
		return false
	}

	// Check capability filter
	if hook.CapabilityFilter != nil && capabilities != nil {
		if !capabilities.Has(*hook.CapabilityFilter) {
			return false
		}
	}

	return true
}

// sortHooksByPriority sorts hooks by priority (lower priority = earlier execution)
func (r *lifecycleHookRegistryImpl) sortHooksByPriority(hookType LifecycleHookType) {
	hooks := r.hooksByType[hookType]
	for i := 1; i < len(hooks); i++ {
		key := hooks[i]
		j := i - 1
		for j >= 0 && hooks[j].Priority > key.Priority {
			hooks[j+1] = hooks[j]
			j--
		}
		hooks[j+1] = key
	}
}

// LifecycleHookManager provides high-level management of lifecycle hooks
type LifecycleHookManager struct {
	registry LifecycleHookRegistry
}

// NewLifecycleHookManager creates a new lifecycle hook manager
func NewLifecycleHookManager(registry LifecycleHookRegistry) *LifecycleHookManager {
	return &LifecycleHookManager{
		registry: registry,
	}
}

// RegisterHook registers a hook
func (m *LifecycleHookManager) RegisterHook(hook *LifecycleHook) error {
	return m.registry.RegisterHook(hook)
}

// UnregisterHook unregisters a hook
func (m *LifecycleHookManager) UnregisterHook(hookID string) error {
	return m.registry.UnregisterHook(hookID)
}

// GetHook retrieves a hook
func (m *LifecycleHookManager) GetHook(hookID string) (*LifecycleHook, error) {
	return m.registry.GetHook(hookID)
}

// ListHooks lists hooks
func (m *LifecycleHookManager) ListHooks(hookType *LifecycleHookType) []*LifecycleHook {
	return m.registry.ListHooks(hookType)
}

// ExecuteDiscoveryHooks executes discovery hooks
func (m *LifecycleHookManager) ExecuteDiscoveryHooks(ctx context.Context, hookCtx *DiscoveryHookContext) error {
	return m.registry.ExecuteDiscoveryHooks(ctx, hookCtx)
}

// ExecuteRegistrationHooks executes registration hooks
func (m *LifecycleHookManager) ExecuteRegistrationHooks(ctx context.Context, hookCtx *RegistrationHookContext) error {
	return m.registry.ExecuteRegistrationHooks(ctx, hookCtx)
}

// ExecuteDataCollectionHooks executes data collection hooks
func (m *LifecycleHookManager) ExecuteDataCollectionHooks(ctx context.Context, hookCtx *DataCollectionHookContext) error {
	return m.registry.ExecuteDataCollectionHooks(ctx, hookCtx)
}

// ExecuteTeardownHooks executes teardown hooks
func (m *LifecycleHookManager) ExecuteTeardownHooks(ctx context.Context, hookCtx *TeardownHookContext) error {
	return m.registry.ExecuteTeardownHooks(ctx, hookCtx)
}

// HookBuilder provides a fluent interface for building lifecycle hooks
type HookBuilder struct {
	hook *LifecycleHook
}

// NewHookBuilder creates a new hook builder
func NewHookBuilder(id, name string, hookType LifecycleHookType) *HookBuilder {
	return &HookBuilder{
		hook: &LifecycleHook{
			ID:      id,
			Name:    name,
			Type:    hookType,
			Enabled: true,
			Priority: 100,
		},
	}
}

// WithDescription sets the hook description
func (b *HookBuilder) WithDescription(description string) *HookBuilder {
	b.hook.Description = description
	return b
}

// WithPriority sets the hook priority
func (b *HookBuilder) WithPriority(priority int) *HookBuilder {
	b.hook.Priority = priority
	return b
}

// WithDeviceTypeFilter sets the device type filter
func (b *HookBuilder) WithDeviceTypeFilter(deviceType DeviceType) *HookBuilder {
	b.hook.DeviceTypeFilter = &deviceType
	return b
}

// WithCapabilityFilter sets the capability filter
func (b *HookBuilder) WithCapabilityFilter(capability DeviceCapability) *HookBuilder {
	b.hook.CapabilityFilter = &capability
	return b
}

// WithDiscoveryHook sets the discovery hook function
func (b *HookBuilder) WithDiscoveryHook(hook DiscoveryHook) *HookBuilder {
	b.hook.DiscoveryHook = hook
	return b
}

// WithRegistrationHook sets the registration hook function
func (b *HookBuilder) WithRegistrationHook(hook RegistrationHook) *HookBuilder {
	b.hook.RegistrationHook = hook
	return b
}

// WithDataCollectionHook sets the data collection hook function
func (b *HookBuilder) WithDataCollectionHook(hook DataCollectionHook) *HookBuilder {
	b.hook.DataCollectionHook = hook
	return b
}

// WithTeardownHook sets the teardown hook function
func (b *HookBuilder) WithTeardownHook(hook TeardownHook) *HookBuilder {
	b.hook.TeardownHook = hook
	return b
}

// WithEnabled sets whether the hook is enabled
func (b *HookBuilder) WithEnabled(enabled bool) *HookBuilder {
	b.hook.Enabled = enabled
	return b
}

// Build builds the hook
func (b *HookBuilder) Build() *LifecycleHook {
	return b.hook
}

