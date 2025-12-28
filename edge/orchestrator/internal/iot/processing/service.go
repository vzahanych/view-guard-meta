package processing

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// DataProcessingService provides high-level data processing service
type DataProcessingService struct {
	pipeline *DataPipeline
	registry types.DataProcessorRegistry
	logger   *zap.Logger
}

// NewDataProcessingService creates a new data processing service
// If logger is nil, a no-op logger will be used.
func NewDataProcessingService(
	registry types.DataProcessorRegistry,
	logger *zap.Logger,
) *DataProcessingService {
	if logger == nil {
		logger = zap.NewNop()
	}
	pipeline := NewDataPipeline(registry, logger)
	return &DataProcessingService{
		pipeline: pipeline,
		registry: registry,
		logger:   logger,
	}
}

// ProcessDeviceData processes data from a device through the pipeline
// Returns the processing context with results
func (s *DataProcessingService) ProcessDeviceData(ctx context.Context, device types.Device, data *types.DeviceData) (*types.DataProcessingContext, error) {
	startTime := time.Now()

	if data == nil {
		s.logger.Warn("Attempted to process nil data",
			zap.String("device_id", device.GetID()),
		)
		return &types.DataProcessingContext{
			OriginalData: nil,
			Errors:       []error{fmt.Errorf("%w: data cannot be nil", types.ErrInvalidDevice)},
		}, fmt.Errorf("%w: data cannot be nil", types.ErrInvalidDevice)
	}

	// Process data through pipeline
	processedData, err := s.pipeline.Process(ctx, data)
	duration := time.Since(startTime)

	if err != nil {
		s.logger.Error("Data processing failed",
			zap.String("device_id", device.GetID()),
			zap.String("data_type", string(data.DataType)),
			zap.Duration("processing_duration", duration),
			zap.Error(err),
		)
		return &types.DataProcessingContext{
			OriginalData: data,
			ProcessedData: nil,
			Errors: []error{err},
			Metadata: map[string]interface{}{
				"device_id":          device.GetID(),
				"processing_duration": duration.String(),
			},
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

	s.logger.Debug("Data processed successfully",
		zap.String("device_id", device.GetID()),
		zap.String("data_type", string(data.DataType)),
		zap.Int("processors_applied", len(processorNames)),
		zap.Duration("processing_duration", duration),
		zap.Bool("data_dropped", processedData == nil),
	)

	return &types.DataProcessingContext{
		OriginalData:      data,
		ProcessedData:     processedData,
		ProcessorsApplied: processorNames,
		Metadata: map[string]interface{}{
			"device_id":           device.GetID(),
			"processing_duration": duration.String(),
			"processing_duration_ms": duration.Milliseconds(),
		},
	}, nil
}

// RegisterProcessor registers a processor with the service
func (s *DataProcessingService) RegisterProcessor(processor types.DataProcessor) error {
	if processor == nil {
		s.logger.Warn("Attempted to register nil processor")
		return fmt.Errorf("%w: processor cannot be nil", types.ErrInvalidDevice)
	}

	err := s.registry.RegisterProcessor(processor)
	if err != nil {
		s.logger.Error("Failed to register processor",
			zap.String("processor_name", processor.Name()),
			zap.Error(err),
		)
		return err
	}

	s.logger.Info("Processor registered via service",
		zap.String("processor_name", processor.Name()),
	)

	return nil
}

// UnregisterProcessor unregisters a processor
func (s *DataProcessingService) UnregisterProcessor(name string) error {
	err := s.registry.UnregisterProcessor(name)
	if err != nil {
		s.logger.Error("Failed to unregister processor",
			zap.String("processor_name", name),
			zap.Error(err),
		)
		return err
	}

	s.logger.Info("Processor unregistered via service",
		zap.String("processor_name", name),
	)

	return nil
}

// ListProcessors lists all registered processors
func (s *DataProcessingService) ListProcessors(dataType *types.DeviceDataType) []types.DataProcessor {
	processors := s.registry.ListProcessors(dataType)
	s.logger.Debug("Listed processors",
		zap.Bool("filtered_by_type", dataType != nil),
		zap.Int("processor_count", len(processors)),
	)
	return processors
}

// GetProcessorsForDataType returns processors for a specific data type
func (s *DataProcessingService) GetProcessorsForDataType(dataType types.DeviceDataType) []types.DataProcessor {
	processors := s.registry.GetProcessorsForDataType(dataType)
	s.logger.Debug("Got processors for data type",
		zap.String("data_type", string(dataType)),
		zap.Int("processor_count", len(processors)),
	)
	return processors
}

