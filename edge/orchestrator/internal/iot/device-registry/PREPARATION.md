# Device Registry Package Preparation

This document summarizes the review of `DeviceRegistry` interface and preparation for creating the `device-registry/` package (Section 7.1).

## Interface Review

### File: `internal/iot/types/registry.go`

**Package**: `types`  
**Interface**: `DeviceRegistry`

### Interface Methods (9 methods)

1. **`DiscoverDevices(ctx context.Context, deviceType DeviceType) ([]Device, error)`**
   - Discovers devices of a specific type
   - Should use `DevicePluginRegistry.DiscoverDevicesByType`
   - Should execute discovery hooks via `LifecycleHookRegistry`

2. **`DiscoverAllDevices(ctx context.Context) ([]Device, error)`**
   - Discovers all supported device types
   - Should use `DevicePluginRegistry.DiscoverDevices`
   - Should execute discovery hooks for all discovered devices

3. **`RegisterDevice(ctx context.Context, device Device) error`**
   - Registers a discovered device
   - Should create state machine via `DeviceStateMachineRegistry`
   - Should execute registration hooks via `LifecycleHookRegistry`
   - Should update internal indexes (by type, by capability)

4. **`GetDevice(ctx context.Context, deviceID string) (Device, error)`**
   - Retrieves a device by ID
   - Returns `types.ErrDeviceNotFound` if not found

5. **`ListDevices(ctx context.Context, filters *DeviceFilters) ([]Device, error)`**
   - Lists all registered devices
   - Supports filtering by type and/or capability
   - Uses `DeviceFilters` struct from `types/device.go`

6. **`UpdateDevice(ctx context.Context, deviceID string, updates *DeviceMetadataUpdate) error`**
   - Updates device metadata
   - Should validate updates
   - Should update internal indexes if type/capabilities change

7. **`DeleteDevice(ctx context.Context, deviceID string) error`**
   - Removes a device from the registry
   - Should execute teardown hooks via `LifecycleHookRegistry`
   - Should clean up state machine
   - Should update internal indexes

8. **`GetDevicesByCapability(ctx context.Context, capability DeviceCapability) ([]Device, error)`**
   - Returns all devices that support a specific capability
   - Uses internal capability index

9. **`GetDevicesByType(ctx context.Context, deviceType DeviceType) ([]Device, error)`**
   - Returns all devices of a specific type
   - Uses internal type index

---

## Dependencies

### 1. `DevicePluginRegistry` (from `plugin-registry/`)
**Package**: `github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry`  
**Interface**: `types.DevicePluginRegistry`

**Usage**:
- `DiscoverDevices(ctx)` - Discover all devices
- `DiscoverDevicesByType(ctx, deviceType)` - Discover devices by type
- `CreateDevice(ctx, metadata)` - Create device from metadata (if needed)

**Import**: `pluginregistry "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"`

---

### 2. `DeviceStateMachineRegistry` (from `state-machine/`)
**Package**: `github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine`  
**Interface**: `types.DeviceStateMachineRegistry`

**Usage**:
- `GetOrCreateStateMachine(deviceID, deviceType)` - Create/get state machine for device
- `GetStateMachine(deviceID)` - Get existing state machine
- `DeleteStateMachine(deviceID)` - Delete state machine (on device deletion)

**Import**: `statemachine "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine"`

---

### 3. `LifecycleHookRegistry` (from `hooks/`)
**Package**: `github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks`  
**Interface**: `types.LifecycleHookRegistry`

**Usage**:
- `ExecuteDiscoveryHooks(ctx, hookCtx)` - Execute discovery hooks
- `ExecuteRegistrationHooks(ctx, hookCtx)` - Execute registration hooks
- `ExecuteTeardownHooks(ctx, hookCtx)` - Execute teardown hooks

**Import**: `"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"`

---

## Type Dependencies

### From `types/device.go`:
- `Device` interface
- `DeviceType` type
- `DeviceCapability` type
- `DeviceMetadata` struct
- `DeviceMetadataUpdate` struct
- `DeviceFilters` struct
- `DeviceCapabilities` type

### From `types/errors.go`:
- `ErrDeviceNotFound`
- `ErrDeviceExists`
- `ErrInvalidDevice`

### From `types/hooks.go`:
- `DiscoveryHookContext`
- `RegistrationHookContext`
- `TeardownHookContext`

---

## Implementation Design

### Struct Design
```go
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
    
    // Observability
    logger *zap.Logger
    
    // Thread safety
    mu sync.RWMutex
}
```

### Constructor Design
```go
func NewDeviceRegistry(
    pluginRegistry types.DevicePluginRegistry,
    stateRegistry types.DeviceStateMachineRegistry,
    hookRegistry types.LifecycleHookRegistry,
    logger *zap.Logger,
) types.DeviceRegistry
```

**Dependencies**:
- All three registries are required (no nil checks needed)
- Logger can be nil (use `zap.NewNop()`)

---

## Integration Points

### Discovery Flow
1. Call `pluginRegistry.DiscoverDevicesByType(ctx, deviceType)`
2. For each discovered device:
   - Execute discovery hooks: `hookRegistry.ExecuteDiscoveryHooks(ctx, hookCtx)`
   - Return discovered devices

### Registration Flow
1. Validate device (not nil, valid ID, valid metadata)
2. Check if device already exists
3. Execute registration hooks: `hookRegistry.ExecuteRegistrationHooks(ctx, hookCtx)`
4. Create/get state machine: `stateRegistry.GetOrCreateStateMachine(deviceID, deviceType)`
5. Store device in registry
6. Update indexes (by type, by capability)
7. Log registration

### Deletion Flow
1. Get device (validate exists)
2. Execute teardown hooks: `hookRegistry.ExecuteTeardownHooks(ctx, hookCtx)`
3. Delete state machine: `stateRegistry.DeleteStateMachine(deviceID)`
4. Remove from registry
5. Update indexes
6. Log deletion

---

## File Structure Plan

```
device-registry/
├── registry.go              # Core implementation
├── registry_test.go          # Unit tests
├── examples_test.go          # Example tests
└── PREPARATION.md            # This file
```

**Future Files** (optional):
- `persistence.go` - Persistence abstraction (for future meta-storage integration)

---

## Implementation Requirements

### Locking Strategy
- ✅ Copy references under lock, call outside lock
- ✅ Use `RLock`/`RUnlock` for read operations
- ✅ Use `Lock`/`Unlock` for write operations
- ✅ Never hold lock while calling external dependencies

### Context Handling
- ✅ Never store context in struct
- ✅ Pass context through all methods
- ✅ Use context for cancellation/timeout

### Error Handling
- ✅ Use sentinel errors from `types/errors.go`
- ✅ Wrap errors with context: `fmt.Errorf("%w: ...", types.ErrDeviceNotFound)`
- ✅ Log errors with structured logging

### Logging
- ✅ Structured logging with `zap.Logger`
- ✅ Log levels: `Info` for operations, `Warn` for validation failures, `Error` for errors
- ✅ Include relevant fields: `device_id`, `device_type`, `capability`, etc.

---

## Statistics

- **Interface Methods**: 9 methods
- **Dependencies**: 3 registries (plugin, state, hooks)
- **Indexes**: 2 (by type, by capability)
- **Estimated Lines**: ~500-600 lines for full implementation

---

## Next Steps

1. ✅ **Section 7.1.1**: Interface review complete
2. ⏭️ **Section 7.1.2**: Create `device-registry/` directory structure
3. ⏭️ **Section 7.2**: Implement `deviceRegistryImpl`

---

## Notes

- This is a **new implementation** (no existing code to migrate)
- Device registry is the **central orchestrator** for device management
- Must integrate with all three subpackages (plugin-registry, state-machine, hooks)
- Follows same patterns as other subpackages (plugin-registry, state-machine, processing, hooks)

