package processors

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
)

// AudioDataProcessor processes audio sample data
// Example: noise reduction, feature extraction, voice activity detection
type AudioDataProcessor struct {
	*BaseProcessor
	// Add processor-specific fields here
}

// NewAudioDataProcessor creates a new audio data processor
func NewAudioDataProcessor(name string, priority int) *AudioDataProcessor {
	return &AudioDataProcessor{
		BaseProcessor: NewBaseProcessor(name, []types.DeviceDataType{types.DeviceDataTypeAudioSample}, priority),
	}
}

// Process processes audio sample data
func (p *AudioDataProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	if data.DataType != types.DeviceDataTypeAudioSample {
		return nil, fmt.Errorf("processor %s only supports audio samples", p.Name())
	}

	// Default implementation: pass-through
	// Concrete processors should override this method
	return data, nil
}

