# Import Updates Documentation

This document tracks all files that need import updates for the state machine refactoring.

## Files Requiring Updates

### 1. `state-mng/impl/state_mng_impl.go`
**Status**: ✅ **UPDATED**

**Changes Made**:
1. ✅ Added imports:
   - `iotadapters "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/state-machine/adapters"`
   - `iottypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"`
2. ✅ Updated calls (3 locations):
   - Line 490: `iot.NewCameraStateMachineAdapter(deviceSM)` → `iotadapters.NewCameraStateMachineAdapter(deviceSM, m.logger)`
   - Line 520: `iot.NewCameraStateMachineAdapter(deviceSM)` → `iotadapters.NewCameraStateMachineAdapter(deviceSM, m.logger)`
   - Line 550: `iot.NewCameraStateMachineAdapter(deviceSM)` → `iotadapters.NewCameraStateMachineAdapter(deviceSM, m.logger)`
3. ✅ Updated type references (2 locations):
   - Line 480: `iot.DeviceTypeCamera` → `iottypes.DeviceTypeCamera`
   - Line 535: `iot.DeviceTypeCamera` → `iottypes.DeviceTypeCamera`

**Impact**: Medium - External package, used by state-mng service

**Verification**: File compiles successfully (pre-existing errors in other packages are unrelated)

---

### 2. `iot/device_state_machine.go` (Root Package)
**Status**: ⚠️ **WILL BE DELETED** (Section 4.8)

**Note**: This file contains old implementations that have been moved to `state-machine/` package. It will be deleted in Section 4.8.

---

### 3. `iot/device_state_configs.go` (Root Package)
**Status**: ⚠️ **WILL BE DELETED** (Section 4.8)

**Note**: This file contains transition configurations that have been moved to `state-machine/transitions/` package. It will be deleted in Section 4.8.

---

### 4. `iot/camera_state_adapter.go` (Root Package)
**Status**: ⚠️ **WILL BE DELETED** (Section 4.8)

**Note**: This file contains `CameraStateAdapter` that has been moved to `state-machine/adapters/camera_workflow.go`. It will be deleted in Section 4.8.

---

### 5. `iot/device_state_adapter.go` (Root Package)
**Status**: ⚠️ **WILL BE DELETED** (Section 4.8)

**Note**: This file contains `CameraStateAdapter` (misnamed file) that has been moved to `state-machine/adapters/camera_workflow.go`. It will be deleted in Section 4.8.

---

## Files Already Updated

### ✅ `iot/device-state-service.go`
- Already uses `statemachine` and `transitions` packages
- Updated in Section 4.5

### ✅ `iot/iot_impl.go`
- Already uses `types` package for state machine interfaces
- No changes needed

### ✅ `state-machine/` package files
- All internal files already use correct imports
- Tests already use correct imports

---

## Summary

**Files to Update**: 1 file
- `state-mng/impl/state_mng_impl.go` - Update adapter imports and calls

**Files to Delete** (Section 4.8): 4 files
- `iot/device_state_machine.go`
- `iot/device_state_configs.go`
- `iot/camera_state_adapter.go`
- `iot/device_state_adapter.go`

**Files Already Updated**: 2 files
- `iot/device-state-service.go`
- `iot/iot_impl.go`

