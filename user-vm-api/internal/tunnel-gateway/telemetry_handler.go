package tunnelgateway

import (
	"context"

	"github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
	"go.uber.org/zap"
)

// DefaultTelemetryHandler is a simple telemetry handler that logs telemetry and heartbeat
type DefaultTelemetryHandler struct {
	logger *zap.Logger
}

// NewDefaultTelemetryHandler creates a new default telemetry handler
func NewDefaultTelemetryHandler(logger *zap.Logger) *DefaultTelemetryHandler {
	return &DefaultTelemetryHandler{
		logger: logger,
	}
}

// HandleTelemetry handles telemetry data from Edge
func (h *DefaultTelemetryHandler) HandleTelemetry(ctx context.Context, edgeID string, telemetry *edge.TelemetryData) error {
	h.logger.Debug("Received telemetry from Edge",
		zap.String("edge_id", edgeID),
		zap.Int("camera_count", len(telemetry.Cameras)),
		zap.Int64("timestamp", telemetry.Timestamp),
	)
	// For now, just log - can be extended to store telemetry data, update metrics, etc.
	return nil
}

// HandleHeartbeat handles heartbeat from Edge
func (h *DefaultTelemetryHandler) HandleHeartbeat(ctx context.Context, edgeID string, timestamp int64) error {
	h.logger.Debug("Received heartbeat from Edge",
		zap.String("edge_id", edgeID),
		zap.Int64("timestamp", timestamp),
	)
	// For now, just log - can be extended to update connection health, metrics, etc.
	return nil
}
