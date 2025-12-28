package processors

import (
	"context"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// MultiTypeProcessor processes multiple data types
// Example: logging processor, metrics processor, routing processor
type MultiTypeProcessor struct {
	*BaseProcessor
	processFunc func(context.Context, *types.DeviceData) (*types.DeviceData, error)
}

// NewMultiTypeProcessor creates a new multi-type processor
func NewMultiTypeProcessor(name string, supportedTypes []types.DeviceDataType, priority int, processFunc func(context.Context, *types.DeviceData) (*types.DeviceData, error)) *MultiTypeProcessor {
	return &MultiTypeProcessor{
		BaseProcessor: NewBaseProcessor(name, supportedTypes, priority),
		processFunc:   processFunc,
	}
}

// Process processes data using the provided function
func (p *MultiTypeProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	if !p.SupportsDataType(data.DataType) {
		return nil, fmt.Errorf("processor %s does not support data type %s", p.Name(), data.DataType)
	}

	if p.processFunc == nil {
		return data, nil
	}

	return p.processFunc(ctx, data)
}

// PassThroughProcessor is a processor that passes data through unchanged
// Useful for testing or as a placeholder
type PassThroughProcessor struct {
	*BaseProcessor
}

// NewPassThroughProcessor creates a new pass-through processor
func NewPassThroughProcessor(name string, supportedTypes []types.DeviceDataType, priority int) *PassThroughProcessor {
	return &PassThroughProcessor{
		BaseProcessor: NewBaseProcessor(name, supportedTypes, priority),
	}
}

// Process passes data through unchanged
func (p *PassThroughProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	return data, nil
}

// FilterProcessor is a processor that can filter (drop) data based on conditions
type FilterProcessor struct {
	*BaseProcessor
	filterFunc func(context.Context, *types.DeviceData) bool
}

// NewFilterProcessor creates a new filter processor
func NewFilterProcessor(name string, supportedTypes []types.DeviceDataType, priority int, filterFunc func(context.Context, *types.DeviceData) bool) *FilterProcessor {
	return &FilterProcessor{
		BaseProcessor: NewBaseProcessor(name, supportedTypes, priority),
		filterFunc:    filterFunc,
	}
}

// Process filters data based on the filter function
// Returns nil if data should be dropped, otherwise returns data unchanged
func (p *FilterProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	if !p.SupportsDataType(data.DataType) {
		return nil, fmt.Errorf("processor %s does not support data type %s", p.Name(), data.DataType)
	}

	if p.filterFunc == nil {
		return data, nil
	}

	// If filter returns false, drop the data (return nil)
	if !p.filterFunc(ctx, data) {
		return nil, nil
	}

	return data, nil
}

// TransformProcessor is a processor that transforms data
type TransformProcessor struct {
	*BaseProcessor
	transformFunc func(context.Context, *types.DeviceData) (*types.DeviceData, error)
}

// NewTransformProcessor creates a new transform processor
func NewTransformProcessor(name string, supportedTypes []types.DeviceDataType, priority int, transformFunc func(context.Context, *types.DeviceData) (*types.DeviceData, error)) *TransformProcessor {
	return &TransformProcessor{
		BaseProcessor:  NewBaseProcessor(name, supportedTypes, priority),
		transformFunc:  transformFunc,
	}
}

// Process transforms data using the transform function
func (p *TransformProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	if !p.SupportsDataType(data.DataType) {
		return nil, fmt.Errorf("processor %s does not support data type %s", p.Name(), data.DataType)
	}

	if p.transformFunc == nil {
		return data, nil
	}

	return p.transformFunc(ctx, data)
}

// TimestampEnrichmentProcessor enriches data with additional timestamp metadata
type TimestampEnrichmentProcessor struct {
	*BaseProcessor
}

// NewTimestampEnrichmentProcessor creates a new timestamp enrichment processor
func NewTimestampEnrichmentProcessor(name string, supportedTypes []types.DeviceDataType, priority int) *TimestampEnrichmentProcessor {
	return &TimestampEnrichmentProcessor{
		BaseProcessor: NewBaseProcessor(name, supportedTypes, priority),
	}
}

// Process enriches data with processing timestamp
func (p *TimestampEnrichmentProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	if !p.SupportsDataType(data.DataType) {
		return nil, fmt.Errorf("processor %s does not support data type %s", p.Name(), data.DataType)
	}

	// Create a copy to avoid modifying original
	enriched := *data
	if enriched.Metadata == nil {
		enriched.Metadata = make(map[string]interface{})
	}

	// Add processing timestamp
	enriched.Metadata["processed_at"] = time.Now().Unix()
	enriched.Metadata["processor"] = p.Name()

	return &enriched, nil
}

// ProcessorBuilder provides a fluent interface for building processors
type ProcessorBuilder struct {
	name           string
	supportedTypes []types.DeviceDataType
	priority       int
	processFunc    func(context.Context, *types.DeviceData) (*types.DeviceData, error)
}

// NewProcessorBuilder creates a new processor builder
func NewProcessorBuilder(name string) *ProcessorBuilder {
	return &ProcessorBuilder{
		name:           name,
		supportedTypes: []types.DeviceDataType{},
		priority:       100,
		processFunc:    func(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) { return data, nil },
	}
}

// WithSupportedTypes sets the supported data types
func (b *ProcessorBuilder) WithSupportedTypes(types ...types.DeviceDataType) *ProcessorBuilder {
	b.supportedTypes = types
	return b
}

// WithPriority sets the processor priority
func (b *ProcessorBuilder) WithPriority(priority int) *ProcessorBuilder {
	b.priority = priority
	return b
}

// WithProcessFunc sets the process function
func (b *ProcessorBuilder) WithProcessFunc(fn func(context.Context, *types.DeviceData) (*types.DeviceData, error)) *ProcessorBuilder {
	b.processFunc = fn
	return b
}

// Build builds a processor
func (b *ProcessorBuilder) Build() types.DataProcessor {
	return &builtProcessor{
		BaseProcessor: *NewBaseProcessor(b.name, b.supportedTypes, b.priority),
		processFunc:   b.processFunc,
	}
}

// builtProcessor is a processor built from a builder
type builtProcessor struct {
	BaseProcessor
	processFunc func(context.Context, *types.DeviceData) (*types.DeviceData, error)
}

// Process processes data using the provided function
func (p *builtProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	return p.processFunc(ctx, data)
}

