package iot

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DataProcessor is an interface for processing device data
// Processors can transform, filter, analyze, or route device data
//
//go:generate go run go.uber.org/mock/mockgen -destination=mocks/mock_data_processor.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot DataProcessor
type DataProcessor interface {
	// Name returns the name of the processor
	Name() string

	// Process processes device data and returns transformed data or error
	// The processor can:
	//   - Transform data (e.g., resize image, normalize sensor values)
	//   - Filter data (return nil to drop data)
	//   - Analyze data (e.g., detect anomalies, classify events)
	//   - Route data to external systems
	//   - Return the same data unchanged (pass-through)
	Process(ctx context.Context, data *DeviceData) (*DeviceData, error)

	// SupportsDataType returns whether this processor supports a specific data type
	SupportsDataType(dataType DeviceDataType) bool

	// GetSupportedDataTypes returns all data types this processor supports
	GetSupportedDataTypes() []DeviceDataType

	// GetPriority returns the processor priority (lower = earlier in pipeline)
	GetPriority() int
}

// DataProcessorRegistry is an interface for managing data processors
//
//go:generate go run go.uber.org/mock/mockgen -destination=mocks/mock_data_processor_registry.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot DataProcessorRegistry
type DataProcessorRegistry interface {
	// RegisterProcessor registers a data processor
	RegisterProcessor(processor DataProcessor) error

	// UnregisterProcessor unregisters a processor by name
	UnregisterProcessor(name string) error

	// GetProcessor retrieves a processor by name
	GetProcessor(name string) (DataProcessor, error)

	// ListProcessors returns all registered processors, optionally filtered by data type
	ListProcessors(dataType *DeviceDataType) []DataProcessor

	// GetProcessorsForDataType returns processors that support a specific data type, sorted by priority
	GetProcessorsForDataType(dataType DeviceDataType) []DataProcessor
}

// dataProcessorRegistryImpl is the default implementation of DataProcessorRegistry
type dataProcessorRegistryImpl struct {
	// processors maps processor name to processor
	processors map[string]DataProcessor

	// processorsByType maps data type to list of processors (for efficient lookup)
	processorsByType map[DeviceDataType][]DataProcessor

	// mu protects the processors maps
	mu sync.RWMutex
}

// NewDataProcessorRegistry creates a new data processor registry
func NewDataProcessorRegistry() DataProcessorRegistry {
	return &dataProcessorRegistryImpl{
		processors:       make(map[string]DataProcessor),
		processorsByType: make(map[DeviceDataType][]DataProcessor),
	}
}

// RegisterProcessor registers a data processor
func (r *dataProcessorRegistryImpl) RegisterProcessor(processor DataProcessor) error {
	if processor == nil {
		return fmt.Errorf("processor cannot be nil")
	}

	name := processor.Name()
	if name == "" {
		return fmt.Errorf("processor name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if processor is already registered
	if _, exists := r.processors[name]; exists {
		return fmt.Errorf("processor with name %s is already registered", name)
	}

	// Register processor
	r.processors[name] = processor

	// Add to type index
	for _, dataType := range processor.GetSupportedDataTypes() {
		r.processorsByType[dataType] = append(r.processorsByType[dataType], processor)
		// Sort by priority
		r.sortProcessorsByPriority(dataType)
	}

	return nil
}

// UnregisterProcessor unregisters a processor by name
func (r *dataProcessorRegistryImpl) UnregisterProcessor(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	processor, exists := r.processors[name]
	if !exists {
		return fmt.Errorf("processor with name %s is not registered", name)
	}

	// Remove from processors map
	delete(r.processors, name)

	// Remove from type index
	for dataType, processors := range r.processorsByType {
		for i, p := range processors {
			if p.Name() == name {
				r.processorsByType[dataType] = append(processors[:i], processors[i+1:]...)
				break
			}
		}
	}

	_ = processor // Suppress unused variable warning
	return nil
}

// GetProcessor retrieves a processor by name
func (r *dataProcessorRegistryImpl) GetProcessor(name string) (DataProcessor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processor, exists := r.processors[name]
	if !exists {
		return nil, fmt.Errorf("processor with name %s is not registered", name)
	}

	return processor, nil
}

// ListProcessors returns all registered processors, optionally filtered by data type
func (r *dataProcessorRegistryImpl) ListProcessors(dataType *DeviceDataType) []DataProcessor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if dataType != nil {
		processors := r.processorsByType[*dataType]
		result := make([]DataProcessor, len(processors))
		copy(result, processors)
		return result
	}

	// Return all processors
	result := make([]DataProcessor, 0, len(r.processors))
	for _, processor := range r.processors {
		result = append(result, processor)
	}
	return result
}

// GetProcessorsForDataType returns processors that support a specific data type, sorted by priority
func (r *dataProcessorRegistryImpl) GetProcessorsForDataType(dataType DeviceDataType) []DataProcessor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processors := r.processorsByType[dataType]
	result := make([]DataProcessor, len(processors))
	copy(result, processors)
	return result
}

// sortProcessorsByPriority sorts processors by priority (lower priority = earlier in pipeline)
func (r *dataProcessorRegistryImpl) sortProcessorsByPriority(dataType DeviceDataType) {
	processors := r.processorsByType[dataType]
	for i := 1; i < len(processors); i++ {
		key := processors[i]
		j := i - 1
		for j >= 0 && processors[j].GetPriority() > key.GetPriority() {
			processors[j+1] = processors[j]
			j--
		}
		processors[j+1] = key
	}
}

// DataPipeline processes device data through a series of processors
type DataPipeline struct {
	registry DataProcessorRegistry
}

// NewDataPipeline creates a new data pipeline
func NewDataPipeline(registry DataProcessorRegistry) *DataPipeline {
	return &DataPipeline{
		registry: registry,
	}
}

// Process processes device data through the pipeline
// Data flows through processors in priority order (lower priority = earlier)
// If a processor returns nil, the data is dropped and processing stops
// If a processor returns an error, processing stops and the error is returned
func (p *DataPipeline) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
	if data == nil {
		return nil, fmt.Errorf("data cannot be nil")
	}

	// Get processors for this data type, sorted by priority
	processors := p.registry.GetProcessorsForDataType(data.DataType)
	if len(processors) == 0 {
		// No processors registered, return data unchanged
		return data, nil
	}

	// Process data through each processor in order
	currentData := data
	for _, processor := range processors {
		// Check if processor supports this data type (double-check)
		if !processor.SupportsDataType(currentData.DataType) {
			continue
		}

		// Process data
		processedData, err := processor.Process(ctx, currentData)
		if err != nil {
			return nil, fmt.Errorf("processor %s failed: %w", processor.Name(), err)
		}

		// If processor returns nil, data is dropped
		if processedData == nil {
			return nil, nil
		}

		currentData = processedData
	}

	return currentData, nil
}

// ProcessBatch processes multiple device data items through the pipeline
// Returns processed data and errors for each item
func (p *DataPipeline) ProcessBatch(ctx context.Context, dataItems []*DeviceData) ([]*DeviceData, []error) {
	results := make([]*DeviceData, 0, len(dataItems))
	errors := make([]error, 0)

	for _, data := range dataItems {
		processed, err := p.Process(ctx, data)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		if processed != nil {
			results = append(results, processed)
		}
		// If processed is nil, data was dropped (not an error)
	}

	return results, errors
}

// DataProcessingContext provides context for data processing operations
type DataProcessingContext struct {
	// Device is the device that produced the data
	Device Device `json:"device"`

	// OriginalData is the original data before processing
	OriginalData *DeviceData `json:"original_data"`

	// ProcessedData is the data after processing (may be nil if dropped)
	ProcessedData *DeviceData `json:"processed_data,omitempty"`

	// ProcessorsApplied is the list of processors that were applied
	ProcessorsApplied []string `json:"processors_applied"`

	// ProcessingDuration is the time taken to process the data
	ProcessingDuration time.Duration `json:"processing_duration"`

	// Metadata contains additional processing metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DataProcessingService provides high-level data processing service
type DataProcessingService struct {
	pipeline *DataPipeline
	registry DataProcessorRegistry
}

// NewDataProcessingService creates a new data processing service
func NewDataProcessingService(registry DataProcessorRegistry) *DataProcessingService {
	pipeline := NewDataPipeline(registry)
	return &DataProcessingService{
		pipeline: pipeline,
		registry: registry,
	}
}

// ProcessDeviceData processes data from a device through the pipeline
// Returns the processing context with results
func (s *DataProcessingService) ProcessDeviceData(ctx context.Context, device Device, data *DeviceData) (*DataProcessingContext, error) {
	startTime := time.Now()

	// Process data through pipeline
	processedData, err := s.pipeline.Process(ctx, data)
	if err != nil {
		return &DataProcessingContext{
			Device:        device,
			OriginalData:  data,
			ProcessedData: nil,
			ProcessingDuration: time.Since(startTime),
		}, err
	}

	// Get list of processors that were applied
	processors := s.registry.GetProcessorsForDataType(data.DataType)
	processorNames := make([]string, 0, len(processors))
	for _, p := range processors {
		if p.SupportsDataType(data.DataType) {
			processorNames = append(processorNames, p.Name())
		}
	}

	return &DataProcessingContext{
		Device:            device,
		OriginalData:      data,
		ProcessedData:     processedData,
		ProcessorsApplied: processorNames,
		ProcessingDuration: time.Since(startTime),
	}, nil
}

// RegisterProcessor registers a processor with the service
func (s *DataProcessingService) RegisterProcessor(processor DataProcessor) error {
	return s.registry.RegisterProcessor(processor)
}

// UnregisterProcessor unregisters a processor
func (s *DataProcessingService) UnregisterProcessor(name string) error {
	return s.registry.UnregisterProcessor(name)
}

// ListProcessors lists all registered processors
func (s *DataProcessingService) ListProcessors(dataType *DeviceDataType) []DataProcessor {
	return s.registry.ListProcessors(dataType)
}

// GetProcessorsForDataType returns processors for a specific data type
func (s *DataProcessingService) GetProcessorsForDataType(dataType DeviceDataType) []DataProcessor {
	return s.registry.GetProcessorsForDataType(dataType)
}

// BaseProcessor provides a base implementation of DataProcessor
// Processors can embed this to get default implementations
type BaseProcessor struct {
	name             string
	supportedTypes   []DeviceDataType
	priority         int
	supportsDataType func(DeviceDataType) bool
}

// NewBaseProcessor creates a new base processor
func NewBaseProcessor(name string, supportedTypes []DeviceDataType, priority int) *BaseProcessor {
	return &BaseProcessor{
		name:           name,
		supportedTypes: supportedTypes,
		priority:       priority,
		supportsDataType: func(dt DeviceDataType) bool {
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
func (p *BaseProcessor) SupportsDataType(dataType DeviceDataType) bool {
	return p.supportsDataType(dataType)
}

// GetSupportedDataTypes returns all data types this processor supports
func (p *BaseProcessor) GetSupportedDataTypes() []DeviceDataType {
	return p.supportedTypes
}

// GetPriority returns the processor priority
func (p *BaseProcessor) GetPriority() int {
	return p.priority
}

// Process must be implemented by concrete processors
func (p *BaseProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
	// Default implementation: pass-through
	return data, nil
}

// ProcessorBuilder provides a fluent interface for building processors
type ProcessorBuilder struct {
	name           string
	supportedTypes []DeviceDataType
	priority       int
	processFunc    func(context.Context, *DeviceData) (*DeviceData, error)
}

// NewProcessorBuilder creates a new processor builder
func NewProcessorBuilder(name string) *ProcessorBuilder {
	return &ProcessorBuilder{
		name:           name,
		supportedTypes: []DeviceDataType{},
		priority:       100,
		processFunc:    func(ctx context.Context, data *DeviceData) (*DeviceData, error) { return data, nil },
	}
}

// WithSupportedTypes sets the supported data types
func (b *ProcessorBuilder) WithSupportedTypes(types ...DeviceDataType) *ProcessorBuilder {
	b.supportedTypes = types
	return b
}

// WithPriority sets the processor priority
func (b *ProcessorBuilder) WithPriority(priority int) *ProcessorBuilder {
	b.priority = priority
	return b
}

// WithProcessFunc sets the process function
func (b *ProcessorBuilder) WithProcessFunc(fn func(context.Context, *DeviceData) (*DeviceData, error)) *ProcessorBuilder {
	b.processFunc = fn
	return b
}

// Build builds a processor
func (b *ProcessorBuilder) Build() DataProcessor {
	return &builtProcessor{
		BaseProcessor: *NewBaseProcessor(b.name, b.supportedTypes, b.priority),
		processFunc:   b.processFunc,
	}
}

// builtProcessor is a processor built from a builder
type builtProcessor struct {
	BaseProcessor
	processFunc func(context.Context, *DeviceData) (*DeviceData, error)
}

// Process processes data using the provided function
func (p *builtProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
	return p.processFunc(ctx, data)
}



