package types

import (
	"context"
	"fmt"
	"strings"
)

// CapabilityRequirement represents a requirement for a device capability
// Used in capability negotiation and validation
type CapabilityRequirement struct {
	// Required capability
	Capability DeviceCapability `json:"capability"`

	// Optional indicates if this capability is optional (default: false, required)
	Optional bool `json:"optional,omitempty"`

	// Description of why this capability is needed
	Description string `json:"description,omitempty"`

	// Dependencies are capabilities that must be present if this capability is required
	Dependencies []DeviceCapability `json:"dependencies,omitempty"`
}

// CapabilityNegotiation represents a capability negotiation request/response
// Used to negotiate which capabilities a device should use
type CapabilityNegotiation struct {
	// Requested capabilities that the consumer needs
	Required []CapabilityRequirement `json:"required"`

	// Optional capabilities that would be nice to have
	Optional []CapabilityRequirement `json:"optional,omitempty"`

	// Negotiated capabilities (set after negotiation)
	Negotiated DeviceCapabilities `json:"negotiated,omitempty"`

	// Missing capabilities that were required but not available
	Missing []DeviceCapability `json:"missing,omitempty"`

	// Unavailable optional capabilities
	Unavailable []DeviceCapability `json:"unavailable,omitempty"`
}

// NegotiateCapabilities negotiates capabilities between a device and a set of requirements
// Returns a CapabilityNegotiation result indicating which capabilities are available
func NegotiateCapabilities(device Device, requirements []CapabilityRequirement) (*CapabilityNegotiation, error) {
	if device == nil {
		return nil, fmt.Errorf("device cannot be nil")
	}

	deviceCaps := device.GetCapabilities()
	negotiation := &CapabilityNegotiation{
		Required:     requirements,
		Negotiated:   make(DeviceCapabilities),
		Missing:      make([]DeviceCapability, 0),
		Unavailable:  make([]DeviceCapability, 0),
	}

	// Process required capabilities
	for _, req := range requirements {
		if req.Optional {
			negotiation.Optional = append(negotiation.Optional, req)
			continue
		}

		// Check if device has the required capability
		if deviceCaps.Has(req.Capability) {
			negotiation.Negotiated.Add(req.Capability)

			// Check dependencies
			for _, dep := range req.Dependencies {
				if !deviceCaps.Has(dep) {
					negotiation.Missing = append(negotiation.Missing, dep)
				} else {
					negotiation.Negotiated.Add(dep)
				}
			}
		} else {
			negotiation.Missing = append(negotiation.Missing, req.Capability)
		}
	}

	// Process optional capabilities
	for _, req := range negotiation.Optional {
		if deviceCaps.Has(req.Capability) {
			negotiation.Negotiated.Add(req.Capability)

			// Check dependencies
			for _, dep := range req.Dependencies {
				if deviceCaps.Has(dep) {
					negotiation.Negotiated.Add(dep)
				} else {
					negotiation.Unavailable = append(negotiation.Unavailable, dep)
				}
			}
		} else {
			negotiation.Unavailable = append(negotiation.Unavailable, req.Capability)
		}
	}

	// If any required capabilities are missing, return error
	if len(negotiation.Missing) > 0 {
		return negotiation, fmt.Errorf("device missing required capabilities: %v", negotiation.Missing)
	}

	return negotiation, nil
}

// ValidateCapabilityRequirements validates that a device meets all capability requirements
// Returns error if any required capability is missing
func ValidateCapabilityRequirements(device Device, requirements []CapabilityRequirement) error {
	negotiation, err := NegotiateCapabilities(device, requirements)
	if err != nil {
		return err
	}

	if len(negotiation.Missing) > 0 {
		return fmt.Errorf("device missing required capabilities: %v", negotiation.Missing)
	}

	return nil
}

// CapabilityGroup represents a group of related capabilities
// Used for organizing and querying capabilities by category
type CapabilityGroup string

const (
	// CapabilityGroupData represents data capture and streaming capabilities
	CapabilityGroupData CapabilityGroup = "data"

	// CapabilityGroupVideo represents video-specific capabilities
	CapabilityGroupVideo CapabilityGroup = "video"

	// CapabilityGroupAudio represents audio-specific capabilities
	CapabilityGroupAudio CapabilityGroup = "audio"

	// CapabilityGroupSensors represents sensor reading capabilities
	CapabilityGroupSensors CapabilityGroup = "sensors"

	// CapabilityGroupControl represents device control capabilities
	CapabilityGroupControl CapabilityGroup = "control"

	// CapabilityGroupAccess represents access control capabilities
	CapabilityGroupAccess CapabilityGroup = "access"

	// CapabilityGroupEvents represents event generation capabilities
	CapabilityGroupEvents CapabilityGroup = "events"
)

// GetCapabilityGroup returns the group that a capability belongs to
func GetCapabilityGroup(capability DeviceCapability) CapabilityGroup {
	switch capability {
	case DeviceCapabilityDataCapture, DeviceCapabilityDataStreaming:
		return CapabilityGroupData
	case DeviceCapabilityVideoCapture, DeviceCapabilityVideoStreaming, DeviceCapabilityVideoRecording, DeviceCapabilitySnapshot, DeviceCapabilityPTZ:
		return CapabilityGroupVideo
	case DeviceCapabilityAudioCapture, DeviceCapabilityAudioStreaming:
		return CapabilityGroupAudio
	case DeviceCapabilitySensorReadings:
		return CapabilityGroupSensors
	case DeviceCapabilityControl:
		return CapabilityGroupControl
	case DeviceCapabilityAccessControl:
		return CapabilityGroupAccess
	case DeviceCapabilityMotionDetection, DeviceCapabilityEventGeneration:
		return CapabilityGroupEvents
	default:
		return CapabilityGroupData // Default group
	}
}

// GetCapabilitiesByGroup returns all capabilities in a specific group
func GetCapabilitiesByGroup(group CapabilityGroup) []DeviceCapability {
	switch group {
	case CapabilityGroupData:
		return []DeviceCapability{
			DeviceCapabilityDataCapture,
			DeviceCapabilityDataStreaming,
		}
	case CapabilityGroupVideo:
		return []DeviceCapability{
			DeviceCapabilityVideoCapture,
			DeviceCapabilityVideoStreaming,
			DeviceCapabilityVideoRecording,
			DeviceCapabilitySnapshot,
			DeviceCapabilityPTZ,
		}
	case CapabilityGroupAudio:
		return []DeviceCapability{
			DeviceCapabilityAudioCapture,
			DeviceCapabilityAudioStreaming,
		}
	case CapabilityGroupSensors:
		return []DeviceCapability{
			DeviceCapabilitySensorReadings,
		}
	case CapabilityGroupControl:
		return []DeviceCapability{
			DeviceCapabilityControl,
		}
	case CapabilityGroupAccess:
		return []DeviceCapability{
			DeviceCapabilityAccessControl,
		}
	case CapabilityGroupEvents:
		return []DeviceCapability{
			DeviceCapabilityMotionDetection,
			DeviceCapabilityEventGeneration,
		}
	default:
		return []DeviceCapability{}
	}
}

// CapabilityQuery represents a query for devices with specific capabilities
type CapabilityQuery struct {
	// Required capabilities (all must be present)
	Required []DeviceCapability `json:"required,omitempty"`

	// AnyOf capabilities (at least one must be present)
	AnyOf []DeviceCapability `json:"any_of,omitempty"`

	// Excluded capabilities (none should be present)
	Excluded []DeviceCapability `json:"excluded,omitempty"`

	// Group filter (devices must have at least one capability from this group)
	Group *CapabilityGroup `json:"group,omitempty"`

	// MinCapabilities is the minimum number of capabilities the device must have
	MinCapabilities *int `json:"min_capabilities,omitempty"`
}

// Matches checks if a device matches the capability query
func (q *CapabilityQuery) Matches(device Device) bool {
	if device == nil {
		return false
	}

	deviceCaps := device.GetCapabilities()

	// Check required capabilities
	for _, req := range q.Required {
		if !deviceCaps.Has(req) {
			return false
		}
	}

	// Check excluded capabilities
	for _, excl := range q.Excluded {
		if deviceCaps.Has(excl) {
			return false
		}
	}

	// Check any-of capabilities
	if len(q.AnyOf) > 0 {
		hasAny := false
		for _, cap := range q.AnyOf {
			if deviceCaps.Has(cap) {
				hasAny = true
				break
			}
		}
		if !hasAny {
			return false
		}
	}

	// Check group filter
	if q.Group != nil {
		groupCaps := GetCapabilitiesByGroup(*q.Group)
		hasGroupCap := false
		for _, cap := range groupCaps {
			if deviceCaps.Has(cap) {
				hasGroupCap = true
				break
			}
		}
		if !hasGroupCap {
			return false
		}
	}

	// Check minimum capabilities
	if q.MinCapabilities != nil {
		if len(deviceCaps) < *q.MinCapabilities {
			return false
		}
	}

	return true
}

// QueryDevicesByCapability queries devices from a registry using a capability query
func QueryDevicesByCapability(ctx context.Context, registry DeviceRegistry, query *CapabilityQuery) ([]Device, error) {
	if registry == nil {
		return nil, fmt.Errorf("device registry cannot be nil")
	}
	if query == nil {
		return nil, fmt.Errorf("capability query cannot be nil")
	}

	// Get all devices
	allDevices, err := registry.ListDevices(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}

	// Filter devices by query
	matchingDevices := make([]Device, 0)
	for _, device := range allDevices {
		if query.Matches(device) {
			matchingDevices = append(matchingDevices, device)
		}
	}

	return matchingDevices, nil
}

// CapabilityFilter provides utility functions for filtering devices by capabilities
type CapabilityFilter struct{}

// NewCapabilityFilter creates a new capability filter
func NewCapabilityFilter() *CapabilityFilter {
	return &CapabilityFilter{}
}

// WithRequired creates a query that requires specific capabilities
func (f *CapabilityFilter) WithRequired(capabilities ...DeviceCapability) *CapabilityQuery {
	return &CapabilityQuery{
		Required: capabilities,
	}
}

// WithAnyOf creates a query that requires at least one of the specified capabilities
func (f *CapabilityFilter) WithAnyOf(capabilities ...DeviceCapability) *CapabilityQuery {
	return &CapabilityQuery{
		AnyOf: capabilities,
	}
}

// WithExcluded creates a query that excludes devices with specific capabilities
func (f *CapabilityFilter) WithExcluded(capabilities ...DeviceCapability) *CapabilityQuery {
	return &CapabilityQuery{
		Excluded: capabilities,
	}
}

// WithGroup creates a query that requires at least one capability from a group
func (f *CapabilityFilter) WithGroup(group CapabilityGroup) *CapabilityQuery {
	return &CapabilityQuery{
		Group: &group,
	}
}

// WithMinCapabilities creates a query that requires a minimum number of capabilities
func (f *CapabilityFilter) WithMinCapabilities(min int) *CapabilityQuery {
	return &CapabilityQuery{
		MinCapabilities: &min,
	}
}

// Combine combines multiple queries with AND logic
func (f *CapabilityFilter) Combine(queries ...*CapabilityQuery) *CapabilityQuery {
	combined := &CapabilityQuery{}

	for _, q := range queries {
		if q == nil {
			continue
		}

		// Merge required capabilities
		combined.Required = append(combined.Required, q.Required...)

		// Merge any-of capabilities
		combined.AnyOf = append(combined.AnyOf, q.AnyOf...)

		// Merge excluded capabilities
		combined.Excluded = append(combined.Excluded, q.Excluded...)

		// Use first group if multiple are specified
		if combined.Group == nil && q.Group != nil {
			combined.Group = q.Group
		}

		// Use maximum min capabilities if multiple are specified
		if q.MinCapabilities != nil {
			if combined.MinCapabilities == nil || *q.MinCapabilities > *combined.MinCapabilities {
				combined.MinCapabilities = q.MinCapabilities
			}
		}
	}

	return combined
}

// CapabilityDependency represents a dependency relationship between capabilities
// Some capabilities may require other capabilities to function
type CapabilityDependency struct {
	// Capability that has the dependency
	Capability DeviceCapability `json:"capability"`

	// Required capabilities that must be present
	Requires []DeviceCapability `json:"requires"`

	// Optional capabilities that enhance functionality
	Enhances []DeviceCapability `json:"enhances,omitempty"`
}

// KnownCapabilityDependencies returns known capability dependencies
// This helps validate that devices have all required dependencies
func KnownCapabilityDependencies() []CapabilityDependency {
	return []CapabilityDependency{
		{
			Capability: DeviceCapabilityPTZ,
			Requires:   []DeviceCapability{DeviceCapabilityControl},
		},
		{
			Capability: DeviceCapabilityVideoStreaming,
			Requires:   []DeviceCapability{DeviceCapabilityVideoCapture},
		},
		{
			Capability: DeviceCapabilityVideoRecording,
			Requires:   []DeviceCapability{DeviceCapabilityVideoCapture},
		},
		{
			Capability: DeviceCapabilitySnapshot,
			Requires:   []DeviceCapability{DeviceCapabilityVideoCapture},
		},
		{
			Capability: DeviceCapabilityAudioStreaming,
			Requires:   []DeviceCapability{DeviceCapabilityAudioCapture},
		},
		{
			Capability: DeviceCapabilityDataStreaming,
			Requires:   []DeviceCapability{DeviceCapabilityDataCapture},
		},
		{
			Capability: DeviceCapabilityAccessControl,
			Requires:   []DeviceCapability{DeviceCapabilityControl},
		},
	}
}

// ValidateCapabilityDependencies validates that a device has all required capability dependencies
// Returns error if any required dependency is missing
func ValidateCapabilityDependencies(device Device) error {
	if device == nil {
		return fmt.Errorf("device cannot be nil")
	}

	deviceCaps := device.GetCapabilities()
	dependencies := KnownCapabilityDependencies()

	for _, dep := range dependencies {
		// If device has this capability, check its dependencies
		if deviceCaps.Has(dep.Capability) {
			for _, req := range dep.Requires {
				if !deviceCaps.Has(req) {
					return fmt.Errorf("capability %s requires %s, but device does not have it", dep.Capability, req)
				}
			}
		}
	}

	return nil
}

// GetCapabilityDescription returns a human-readable description of a capability
func GetCapabilityDescription(capability DeviceCapability) string {
	descriptions := map[DeviceCapability]string{
		DeviceCapabilityDataCapture:      "Device can capture data (frames, samples, etc.)",
		DeviceCapabilityDataStreaming:     "Device can stream continuous data",
		DeviceCapabilityVideoCapture:     "Device can capture video frames",
		DeviceCapabilityVideoStreaming:    "Device can stream video",
		DeviceCapabilityVideoRecording:   "Device can record video clips",
		DeviceCapabilitySnapshot:          "Device can capture snapshots/images",
		DeviceCapabilityAudioCapture:      "Device can capture audio",
		DeviceCapabilityAudioStreaming:    "Device can stream audio",
		DeviceCapabilitySensorReadings:    "Device can read sensor values",
		DeviceCapabilityControl:           "Device can be controlled (e.g., PTZ, door lock)",
		DeviceCapabilityAccessControl:     "Device can control access (locks, gates)",
		DeviceCapabilityPTZ:               "Camera supports PTZ (Pan-Tilt-Zoom) control",
		DeviceCapabilityMotionDetection:   "Device has built-in motion detection",
		DeviceCapabilityEventGeneration:   "Device can generate events (door open, motion detected, etc.)",
	}

	if desc, ok := descriptions[capability]; ok {
		return desc
	}
	return string(capability)
}

// GetCapabilityName returns a human-readable name for a capability
func GetCapabilityName(capability DeviceCapability) string {
	names := map[DeviceCapability]string{
		DeviceCapabilityDataCapture:      "Data Capture",
		DeviceCapabilityDataStreaming:    "Data Streaming",
		DeviceCapabilityVideoCapture:     "Video Capture",
		DeviceCapabilityVideoStreaming:   "Video Streaming",
		DeviceCapabilityVideoRecording:   "Video Recording",
		DeviceCapabilitySnapshot:         "Snapshot",
		DeviceCapabilityAudioCapture:     "Audio Capture",
		DeviceCapabilityAudioStreaming:   "Audio Streaming",
		DeviceCapabilitySensorReadings:   "Sensor Readings",
		DeviceCapabilityControl:          "Control",
		DeviceCapabilityAccessControl:    "Access Control",
		DeviceCapabilityPTZ:              "PTZ (Pan-Tilt-Zoom)",
		DeviceCapabilityMotionDetection:  "Motion Detection",
		DeviceCapabilityEventGeneration:  "Event Generation",
	}

	if name, ok := names[capability]; ok {
		return name
	}
	// Convert snake_case to Title Case
	parts := strings.Split(string(capability), "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, " ")
}

// CapabilitySummary provides a summary of device capabilities
type CapabilitySummary struct {
	// Total number of capabilities
	Total int `json:"total"`

	// Capabilities by group
	ByGroup map[CapabilityGroup]int `json:"by_group"`

	// List of all capabilities
	Capabilities []DeviceCapability `json:"capabilities"`

	// Groups present
	Groups []CapabilityGroup `json:"groups"`
}

// GetCapabilitySummary returns a summary of a device's capabilities
func GetCapabilitySummary(device Device) *CapabilitySummary {
	if device == nil {
		return &CapabilitySummary{
			ByGroup:     make(map[CapabilityGroup]int),
			Capabilities: []DeviceCapability{},
			Groups:      []CapabilityGroup{},
		}
	}

	deviceCaps := device.GetCapabilities()
	summary := &CapabilitySummary{
		Total:        len(deviceCaps),
		ByGroup:      make(map[CapabilityGroup]int),
		Capabilities: make([]DeviceCapability, 0, len(deviceCaps)),
		Groups:       make([]CapabilityGroup, 0),
	}

	groupSet := make(map[CapabilityGroup]bool)

	for cap := range deviceCaps {
		summary.Capabilities = append(summary.Capabilities, cap)
		group := GetCapabilityGroup(cap)
		summary.ByGroup[group]++
		if !groupSet[group] {
			summary.Groups = append(summary.Groups, group)
			groupSet[group] = true
		}
	}

	return summary
}

