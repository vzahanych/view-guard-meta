package processing

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// dataProcessorRegistryImpl is the default implementation of DataProcessorRegistry
type dataProcessorRegistryImpl struct {
	// processors maps processor name to processor
	processors map[string]types.DataProcessor

	// processorsByType maps data type to list of processors (for efficient lookup)
	processorsByType map[types.DeviceDataType][]types.DataProcessor

	// mu protects the processors maps
	mu sync.RWMutex

	// logger for structured logging
	logger *zap.Logger
}

// NewDataProcessorRegistry creates a new data processor registry
// If logger is nil, a no-op logger will be used.
func NewDataProcessorRegistry(logger *zap.Logger) types.DataProcessorRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &dataProcessorRegistryImpl{
		processors:       make(map[string]types.DataProcessor),
		processorsByType: make(map[types.DeviceDataType][]types.DataProcessor),
		logger:           logger,
	}
}

// RegisterProcessor registers a data processor
func (r *dataProcessorRegistryImpl) RegisterProcessor(processor types.DataProcessor) error {
	if processor == nil {
		r.logger.Warn("Attempted to register nil processor")
		return fmt.Errorf("%w: processor cannot be nil", types.ErrInvalidDevice)
	}

	name := processor.Name()
	if name == "" {
		r.logger.Warn("Attempted to register processor with empty name")
		return fmt.Errorf("%w: processor name cannot be empty", types.ErrInvalidDevice)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if processor is already registered
	if _, exists := r.processors[name]; exists {
		r.logger.Warn("Processor already registered",
			zap.String("processor_name", name),
		)
		return fmt.Errorf("%w: processor with name %s", types.ErrProcessorExists, name)
	}

	// Register processor
	r.processors[name] = processor

	// Add to type index
	for _, dataType := range processor.GetSupportedDataTypes() {
		r.processorsByType[dataType] = append(r.processorsByType[dataType], processor)
		// Sort by priority
		r.sortProcessorsByPriority(dataType)
	}

	r.logger.Info("Processor registered",
		zap.String("processor_name", name),
		zap.Int("supported_types", len(processor.GetSupportedDataTypes())),
	)

	return nil
}

// UnregisterProcessor unregisters a processor by name
func (r *dataProcessorRegistryImpl) UnregisterProcessor(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	processor, exists := r.processors[name]
	if !exists {
		r.logger.Warn("Processor not found for unregistration",
			zap.String("processor_name", name),
		)
		return fmt.Errorf("%w: processor with name %s", types.ErrProcessorNotFound, name)
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

	r.logger.Info("Processor unregistered",
		zap.String("processor_name", name),
	)

	_ = processor // Suppress unused variable warning
	return nil
}

// GetProcessor retrieves a processor by name
func (r *dataProcessorRegistryImpl) GetProcessor(name string) (types.DataProcessor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processor, exists := r.processors[name]
	if !exists {
		r.logger.Debug("Processor not found",
			zap.String("processor_name", name),
		)
		return nil, fmt.Errorf("%w: processor with name %s", types.ErrProcessorNotFound, name)
	}

	return processor, nil
}

// ListProcessors returns all registered processors, optionally filtered by data type
func (r *dataProcessorRegistryImpl) ListProcessors(dataType *types.DeviceDataType) []types.DataProcessor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if dataType != nil {
		processors := r.processorsByType[*dataType]
		result := make([]types.DataProcessor, len(processors))
		copy(result, processors)
		return result
	}

	// Return all processors
	result := make([]types.DataProcessor, 0, len(r.processors))
	for _, processor := range r.processors {
		result = append(result, processor)
	}
	return result
}

// GetProcessorsForDataType returns processors that support a specific data type, sorted by priority
func (r *dataProcessorRegistryImpl) GetProcessorsForDataType(dataType types.DeviceDataType) []types.DataProcessor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processors := r.processorsByType[dataType]
	result := make([]types.DataProcessor, len(processors))
	copy(result, processors)
	return result
}

// sortProcessorsByPriority sorts processors by priority (lower priority = earlier in pipeline)
func (r *dataProcessorRegistryImpl) sortProcessorsByPriority(dataType types.DeviceDataType) {
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
	registry types.DataProcessorRegistry
	logger   *zap.Logger
}

// NewDataPipeline creates a new data pipeline
// If logger is nil, a no-op logger will be used.
func NewDataPipeline(registry types.DataProcessorRegistry, logger *zap.Logger) *DataPipeline {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DataPipeline{
		registry: registry,
		logger:   logger,
	}
}

// Process processes device data through the pipeline
// Data flows through processors in priority order (lower priority = earlier)
// If a processor returns nil, the data is dropped and processing stops
// If a processor returns an error, processing stops and the error is returned
func (p *DataPipeline) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	if data == nil {
		p.logger.Warn("Attempted to process nil data")
		return nil, fmt.Errorf("%w: data cannot be nil", types.ErrInvalidDevice)
	}

	// Get processors for this data type, sorted by priority
	// Locking strategy: The registry's GetProcessorsForDataType method handles locking internally
	processors := p.registry.GetProcessorsForDataType(data.DataType)

	if len(processors) == 0 {
		// No processors registered, return data unchanged
		p.logger.Debug("No processors registered for data type",
			zap.String("data_type", string(data.DataType)),
		)
		return data, nil
	}

	// Process data through each processor in order
	currentData := data
	processorsApplied := make([]string, 0, len(processors))
	for _, processor := range processors {
		// Check if processor supports this data type (double-check)
		if !processor.SupportsDataType(currentData.DataType) {
			continue
		}

		// Process data
		processedData, err := processor.Process(ctx, currentData)
		if err != nil {
			p.logger.Error("Processor failed",
				zap.String("processor_name", processor.Name()),
				zap.String("data_type", string(currentData.DataType)),
				zap.Error(err),
			)
			return nil, fmt.Errorf("processor %s failed: %w", processor.Name(), err)
		}

		// If processor returns nil, data is dropped
		if processedData == nil {
			p.logger.Debug("Data dropped by processor",
				zap.String("processor_name", processor.Name()),
				zap.String("data_type", string(currentData.DataType)),
			)
			return nil, nil
		}

		processorsApplied = append(processorsApplied, processor.Name())
		currentData = processedData
	}

	p.logger.Debug("Data processed successfully",
		zap.String("data_type", string(data.DataType)),
		zap.Int("processors_applied", len(processorsApplied)),
	)

	return currentData, nil
}

// ProcessBatch processes multiple device data items through the pipeline
// Returns processed data and errors for each item
func (p *DataPipeline) ProcessBatch(ctx context.Context, dataItems []*types.DeviceData) ([]*types.DeviceData, []error) {
	results := make([]*types.DeviceData, 0, len(dataItems))
	errs := make([]error, 0)

	for i, data := range dataItems {
		processed, err := p.Process(ctx, data)
		if err != nil {
			p.logger.Warn("Batch processing item failed",
				zap.Int("item_index", i),
				zap.String("data_type", string(data.DataType)),
				zap.Error(err),
			)
			errs = append(errs, err)
			continue
		}
		if processed != nil {
			results = append(results, processed)
		}
		// If processed is nil, data was dropped (not an error)
	}

	p.logger.Debug("Batch processing completed",
		zap.Int("total_items", len(dataItems)),
		zap.Int("processed_items", len(results)),
		zap.Int("errors", len(errs)),
	)

	return results, errs
}

