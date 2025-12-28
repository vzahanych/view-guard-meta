package processors

import (
	"context"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// BaseProcessor provides a base implementation of DataProcessor
// Processors can embed this to get default implementations
type BaseProcessor struct {
	name           string
	supportedTypes []types.DeviceDataType
	priority       int
	supportsDataType func(types.DeviceDataType) bool
}

// NewBaseProcessor creates a new base processor
func NewBaseProcessor(name string, supportedTypes []types.DeviceDataType, priority int) *BaseProcessor {
	return &BaseProcessor{
		name:           name,
		supportedTypes: supportedTypes,
		priority:       priority,
		supportsDataType: func(dt types.DeviceDataType) bool {
			for _, st := range supportedTypes {
				if st == dt {
					return true
				}
			}
			return false
		},
	}
}

// Name returns the processor name
func (p *BaseProcessor) Name() string {
	return p.name
}

// SupportsDataType returns whether this processor supports a specific data type
func (p *BaseProcessor) SupportsDataType(dataType types.DeviceDataType) bool {
	return p.supportsDataType(dataType)
}

// GetSupportedDataTypes returns all data types this processor supports
func (p *BaseProcessor) GetSupportedDataTypes() []types.DeviceDataType {
	return p.supportedTypes
}

// GetPriority returns the processor priority
func (p *BaseProcessor) GetPriority() int {
	return p.priority
}

// Process must be implemented by concrete processors
func (p *BaseProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	// Default implementation: pass-through
	return data, nil
}

