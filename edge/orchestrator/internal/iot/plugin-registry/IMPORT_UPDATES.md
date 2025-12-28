# Plugin Registry Import Updates

This document tracks all files that need to be updated to use the new `plugin-registry/` package instead of the root package.

## Files Requiring Updates

### Root Package Files

1. **`plugin_registry.go`** (TO BE DELETED in Section 3.5)
   - Status: Contains old implementation
   - Action: DELETE - implementation moved to `plugin-registry/`
   - Note: This file will be removed entirely

2. **`iot_impl.go`**
   - Status: ✅ Already correct
   - Current: Uses `types.DevicePluginRegistry` (interface)
   - Action: None needed - interface usage is correct
   - Note: Will be wired up with actual implementation in Epic 8

3. **`mocks/mock_plugin_registry.go`**
   - Status: Needs regeneration
   - Current: Generated from `github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot DevicePluginRegistry`
   - Action: Regenerate from `github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types DevicePluginRegistry`
   - Note: Mock should be generated from types package, not root package

### External Packages

**Status**: No external packages found that directly use plugin registry implementation.

All external code should use:
- `types.DevicePluginRegistry` interface (from `internal/iot/types`)
- `pluginregistry.NewDevicePluginRegistry()` constructor (from `internal/iot/plugin-registry`)

## Update Checklist

### Root Package
- [x] `iot_impl.go` - Already uses interface (no changes needed)
- [ ] `mocks/mock_plugin_registry.go` - Regenerate from types package
- [ ] `plugin_registry.go` - DELETE (in Section 3.5)

### External Packages
- [x] No external packages found using plugin registry directly

## Import Patterns

### Correct Import Pattern

```go
import (
    pluginregistry "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"
    "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// Use interface type from types package
func MyFunction(registry types.DevicePluginRegistry) {
    // ...
}

// Use constructor from plugin-registry package
registry := pluginregistry.NewDevicePluginRegistry(logger)
```

### Incorrect Patterns (to avoid)

```go
// DON'T: Import from root package
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot"
registry := iot.NewDevicePluginRegistry() // WRONG - old pattern

// DON'T: Use concrete type
import "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/plugin-registry"
var registry *pluginregistry.devicePluginRegistryImpl // WRONG - use interface
```

## Mock Generation

The mock should be regenerated using:

```bash
go run go.uber.org/mock/mockgen \
  -source=internal/iot/types/plugin.go \
  -destination=internal/iot/mocks/mock_plugin_registry.go \
  -package=mocks \
  github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types \
  DevicePluginRegistry
```

This will generate a mock from the interface in the types package, which is the correct source.

## Verification Steps

1. ✅ Check `iot_impl.go` - Uses interface correctly
2. [ ] Regenerate mock from types package
3. [ ] Delete `plugin_registry.go`
4. [ ] Run all tests to verify no breakage
5. [ ] Verify no compilation errors

