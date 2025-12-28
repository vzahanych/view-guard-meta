package hooks

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// lifecycleHookRegistryImpl is the default implementation of LifecycleHookRegistry
type lifecycleHookRegistryImpl struct {
	// hooks maps hook ID to hook
	hooks map[string]*types.LifecycleHook

	// hooksByType maps hook type to list of hooks (for efficient execution)
	hooksByType map[types.LifecycleHookType][]*types.LifecycleHook

	// mu protects the hooks maps
	mu sync.RWMutex

	// logger provides structured logging
	logger *zap.Logger
}

// NewLifecycleHookRegistry creates a new lifecycle hook registry
// If logger is nil, a no-op logger will be used.
func NewLifecycleHookRegistry(logger *zap.Logger) types.LifecycleHookRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &lifecycleHookRegistryImpl{
		hooks:       make(map[string]*types.LifecycleHook),
		hooksByType: make(map[types.LifecycleHookType][]*types.LifecycleHook),
		logger:      logger,
	}
}

// RegisterHook registers a lifecycle hook
func (r *lifecycleHookRegistryImpl) RegisterHook(hook *types.LifecycleHook) error {
	if hook == nil {
		r.logger.Warn("Attempted to register nil hook")
		return fmt.Errorf("%w: hook cannot be nil", types.ErrInvalidDevice)
	}

	if hook.ID == "" {
		r.logger.Warn("Attempted to register hook with empty ID")
		return fmt.Errorf("%w: hook ID cannot be empty", types.ErrInvalidDevice)
	}

	if hook.Name == "" {
		r.logger.Warn("Attempted to register hook with empty name",
			zap.String("hook_id", hook.ID),
		)
		return fmt.Errorf("%w: hook name cannot be empty", types.ErrInvalidDevice)
	}

	// Validate hook type and function
	switch hook.Type {
	case types.HookTypeDiscovery:
		if hook.DiscoveryHook == nil {
			r.logger.Warn("Attempted to register discovery hook with nil function",
				zap.String("hook_id", hook.ID),
			)
			return fmt.Errorf("%w: discovery hook function cannot be nil", types.ErrInvalidDevice)
		}
	case types.HookTypeRegistration:
		if hook.RegistrationHook == nil {
			r.logger.Warn("Attempted to register registration hook with nil function",
				zap.String("hook_id", hook.ID),
			)
			return fmt.Errorf("%w: registration hook function cannot be nil", types.ErrInvalidDevice)
		}
	case types.HookTypeDataCollection:
		if hook.DataCollectionHook == nil {
			r.logger.Warn("Attempted to register data collection hook with nil function",
				zap.String("hook_id", hook.ID),
			)
			return fmt.Errorf("%w: data collection hook function cannot be nil", types.ErrInvalidDevice)
		}
	case types.HookTypeTeardown:
		if hook.TeardownHook == nil {
			r.logger.Warn("Attempted to register teardown hook with nil function",
				zap.String("hook_id", hook.ID),
			)
			return fmt.Errorf("%w: teardown hook function cannot be nil", types.ErrInvalidDevice)
		}
	default:
		r.logger.Warn("Attempted to register hook with unknown type",
			zap.String("hook_id", hook.ID),
			zap.String("hook_type", string(hook.Type)),
		)
		return fmt.Errorf("%w: unknown hook type: %s", types.ErrInvalidDevice, hook.Type)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if hook is already registered
	if _, exists := r.hooks[hook.ID]; exists {
		r.logger.Warn("Hook already registered",
			zap.String("hook_id", hook.ID),
		)
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

	r.logger.Info("Hook registered",
		zap.String("hook_id", hook.ID),
		zap.String("hook_name", hook.Name),
		zap.String("hook_type", string(hook.Type)),
		zap.Int("priority", hook.Priority),
	)

	return nil
}

// UnregisterHook unregisters a lifecycle hook by ID
func (r *lifecycleHookRegistryImpl) UnregisterHook(hookID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	hook, exists := r.hooks[hookID]
	if !exists {
		r.logger.Warn("Hook not found for unregistration",
			zap.String("hook_id", hookID),
		)
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

	r.logger.Info("Hook unregistered",
		zap.String("hook_id", hookID),
	)

	_ = hook // Suppress unused variable warning
	return nil
}

// GetHook retrieves a hook by ID
func (r *lifecycleHookRegistryImpl) GetHook(hookID string) (*types.LifecycleHook, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hook, exists := r.hooks[hookID]
	if !exists {
		r.logger.Debug("Hook not found",
			zap.String("hook_id", hookID),
		)
		return nil, fmt.Errorf("hook with ID %s is not registered", hookID)
	}

	return hook, nil
}

// ListHooks returns all registered hooks, optionally filtered by type
func (r *lifecycleHookRegistryImpl) ListHooks(hookType *types.LifecycleHookType) []*types.LifecycleHook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if hookType != nil {
		hooks := r.hooksByType[*hookType]
		result := make([]*types.LifecycleHook, len(hooks))
		copy(result, hooks)
		return result
	}

	// Return all hooks
	result := make([]*types.LifecycleHook, 0, len(r.hooks))
	for _, hook := range r.hooks {
		result = append(result, hook)
	}
	return result
}

// ExecuteDiscoveryHooks executes all discovery hooks
func (r *lifecycleHookRegistryImpl) ExecuteDiscoveryHooks(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
	return r.executeHooks(ctx, types.HookTypeDiscovery, func(hook *types.LifecycleHook) error {
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
func (r *lifecycleHookRegistryImpl) ExecuteRegistrationHooks(ctx context.Context, hookCtx *types.RegistrationHookContext) error {
	return r.executeHooks(ctx, types.HookTypeRegistration, func(hook *types.LifecycleHook) error {
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
func (r *lifecycleHookRegistryImpl) ExecuteDataCollectionHooks(ctx context.Context, hookCtx *types.DataCollectionHookContext) error {
	deviceMetadata := hookCtx.Device.GetMetadata()
	return r.executeHooks(ctx, types.HookTypeDataCollection, func(hook *types.LifecycleHook) error {
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
func (r *lifecycleHookRegistryImpl) ExecuteTeardownHooks(ctx context.Context, hookCtx *types.TeardownHookContext) error {
	deviceMetadata := hookCtx.Device.GetMetadata()
	return r.executeHooks(ctx, types.HookTypeTeardown, func(hook *types.LifecycleHook) error {
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
// Locking strategy: Copy hooks under lock, execute outside lock
func (r *lifecycleHookRegistryImpl) executeHooks(ctx context.Context, hookType types.LifecycleHookType, execute func(*types.LifecycleHook) error) error {
	r.mu.RLock()
	hooks := make([]*types.LifecycleHook, len(r.hooksByType[hookType]))
	copy(hooks, r.hooksByType[hookType])
	r.mu.RUnlock()

	// Execute hooks in priority order (already sorted)
	var firstError error
	for _, hook := range hooks {
		if err := execute(hook); err != nil {
			if firstError == nil {
				firstError = err
			}
			r.logger.Warn("Hook execution failed",
				zap.String("hook_id", hook.ID),
				zap.String("hook_name", hook.Name),
				zap.String("hook_type", string(hookType)),
				zap.Error(err),
			)
			// Continue executing other hooks even if one fails
			// This allows multiple hooks to run and collect all errors
		}
	}

	if firstError != nil {
		r.logger.Error("Hook execution completed with errors",
			zap.String("hook_type", string(hookType)),
			zap.Error(firstError),
		)
	}

	return firstError
}

// hookMatchesFilters checks if a hook matches the device type and capability filters
func (r *lifecycleHookRegistryImpl) hookMatchesFilters(hook *types.LifecycleHook, deviceType types.DeviceType, capabilities *types.DeviceCapabilities) bool {
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
func (r *lifecycleHookRegistryImpl) sortHooksByPriority(hookType types.LifecycleHookType) {
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

