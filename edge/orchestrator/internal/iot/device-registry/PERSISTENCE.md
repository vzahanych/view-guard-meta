# Device Registry Persistence

This document summarizes the persistence implementation for Section 7.4: Design Persistence Abstraction.

## Overview

The device registry now supports optional persistence through a `DeviceStorageBackend` interface. Two implementations are provided:

1. **In-Memory Storage** (`inMemoryStorage`) - Default, no persistence
2. **Meta Storage Backend** (`metaStorageDeviceBackend`) - Uses MetaDataStore for persistence

---

## Architecture

### DeviceStorageBackend Interface

```go
type DeviceStorageBackend interface {
    SaveDevice(ctx context.Context, device types.Device) error
    LoadDevice(ctx context.Context, deviceID string) (types.Device, error)
    LoadAllDevices(ctx context.Context) ([]types.Device, error)
    DeleteDevice(ctx context.Context, deviceID string) error
}
```

**Purpose**: Abstract persistence layer that allows different storage backends to be plugged in.

---

## Implementations

### 1. In-Memory Storage (`inMemoryStorage`)

**Purpose**: Default implementation with no persistence (for testing or in-memory only operation)

**Features**:
- ✅ No-op operations (all methods return success or "not found")
- ✅ No external dependencies
- ✅ Suitable for testing and development

**Usage**:
```go
storage := NewInMemoryStorage(logger)
registry := NewDeviceRegistryWithStorage(pluginReg, stateReg, hookReg, storage, logger)
```

---

### 2. Meta Storage Backend (`metaStorageDeviceBackend`)

**Purpose**: Persists devices using the MetaDataStore service

**Features**:
- ✅ Uses MetaDataStore for persistence
- ✅ Stores device metadata as JSON
- ✅ Integrates with existing meta storage infrastructure
- ✅ Automatic persistence on register/update/delete

**Usage**:
```go
metaStore := metastorage.NewMetaDataStore(ctx, config, logger)
storage := NewMetaStorageDeviceBackend(metaStore, logger)
registry := NewDeviceRegistryWithStorage(pluginReg, stateReg, hookReg, storage, logger)
```

**Storage Format**:
- Devices are stored using `MetaDataStore.SaveEvent()` with device ID as event ID
- Device metadata is serialized to JSON and stored as `map[string]interface{}`
- Metadata includes: ID, Type, Name, Enabled, Status, Capabilities, Config, Location, Zone, Tags, timestamps

---

## Integration with Device Registry

### Constructor Options

**Default (In-Memory)**:
```go
registry := NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
// Uses in-memory storage automatically
```

**With Persistence**:
```go
storage := NewMetaStorageDeviceBackend(metaStore, logger)
registry := NewDeviceRegistryWithStorage(pluginReg, stateReg, hookReg, storage, logger)
```

### Automatic Persistence

The registry automatically persists devices when:
- ✅ **RegisterDevice**: Device is saved to storage after successful registration
- ✅ **UpdateDevice**: Device is saved to storage after successful update
- ✅ **DeleteDevice**: Device is deleted from storage after successful deletion

**Error Handling**:
- Persistence failures are logged as warnings but do not fail the operation
- Device remains registered/updated/deleted in memory even if persistence fails
- This ensures the registry continues to function even if storage is unavailable

---

## Device Metadata Format

### Stored Fields

```go
type deviceMetadata struct {
    ID           string                 // Device ID
    Type         string                 // Device type (camera, sensor, etc.)
    Name         string                 // Device name
    Enabled      bool                   // Enabled status
    Status       string                 // Device status (online, offline, etc.)
    Capabilities map[string]bool        // Device capabilities
    Metadata     map[string]interface{} // Additional metadata
    Config       map[string]interface{} // Device configuration
    Location     string                 // Device location
    Zone         string                 // Device zone
    Tags         []string               // Device tags
    CreatedAt    string                 // Creation timestamp (ISO 8601)
    UpdatedAt    string                 // Update timestamp (ISO 8601)
}
```

### Serialization

- Devices are converted to `deviceMetadata` struct
- `deviceMetadata` is serialized to JSON
- JSON is stored as `map[string]interface{}` in meta storage

---

## Current Limitations

### Device Recreation

**Issue**: `LoadDevice` and `LoadAllDevices` cannot recreate full `Device` instances because:
1. `Device` is an interface with methods that require implementation
2. Devices are created via `DevicePlugin.CreateDevice()`
3. Device instances need plugin-specific initialization

**Current Behavior**:
- `LoadDevice` returns an error indicating device recreation is not yet implemented
- `LoadAllDevices` returns an empty slice
- Metadata is stored and can be retrieved, but devices must be recreated via plugins

**Future Enhancement**:
- Add `LoadDeviceMetadata()` method that returns `DeviceMetadata` instead of `Device`
- Registry can use metadata to recreate devices via `DevicePlugin.CreateDevice()`
- Add `RestoreDevices()` method to registry that loads metadata and recreates devices

### Storage Backend

**Current Implementation**: Uses `MetaDataStore.SaveEvent()` / `GetEvent()` / `DeleteEvent()`

**Limitation**: Devices are stored in the events bucket, which is not ideal for device-specific operations

**Future Enhancement**:
- Add dedicated `devices` bucket to MetaDataStore
- Add `SaveDevice()`, `GetDevice()`, `ListDevices()`, `DeleteDevice()` methods to MetaDataStore
- Update `metaStorageDeviceBackend` to use dedicated device methods

---

## Usage Examples

### Example 1: In-Memory Registry (Default)

```go
logger := zap.NewNop()
pluginReg := pluginregistry.NewDevicePluginRegistry(logger)
factory := statemachine.NewDeviceStateMachineFactory(logger)
stateReg := statemachine.NewDeviceStateMachineRegistry(factory, logger)
hookReg := hooks.NewLifecycleHookRegistry(logger)

// Uses in-memory storage automatically
registry := deviceregistry.NewDeviceRegistry(pluginReg, stateReg, hookReg, logger)
```

### Example 2: Registry with Meta Storage Persistence

```go
logger := zap.NewNop()

// Create meta storage
metaStore, err := metastorage.NewMetaDataStore(ctx, config, logger)
if err != nil {
    return err
}

// Create persistence backend
storage := deviceregistry.NewMetaStorageDeviceBackend(metaStore, logger)

// Create registries
pluginReg := pluginregistry.NewDevicePluginRegistry(logger)
factory := statemachine.NewDeviceStateMachineFactory(logger)
stateReg := statemachine.NewDeviceStateMachineRegistry(factory, logger)
hookReg := hooks.NewLifecycleHookRegistry(logger)

// Create registry with persistence
registry := deviceregistry.NewDeviceRegistryWithStorage(
    pluginReg,
    stateReg,
    hookReg,
    storage,
    logger,
)
```

### Example 3: Fx Provider Integration

```go
func DeviceRegistryProvider(
    lc fx.Lifecycle,
    pluginReg types.DevicePluginRegistry,
    stateReg types.DeviceStateMachineRegistry,
    hookReg types.LifecycleHookRegistry,
    metaStore metastorage.MetaDataStore, // Optional
    logger *zap.Logger,
) (types.DeviceRegistry, error) {
    var storage deviceregistry.DeviceStorageBackend
    if metaStore != nil {
        storage = deviceregistry.NewMetaStorageDeviceBackend(metaStore, logger)
    } else {
        storage = deviceregistry.NewInMemoryStorage(logger)
    }

    registry := deviceregistry.NewDeviceRegistryWithStorage(
        pluginReg,
        stateReg,
        hookReg,
        storage,
        logger,
    )

    // Optional: Restore devices from storage on startup
    // This would require implementing device recreation from metadata

    return registry, nil
}
```

---

## Testing

### Unit Tests

Persistence backend implementations should be tested separately:
- ✅ Test `SaveDevice` serialization
- ✅ Test `LoadDevice` deserialization
- ✅ Test `LoadAllDevices` listing
- ✅ Test `DeleteDevice` removal
- ✅ Test error handling (nil device, empty ID, etc.)

### Integration Tests

Test registry with persistence:
- ✅ Register device → verify persisted
- ✅ Update device → verify update persisted
- ✅ Delete device → verify deletion persisted
- ✅ Test persistence failure handling (device still works in memory)

---

## Future Enhancements

1. **Dedicated Device Storage in Meta Storage**
   - Add `devices` bucket to MetaDataStore
   - Add device-specific methods to MetaDataStore interface
   - Update `metaStorageDeviceBackend` to use dedicated methods

2. **Device Recreation from Metadata**
   - Add `LoadDeviceMetadata()` to return `DeviceMetadata`
   - Add `RestoreDevices()` to registry that recreates devices from metadata
   - Use `DevicePlugin.CreateDevice()` to recreate device instances

3. **Batch Operations**
   - Add `SaveDevices()` for batch persistence
   - Add `LoadDevicesByType()` for filtered loading
   - Optimize persistence operations

4. **Persistence Configuration**
   - Add persistence enable/disable flag
   - Add persistence sync mode (sync vs async)
   - Add persistence retry logic

5. **Additional Backends**
   - Database-backed storage (PostgreSQL, SQLite)
   - Cloud storage backend (S3, etc.)
   - Hybrid storage (memory + persistent)

---

## Statistics

- **Interface Methods**: 4 methods
- **Implementations**: 2 (in-memory, meta storage)
- **Storage Format**: JSON serialization
- **Integration Points**: RegisterDevice, UpdateDevice, DeleteDevice

---

## Notes

- Persistence is **optional** - registry works without it
- Persistence failures are **non-fatal** - registry continues to function
- Device recreation from metadata is **not yet implemented** - requires plugin integration
- Current implementation uses event storage as a workaround - dedicated device storage is recommended for future

---

## Related Files

- `persistence.go` - Persistence interface and implementations
- `registry.go` - Registry integration with persistence
- `meta-storage/meta-storage-iface.go` - MetaDataStore interface
- `meta-storage/bbolt-imp/meta-storage-impl.go` - MetaDataStore implementation

