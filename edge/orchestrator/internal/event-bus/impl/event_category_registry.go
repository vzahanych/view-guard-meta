package impl

import (
	"sync"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"
)

// EventCategoryRegistry manages event type to category mappings.
// It provides thread-safe access to event categorization rules and supports dynamic registration.
type EventCategoryRegistry struct {
	mu            sync.RWMutex
	categoryRules map[types.EventType]types.EventCategory
	defaultCategory types.EventCategory
	logger        *zap.Logger
}

// NewEventCategoryRegistry creates a new event category registry with default categorization rules.
func NewEventCategoryRegistry(logger *zap.Logger) *EventCategoryRegistry {
	policy := types.DefaultEventDropPolicy()
	return &EventCategoryRegistry{
		categoryRules:   policy.CategoryRules,
		defaultCategory: policy.DefaultCategory,
		logger:         logger,
	}
}

// NewEventCategoryRegistryWithPolicy creates a new event category registry with a custom policy.
func NewEventCategoryRegistryWithPolicy(policy *types.EventDropPolicy, logger *zap.Logger) *EventCategoryRegistry {
	if policy == nil {
		policy = types.DefaultEventDropPolicy()
	}
	return &EventCategoryRegistry{
		categoryRules:   policy.CategoryRules,
		defaultCategory: policy.DefaultCategory,
		logger:         logger,
	}
}

// GetCategory returns the category for an event type.
// Returns the mapped category if found, otherwise returns the default category.
func (r *EventCategoryRegistry) GetCategory(eventType types.EventType) types.EventCategory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if category, ok := r.categoryRules[eventType]; ok {
		return category
	}
	return r.defaultCategory
}

// IsDroppable returns true if an event type can be dropped under storage pressure.
// Returns true if the event category is EventCategoryWorkflowTrigger.
// Returns false for operational/health/critical events.
func (r *EventCategoryRegistry) IsDroppable(eventType types.EventType) bool {
	category := r.GetCategory(eventType)
	return category.IsDroppable()
}

// RegisterEventCategory registers or updates the category for an event type.
// This allows dynamic registration of event categories at runtime.
func (r *EventCategoryRegistry) RegisterEventCategory(eventType types.EventType, category types.EventCategory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate category
	if category != types.EventCategoryWorkflowTrigger &&
		category != types.EventCategoryOperationalHealth &&
		category != types.EventCategoryCritical {
		if r.logger != nil {
			r.logger.Warn("Invalid event category, using default",
				zap.String("event_type", string(eventType)),
				zap.String("category", string(category)))
		}
		category = r.defaultCategory
	}

	// Warn if changing a critical or operational event to workflow trigger
	if oldCategory, exists := r.categoryRules[eventType]; exists {
		if (oldCategory == types.EventCategoryCritical || oldCategory == types.EventCategoryOperationalHealth) &&
			category == types.EventCategoryWorkflowTrigger {
			if r.logger != nil {
				r.logger.Warn("Changing non-droppable event to droppable category",
					zap.String("event_type", string(eventType)),
					zap.String("old_category", string(oldCategory)),
					zap.String("new_category", string(category)))
			}
		}
	}

	r.categoryRules[eventType] = category

	if r.logger != nil {
		r.logger.Debug("Registered event category",
			zap.String("event_type", string(eventType)),
			zap.String("category", string(category)))
	}
}

// SetDefaultCategory sets the default category for unmapped events.
func (r *EventCategoryRegistry) SetDefaultCategory(category types.EventCategory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate category
	if category != types.EventCategoryWorkflowTrigger &&
		category != types.EventCategoryOperationalHealth &&
		category != types.EventCategoryCritical {
		if r.logger != nil {
			r.logger.Warn("Invalid default category, keeping current",
				zap.String("category", string(category)))
		}
		return
	}

	r.defaultCategory = category

	if r.logger != nil {
		r.logger.Debug("Set default event category",
			zap.String("category", string(category)))
	}
}

// GetDefaultCategory returns the default category for unmapped events.
func (r *EventCategoryRegistry) GetDefaultCategory() types.EventCategory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultCategory
}

// GetAllCategories returns a copy of all category rules.
// This is useful for debugging and monitoring.
func (r *EventCategoryRegistry) GetAllCategories() map[types.EventType]types.EventCategory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[types.EventType]types.EventCategory, len(r.categoryRules))
	for k, v := range r.categoryRules {
		result[k] = v
	}
	return result
}

