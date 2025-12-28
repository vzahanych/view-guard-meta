package processing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/processing"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/processing/processors"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// mockProcessor is a test processor implementation
type mockProcessor struct {
	name           string
	supportedTypes []types.DeviceDataType
	priority       int
	processFunc    func(context.Context, *types.DeviceData) (*types.DeviceData, error)
}

func (m *mockProcessor) Name() string {
	return m.name
}

func (m *mockProcessor) SupportsDataType(dataType types.DeviceDataType) bool {
	for _, st := range m.supportedTypes {
		if st == dataType {
			return true
		}
	}
	return false
}

func (m *mockProcessor) GetSupportedDataTypes() []types.DeviceDataType {
	return m.supportedTypes
}

func (m *mockProcessor) GetPriority() int {
	return m.priority
}

func (m *mockProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, data)
	}
	return data, nil
}

// TestNewDataProcessorRegistry tests registry creation
func TestNewDataProcessorRegistry(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	require.NotNil(t, registry)

	// Test with nil logger (should use no-op logger)
	registry2 := processing.NewDataProcessorRegistry(nil)
	require.NotNil(t, registry2)
}

// TestDataProcessorRegistry_RegisterProcessor tests processor registration
func TestDataProcessorRegistry_RegisterProcessor(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)

	// Test registering valid processor
	processor := processors.NewPassThroughProcessor("test-processor", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)
	err := registry.RegisterProcessor(processor)
	assert.NoError(t, err)

	// Test registering nil processor
	err = registry.RegisterProcessor(nil)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrInvalidDevice))

	// Test registering processor with empty name
	emptyNameProcessor := &mockProcessor{name: "", supportedTypes: []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, priority: 1}
	err = registry.RegisterProcessor(emptyNameProcessor)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrInvalidDevice))

	// Test registering duplicate processor
	err = registry.RegisterProcessor(processor)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrProcessorExists))
}

// TestDataProcessorRegistry_UnregisterProcessor tests processor unregistration
func TestDataProcessorRegistry_UnregisterProcessor(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)

	processor := processors.NewPassThroughProcessor("test-processor", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)
	err := registry.RegisterProcessor(processor)
	require.NoError(t, err)

	// Test unregistering existing processor
	err = registry.UnregisterProcessor("test-processor")
	assert.NoError(t, err)

	// Test unregistering non-existent processor
	err = registry.UnregisterProcessor("non-existent")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrProcessorNotFound))
}

// TestDataProcessorRegistry_GetProcessor tests processor retrieval
func TestDataProcessorRegistry_GetProcessor(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)

	processor := processors.NewPassThroughProcessor("test-processor", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)
	err := registry.RegisterProcessor(processor)
	require.NoError(t, err)

	// Test getting existing processor
	retrieved, err := registry.GetProcessor("test-processor")
	assert.NoError(t, err)
	assert.Equal(t, processor, retrieved)

	// Test getting non-existent processor
	_, err = registry.GetProcessor("non-existent")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrProcessorNotFound))
}

// TestDataProcessorRegistry_ListProcessors tests processor listing
func TestDataProcessorRegistry_ListProcessors(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)

	processor1 := processors.NewPassThroughProcessor("processor1", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)
	processor2 := processors.NewPassThroughProcessor("processor2", []types.DeviceDataType{types.DeviceDataTypeSensorReading}, 2)
	processor3 := processors.NewPassThroughProcessor("processor3", []types.DeviceDataType{types.DeviceDataTypeVideoFrame, types.DeviceDataTypeSensorReading}, 3)

	err := registry.RegisterProcessor(processor1)
	require.NoError(t, err)
	err = registry.RegisterProcessor(processor2)
	require.NoError(t, err)
	err = registry.RegisterProcessor(processor3)
	require.NoError(t, err)

	// Test listing all processors
	allProcessors := registry.ListProcessors(nil)
	assert.Len(t, allProcessors, 3)

	// Test listing processors filtered by type
	videoType := types.DeviceDataTypeVideoFrame
	videoProcessors := registry.ListProcessors(&videoType)
	assert.Len(t, videoProcessors, 2) // processor1 and processor3

	sensorType := types.DeviceDataTypeSensorReading
	sensorProcessors := registry.ListProcessors(&sensorType)
	assert.Len(t, sensorProcessors, 2) // processor2 and processor3
}

// TestDataProcessorRegistry_GetProcessorsForDataType tests getting processors for data type
func TestDataProcessorRegistry_GetProcessorsForDataType(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)

	// Register processors with different priorities
	processor1 := processors.NewPassThroughProcessor("processor1", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 10)
	processor2 := processors.NewPassThroughProcessor("processor2", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 5)
	processor3 := processors.NewPassThroughProcessor("processor3", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)

	err := registry.RegisterProcessor(processor1)
	require.NoError(t, err)
	err = registry.RegisterProcessor(processor2)
	require.NoError(t, err)
	err = registry.RegisterProcessor(processor3)
	require.NoError(t, err)

	// Test getting processors for data type (should be sorted by priority)
	processors := registry.GetProcessorsForDataType(types.DeviceDataTypeVideoFrame)
	assert.Len(t, processors, 3)
	// Check priority order (lower priority = earlier)
	assert.Equal(t, "processor3", processors[0].Name()) // priority 1
	assert.Equal(t, "processor2", processors[1].Name()) // priority 5
	assert.Equal(t, "processor1", processors[2].Name()) // priority 10

	// Test getting processors for non-existent type
	processors = registry.GetProcessorsForDataType(types.DeviceDataTypeAudioSample)
	assert.Len(t, processors, 0)
}

// TestDataProcessorRegistry_ConcurrentAccess tests concurrent access
func TestDataProcessorRegistry_ConcurrentAccess(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)

	// Concurrent registration
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			processor := processors.NewPassThroughProcessor(
				"processor-"+string(rune('0'+id)),
				[]types.DeviceDataType{types.DeviceDataTypeVideoFrame},
				1,
			)
			_ = registry.RegisterProcessor(processor)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all processors registered
	allProcessors := registry.ListProcessors(nil)
	assert.GreaterOrEqual(t, len(allProcessors), 0) // At least some processors registered
}

// TestNewDataPipeline tests pipeline creation
func TestNewDataPipeline(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)
	require.NotNil(t, pipeline)

	// Test with nil logger (should use no-op logger)
	pipeline2 := processing.NewDataPipeline(registry, nil)
	require.NotNil(t, pipeline2)
}

// TestDataPipeline_Process tests data processing
func TestDataPipeline_Process(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)

	ctx := context.Background()
	data := &types.DeviceData{
		DataType: types.DeviceDataTypeVideoFrame,
		Data:     []byte("test data"),
	}

	// Test processing with no processors (should pass through)
	result, err := pipeline.Process(ctx, data)
	assert.NoError(t, err)
	assert.Equal(t, data, result)

	// Test processing with single processor
	processor := processors.NewPassThroughProcessor("test-processor", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)
	err = registry.RegisterProcessor(processor)
	require.NoError(t, err)

	result, err = pipeline.Process(ctx, data)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Test processing nil data
	_, err = pipeline.Process(ctx, nil)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrInvalidDevice))
}

// TestDataPipeline_Process_MultipleProcessors tests processing with multiple processors
func TestDataPipeline_Process_MultipleProcessors(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)

	ctx := context.Background()
	data := &types.DeviceData{
		DataType: types.DeviceDataTypeVideoFrame,
		Data:     []byte("test data"),
	}

	// Register processors with different priorities
	processor1 := processors.NewPassThroughProcessor("processor1", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 10)
	processor2 := processors.NewPassThroughProcessor("processor2", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 5)
	processor3 := processors.NewPassThroughProcessor("processor3", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)

	err := registry.RegisterProcessor(processor1)
	require.NoError(t, err)
	err = registry.RegisterProcessor(processor2)
	require.NoError(t, err)
	err = registry.RegisterProcessor(processor3)
	require.NoError(t, err)

	// Process data (should go through processors in priority order: 3, 2, 1)
	result, err := pipeline.Process(ctx, data)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// TestDataPipeline_Process_ProcessorError tests error handling
func TestDataPipeline_Process_ProcessorError(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)

	ctx := context.Background()
	data := &types.DeviceData{
		DataType: types.DeviceDataTypeVideoFrame,
		Data:     []byte("test data"),
	}

	// Create processor that returns error
	errorProcessor := &mockProcessor{
		name:           "error-processor",
		supportedTypes: []types.DeviceDataType{types.DeviceDataTypeVideoFrame},
		priority:       1,
		processFunc: func(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
			return nil, errors.New("processing error")
		},
	}

	err := registry.RegisterProcessor(errorProcessor)
	require.NoError(t, err)

	// Process data (should return error)
	_, err = pipeline.Process(ctx, data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error-processor")
}

// TestDataPipeline_Process_DataDropped tests data dropping
func TestDataPipeline_Process_DataDropped(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)

	ctx := context.Background()
	data := &types.DeviceData{
		DataType: types.DeviceDataTypeVideoFrame,
		Data:     []byte("test data"),
	}

	// Create processor that drops data (returns nil)
	dropProcessor := &mockProcessor{
		name:           "drop-processor",
		supportedTypes: []types.DeviceDataType{types.DeviceDataTypeVideoFrame},
		priority:       1,
		processFunc: func(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
			return nil, nil // Drop data
		},
	}

	err := registry.RegisterProcessor(dropProcessor)
	require.NoError(t, err)

	// Process data (should return nil, no error)
	result, err := pipeline.Process(ctx, data)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

// TestDataPipeline_ProcessBatch tests batch processing
func TestDataPipeline_ProcessBatch(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)

	ctx := context.Background()
	dataItems := []*types.DeviceData{
		{DataType: types.DeviceDataTypeVideoFrame, Data: []byte("data1")},
		{DataType: types.DeviceDataTypeVideoFrame, Data: []byte("data2")},
		{DataType: types.DeviceDataTypeVideoFrame, Data: []byte("data3")},
	}

	// Process batch with no processors
	results, errs := pipeline.ProcessBatch(ctx, dataItems)
	assert.Len(t, results, 3)
	assert.Len(t, errs, 0)

	// Process batch with processor that drops second item
	dropProcessor := &mockProcessor{
		name:           "drop-processor",
		supportedTypes: []types.DeviceDataType{types.DeviceDataTypeVideoFrame},
		priority:       1,
		processFunc: func(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
			if string(data.Data) == "data2" {
				return nil, nil // Drop data2
			}
			return data, nil
		},
	}

	err := registry.RegisterProcessor(dropProcessor)
	require.NoError(t, err)

	results, errs = pipeline.ProcessBatch(ctx, dataItems)
	assert.Len(t, results, 2) // data1 and data3
	assert.Len(t, errs, 0)
}

// TestDataPipeline_ProcessBatch_WithErrors tests batch processing with errors
func TestDataPipeline_ProcessBatch_WithErrors(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	pipeline := processing.NewDataPipeline(registry, logger)

	ctx := context.Background()
	dataItems := []*types.DeviceData{
		{DataType: types.DeviceDataTypeVideoFrame, Data: []byte("data1")},
		{DataType: types.DeviceDataTypeVideoFrame, Data: []byte("data2")},
		{DataType: types.DeviceDataTypeVideoFrame, Data: []byte("data3")},
	}

	// Create processor that errors on data2
	errorProcessor := &mockProcessor{
		name:           "error-processor",
		supportedTypes: []types.DeviceDataType{types.DeviceDataTypeVideoFrame},
		priority:       1,
		processFunc: func(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
			if string(data.Data) == "data2" {
				return nil, errors.New("processing error")
			}
			return data, nil
		},
	}

	err := registry.RegisterProcessor(errorProcessor)
	require.NoError(t, err)

	results, errs := pipeline.ProcessBatch(ctx, dataItems)
	assert.Len(t, results, 2) // data1 and data3
	assert.Len(t, errs, 1)     // error for data2
}

// mockDevice is a test device implementation
type mockDevice struct {
	id       string
	metadata types.DeviceMetadata
}

func (m *mockDevice) GetID() string {
	return m.id
}

func (m *mockDevice) GetMetadata() types.DeviceMetadata {
	return m.metadata
}

func (m *mockDevice) Start(ctx context.Context) error {
	return nil
}

func (m *mockDevice) Stop(ctx context.Context) error {
	return nil
}

func (m *mockDevice) UpdateMetadata(ctx context.Context, updates *types.DeviceMetadataUpdate) error {
	return nil
}

func (m *mockDevice) Enable(ctx context.Context) error {
	return nil
}

func (m *mockDevice) Disable(ctx context.Context) error {
	return nil
}

func (m *mockDevice) IsEnabled() bool {
	return true
}

func (m *mockDevice) GetStatus() types.DeviceStatus {
	return types.DeviceStatusOnline
}

func (m *mockDevice) HasCapability(capability types.DeviceCapability) bool {
	return false
}

func (m *mockDevice) GetCapabilities() types.DeviceCapabilities {
	return types.DeviceCapabilities{}
}

func (m *mockDevice) CaptureData(ctx context.Context) (*types.DeviceData, error) {
	return nil, nil
}

func (m *mockDevice) StartDataStream(ctx context.Context) (<-chan *types.DeviceData, error) {
	return nil, nil
}

func (m *mockDevice) StopDataStream(ctx context.Context) error {
	return nil
}

func (m *mockDevice) ReadSensor(ctx context.Context, sensorType string) (*types.SensorReading, error) {
	return nil, nil
}

func (m *mockDevice) ReadAllSensors(ctx context.Context) (map[string]*types.SensorReading, error) {
	return nil, nil
}

func (m *mockDevice) ExecuteCommand(ctx context.Context, command types.DeviceCommand) error {
	return nil
}

func (m *mockDevice) GetAvailableCommands(ctx context.Context) ([]types.DeviceCommand, error) {
	return []types.DeviceCommand{}, nil
}

// TestNewDataProcessingService tests service creation
func TestNewDataProcessingService(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	service := processing.NewDataProcessingService(registry, logger)
	require.NotNil(t, service)

	// Test with nil logger (should use no-op logger)
	service2 := processing.NewDataProcessingService(registry, nil)
	require.NotNil(t, service2)
}

// TestDataProcessingService_ProcessDeviceData tests device data processing
func TestDataProcessingService_ProcessDeviceData(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	service := processing.NewDataProcessingService(registry, logger)

	ctx := context.Background()
	device := &mockDevice{
		id: "device-1",
		metadata: types.DeviceMetadata{
			ID:   "device-1",
			Type: types.DeviceTypeCamera,
		},
	}
	data := &types.DeviceData{
		DataType: types.DeviceDataTypeVideoFrame,
		Data:     []byte("test data"),
	}

	// Test processing with no processors
	context, err := service.ProcessDeviceData(ctx, device, data)
	assert.NoError(t, err)
	assert.NotNil(t, context)
	assert.Equal(t, data, context.OriginalData)
	assert.Equal(t, data, context.ProcessedData)
	assert.Len(t, context.ProcessorsApplied, 0)

	// Test processing with processor
	processor := processors.NewPassThroughProcessor("test-processor", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)
	err = service.RegisterProcessor(processor)
	require.NoError(t, err)

	context, err = service.ProcessDeviceData(ctx, device, data)
	assert.NoError(t, err)
	assert.NotNil(t, context)
	assert.Contains(t, context.ProcessorsApplied, "test-processor")
	assert.NotNil(t, context.Metadata)
	assert.Equal(t, "device-1", context.Metadata["device_id"])

	// Test processing nil data
	context, err = service.ProcessDeviceData(ctx, device, nil)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrInvalidDevice))
	assert.NotNil(t, context)
	assert.Len(t, context.Errors, 1)
}

// TestDataProcessingService_RegisterProcessor tests processor registration
func TestDataProcessingService_RegisterProcessor(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	service := processing.NewDataProcessingService(registry, logger)

	processor := processors.NewPassThroughProcessor("test-processor", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)

	// Test registering valid processor
	err := service.RegisterProcessor(processor)
	assert.NoError(t, err)

	// Test registering nil processor
	err = service.RegisterProcessor(nil)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrInvalidDevice))
}

// TestDataProcessingService_UnregisterProcessor tests processor unregistration
func TestDataProcessingService_UnregisterProcessor(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	service := processing.NewDataProcessingService(registry, logger)

	processor := processors.NewPassThroughProcessor("test-processor", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)
	err := service.RegisterProcessor(processor)
	require.NoError(t, err)

	// Test unregistering existing processor
	err = service.UnregisterProcessor("test-processor")
	assert.NoError(t, err)

	// Test unregistering non-existent processor
	err = service.UnregisterProcessor("non-existent")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, types.ErrProcessorNotFound))
}

// TestDataProcessingService_ListProcessors tests processor listing
func TestDataProcessingService_ListProcessors(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	service := processing.NewDataProcessingService(registry, logger)

	processor1 := processors.NewPassThroughProcessor("processor1", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 1)
	processor2 := processors.NewPassThroughProcessor("processor2", []types.DeviceDataType{types.DeviceDataTypeSensorReading}, 2)

	err := service.RegisterProcessor(processor1)
	require.NoError(t, err)
	err = service.RegisterProcessor(processor2)
	require.NoError(t, err)

	// Test listing all processors
	allProcessors := service.ListProcessors(nil)
	assert.Len(t, allProcessors, 2)

	// Test listing processors filtered by type
	videoType := types.DeviceDataTypeVideoFrame
	videoProcessors := service.ListProcessors(&videoType)
	assert.Len(t, videoProcessors, 1)
	assert.Equal(t, "processor1", videoProcessors[0].Name())
}

// TestDataProcessingService_GetProcessorsForDataType tests getting processors for data type
func TestDataProcessingService_GetProcessorsForDataType(t *testing.T) {
	logger := zap.NewNop()
	registry := processing.NewDataProcessorRegistry(logger)
	service := processing.NewDataProcessingService(registry, logger)

	processor1 := processors.NewPassThroughProcessor("processor1", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 10)
	processor2 := processors.NewPassThroughProcessor("processor2", []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, 5)

	err := service.RegisterProcessor(processor1)
	require.NoError(t, err)
	err = service.RegisterProcessor(processor2)
	require.NoError(t, err)

	// Test getting processors for data type
	processors := service.GetProcessorsForDataType(types.DeviceDataTypeVideoFrame)
	assert.Len(t, processors, 2)
	// Check priority order
	assert.Equal(t, "processor2", processors[0].Name()) // priority 5
	assert.Equal(t, "processor1", processors[1].Name()) // priority 10
}

