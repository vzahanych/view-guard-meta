package types

import "context"

// DataProcessor is an interface for processing device data
// Processors can transform, filter, analyze, or route device data
//
//go:generate go run go.uber.org/mock/mockgen -destination=../mocks/mock_data_processor.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types DataProcessor
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
//go:generate go run go.uber.org/mock/mockgen -destination=../mocks/mock_data_processor_registry.go -package=mocks github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types DataProcessorRegistry
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

// DataProcessingContext contains context and results from data processing
type DataProcessingContext struct {
	// OriginalData is the original device data
	OriginalData *DeviceData `json:"original_data"`

	// ProcessedData is the processed data (may be nil if filtered)
	ProcessedData *DeviceData `json:"processed_data,omitempty"`

	// ProcessorsApplied is the list of processors that were applied
	ProcessorsApplied []string `json:"processors_applied"`

	// Errors contains any errors that occurred during processing
	Errors []error `json:"errors,omitempty"`

	// Metadata contains additional processing metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

