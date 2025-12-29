package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventCategory_String(t *testing.T) {
	tests := []struct {
		name     string
		category EventCategory
		expected string
	}{
		{
			name:     "Workflow trigger",
			category: EventCategoryWorkflowTrigger,
			expected: "workflow_trigger",
		},
		{
			name:     "Operational health",
			category: EventCategoryOperationalHealth,
			expected: "operational_health",
		},
		{
			name:     "Critical",
			category: EventCategoryCritical,
			expected: "critical",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.category.String())
		})
	}
}

func TestEventCategory_IsDroppable(t *testing.T) {
	tests := []struct {
		name     string
		category EventCategory
		expected bool
	}{
		{
			name:     "Workflow trigger is droppable",
			category: EventCategoryWorkflowTrigger,
			expected: true,
		},
		{
			name:     "Operational health is not droppable",
			category: EventCategoryOperationalHealth,
			expected: false,
		},
		{
			name:     "Critical is not droppable",
			category: EventCategoryCritical,
			expected: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.category.IsDroppable())
		})
	}
}

func TestEventDropPolicy_GetCategory(t *testing.T) {
	policy := &EventDropPolicy{
		CategoryRules: map[EventType]EventCategory{
			EventTypeDeviceDiscovered: EventCategoryWorkflowTrigger,
			EventTypeStorageFull:      EventCategoryOperationalHealth,
			EventTypeStorageCorruptionDetected: EventCategoryCritical,
		},
		DefaultCategory: EventCategoryWorkflowTrigger,
	}
	
	tests := []struct {
		name      string
		eventType EventType
		expected  EventCategory
	}{
		{
			name:      "Mapped event type",
			eventType: EventTypeDeviceDiscovered,
			expected:  EventCategoryWorkflowTrigger,
		},
		{
			name:      "Mapped operational health event",
			eventType: EventTypeStorageFull,
			expected:  EventCategoryOperationalHealth,
		},
		{
			name:      "Mapped critical event",
			eventType: EventTypeStorageCorruptionDetected,
			expected:  EventCategoryCritical,
		},
		{
			name:      "Unmapped event uses default",
			eventType: EventType("unknown.event"),
			expected:  EventCategoryWorkflowTrigger,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := policy.GetCategory(tt.eventType)
			assert.Equal(t, tt.expected, category)
		})
	}
}

func TestEventDropPolicy_IsDroppable(t *testing.T) {
	policy := &EventDropPolicy{
		CategoryRules: map[EventType]EventCategory{
			EventTypeDeviceDiscovered: EventCategoryWorkflowTrigger,
			EventTypeStorageFull:      EventCategoryOperationalHealth,
			EventTypeStorageCorruptionDetected: EventCategoryCritical,
		},
		DefaultCategory: EventCategoryWorkflowTrigger,
	}
	
	tests := []struct {
		name      string
		eventType EventType
		expected  bool
	}{
		{
			name:      "Workflow trigger is droppable",
			eventType: EventTypeDeviceDiscovered,
			expected:  true,
		},
		{
			name:      "Operational health is not droppable",
			eventType: EventTypeStorageFull,
			expected:  false,
		},
		{
			name:      "Critical is not droppable",
			eventType: EventTypeStorageCorruptionDetected,
			expected:  false,
		},
		{
			name:      "Unmapped event uses default (droppable)",
			eventType: EventType("unknown.event"),
			expected:  true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDroppable := policy.IsDroppable(tt.eventType)
			assert.Equal(t, tt.expected, isDroppable)
		})
	}
}

func TestDefaultEventDropPolicy(t *testing.T) {
	policy := DefaultEventDropPolicy()
	require.NotNil(t, policy)
	require.NotNil(t, policy.CategoryRules)
	
	// Test that all known event types are categorized
	tests := []struct {
		name      string
		eventType EventType
		expected  EventCategory
	}{
		// Workflow trigger events
		{
			name:      "Device discovered",
			eventType: EventTypeDeviceDiscovered,
			expected:  EventCategoryWorkflowTrigger,
		},
		{
			name:      "Device registered",
			eventType: EventTypeDeviceRegistered,
			expected:  EventCategoryWorkflowTrigger,
		},
		{
			name:      "Data unit saved",
			eventType: EventTypeDataUnitSaved,
			expected:  EventCategoryWorkflowTrigger,
		},
		{
			name:      "Network tunnel connected",
			eventType: EventTypeNetworkTunnelConnected,
			expected:  EventCategoryWorkflowTrigger,
		},
		{
			name:      "Edge authenticated",
			eventType: EventTypeEdgeAuthenticated,
			expected:  EventCategoryWorkflowTrigger,
		},
		
		// Operational health events
		{
			name:      "Storage full",
			eventType: EventTypeStorageFull,
			expected:  EventCategoryOperationalHealth,
		},
		{
			name:      "Storage warning",
			eventType: EventTypeStorageWarning,
			expected:  EventCategoryOperationalHealth,
		},
		{
			name:      "Storage cleanup started",
			eventType: EventTypeStorageCleanupStarted,
			expected:  EventCategoryOperationalHealth,
		},
		
		// Critical events
		{
			name:      "Storage corruption detected",
			eventType: EventTypeStorageCorruptionDetected,
			expected:  EventCategoryCritical,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := policy.GetCategory(tt.eventType)
			assert.Equal(t, tt.expected, category, "Event type %s should be categorized as %s", tt.eventType, tt.expected)
		})
	}
	
	// Test default category
	assert.Equal(t, EventCategoryWorkflowTrigger, policy.DefaultCategory)
	
	// Test that unmapped events use default
	unknownEvent := EventType("unknown.event.type")
	category := policy.GetCategory(unknownEvent)
	assert.Equal(t, EventCategoryWorkflowTrigger, category)
}

func TestDefaultEventDropPolicy_IsDroppable(t *testing.T) {
	policy := DefaultEventDropPolicy()
	
	tests := []struct {
		name      string
		eventType EventType
		expected  bool
	}{
		{
			name:      "Workflow trigger is droppable",
			eventType: EventTypeDeviceDiscovered,
			expected:  true,
		},
		{
			name:      "Operational health is not droppable",
			eventType: EventTypeStorageFull,
			expected:  false,
		},
		{
			name:      "Critical is not droppable",
			eventType: EventTypeStorageCorruptionDetected,
			expected:  false,
		},
		{
			name:      "Unknown event uses default (droppable)",
			eventType: EventType("unknown.event"),
			expected:  true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDroppable := policy.IsDroppable(tt.eventType)
			assert.Equal(t, tt.expected, isDroppable)
		})
	}
}

func TestEventDropPolicy_CustomDefaultCategory(t *testing.T) {
	policy := &EventDropPolicy{
		CategoryRules: map[EventType]EventCategory{
			EventTypeDeviceDiscovered: EventCategoryWorkflowTrigger,
		},
		DefaultCategory: EventCategoryOperationalHealth, // Custom default
	}
	
	// Unmapped event should use custom default
	category := policy.GetCategory(EventType("unknown.event"))
	assert.Equal(t, EventCategoryOperationalHealth, category)
	
	// Unmapped event should not be droppable (operational health)
	isDroppable := policy.IsDroppable(EventType("unknown.event"))
	assert.False(t, isDroppable)
}

