package processors

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
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
		BaseProcessor: NewBaseProcessor(name, []types.DeviceDataType{types.DeviceDataTypeVideoFrame}, priority),
	}
}

// Process processes video frame data
func (p *VideoFrameProcessor) Process(ctx context.Context, data *types.DeviceData) (*types.DeviceData, error) {
	if data.DataType != types.DeviceDataTypeVideoFrame {
		return nil, fmt.Errorf("processor %s only supports video frames", p.Name())
	}

	// Default implementation: pass-through
	// Concrete processors should override this method
	return data, nil
}

