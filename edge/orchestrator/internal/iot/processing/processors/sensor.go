package processors

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// SensorDataProcessor processes sensor reading data
// Example: normalize values, detect thresholds, aggregate readings
type SensorDataProcessor struct {
	*BaseProcessor
	// Add processor-specific fields here
}

// NewSensorDataProcessor creates a new sensor data processor
func NewSensorDataProcessor(name string, priority int) *SensorDataProcessor {
	return &SensorDataProcessor{
		BaseProcessor: NewBaseProcessor(name, []types.DeviceDataType{types.DeviceDataTypeSensorReading}, priority),
	}
}

// Process processes sensor reading data
func (p *SensorDataProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	if data.DataType != types.DeviceDataTypeSensorReading {
		return nil, fmt.Errorf("processor %s only supports sensor readings", p.Name())
	}

	// Default implementation: pass-through
	// Concrete processors should override this method
	return data, nil
}

