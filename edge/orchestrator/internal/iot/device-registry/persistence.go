package deviceregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
)

// DeviceStorageBackend is an abstraction for device persistence.
// It provides methods to save, load, and delete devices from persistent storage.
type DeviceStorageBackend interface {
	// SaveDevice saves a device to persistent storage
	SaveDevice(ctx context.Context, device types.Device) error

	// LoadDevice loads a device from persistent storage by ID
	// Returns the device and a boolean indicating if it was found
	LoadDevice(ctx context.Context, deviceID string) (types.Device, error)

	// LoadAllDevices loads all devices from persistent storage
	LoadAllDevices(ctx context.Context) ([]types.Device, error)

	// DeleteDevice deletes a device from persistent storage
	DeleteDevice(ctx context.Context, deviceID string) error
}

// inMemoryStorage is a no-op implementation for in-memory only operation
type inMemoryStorage struct {
	logger *zap.Logger
}

// NewInMemoryStorage creates a new in-memory storage backend (no persistence)
func NewInMemoryStorage(logger *zap.Logger) DeviceStorageBackend {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &inMemoryStorage{
		logger: logger,
	}
}

func (s *inMemoryStorage) SaveDevice(ctx context.Context, device types.Device) error {
	// No-op for in-memory storage
	s.logger.Debug("In-memory storage: SaveDevice (no-op)",
		zap.String("device_id", device.GetID()),
	)
	return nil
}

func (s *inMemoryStorage) LoadDevice(ctx context.Context, deviceID string) (types.Device, error) {
	// No persistence, device not found
	s.logger.Debug("In-memory storage: LoadDevice (not found)",
		zap.String("device_id", deviceID),
	)
	return nil, fmt.Errorf("device %s not found in persistent storage", deviceID)
}

func (s *inMemoryStorage) LoadAllDevices(ctx context.Context) ([]types.Device, error) {
	// No persistence, return empty list
	s.logger.Debug("In-memory storage: LoadAllDevices (empty)")
	return []types.Device{}, nil
}

func (s *inMemoryStorage) DeleteDevice(ctx context.Context, deviceID string) error {
	// No-op for in-memory storage
	s.logger.Debug("In-memory storage: DeleteDevice (no-op)",
		zap.String("device_id", deviceID),
	)
	return nil
}

// metaStorageDeviceBackend implements DeviceStorageBackend using MetaDataStore
type metaStorageDeviceBackend struct {
	metaStore metastorage.MetaDataStore
	logger    *zap.Logger
}

// NewMetaStorageDeviceBackend creates a new meta storage backend for device persistence
func NewMetaStorageDeviceBackend(metaStore metastorage.MetaDataStore, logger *zap.Logger) DeviceStorageBackend {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &metaStorageDeviceBackend{
		metaStore: metaStore,
		logger:    logger,
	}
}

// deviceMetadata represents device data stored in meta storage
type deviceMetadata struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Name         string                 `json:"name"`
	Enabled      bool                   `json:"enabled"`
	Status       string                 `json:"status"`
	Capabilities map[string]bool        `json:"capabilities"`
	Metadata     map[string]interface{} `json:"metadata"`
	Config       map[string]interface{} `json:"config"`
	Location     string                 `json:"location"`
	Zone         string                 `json:"zone"`
	Tags         []string               `json:"tags"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
}

// deviceToMetadata converts a Device to deviceMetadata for storage
func deviceToMetadata(device types.Device) deviceMetadata {
	meta := device.GetMetadata()
	capabilities := make(map[string]bool)
	for cap := range meta.Capabilities {
		capabilities[string(cap)] = true
	}

	return deviceMetadata{
		ID:           meta.ID,
		Type:         string(meta.Type),
		Name:         meta.Name,
		Enabled:      device.IsEnabled(),
		Status:       string(device.GetStatus()),
		Capabilities: capabilities,
		Metadata:     meta.Metadata,
		Config:       meta.Config,
		Location:     meta.Location,
		Zone:         meta.Zone,
		Tags:         meta.Tags,
		CreatedAt:    meta.DiscoveredAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    time.Now().Format("2006-01-02T15:04:05Z07:00"), // Use current time for updates
	}
}

// metadataToMap converts deviceMetadata to a map for meta storage
func metadataToMap(meta deviceMetadata) map[string]interface{} {
	data, _ := json.Marshal(meta)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// mapToMetadata converts a map from meta storage to deviceMetadata
func mapToMetadata(data map[string]interface{}) (deviceMetadata, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return deviceMetadata{}, fmt.Errorf("failed to marshal device data: %w", err)
	}

	var meta deviceMetadata
	if err := json.Unmarshal(dataBytes, &meta); err != nil {
		return deviceMetadata{}, fmt.Errorf("failed to unmarshal device data: %w", err)
	}

	return meta, nil
}

// SaveDevice saves a device to meta storage
func (s *metaStorageDeviceBackend) SaveDevice(ctx context.Context, device types.Device) error {
	if device == nil {
		return fmt.Errorf("device cannot be nil")
	}

	deviceID := device.GetID()
	if deviceID == "" {
		return fmt.Errorf("device ID cannot be empty")
	}

	meta := deviceToMetadata(device)
	data := metadataToMap(meta)

	// Use SaveEvent as a generic storage mechanism (similar to how other metadata is stored)
	// The event ID is the device ID, and the event data contains the device metadata
	err := s.metaStore.SaveEvent(ctx, deviceID, data)
	if err != nil {
		s.logger.Error("Failed to save device to meta storage",
			zap.String("device_id", deviceID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to save device to meta storage: %w", err)
	}

	s.logger.Debug("Device saved to meta storage",
		zap.String("device_id", deviceID),
		zap.String("device_type", meta.Type),
	)

	return nil
}

// LoadDevice loads a device from meta storage
// Note: This returns the metadata, but not a fully functional Device instance.
// The device registry should use this metadata to recreate devices via plugins.
func (s *metaStorageDeviceBackend) LoadDevice(ctx context.Context, deviceID string) (types.Device, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("device ID cannot be empty")
	}

	data, found := s.metaStore.GetEvent(ctx, deviceID)
	if !found {
		s.logger.Debug("Device not found in meta storage",
			zap.String("device_id", deviceID),
		)
		return nil, fmt.Errorf("device %s not found in persistent storage", deviceID)
	}

	meta, err := mapToMetadata(data)
	if err != nil {
		s.logger.Error("Failed to parse device metadata from meta storage",
			zap.String("device_id", deviceID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to parse device metadata: %w", err)
	}

	s.logger.Debug("Device loaded from meta storage",
		zap.String("device_id", deviceID),
		zap.String("device_type", meta.Type),
	)

	// Note: We cannot recreate a full Device instance here because:
	// 1. Device is an interface with methods that require implementation
	// 2. Devices are created via DevicePlugin.CreateDevice()
	// 3. The registry should use this metadata to recreate devices via plugins
	//
	// For now, we return nil and let the caller handle device recreation.
	// In a future enhancement, we could return a DeviceMetadata struct or
	// use a factory pattern to recreate devices.
	return nil, fmt.Errorf("device recreation from metadata not yet implemented - use metadata to recreate via plugin")
}

// LoadAllDevices loads all devices from meta storage
// Returns device metadata that can be used to recreate devices via plugins
func (s *metaStorageDeviceBackend) LoadAllDevices(ctx context.Context) ([]types.Device, error) {
	// List all events (devices are stored as events with device IDs as event IDs)
	// Note: This is a limitation - we're using the event storage for devices.
	// In a future enhancement, we should add a dedicated devices bucket to meta storage.
	events, err := s.metaStore.ListEvents(ctx, map[string]interface{}{
		"type": "device", // Filter by type if we add that field
	})
	if err != nil {
		s.logger.Error("Failed to list devices from meta storage",
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to list devices from meta storage: %w", err)
	}

	// Convert events to device metadata
	// Note: Similar to LoadDevice, we cannot recreate full Device instances here
	devices := make([]types.Device, 0, len(events))
	for _, eventData := range events {
		meta, err := mapToMetadata(eventData)
		if err != nil {
			s.logger.Warn("Failed to parse device metadata",
				zap.Error(err),
			)
			continue
		}

		// For now, we return empty slice since we can't recreate devices
		// In a future enhancement, we should return metadata that can be used
		// to recreate devices via plugins
		_ = meta // Suppress unused variable warning
	}

	s.logger.Debug("Loaded devices from meta storage",
		zap.Int("device_count", len(devices)),
	)

	// Return empty for now - device recreation needs to be handled by the registry
	return devices, nil
}

// DeleteDevice deletes a device from meta storage
func (s *metaStorageDeviceBackend) DeleteDevice(ctx context.Context, deviceID string) error {
	if deviceID == "" {
		return fmt.Errorf("device ID cannot be empty")
	}

	err := s.metaStore.DeleteEvent(ctx, deviceID)
	if err != nil {
		s.logger.Error("Failed to delete device from meta storage",
			zap.String("device_id", deviceID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete device from meta storage: %w", err)
	}

	s.logger.Debug("Device deleted from meta storage",
		zap.String("device_id", deviceID),
	)

	return nil
}

