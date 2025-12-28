# Hooks Package Preparation

This document summarizes the review of `lifecycle_hooks.go` for Section 6.1.1.

## File Review

**File**: `internal/iot/lifecycle_hooks.go`  
**Size**: 585 lines  
**Package**: `package iot`

---

## Components to Move

### 1. `lifecycleHookRegistryImpl` struct (Lines 194-203)
**Status**: ✅ **MOVE TO** `hooks/registry.go`

**Fields**:
- `hooks map[string]*LifecycleHook` - Maps hook ID to hook
- `hooksByType map[LifecycleHookType][]*LifecycleHook` - Maps hook type to list of hooks
- `mu sync.RWMutex` - Protects the hooks maps
- ⚠️ **Missing**: `logger *zap.Logger` - Needs to be added

**Methods** (11 methods):
- `RegisterHook(hook *LifecycleHook) error` - Register a hook
- `UnregisterHook(hookID string) error` - Unregister a hook
- `GetHook(hookID string) (*LifecycleHook, error)` - Get a hook by ID
- `ListHooks(hookType *LifecycleHookType) []*LifecycleHook` - List hooks
- `ExecuteDiscoveryHooks(ctx context.Context, hookCtx *DiscoveryHookContext) error` - Execute discovery hooks
- `ExecuteRegistrationHooks(ctx context.Context, hookCtx *RegistrationHookContext) error` - Execute registration hooks
- `ExecuteDataCollectionHooks(ctx context.Context, hookCtx *DataCollectionHookContext) error` - Execute data collection hooks
- `ExecuteTeardownHooks(ctx context.Context, hookCtx *TeardownHookContext) error` - Execute teardown hooks
- `executeHooks(ctx context.Context, hookType LifecycleHookType, execute func(*LifecycleHook) error) error` - Private helper
- `hookMatchesFilters(hook *LifecycleHook, deviceType DeviceType, capabilities *DeviceCapabilities) bool` - Private helper
- `sortHooksByPriority(hookType LifecycleHookType)` - Private helper

**Constructor**:
- `NewLifecycleHookRegistry() LifecycleHookRegistry` - Creates registry
- ⚠️ **Needs Update**: Should accept `logger *zap.Logger` parameter

**Issues Found**:
- ❌ No structured logging
- ❌ No sentinel errors (uses `fmt.Errorf`)
- ❌ No logger field
- ❌ Constructor doesn't accept logger

---

### 2. `LifecycleHookManager` struct (Lines 457-459)
**Status**: ✅ **MOVE TO** `hooks/manager.go`

**Fields**:
- `registry LifecycleHookRegistry` - Wraps the registry

**Methods** (8 methods):
- `RegisterHook(hook *LifecycleHook) error` - Delegates to registry
- `UnregisterHook(hookID string) error` - Delegates to registry
- `GetHook(hookID string) (*LifecycleHook, error)` - Delegates to registry
- `ListHooks(hookType *LifecycleHookType) []*LifecycleHook` - Delegates to registry
- `ExecuteDiscoveryHooks(ctx context.Context, hookCtx *DiscoveryHookContext) error` - Delegates to registry
- `ExecuteRegistrationHooks(ctx context.Context, hookCtx *RegistrationHookContext) error` - Delegates to registry
- `ExecuteDataCollectionHooks(ctx context.Context, hookCtx *DataCollectionHookContext) error` - Delegates to registry
- `ExecuteTeardownHooks(ctx context.Context, hookCtx *TeardownHookContext) error` - Delegates to registry

**Constructor**:
- `NewLifecycleHookManager(registry LifecycleHookRegistry) *LifecycleHookManager` - Creates manager

**Issues Found**:
- ⚠️ Simple wrapper - no logging needed (delegates to registry)

---

### 3. `HookBuilder` struct (Lines 509-511)
**Status**: ✅ **MOVE TO** `hooks/builder.go`

**Fields**:
- `hook *LifecycleHook` - The hook being built

**Methods** (10 methods):
- `WithDescription(description string) *HookBuilder` - Set description
- `WithPriority(priority int) *HookBuilder` - Set priority
- `WithDeviceTypeFilter(deviceType DeviceType) *HookBuilder` - Set device type filter
- `WithCapabilityFilter(capability DeviceCapability) *HookBuilder` - Set capability filter
- `WithDiscoveryHook(hook DiscoveryHook) *HookBuilder` - Set discovery hook
- `WithRegistrationHook(hook RegistrationHook) *HookBuilder` - Set registration hook
- `WithDataCollectionHook(hook DataCollectionHook) *HookBuilder` - Set data collection hook
- `WithTeardownHook(hook TeardownHook) *HookBuilder` - Set teardown hook
- `WithEnabled(enabled bool) *HookBuilder` - Set enabled flag
- `Build() *LifecycleHook` - Build the hook

**Constructor**:
- `NewHookBuilder(id, name string, hookType LifecycleHookType) *HookBuilder` - Creates builder

**Issues Found**:
- ✅ No issues - builder pattern is clean

---

## Components That Stay in Types (Already in `types/hooks.go`)

### ✅ Already in `types/hooks.go`:
- `LifecycleHookType` type and constants
- `DiscoveryHookContext` struct
- `RegistrationHookContext` struct
- `DataCollectionHookContext` struct
- `TeardownHookContext` struct
- `DiscoveryHook` function type
- `RegistrationHook` function type
- `DataCollectionHook` function type
- `TeardownHook` function type
- `LifecycleHook` struct
- `LifecycleHookRegistry` interface

**Note**: All type definitions are already in `types/hooks.go` from Epic 1. No duplication needed.

---

## Dependencies

### Current Imports in `lifecycle_hooks.go`:
```go
import (
    "context"
    "fmt"
    "sync"
)
```

### Required Imports for `hooks/registry.go`:
```go
import (
    "context"
    "fmt"
    "sort"  // For sorting (if needed)
    "sync"
    
    "go.uber.org/zap"
    "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)
```

### Type References to Update:
- `Device` → `types.Device`
- `DeviceType` → `types.DeviceType`
- `DeviceCapability` → `types.DeviceCapability`
- `DeviceCapabilities` → `types.DeviceCapabilities`
- `DeviceMetadata` → `types.DeviceMetadata`
- `DeviceRegistry` → `types.DeviceRegistry`
- `DevicePlugin` → `types.DevicePlugin`
- `DeviceData` → `types.DeviceData`
- `DeviceDataType` → `types.DeviceDataType`
- `LifecycleHookType` → `types.LifecycleHookType`
- `LifecycleHook` → `types.LifecycleHook`
- `LifecycleHookRegistry` → `types.LifecycleHookRegistry`
- `DiscoveryHookContext` → `types.DiscoveryHookContext`
- `RegistrationHookContext` → `types.RegistrationHookContext`
- `DataCollectionHookContext` → `types.DataCollectionHookContext`
- `TeardownHookContext` → `types.TeardownHookContext`
- `DiscoveryHook` → `types.DiscoveryHook`
- `RegistrationHook` → `types.RegistrationHook`
- `DataCollectionHook` → `types.DataCollectionHook`
- `TeardownHook` → `types.TeardownHook`

---

## Test Files

### Current Test Files:
- ❌ **No test files found** for `lifecycle_hooks.go`
- ⚠️ Tests need to be created in Section 6.3

---

## Issues to Fix During Migration

### 1. Missing Structured Logging
- ❌ No logging in `lifecycleHookRegistryImpl`
- ✅ **Fix**: Add `logger *zap.Logger` field and structured logging to all methods

### 2. Missing Sentinel Errors
- ❌ Uses `fmt.Errorf` for all errors
- ✅ **Fix**: Use sentinel errors from `types/errors.go`:
  - `types.ErrInvalidDevice` for nil/invalid hooks
  - `types.ErrHookNotFound` (may need to create)
  - `types.ErrHookExists` (may need to create)

### 3. Constructor Signature
- ❌ `NewLifecycleHookRegistry()` doesn't accept logger
- ✅ **Fix**: Update to `NewLifecycleHookRegistry(logger *zap.Logger)`

### 4. Locking Strategy
- ✅ Already follows correct pattern: copy under lock, call outside lock
- ✅ `executeHooks` method correctly copies hooks under lock

### 5. Context Handling
- ✅ Never stores context in struct
- ✅ Context passed through all methods correctly

---

## File Structure Plan

```
hooks/
├── registry.go        # lifecycleHookRegistryImpl + NewLifecycleHookRegistry
├── manager.go         # LifecycleHookManager + NewLifecycleHookManager
├── builder.go         # HookBuilder + NewHookBuilder
├── registry_test.go    # Unit tests (to be created in Section 6.3)
└── examples_test.go    # Example tests (to be created in Section 6.3)
```

---

## Statistics

- **Total Lines**: 585 lines
- **Components to Move**: 3 (registry, manager, builder)
- **Methods to Move**: 29 methods total
- **Constructors to Move**: 3 constructors
- **Test Files**: 0 (need to create)

---

## Summary

### What Moves:
- ✅ `lifecycleHookRegistryImpl` → `hooks/registry.go`
- ✅ `LifecycleHookManager` → `hooks/manager.go`
- ✅ `HookBuilder` → `hooks/builder.go`

### What Stays:
- ✅ All type definitions already in `types/hooks.go` (from Epic 1)

### Improvements Needed:
- ✅ Add structured logging
- ✅ Add sentinel errors
- ✅ Update constructor to accept logger
- ✅ Update all type references to use `types` package

### Dependencies:
- ✅ `types` package (already exists)
- ✅ `zap` for logging (to be added)

---

## Next Steps

1. ✅ **Section 6.1.1**: Review complete
2. ⏭️ **Section 6.1.2**: Create `hooks/` directory structure
3. ⏭️ **Section 6.2**: Move implementation to `hooks/`
4. ⏭️ **Section 6.3**: Create tests
5. ⏭️ **Section 6.4**: Update imports
6. ⏭️ **Section 6.5**: Delete old file

