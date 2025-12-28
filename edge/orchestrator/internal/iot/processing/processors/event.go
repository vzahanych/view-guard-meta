package processors

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// EventDataProcessor processes event data
// Example: enrich events, route to different handlers, aggregate events
type EventDataProcessor struct {
	*BaseProcessor
	// Add processor-specific fields here
}

// NewEventDataProcessor creates a new event data processor
func NewEventDataProcessor(name string, priority int) *EventDataProcessor {
	return &EventDataProcessor{
		BaseProcessor: NewBaseProcessor(name, []types.DeviceDataType{types.DeviceDataTypeEvent}, priority),
	}
}

// Process processes event data
func (p *EventDataProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	if data.DataType != types.DeviceDataTypeEvent {
		return nil, fmt.Errorf("processor %s only supports events", p.Name())
	}

	// Default implementation: pass-through
	// Concrete processors should override this method
	return data, nil
}

