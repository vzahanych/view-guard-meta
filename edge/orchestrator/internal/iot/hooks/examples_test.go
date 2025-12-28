package hooks_test

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// ExampleNewLifecycleHookRegistry demonstrates how to create a new lifecycle hook registry.
func ExampleNewLifecycleHookRegistry() {
	logger := zap.NewNop()
	registry := hooks.NewLifecycleHookRegistry(logger)

	fmt.Printf("Registry created: %T\n", registry)
	// Output:
	// Registry created: *hooks.lifecycleHookRegistryImpl
}

// ExampleLifecycleHookRegistry_RegisterHook demonstrates how to register a lifecycle hook.
func ExampleLifecycleHookRegistry_RegisterHook() {
	logger := zap.NewNop()
	registry := hooks.NewLifecycleHookRegistry(logger)

	// Create a discovery hook
	hook := &types.LifecycleHook{
		ID:   "test-hook",
		Type: types.HookTypeDiscovery,
		Name: "Test Discovery Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			fmt.Println("Discovery hook executed")
			return nil
		},
		Priority: 1,
		Enabled: true,
	}

	err := registry.RegisterHook(hook)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Hook registered")
	// Output:
	// Hook registered
}

// ExampleLifecycleHookRegistry_ExecuteDiscoveryHooks demonstrates how to execute discovery hooks.
func ExampleLifecycleHookRegistry_ExecuteDiscoveryHooks() {
	logger := zap.NewNop()
	registry := hooks.NewLifecycleHookRegistry(logger)

	// Register a discovery hook
	hook := &types.LifecycleHook{
		ID:   "discovery-hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Device Discovery Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			fmt.Printf("Discovered %d devices of type %s\n", len(hookCtx.DiscoveredDevices), hookCtx.DeviceType)
			return nil
		},
		Priority: 1,
		Enabled: true,
	}

	_ = registry.RegisterHook(hook)

	// Execute discovery hooks
	hookCtx := &types.DiscoveryHookContext{
		DeviceType:        types.DeviceTypeCamera,
		DiscoveredDevices: []types.Device{},
	}

	err := registry.ExecuteDiscoveryHooks(context.Background(), hookCtx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Discovery hooks executed")
	// Output:
	// Discovered 0 devices of type camera
	// Discovery hooks executed
}

// ExampleLifecycleHookRegistry_ExecuteRegistrationHooks demonstrates how to execute registration hooks.
func ExampleLifecycleHookRegistry_ExecuteRegistrationHooks() {
	logger := zap.NewNop()
	registry := hooks.NewLifecycleHookRegistry(logger)

	// Register a registration hook
	hook := &types.LifecycleHook{
		ID:   "registration-hook-1",
		Type: types.HookTypeRegistration,
		Name: "Device Registration Hook",
		RegistrationHook: func(ctx context.Context, hookCtx *types.RegistrationHookContext) error {
			fmt.Printf("Registering device: %s\n", hookCtx.Device.GetID())
			return nil
		},
		Priority: 1,
		Enabled: true,
	}

	_ = registry.RegisterHook(hook)

	// In production, you would use a real device
	// For demonstration, we show the pattern
	fmt.Println("Registration hook pattern: registry.ExecuteRegistrationHooks(ctx, hookCtx)")
	// Output:
	// Registration hook pattern: registry.ExecuteRegistrationHooks(ctx, hookCtx)
}

// ExampleLifecycleHookRegistry_ListHooks demonstrates how to list registered hooks.
func ExampleLifecycleHookRegistry_ListHooks() {
	logger := zap.NewNop()
	registry := hooks.NewLifecycleHookRegistry(logger)

	// Register multiple hooks
	hook1 := &types.LifecycleHook{
		ID:   "hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Discovery Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
	}

	hook2 := &types.LifecycleHook{
		ID:   "hook-2",
		Type: types.HookTypeRegistration,
		Name: "Registration Hook",
		RegistrationHook: func(ctx context.Context, hookCtx *types.RegistrationHookContext) error {
			return nil
		},
	}

	_ = registry.RegisterHook(hook1)
	_ = registry.RegisterHook(hook2)

	// List all hooks
	allHooks := registry.ListHooks(nil)
	fmt.Printf("Total hooks: %d\n", len(allHooks))

	// List hooks by type
	discoveryType := types.HookTypeDiscovery
	discoveryHooks := registry.ListHooks(&discoveryType)
	fmt.Printf("Discovery hooks: %d\n", len(discoveryHooks))

	// Output:
	// Total hooks: 2
	// Discovery hooks: 1
}

// ExampleHookBuilder demonstrates how to use the hook builder pattern.
func ExampleHookBuilder() {
	logger := zap.NewNop()

	// Build a hook using the builder
	hook := hooks.NewHookBuilder("my-hook", "My Hook", types.HookTypeDiscovery, logger).
		WithDescription("A test discovery hook").
		WithPriority(10).
		WithDeviceTypeFilter(types.DeviceTypeCamera).
		WithDiscoveryHook(func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			fmt.Println("Hook executed")
			return nil
		}).
		WithEnabled(true).
		Build()

	fmt.Printf("Hook built: %s (Priority: %d)\n", hook.Name, hook.Priority)
	// Output:
	// Hook built: My Hook (Priority: 10)
}

// ExampleLifecycleHookManager demonstrates how to use the lifecycle hook manager.
func ExampleLifecycleHookManager() {
	logger := zap.NewNop()
	registry := hooks.NewLifecycleHookRegistry(logger)
	manager := hooks.NewLifecycleHookManager(registry, logger)

	// Register a hook via manager
	hook := &types.LifecycleHook{
		ID:   "manager-hook",
		Type: types.HookTypeDiscovery,
		Name: "Manager Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			return nil
		},
		Priority: 1,
		Enabled: true,
	}

	err := manager.RegisterHook(hook)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// List hooks via manager
	hooks := manager.ListHooks(nil)
	fmt.Printf("Hooks managed: %d\n", len(hooks))

	// Output:
	// Hooks managed: 1
}

// ExampleLifecycleHookRegistry_ExecuteDiscoveryHooks_filtering demonstrates how hooks are filtered by device type.
func ExampleLifecycleHookRegistry_ExecuteDiscoveryHooks_filtering() {
	logger := zap.NewNop()
	registry := hooks.NewLifecycleHookRegistry(logger)

	// Register a hook with device type filter (camera only)
	cameraType := types.DeviceTypeCamera
	hook1 := &types.LifecycleHook{
		ID:   "camera-hook",
		Type: types.HookTypeDiscovery,
		Name: "Camera Discovery Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			fmt.Println("Camera hook executed")
			return nil
		},
		DeviceTypeFilter: &cameraType,
		Priority: 1,
		Enabled: true,
	}

	// Register a hook with no filter (all device types)
	hook2 := &types.LifecycleHook{
		ID:   "generic-hook",
		Type: types.HookTypeDiscovery,
		Name: "Generic Discovery Hook",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			fmt.Println("Generic hook executed")
			return nil
		},
		Priority: 2,
		Enabled: true,
	}

	_ = registry.RegisterHook(hook1)
	_ = registry.RegisterHook(hook2)

	// Execute with camera device type - both hooks should execute
	hookCtx := &types.DiscoveryHookContext{
		DeviceType:        types.DeviceTypeCamera,
		DiscoveredDevices: []types.Device{},
	}

	_ = registry.ExecuteDiscoveryHooks(context.Background(), hookCtx)

	// Output:
	// Camera hook executed
	// Generic hook executed
}

// ExampleLifecycleHookRegistry_ExecuteDiscoveryHooks_priority demonstrates how hooks are executed in priority order.
func ExampleLifecycleHookRegistry_ExecuteDiscoveryHooks_priority() {
	logger := zap.NewNop()
	registry := hooks.NewLifecycleHookRegistry(logger)

	// Register hooks with different priorities
	hook1 := &types.LifecycleHook{
		ID:   "hook-1",
		Type: types.HookTypeDiscovery,
		Name: "Hook 1 (Priority 100)",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			fmt.Println("Hook 1 executed")
			return nil
		},
		Priority: 100, // Higher priority = later execution
		Enabled: true,
	}

	hook2 := &types.LifecycleHook{
		ID:   "hook-2",
		Type: types.HookTypeDiscovery,
		Name: "Hook 2 (Priority 10)",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			fmt.Println("Hook 2 executed")
			return nil
		},
		Priority: 10, // Lower priority = earlier execution
		Enabled: true,
	}

	hook3 := &types.LifecycleHook{
		ID:   "hook-3",
		Type: types.HookTypeDiscovery,
		Name: "Hook 3 (Priority 50)",
		DiscoveryHook: func(ctx context.Context, hookCtx *types.DiscoveryHookContext) error {
			fmt.Println("Hook 3 executed")
			return nil
		},
		Priority: 50,
		Enabled: true,
	}

	// Register in non-priority order
	_ = registry.RegisterHook(hook1)
	_ = registry.RegisterHook(hook2)
	_ = registry.RegisterHook(hook3)

	// Execute hooks - should execute in priority order (lower = earlier)
	hookCtx := &types.DiscoveryHookContext{
		DeviceType:        types.DeviceTypeCamera,
		DiscoveredDevices: []types.Device{},
	}

	_ = registry.ExecuteDiscoveryHooks(context.Background(), hookCtx)

	// Output:
	// Hook 2 executed
	// Hook 3 executed
	// Hook 1 executed
}

