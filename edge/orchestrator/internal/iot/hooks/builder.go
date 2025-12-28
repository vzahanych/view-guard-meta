package hooks

import (
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// HookBuilder provides a fluent interface for building lifecycle hooks
type HookBuilder struct {
	hook   *types.LifecycleHook
	logger *zap.Logger
}

// NewHookBuilder creates a new hook builder
// If logger is nil, a no-op logger will be used.
func NewHookBuilder(id, name string, hookType types.LifecycleHookType, logger *zap.Logger) *HookBuilder {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &HookBuilder{
		hook: &types.LifecycleHook{
			ID:       id,
			Name:     name,
			Type:     hookType,
			Enabled:  true,
			Priority: 100,
		},
		logger: logger,
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
func (b *HookBuilder) WithDeviceTypeFilter(deviceType types.DeviceType) *HookBuilder {
	b.hook.DeviceTypeFilter = &deviceType
	return b
}

// WithCapabilityFilter sets the capability filter
func (b *HookBuilder) WithCapabilityFilter(capability types.DeviceCapability) *HookBuilder {
	b.hook.CapabilityFilter = &capability
	return b
}

// WithDiscoveryHook sets the discovery hook function
func (b *HookBuilder) WithDiscoveryHook(hook types.DiscoveryHook) *HookBuilder {
	b.hook.DiscoveryHook = hook
	return b
}

// WithRegistrationHook sets the registration hook function
func (b *HookBuilder) WithRegistrationHook(hook types.RegistrationHook) *HookBuilder {
	b.hook.RegistrationHook = hook
	return b
}

// WithDataCollectionHook sets the data collection hook function
func (b *HookBuilder) WithDataCollectionHook(hook types.DataCollectionHook) *HookBuilder {
	b.hook.DataCollectionHook = hook
	return b
}

// WithTeardownHook sets the teardown hook function
func (b *HookBuilder) WithTeardownHook(hook types.TeardownHook) *HookBuilder {
	b.hook.TeardownHook = hook
	return b
}

// WithEnabled sets whether the hook is enabled
func (b *HookBuilder) WithEnabled(enabled bool) *HookBuilder {
	b.hook.Enabled = enabled
	return b
}

// Build builds the hook
func (b *HookBuilder) Build() *types.LifecycleHook {
	b.logger.Debug("Hook built",
		zap.String("hook_id", b.hook.ID),
		zap.String("hook_name", b.hook.Name),
		zap.String("hook_type", string(b.hook.Type)),
	)
	return b.hook
}

