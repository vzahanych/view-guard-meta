package impl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap/zaptest"
)

func TestNewEventCategoryRegistry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := NewEventCategoryRegistry(logger)

	require.NotNil(t, registry)
	
	// Test that default policy is applied
	category := registry.GetCategory(types.EventTypeDeviceDiscovered)
	assert.Equal(t, types.EventCategoryWorkflowTrigger, category)
	
	// Test default category
	defaultCategory := registry.GetDefaultCategory()
	assert.Equal(t, types.EventCategoryWorkflowTrigger, defaultCategory)
}

func TestNewEventCategoryRegistryWithPolicy(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	// Test with custom policy
	customPolicy := &types.EventDropPolicy{
		CategoryRules: map[types.EventType]types.EventCategory{
			types.EventTypeDeviceDiscovered: types.EventCategoryOperationalHealth,
		},
		DefaultCategory: types.EventCategoryCritical,
	}
	
	registry := NewEventCategoryRegistryWithPolicy(customPolicy, logger)
	require.NotNil(t, registry)
	
	// Test custom category
	category := registry.GetCategory(types.EventTypeDeviceDiscovered)
	assert.Equal(t, types.EventCategoryOperationalHealth, category)
	
	// Test default category
	defaultCategory := registry.GetDefaultCategory()
	assert.Equal(t, types.EventCategoryCritical, defaultCategory)
	
	// Test with nil policy (should use defaults)
	registry2 := NewEventCategoryRegistryWithPolicy(nil, logger)
	require.NotNil(t, registry2)
	category2 := registry2.GetCategory(types.EventTypeDeviceDiscovered)
	assert.Equal(t, types.EventCategoryWorkflowTrigger, category2)
}

func TestEventCategoryRegistry_GetCategory(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := NewEventCategoryRegistry(logger)
	
	tests := []struct {
		name      string
		eventType types.EventType
		expected  types.EventCategory
	}{
		{
			name:      "Workflow trigger event",
			eventType: types.EventTypeDeviceDiscovered,
			expected:  types.EventCategoryWorkflowTrigger,
		},
		{
			name:      "Operational health event",
			eventType: types.EventTypeStorageFull,
			expected:  types.EventCategoryOperationalHealth,
		},
		{
			name:      "Critical event",
			eventType: types.EventTypeStorageCorruptionDetected,
			expected:  types.EventCategoryCritical,
		},
		{
			name:      "Unknown event type uses default",
			eventType: types.EventType("unknown.event"),
			expected:  types.EventCategoryWorkflowTrigger,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := registry.GetCategory(tt.eventType)
			assert.Equal(t, tt.expected, category)
		})
	}
}

func TestEventCategoryRegistry_IsDroppable(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := NewEventCategoryRegistry(logger)
	
	tests := []struct {
		name      string
		eventType types.EventType
		expected  bool
	}{
		{
			name:      "Workflow trigger is droppable",
			eventType: types.EventTypeDeviceDiscovered,
			expected:  true,
		},
		{
			name:      "Operational health is not droppable",
			eventType: types.EventTypeStorageFull,
			expected:  false,
		},
		{
			name:      "Critical is not droppable",
			eventType: types.EventTypeStorageCorruptionDetected,
			expected:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDroppable := registry.IsDroppable(tt.eventType)
			assert.Equal(t, tt.expected, isDroppable)
		})
	}
}

func TestEventCategoryRegistry_RegisterEventCategory(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := NewEventCategoryRegistry(logger)
	
	// Register a new category
	registry.RegisterEventCategory(types.EventType("custom.event"), types.EventCategoryOperationalHealth)
	category := registry.GetCategory(types.EventType("custom.event"))
	assert.Equal(t, types.EventCategoryOperationalHealth, category)
	
	// Update existing category
	registry.RegisterEventCategory(types.EventTypeDeviceDiscovered, types.EventCategoryOperationalHealth)
	category = registry.GetCategory(types.EventTypeDeviceDiscovered)
	assert.Equal(t, types.EventCategoryOperationalHealth, category)
	
	// Test invalid category (should use default)
	registry.RegisterEventCategory(types.EventType("test.event"), types.EventCategory("invalid"))
	category = registry.GetCategory(types.EventType("test.event"))
	assert.Equal(t, types.EventCategoryWorkflowTrigger, category) // Should use default
}

func TestEventCategoryRegistry_SetDefaultCategory(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := NewEventCategoryRegistry(logger)
	
	// Set default to operational health
	registry.SetDefaultCategory(types.EventCategoryOperationalHealth)
	defaultCategory := registry.GetDefaultCategory()
	assert.Equal(t, types.EventCategoryOperationalHealth, defaultCategory)
	
	// Unknown event should use new default
	category := registry.GetCategory(types.EventType("unknown.event"))
	assert.Equal(t, types.EventCategoryOperationalHealth, category)
	
	// Test invalid default category (should not change)
	registry.SetDefaultCategory(types.EventCategory("invalid"))
	defaultCategory = registry.GetDefaultCategory()
	assert.Equal(t, types.EventCategoryOperationalHealth, defaultCategory) // Should remain unchanged
}

func TestEventCategoryRegistry_GetAllCategories(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := NewEventCategoryRegistry(logger)
	
	categories := registry.GetAllCategories()
	require.NotNil(t, categories)
	
	// Should contain default policy categories
	assert.Contains(t, categories, types.EventTypeDeviceDiscovered)
	assert.Equal(t, types.EventCategoryWorkflowTrigger, categories[types.EventTypeDeviceDiscovered])
	
	// Register a new category
	registry.RegisterEventCategory(types.EventType("custom.event"), types.EventCategoryCritical)
	
	// Get all categories again (should include new one)
	categories2 := registry.GetAllCategories()
	assert.Contains(t, categories2, types.EventType("custom.event"))
	assert.Equal(t, types.EventCategoryCritical, categories2[types.EventType("custom.event")])
	
	// Verify it's a copy (modifying shouldn't affect registry)
	categories2[types.EventType("test")] = types.EventCategoryOperationalHealth
	categories3 := registry.GetAllCategories()
	assert.NotContains(t, categories3, types.EventType("test"))
}

func TestEventCategoryRegistry_ThreadSafety(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := NewEventCategoryRegistry(logger)
	
	// Concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = registry.GetCategory(types.EventTypeDeviceDiscovered)
				_ = registry.IsDroppable(types.EventTypeDeviceDiscovered)
			}
			done <- true
		}()
	}
	
	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				registry.RegisterEventCategory(
					types.EventType("test.event"),
					types.EventCategoryOperationalHealth,
				)
			}
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
	
	// Verify final state
	category := registry.GetCategory(types.EventType("test.event"))
	assert.Equal(t, types.EventCategoryOperationalHealth, category)
}

