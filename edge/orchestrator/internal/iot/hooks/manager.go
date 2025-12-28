package hooks

import (
	"context"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// LifecycleHookManager provides high-level management of lifecycle hooks
type LifecycleHookManager struct {
	registry types.LifecycleHookRegistry
	logger   *zap.Logger
}

// NewLifecycleHookManager creates a new lifecycle hook manager
// If logger is nil, a no-op logger will be used.
func NewLifecycleHookManager(registry types.LifecycleHookRegistry, logger *zap.Logger) *LifecycleHookManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LifecycleHookManager{
		registry: registry,
		logger:   logger,
	}
}

// RegisterHook registers a hook
func (m *LifecycleHookManager) RegisterHook(hook *types.LifecycleHook) error {
	err := m.registry.RegisterHook(hook)
	if err != nil {
		m.logger.Error("Failed to register hook via manager",
			zap.String("hook_id", hook.ID),
			zap.Error(err),
		)
		return err
	}

	m.logger.Info("Hook registered via manager",
		zap.String("hook_id", hook.ID),
		zap.String("hook_name", hook.Name),
	)
	return nil
}

// UnregisterHook unregisters a hook
func (m *LifecycleHookManager) UnregisterHook(hookID string) error {
	err := m.registry.UnregisterHook(hookID)
	if err != nil {
		m.logger.Error("Failed to unregister hook via manager",
			zap.String("hook_id", hookID),
			zap.Error(err),
		)
		return err
	}

	m.logger.Info("Hook unregistered via manager",
		zap.String("hook_id", hookID),
	)
	return nil
}

// GetHook retrieves a hook
func (m *LifecycleHookManager) GetHook(hookID string) (*types.LifecycleHook, error) {
	hook, err := m.registry.GetHook(hookID)
	if err != nil {
		m.logger.Debug("Failed to get hook via manager",
			zap.String("hook_id", hookID),
			zap.Error(err),
		)
		return nil, err
	}

	return hook, nil
}

// ListHooks lists hooks
func (m *LifecycleHookManager) ListHooks(hookType *types.LifecycleHookType) []*types.LifecycleHook {
	hooks := m.registry.ListHooks(hookType)
	m.logger.Debug("Listed hooks via manager",
		zap.Bool("filtered_by_type", hookType != nil),
		zap.Int("hook_count", len(hooks)),
	)
	return hooks
}

// ExecuteDiscoveryHooks executes discovery hooks
func (m *LifecycleHookManager) ExecuteDiscoveryHooks(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
	return m.registry.ExecuteDiscoveryHooks(ctx, hookCtx)
}

// ExecuteRegistrationHooks executes registration hooks
func (m *LifecycleHookManager) ExecuteRegistrationHooks(ctx context.Context, hookCtx *types.RegistrationHookContext) error {
	return m.registry.ExecuteRegistrationHooks(ctx, hookCtx)
}

// ExecuteDataCollectionHooks executes data collection hooks
func (m *LifecycleHookManager) ExecuteDataCollectionHooks(ctx context.Context, hookCtx *types.DataCollectionHookContext) error {
	return m.registry.ExecuteDataCollectionHooks(ctx, hookCtx)
}

// ExecuteTeardownHooks executes teardown hooks
func (m *LifecycleHookManager) ExecuteTeardownHooks(ctx context.Context, hookCtx *types.TeardownHookContext) error {
	return m.registry.ExecuteTeardownHooks(ctx, hookCtx)
}

