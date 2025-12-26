package iot

import (
	"context"
	"fmt"
	"time"
)

// VideoFrameProcessor processes video frame data
// Example: resize, compress, normalize, detect objects
type VideoFrameProcessor struct {
	*BaseProcessor
	// Add processor-specific fields here
}

// NewVideoFrameProcessor creates a new video frame processor
func NewVideoFrameProcessor(name string, priority int) *VideoFrameProcessor {
	return &VideoFrameProcessor{
		BaseProcessor: NewBaseProcessor(name, []DeviceDataType{DeviceDataTypeVideoFrame}, priority),
	}
}

// Process processes video frame data
func (p *VideoFrameProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
	if data.DataType != DeviceDataTypeVideoFrame {
		return nil, fmt.Errorf("processor %s only supports video frames", p.Name())
	}

	// Default implementation: pass-through
	// Concrete processors should override this method
	return data, nil
}

// SensorDataProcessor processes sensor reading data
// Example: normalize values, detect thresholds, aggregate readings
type SensorDataProcessor struct {
	*BaseProcessor
	// Add processor-specific fields here
}

// NewSensorDataProcessor creates a new sensor data processor
func NewSensorDataProcessor(name string, priority int) *SensorDataProcessor {
	return &SensorDataProcessor{
		BaseProcessor: NewBaseProcessor(name, []DeviceDataType{DeviceDataTypeSensorReading}, priority),
	}
}

// Process processes sensor reading data
func (p *SensorDataProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
	if data.DataType != DeviceDataTypeSensorReading {
		return nil, fmt.Errorf("processor %s only supports sensor readings", p.Name())
	}

	// Default implementation: pass-through
	// Concrete processors should override this method
	return data, nil
}

// AudioDataProcessor processes audio sample data
// Example: noise reduction, feature extraction, voice activity detection
type AudioDataProcessor struct {
	*BaseProcessor
	// Add processor-specific fields here
}

// NewAudioDataProcessor creates a new audio data processor
func NewAudioDataProcessor(name string, priority int) *AudioDataProcessor {
	return &AudioDataProcessor{
		BaseProcessor: NewBaseProcessor(name, []DeviceDataType{DeviceDataTypeAudioSample}, priority),
	}
}

// Process processes audio sample data
func (p *AudioDataProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
	if data.DataType != DeviceDataTypeAudioSample {
		return nil, fmt.Errorf("processor %s only supports audio samples", p.Name())
	}

	// Default implementation: pass-through
	// Concrete processors should override this method
	return data, nil
}

// EventDataProcessor processes event data
// Example: enrich events, route to different handlers, aggregate events
type EventDataProcessor struct {
	*BaseProcessor
	// Add processor-specific fields here
}

// NewEventDataProcessor creates a new event data processor
func NewEventDataProcessor(name string, priority int) *EventDataProcessor {
	return &EventDataProcessor{
		BaseProcessor: NewBaseProcessor(name, []DeviceDataType{DeviceDataTypeEvent}, priority),
	}
}

// Process processes event data
func (p *EventDataProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
	if data.DataType != DeviceDataTypeEvent {
		return nil, fmt.Errorf("processor %s only supports events", p.Name())
	}

	// Default implementation: pass-through
	// Concrete processors should override this method
	return data, nil
}

// MultiTypeProcessor processes multiple data types
// Example: logging processor, metrics processor, routing processor
type MultiTypeProcessor struct {
	*BaseProcessor
	processFunc func(context.Context, *DeviceData) (*DeviceData, error)
}

// NewMultiTypeProcessor creates a new multi-type processor
func NewMultiTypeProcessor(name string, supportedTypes []DeviceDataType, priority int, processFunc func(context.Context, *DeviceData) (*DeviceData, error)) *MultiTypeProcessor {
	return &MultiTypeProcessor{
		BaseProcessor: NewBaseProcessor(name, supportedTypes, priority),
		processFunc:   processFunc,
	}
}

// Process processes data using the provided function
func (p *MultiTypeProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
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
func NewPassThroughProcessor(name string, supportedTypes []DeviceDataType, priority int) *PassThroughProcessor {
	return &PassThroughProcessor{
		BaseProcessor: NewBaseProcessor(name, supportedTypes, priority),
	}
}

// Process passes data through unchanged
func (p *PassThroughProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
	return data, nil
}

// FilterProcessor is a processor that can filter (drop) data based on conditions
type FilterProcessor struct {
	*BaseProcessor
	filterFunc func(context.Context, *DeviceData) bool
}

// NewFilterProcessor creates a new filter processor
func NewFilterProcessor(name string, supportedTypes []DeviceDataType, priority int, filterFunc func(context.Context, *DeviceData) bool) *FilterProcessor {
	return &FilterProcessor{
		BaseProcessor: NewBaseProcessor(name, supportedTypes, priority),
		filterFunc:    filterFunc,
	}
}

// Process filters data based on the filter function
// Returns nil if data should be dropped, otherwise returns data unchanged
func (p *FilterProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
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
	transformFunc func(context.Context, *DeviceData) (*DeviceData, error)
}

// NewTransformProcessor creates a new transform processor
func NewTransformProcessor(name string, supportedTypes []DeviceDataType, priority int, transformFunc func(context.Context, *DeviceData) (*DeviceData, error)) *TransformProcessor {
	return &TransformProcessor{
		BaseProcessor:  NewBaseProcessor(name, supportedTypes, priority),
		transformFunc:  transformFunc,
	}
}

// Process transforms data using the transform function
func (p *TransformProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
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
func NewTimestampEnrichmentProcessor(name string, supportedTypes []DeviceDataType, priority int) *TimestampEnrichmentProcessor {
	return &TimestampEnrichmentProcessor{
		BaseProcessor: NewBaseProcessor(name, supportedTypes, priority),
	}
}

// Process enriches data with processing timestamp
func (p *TimestampEnrichmentProcessor) Process(ctx context.Context, data *DeviceData) (*DeviceData, error) {
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

