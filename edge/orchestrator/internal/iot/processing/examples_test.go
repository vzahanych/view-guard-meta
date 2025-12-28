package processing_test

import (
	"context"
	"fmt"
	"log"

	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/processing"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/processing/processors"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// ExampleNewDataProcessorRegistry demonstrates how to create a data processor registry.
func ExampleNewDataProcessorRegistry() {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)

	fmt.Printf("Registry created: %T\n", registry)
	// Output:
	// Registry created: *processing.dataProcessorRegistryImpl
}

// ExampleDataProcessorRegistry_RegisterProcessor demonstrates how to register a processor.
func ExampleDataProcessorRegistry_RegisterProcessor() {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)

	// Create a pass-through processor
	processor := processors.NewPassThroughProcessor(
		"video-processor",
		[]types.DeviceDataType{types.DeviceDataTypeVideoFrame},
		1,
	)

	// Register the processor
	err := registry.RegisterProcessor(processor)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Processor registered: %s\n", processor.Name())
	// Output:
	// Processor registered: video-processor
}

// ExampleDataPipeline_Process demonstrates how to process data through a pipeline.
func ExampleDataPipeline_Process() {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)

	// Register a processor
	processor := processors.NewPassThroughProcessor(
		"test-processor",
		[]types.DeviceDataType{types.DeviceDataTypeVideoFrame},
		1,
	)
	registry.RegisterProcessor(processor)

	// Process data
	ctx := context.Background()
	data := &types.DeviceData{
		DataType: types.DeviceDataTypeVideoFrame,
		Data:     []byte("test data"),
	}

	result, err := pipeline.Process(ctx, data)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Processed: %v\n", result != nil)
	// Output:
	// Processed: true
}

// ExampleDataPipeline_ProcessBatch demonstrates batch processing.
func ExampleDataPipeline_ProcessBatch() {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)

	// Register a processor
	processor := processors.NewPassThroughProcessor(
		"batch-processor",
		[]types.DeviceDataType{types.DeviceDataTypeVideoFrame},
		1,
	)
	registry.RegisterProcessor(processor)

	// Process batch
	ctx := context.Background()
	dataItems := []*types.DeviceData{
		{DataType: types.DeviceDataTypeVideoFrame, Data: []byte("data1")},
		{DataType: types.DeviceDataTypeVideoFrame, Data: []byte("data2")},
		{DataType: types.DeviceDataTypeVideoFrame, Data: []byte("data3")},
	}

	results, errs := pipeline.ProcessBatch(ctx, dataItems)

	fmt.Printf("Processed: %d items, %d errors\n", len(results), len(errs))
	// Output:
	// Processed: 3 items, 0 errors
}

// ExampleDataProcessingService_ProcessDeviceData demonstrates high-level data processing.
func ExampleDataProcessingService_ProcessDeviceData() {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	service := processing.NewDataProcessingService(registry, logger)

	// Register a processor
	processor := processors.NewVideoFrameProcessor("video-processor", 1)
	service.RegisterProcessor(processor)

	// Create a mock device
	device := &mockDevice{
		id: "camera-1",
		metadata: types.DeviceMetadata{
			ID:   "camera-1",
			Type: types.DeviceTypeCamera,
		},
	}

	// Process device data
	ctx := context.Background()
	data := &types.DeviceData{
		DataType: types.DeviceDataTypeVideoFrame,
		Data:     []byte("frame data"),
	}

	context, err := service.ProcessDeviceData(ctx, device, data)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Processed: %v, Processors: %d\n", context.ProcessedData != nil, len(context.ProcessorsApplied))
	// Output:
	// Processed: true, Processors: 1
}

// ExampleDataProcessingService_RegisterProcessor demonstrates registering processors via service.
func ExampleDataProcessingService_RegisterProcessor() {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	service := processing.NewDataProcessingService(registry, logger)

	// Register multiple processors
	videoProcessor := processors.NewVideoFrameProcessor("video-processor", 1)
	sensorProcessor := processors.NewSensorDataProcessor("sensor-processor", 2)

	service.RegisterProcessor(videoProcessor)
	service.RegisterProcessor(sensorProcessor)

	// List processors
	processors := service.ListProcessors(nil)

	fmt.Printf("Registered processors: %d\n", len(processors))
	// Output:
	// Registered processors: 2
}

// ExampleDataProcessingService_ListProcessors demonstrates listing processors.
func ExampleDataProcessingService_ListProcessors() {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	service := processing.NewDataProcessingService(registry, logger)

	// Register processors for different data types
	videoProcessor := processors.NewVideoFrameProcessor("video-processor", 1)
	sensorProcessor := processors.NewSensorDataProcessor("sensor-processor", 2)

	service.RegisterProcessor(videoProcessor)
	service.RegisterProcessor(sensorProcessor)

	// List all processors
	allProcessors := service.ListProcessors(nil)
	fmt.Printf("All processors: %d\n", len(allProcessors))

	// List processors for specific type
	videoType := types.DeviceDataTypeVideoFrame
	videoProcessors := service.ListProcessors(&videoType)
	fmt.Printf("Video processors: %d\n", len(videoProcessors))

	// Output:
	// All processors: 2
	// Video processors: 1
}

// ExampleDataProcessingService_GetProcessorsForDataType demonstrates getting processors for a data type.
func ExampleDataProcessingService_GetProcessorsForDataType() {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	service := processing.NewDataProcessingService(registry, logger)

	// Register processors with different priorities
	processor1 := processors.NewVideoFrameProcessor("processor1", 10)
	processor2 := processors.NewVideoFrameProcessor("processor2", 5)
	processor3 := processors.NewVideoFrameProcessor("processor3", 1)

	service.RegisterProcessor(processor1)
	service.RegisterProcessor(processor2)
	service.RegisterProcessor(processor3)

	// Get processors for video frame type (sorted by priority)
	processors := service.GetProcessorsForDataType(types.DeviceDataTypeVideoFrame)

	fmt.Printf("Processors for video frames: %d\n", len(processors))
	if len(processors) > 0 {
		fmt.Printf("First processor: %s (priority: %d)\n", processors[0].Name(), processors[0].GetPriority())
	}

	// Output:
	// Processors for video frames: 3
	// First processor: processor3 (priority: 1)
}

// ExampleFilterProcessor demonstrates using a filter processor.
func ExampleFilterProcessor() {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)

	// Create a filter processor that drops data with value "drop-me"
	filterProcessor := processors.NewFilterProcessor(
		"filter-processor",
		[]types.DeviceDataType{types.DeviceDataTypeVideoFrame},
		1,
		func(ctx context.Context, data *types.DeviceData) bool {
			return string(data.Data) != "drop-me"
		},
	)

	registry.RegisterProcessor(filterProcessor)

	// Process data that should pass
	ctx := context.Background()
	data1 := &types.DeviceData{
		DataType: types.DeviceDataTypeVideoFrame,
		Data:     []byte("keep-me"),
	}
	result1, _ := pipeline.Process(ctx, data1)
	fmt.Printf("Data1 processed: %v\n", result1 != nil)

	// Process data that should be dropped
	data2 := &types.DeviceData{
		DataType: types.DeviceDataTypeVideoFrame,
		Data:     []byte("drop-me"),
	}
	result2, _ := pipeline.Process(ctx, data2)
	fmt.Printf("Data2 processed: %v\n", result2 != nil)

	// Output:
	// Data1 processed: true
	// Data2 processed: false
}

// ExampleTransformProcessor demonstrates using a transform processor.
func ExampleTransformProcessor() {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)

	// Create a transform processor that adds metadata
	transformProcessor := processors.NewTransformProcessor(
		"transform-processor",
		[]types.DeviceDataType{types.DeviceDataTypeVideoFrame},
		1,
		func(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
			transformed := *data
			if transformed.Metadata == nil {
				transformed.Metadata = make(map[string]interface{})
			}
			transformed.Metadata["transformed"] = true
			return &transformed, nil
		},
	)

	registry.RegisterProcessor(transformProcessor)

	// Process data
	ctx := context.Background()
	data := &types.DeviceData{
		DataType: types.DeviceDataTypeVideoFrame,
		Data:     []byte("test data"),
	}

	result, err := pipeline.Process(ctx, data)
	if err != nil {
		log.Fatal(err)
	}

	transformed := result.Metadata["transformed"]
	fmt.Printf("Data transformed: %v\n", transformed)
	// Output:
	// Data transformed: true
}

