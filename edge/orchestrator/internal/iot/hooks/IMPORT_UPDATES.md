# Hooks Package Import Updates

This document summarizes all files that needed import updates after moving lifecycle hooks to the `hooks/` package (Section 6.4).

## Files Reviewed

### 1. `internal/iot/iot_impl.go`
**Status**: ✅ **No changes needed**

**Current Usage**:
- Uses `types.LifecycleHookRegistry` interface (line 23)
- Field: `hookRegistry types.LifecycleHookRegistry`

**Analysis**:
- ✅ Already using `types` package for interface
- ✅ No constructor calls found
- ✅ Hook registry is injected as dependency (not created directly)
- ✅ No import changes needed

**Note**: If `iot_impl.go` needs to create a hook registry in the future, it should import:
```go
import (
    "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"
)
```
And use: `hooks.NewLifecycleHookRegistry(logger)`

---

### 2. `internal/iot/iot_provider.go`
**Status**: ✅ **No changes needed**

**Current Usage**:
- No direct usage of lifecycle hooks
- Only mentions hooks in comments

**Analysis**:
- ✅ No import changes needed

---

### 3. `internal/iot/iot.go`
**Status**: ✅ **No changes needed**

**Current Usage**:
- No direct usage of lifecycle hooks

**Analysis**:
- ✅ No import changes needed

---

### 4. `internal/iot/doc.go`
**Status**: ✅ **No changes needed**

**Current Usage**:
- Mentions `hooks.LifecycleHookRegistry` in documentation (line ~80)

**Analysis**:
- ✅ Documentation already references `hooks` package
- ✅ No import changes needed (documentation only)

---

### 5. `internal/iot/types/hooks.go`
**Status**: ✅ **No changes needed**

**Current Usage**:
- Defines `LifecycleHookRegistry` interface
- Defines all hook types and contexts

**Analysis**:
- ✅ This is the interface definition (stays in types)
- ✅ No import changes needed

---

### 6. `internal/iot/types/config.go`
**Status**: ✅ **No changes needed**

**Current Usage**:
- Defines `HooksConfig` struct
- No direct hook usage

**Analysis**:
- ✅ No import changes needed

---

## Summary

### Files That Need Updates: **0**

**Reason**: 
- The `iot_impl.go` file already uses `types.LifecycleHookRegistry` interface, which is correct
- No files directly call `NewLifecycleHookRegistry()` or other constructors
- Hook registry is injected as a dependency, not created directly
- All type references already use `types` package

### Files That Reference Hooks:
1. ✅ `iot_impl.go` - Uses interface from `types` package (correct)
2. ✅ `doc.go` - Documentation only (already references `hooks` package)
3. ✅ `types/hooks.go` - Interface definitions (correct location)
4. ✅ `types/config.go` - Configuration structs (no changes needed)

---

## Future Integration Points

When `IoTService` is fully wired up in later epics, it may need to:

1. **Import hooks package** in `iot_impl.go` or `iot_provider.go`:
   ```go
   import (
       "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/hooks"
   )
   ```

2. **Create hook registry** in provider or constructor:
   ```go
   hookRegistry := hooks.NewLifecycleHookRegistry(logger)
   ```

3. **Use hook manager** if needed:
   ```go
   hookManager := hooks.NewLifecycleHookManager(hookRegistry, logger)
   ```

4. **Use hook builder** for creating hooks:
   ```go
   hook := hooks.NewHookBuilder("hook-id", "Hook Name", types.HookTypeDiscovery, logger).
       WithPriority(10).
       WithDiscoveryHook(myHook).
       Build()
   ```

---

## Verification

### Compilation Check
```bash
go build ./edge/orchestrator/internal/iot/...
```

**Result**: ✅ All files compile successfully

### Test Check
```bash
go test ./edge/orchestrator/internal/iot/... -v
```

**Result**: ✅ All tests pass

---

## Conclusion

**No import updates are needed** at this time because:
- All type references already use `types` package
- No constructor calls exist in the current codebase
- Hook registry is designed to be injected as a dependency

The hooks package is ready to be integrated when `IoTService` is fully wired up in later epics.

