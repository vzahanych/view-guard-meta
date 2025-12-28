# Processing Package Preparation

This document summarizes the preparation for moving data processing functionality to the `processing/` package.

## Current Implementation Review

### Files to Review

1. **`data_pipeline.go`** (428 lines)
2. **`processors.go`** (245 lines)
3. **`types/processing.go`** (70 lines) - Already contains interfaces

**Total Lines**: 743 lines to review and migrate

---

## Components to Move

### From `data_pipeline.go`:

#### 1. `dataProcessorRegistryImpl` struct (29 lines)
**Location**: Lines 19-29

**Fields**:
- `processors map[string]DataProcessor` - Maps processor name to processor
- `processorsByType map[DeviceDataType][]DataProcessor` - Maps data type to processors
- `mu sync.RWMutex` - Protects maps

**Methods** (6 methods):
- `RegisterProcessor(processor DataProcessor) error` - Register a processor
- `UnregisterProcessor(name string) error` - Unregister a processor
- `GetProcessor(name string) (DataProcessor, error)` - Get processor by name
- `ListProcessors(dataType *DeviceDataType) []DataProcessor` - List processors
- `GetProcessorsForDataType(dataType DeviceDataType) []DataProcessor` - Get processors for type
- `sortProcessorsByPriority(dataType DeviceDataType)` - Private helper to sort processors

**Constructor**:
- `NewDataProcessorRegistry() DataProcessorRegistry` - Creates new registry (no logger currently)

**Issues to Fix**:
- ❌ No logger field (should add `*zap.Logger`)
- ❌ No structured logging
- ❌ Error messages use `fmt.Errorf` (should use sentinel errors)
- ❌ Constructor doesn't accept logger parameter

---

#### 2. `DataPipeline` struct (50 lines)
**Location**: Lines 156-228

**Fields**:
- `registry DataProcessorRegistry` - Processor registry

**Methods** (2 methods):
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Process single data item
- `ProcessBatch(ctx context.Context, dataItems []*DeviceData) ([]*DeviceData, []error)` - Process batch

**Constructor**:
- `NewDataPipeline(registry DataProcessorRegistry) *DataPipeline` - Creates new pipeline (no logger currently)

**Issues to Fix**:
- ❌ No logger field (should add `*zap.Logger`)
- ❌ No structured logging
- ❌ Error messages use `fmt.Errorf` (should use sentinel errors)
- ❌ Constructor doesn't accept logger parameter

---

#### 3. `DataProcessingContext` struct (19 lines)
**Location**: Lines 230-249

**Note**: This is a **DUPLICATE** definition!

**Current Definition in `data_pipeline.go`**:
```go
type DataProcessingContext struct {
    Device Device `json:"device"`
    OriginalData *DeviceData `json:"original_data"`
    ProcessedData *DeviceData `json:"processed_data,omitempty"`
    ProcessorsApplied []string `json:"processors_applied"`
    ProcessingDuration time.Duration `json:"processing_duration"`
    Metadata map[string]interface{} `json:"metadata,omitempty"`
}
```

**Definition in `types/processing.go`**:
```go
type DataProcessingContext struct {
    OriginalData *DeviceData `json:"original_data"`
    ProcessedData *DeviceData `json:"processed_data,omitempty"`
    ProcessorsApplied []string `json:"processors_applied"`
    Errors []error `json:"errors,omitempty"`
    Metadata map[string]interface{} `json:"metadata,omitempty"`
}
```

**Differences**:
- Root version has `Device` field (types version doesn't)
- Root version has `ProcessingDuration` field (types version doesn't)
- Types version has `Errors []error` field (root version doesn't)

**Decision**: 
- ✅ Use `types.DataProcessingContext` as the canonical definition
- ⚠️ Need to handle `Device` and `ProcessingDuration` - either add to types or handle differently
- **Action**: Update `data_pipeline.go` to use `types.DataProcessingContext` and remove duplicate

---

#### 4. `DataProcessingService` struct (67 lines)
**Location**: Lines 251-318

**Fields**:
- `pipeline *DataPipeline` - Data pipeline
- `registry DataProcessorRegistry` - Processor registry

**Methods** (5 methods):
- `ProcessDeviceData(ctx context.Context, device Device, data *DeviceData) (*DataProcessingContext, error)` - Process device data
- `RegisterProcessor(processor DataProcessor) error` - Register processor
- `UnregisterProcessor(name string) error` - Unregister processor
- `ListProcessors(dataType *DeviceDataType) []DataProcessor` - List processors
- `GetProcessorsForDataType(dataType DeviceDataType) []DataProcessor` - Get processors for type

**Constructor**:
- `NewDataProcessingService(registry DataProcessorRegistry) *DataProcessingService` - Creates new service (no logger currently)

**Issues to Fix**:
- ❌ No logger field (should add `*zap.Logger`)
- ❌ No structured logging
- ❌ Uses duplicate `DataProcessingContext` definition
- ❌ Constructor doesn't accept logger parameter

---

#### 5. `BaseProcessor` struct (50 lines)
**Location**: Lines 320-370

**Fields**:
- `name string` - Processor name
- `supportedTypes []DeviceDataType` - Supported data types
- `priority int` - Processor priority
- `supportsDataType func(DeviceDataType) bool` - Function to check support

**Methods** (5 methods):
- `Name() string` - Get processor name
- `SupportsDataType(dataType DeviceDataType) bool` - Check if supports type
- `GetSupportedDataTypes() []DeviceDataType` - Get supported types
- `GetPriority() int` - Get priority
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Default pass-through

**Constructor**:
- `NewBaseProcessor(name string, supportedTypes []DeviceDataType, priority int) *BaseProcessor` - Creates base processor

**Issues to Fix**:
- ✅ No logger needed (base implementation)
- ✅ No structured logging needed (base implementation)

---

#### 6. `ProcessorBuilder` struct (45 lines)
**Location**: Lines 372-425

**Fields**:
- `name string` - Processor name
- `supportedTypes []DeviceDataType` - Supported types
- `priority int` - Priority
- `processFunc func(context.Context, *DeviceData) (*DeviceData, error)` - Process function

**Methods** (4 methods):
- `WithSupportedTypes(types ...DeviceDataType) *ProcessorBuilder` - Set supported types
- `WithPriority(priority int) *ProcessorBuilder` - Set priority
- `WithProcessFunc(fn func(context.Context, *DeviceData) (*DeviceData, error)) *ProcessorBuilder` - Set process function
- `Build() DataProcessor` - Build processor

**Constructor**:
- `NewProcessorBuilder(name string) *ProcessorBuilder` - Creates builder

**Helper Type**:
- `builtProcessor` struct - Processor built from builder

**Issues to Fix**:
- ✅ No logger needed (builder pattern)

---

### From `processors.go`:

#### 7. `VideoFrameProcessor` struct (23 lines)
**Location**: Lines 9-32

**Fields**:
- `*BaseProcessor` - Embedded base processor

**Methods** (1 method):
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Process video frames

**Constructor**:
- `NewVideoFrameProcessor(name string, priority int) *VideoFrameProcessor` - Creates video processor

**Category**: Video-related processor

---

#### 8. `SensorDataProcessor` struct (23 lines)
**Location**: Lines 34-57

**Fields**:
- `*BaseProcessor` - Embedded base processor

**Methods** (1 method):
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Process sensor data

**Constructor**:
- `NewSensorDataProcessor(name string, priority int) *SensorDataProcessor` - Creates sensor processor

**Category**: Sensor-related processor

---

#### 9. `AudioDataProcessor` struct (23 lines)
**Location**: Lines 59-82

**Fields**:
- `*BaseProcessor` - Embedded base processor

**Methods** (1 method):
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Process audio data

**Constructor**:
- `NewAudioDataProcessor(name string, priority int) *AudioDataProcessor` - Creates audio processor

**Category**: Audio-related processor

---

#### 10. `EventDataProcessor` struct (23 lines)
**Location**: Lines 84-107

**Fields**:
- `*BaseProcessor` - Embedded base processor

**Methods** (1 method):
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Process event data

**Constructor**:
- `NewEventDataProcessor(name string, priority int) *EventDataProcessor` - Creates event processor

**Category**: Event-related processor

---

#### 11. `MultiTypeProcessor` struct (25 lines)
**Location**: Lines 109-135

**Fields**:
- `*BaseProcessor` - Embedded base processor
- `processFunc func(context.Context, *DeviceData) (*DeviceData, error)` - Process function

**Methods** (1 method):
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Process using function

**Constructor**:
- `NewMultiTypeProcessor(name string, supportedTypes []DeviceDataType, priority int, processFunc func(context.Context, *DeviceData) (*DeviceData, error)) *MultiTypeProcessor` - Creates multi-type processor

**Category**: Common processor

---

#### 12. `PassThroughProcessor` struct (15 lines)
**Location**: Lines 137-153

**Fields**:
- `*BaseProcessor` - Embedded base processor

**Methods** (1 method):
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Pass-through (returns data unchanged)

**Constructor**:
- `NewPassThroughProcessor(name string, supportedTypes []DeviceDataType, priority int) *PassThroughProcessor` - Creates pass-through processor

**Category**: Common processor

---

#### 13. `FilterProcessor` struct (26 lines)
**Location**: Lines 155-186

**Fields**:
- `*BaseProcessor` - Embedded base processor
- `filterFunc func(context.Context, *DeviceData) bool` - Filter function

**Methods** (1 method):
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Filter data (returns nil if filtered)

**Constructor**:
- `NewFilterProcessor(name string, supportedTypes []DeviceDataType, priority int, filterFunc func(context.Context, *DeviceData) bool) *FilterProcessor` - Creates filter processor

**Category**: Common processor

---

#### 14. `TransformProcessor` struct (25 lines)
**Location**: Lines 188-213

**Fields**:
- `*BaseProcessor` - Embedded base processor
- `transformFunc func(context.Context, *DeviceData) (*DeviceData, error)` - Transform function

**Methods** (1 method):
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Transform data

**Constructor**:
- `NewTransformProcessor(name string, supportedTypes []DeviceDataType, priority int, transformFunc func(context.Context, *DeviceData) (*DeviceData, error)) *TransformProcessor` - Creates transform processor

**Category**: Common processor

---

#### 15. `TimestampEnrichmentProcessor` struct (18 lines)
**Location**: Lines 215-244

**Fields**:
- `*BaseProcessor` - Embedded base processor

**Methods** (1 method):
- `Process(ctx context.Context, data *DeviceData) (*DeviceData, error)` - Enrich data with timestamp

**Constructor**:
- `NewTimestampEnrichmentProcessor(name string, supportedTypes []DeviceDataType, priority int) *TimestampEnrichmentProcessor` - Creates timestamp enrichment processor

**Category**: Common processor

---

## Dependencies

### Imports from `iot` package (should come from `iot/types`):
- ✅ `Device` → `types.Device`
- ✅ `DeviceData` → `types.DeviceData`
- ✅ `DeviceDataType` → `types.DeviceDataType`
- ✅ `DataProcessor` → `types.DataProcessor`
- ✅ `DataProcessorRegistry` → `types.DataProcessorRegistry`
- ✅ `DataProcessingContext` → `types.DataProcessingContext` (after fixing duplicate)

### External Dependencies:
- ✅ `context` - Standard library
- ✅ `fmt` - Standard library (for error formatting)
- ✅ `sync` - Standard library (for mutex)
- ✅ `time` - Standard library (for timestamps)
- ✅ `sort` - Standard library (for sorting processors)
- ✅ `go.uber.org/zap` - For structured logging (to be added)

### Dependencies on Other Packages:
- ❌ None - Processing is self-contained

---

## Test Files

### Current Test Files:
- ❌ **No test files found** for `data_pipeline.go` or `processors.go`
- **Action**: Create comprehensive tests in new package

---

## Processor Categories

### Video-Related Processors:
- `VideoFrameProcessor` → `processors/video.go`

### Sensor-Related Processors:
- `SensorDataProcessor` → `processors/sensor.go`

### Audio-Related Processors:
- `AudioDataProcessor` → `processors/audio.go`

### Event-Related Processors:
- `EventDataProcessor` → `processors/event.go`

### Common Processors:
- `BaseProcessor` → `processors/base.go`
- `MultiTypeProcessor` → `processors/common.go`
- `PassThroughProcessor` → `processors/common.go`
- `FilterProcessor` → `processors/common.go`
- `TransformProcessor` → `processors/common.go`
- `TimestampEnrichmentProcessor` → `processors/common.go`
- `ProcessorBuilder` → `processors/common.go`
- `builtProcessor` → `processors/common.go`

---

## Issues to Fix During Migration

### 1. Duplicate `DataProcessingContext` Definition
- **Issue**: Defined in both `data_pipeline.go` and `types/processing.go` with different fields
- **Solution**: Use `types.DataProcessingContext` as canonical, handle missing fields appropriately

### 2. Missing Logger Support
- **Issue**: No logger fields in registry, pipeline, or service
- **Solution**: Add `*zap.Logger` fields and structured logging

### 3. Missing Sentinel Errors
- **Issue**: Error messages use `fmt.Errorf` instead of sentinel errors
- **Solution**: Use sentinel errors from `types/errors.go`:
  - `types.ErrProcessorNotFound`
  - `types.ErrProcessorExists`
  - `types.ErrInvalidData` (for nil data)

### 4. Missing Context Handling
- **Issue**: Context is passed correctly, but no validation
- **Solution**: Ensure context is never stored in structs (already correct)

### 5. Missing Locking Strategy Documentation
- **Issue**: Locking is used but not explicitly documented
- **Solution**: Document locking strategy (copy references under lock, call outside lock)

---

## File Structure Plan

```
processing/
├── pipeline.go              # Registry and Pipeline implementations
├── service.go                # DataProcessingService implementation
├── pipeline_test.go          # Unit tests for registry and pipeline
├── service_test.go           # Unit tests for service
├── examples_test.go          # Example tests
└── processors/
    ├── base.go               # BaseProcessor implementation
    ├── video.go              # VideoFrameProcessor
    ├── sensor.go             # SensorDataProcessor
    ├── audio.go              # AudioDataProcessor
    ├── event.go              # EventDataProcessor
    └── common.go             # Common processors (PassThrough, Filter, Transform, TimestampEnrichment, Builder)
```

---

## Statistics

- **Total Components**: 15 components
- **Total Lines**: 743 lines
- **Processors**: 9 processor types
- **Core Components**: 3 (Registry, Pipeline, Service)
- **Helper Components**: 3 (BaseProcessor, ProcessorBuilder, builtProcessor)

---

## Next Steps

1. ✅ **Section 5.1.1**: Review complete
2. ⏭️ **Section 5.1.2**: Create directory structure
3. ⏭️ **Section 5.2**: Move core pipeline implementation
4. ⏭️ **Section 5.3**: Move processor implementations
5. ⏭️ **Section 5.4**: Create tests
6. ⏭️ **Section 5.5**: Update imports
7. ⏭️ **Section 5.6**: Delete old files

