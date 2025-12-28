package types

import "context"

// DeviceRegistry is an interface for managing device discovery and registration
//
//go:generate go run go.uber.org/mock/mockgen -destination=../mocks/mock_device_registry.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types DeviceRegistry
type DeviceRegistry interface {
	// DiscoverDevices discovers devices of a specific type
	DiscoverDevices(ctx context.Context, deviceType DeviceType) ([]Device, error)

	// DiscoverAllDevices discovers all supported device types
	DiscoverAllDevices(ctx context.Context) ([]Device, error)

	// RegisterDevice registers a discovered device
	RegisterDevice(ctx context.Context, device Device) error

	// GetDevice retrieves a device by ID
	GetDevice(ctx context.Context, deviceID string) (Device, error)

	// ListDevices lists all registered devices, optionally filtered by type or capability
	ListDevices(ctx context.Context, filters *DeviceFilters) ([]Device, error)

	// UpdateDevice updates device metadata
	UpdateDevice(ctx context.Context, deviceID string, updates *DeviceMetadataUpdate) error

	// DeleteDevice removes a device from the registry
	DeleteDevice(ctx context.Context, deviceID string) error

	// GetDevicesByCapability returns all devices that support a specific capability
	GetDevicesByCapability(ctx context.Context, capability DeviceCapability) ([]Device, error)

	// GetDevicesByType returns all devices of a specific type
	GetDevicesByType(ctx context.Context, deviceType DeviceType) ([]Device, error)
}

