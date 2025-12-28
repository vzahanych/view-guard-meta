# Plugin Registry Package Preparation

This document summarizes the findings from reviewing the current plugin registry implementation and the preparation for moving it to the `plugin-registry/` subpackage.

## Current Implementation Review

### File: `internal/iot/plugin_registry.go`

**Package**: `iot`  
**Lines**: 340 lines

### Components to Move

#### 1. Core Implementation (`devicePluginRegistryImpl`)

**Struct**: `devicePluginRegistryImpl`
- Fields:
  - `plugins map[iottypes.DeviceType]iottypes.DevicePlugin` - plugin registry map
  - `mu sync.RWMutex` - mutex for thread safety
- **Note**: Currently missing logger field (should be added)

**Constructor**: `NewDevicePluginRegistry()`
- Currently takes no parameters
- Should be updated to accept `*zap.Logger` parameter

**Methods** (13 methods):
1. `RegisterPlugin(plugin iottypes.DevicePlugin) error`
2. `UnregisterPlugin(deviceType iottypes.DeviceType) error`
3. `GetPlugin(deviceType iottypes.DeviceType) (iottypes.DevicePlugin, error)`
4. `ListPlugins() []iottypes.DevicePlugin`
5. `GetPluginForDeviceType(deviceType iottypes.DeviceType) (iottypes.DevicePlugin, error)` - alias for GetPlugin
6. `DiscoverDevices(ctx context.Context) ([]iottypes.Device, error)`
7. `DiscoverDevicesByType(ctx context.Context, deviceType iottypes.DeviceType) ([]iottypes.Device, error)`
8. `CreateDevice(ctx context.Context, metadata iottypes.DeviceMetadata) (iottypes.Device, error)`
9. `ValidateMetadata(metadata iottypes.DeviceMetadata) error`
10. `GetSupportedDeviceTypes() []iottypes.DeviceType`
11. `IsDeviceTypeSupported(deviceType iottypes.DeviceType) bool`
12. `validatePlugin(plugin iottypes.DevicePlugin) error` - private helper

#### 2. PluginManager Wrapper

**Struct**: `PluginManager`
- Fields:
  - `registry iottypes.DevicePluginRegistry` - wrapped registry

**Constructor**: `NewPluginManager(registry iottypes.DevicePluginRegistry) *PluginManager`
- **Note**: Should be updated to accept `*zap.Logger` parameter

**Methods** (8 methods - all delegate to registry):
1. `RegisterPlugin(plugin iottypes.DevicePlugin) error`
2. `UnregisterPlugin(deviceType iottypes.DeviceType) error`
3. `GetPlugin(deviceType iottypes.DeviceType) (iottypes.DevicePlugin, error)`
4. `DiscoverAllDevices(ctx context.Context) ([]iottypes.Device, error)`
5. `DiscoverDevicesByType(ctx context.Context, deviceType iottypes.DeviceType) ([]iottypes.Device, error)`
6. `CreateDeviceFromMetadata(ctx context.Context, metadata iottypes.DeviceMetadata) (iottypes.Device, error)`
7. `GetSupportedDeviceTypes() []iottypes.DeviceType`
8. `IsDeviceTypeSupported(deviceType iottypes.DeviceType) bool`

#### 3. Helper Functions and Types

**Function**: `DiscoverPlugins(ctx context.Context, registry iottypes.DevicePluginRegistry, config *PluginDiscoveryConfig) (*PluginDiscoveryResult, error)`
- Placeholder for future file-based plugin discovery
- Currently returns already registered plugins

**Types** (defined in root package, but also in types/plugin.go):
- `PluginDiscoveryConfig` - **DUPLICATE**: Also defined in `types/plugin.go` (different structure)
- `PluginDiscoveryResult` - **DUPLICATE**: Also defined in `types/plugin.go` (different structure)
- `PluginDiscoveryError` - Only in root package

**Note**: The root package versions have different fields than the types package versions. The root package versions should be removed in favor of types package versions, or consolidated.

### Dependencies

#### Imports from `iot/types` package:
- `iottypes.DeviceType`
- `iottypes.DevicePlugin`
- `iottypes.DevicePluginRegistry`
- `iottypes.Device`
- `iottypes.DeviceMetadata`
- `iottypes.DeviceCapability` (used in validatePlugin)

#### Standard library imports:
- `context`
- `fmt`
- `sync`

#### External dependencies:
- None currently (should add `go.uber.org/zap` for logging)

### Type References to Update

All type references are already using `iottypes.` prefix, so they just need to be changed to `types.`:
- `iottypes.DeviceType` → `types.DeviceType`
- `iottypes.DevicePlugin` → `types.DevicePlugin`
- `iottypes.DevicePluginRegistry` → `types.DevicePluginRegistry`
- `iottypes.Device` → `types.Device`
- `iottypes.DeviceMetadata` → `types.DeviceMetadata`
- `iottypes.DeviceCapability` → `types.DeviceCapability`

### Test Files

**Status**: No test files found for plugin registry
- No `*plugin*test*.go` files in `internal/iot/`
- Tests will need to be created in `plugin-registry/registry_test.go`

### Helper Types Location Verification

**In `types/plugin.go`**:
- ✅ `DevicePlugin` interface - **EXISTS**
- ✅ `DevicePluginRegistry` interface - **EXISTS**
- ✅ `PluginDiscoveryConfig` struct - **EXISTS** (different structure than root package)
- ✅ `PluginDiscoveryResult` struct - **EXISTS** (different structure than root package)

**In root package (`plugin_registry.go`)**:
- `PluginDiscoveryConfig` - **DUPLICATE** (different fields)
- `PluginDiscoveryResult` - **DUPLICATE** (different fields)
- `PluginDiscoveryError` - **NOT IN TYPES** (should be added to types or kept in plugin-registry)

**Decision**: 
- Use `types.PluginDiscoveryConfig` and `types.PluginDiscoveryResult` from types package
- Move `PluginDiscoveryError` to plugin-registry package (it's implementation-specific)
- Remove duplicate definitions from root package

### Error Handling

**Current**: Uses `fmt.Errorf` with string messages

**Target**: Use sentinel errors from `types/errors.go`:
- `types.ErrPluginNotFound` - when plugin not found
- `types.ErrPluginExists` - when plugin already registered
- `types.ErrNoPluginForType` - when no plugin for device type

**Note**: Need to verify these errors exist in `types/errors.go`, or add them if missing.

### Logging

**Current**: No structured logging (only comments mentioning "In production, you might want to use a logger here")

**Target**: Add structured logging using `zap.Logger`:
- Add `logger *zap.Logger` field to `devicePluginRegistryImpl`
- Add `logger *zap.Logger` field to `PluginManager`
- Add logging to all methods (Info, Warn, Error, Debug as appropriate)

### Locking Strategy

**Current**: Uses `defer r.mu.Unlock()` pattern (holds lock during method execution)

**Target**: Follow VMGateway pattern - copy references under lock, call outside lock:
- For read operations: copy slice/map under lock, return/use outside lock
- For write operations: current pattern is acceptable (short critical sections)

**Note**: Current implementation already follows good locking practices for most methods. `DiscoverDevices` already copies plugins slice under lock and calls outside lock.

### Context Handling

**Current**: Context passed as parameter, never stored in struct ✅

**Target**: Continue this pattern ✅

## Target Package Structure

```
internal/iot/plugin-registry/
  registry.go          # devicePluginRegistryImpl and NewDevicePluginRegistry
  manager.go           # PluginManager wrapper
  registry_test.go     # Unit tests
  examples_test.go     # Example tests (optional)
  PREPARATION.md       # This document
```

## Migration Checklist

### Section 3.1: Preparation ✅
- [x] Review current implementation
- [x] Identify all components
- [x] Document dependencies
- [x] Verify helper types location
- [x] Create directory structure
- [x] Document findings

### Section 3.2: Move Implementation (Next)
- [ ] Create registry.go with devicePluginRegistryImpl
- [ ] Add logger field and parameter
- [ ] Update all type references to use `types` package
- [ ] Add structured logging
- [ ] Use sentinel errors
- [ ] Create manager.go with PluginManager
- [ ] Update DiscoverPlugins function
- [ ] Handle PluginDiscoveryError type

### Section 3.3: Tests (Future)
- [ ] Create registry_test.go
- [ ] Create examples_test.go
- [ ] Test all methods
- [ ] Test error cases
- [ ] Test concurrent access

### Section 3.4: Integration (Future)
- [ ] Update root package to use plugin-registry
- [ ] Update iot_impl.go to use plugin-registry
- [ ] Update provider to create plugin registry
- [ ] Remove old plugin_registry.go from root

## Notes

1. **Logger Addition**: The current implementation doesn't have logging. We should add it following VMGateway patterns.

2. **Error Types**: Need to verify/update `types/errors.go` to include plugin-related errors.

3. **PluginDiscoveryConfig/Result**: There are two different versions - one in root package and one in types package. We should use the types package version and remove the root package version.

4. **PluginDiscoveryError**: This is implementation-specific and should stay in plugin-registry package.

5. **No Tests**: Currently no tests exist. We'll need to create comprehensive tests.

6. **Package Name**: Using `pluginregistry` as package name (following Go naming conventions for hyphenated directories).

