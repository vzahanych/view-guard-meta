package grpc

import (
	"context"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"

	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// TelemetrySender handles sending telemetry and heartbeats via gRPC
// Implements the telemetry.TelemetryClient interface
type TelemetrySender struct {
	client *Client
	logger *logger.Logger
}

// NewTelemetrySender creates a new telemetry sender
func NewTelemetrySender(client *Client, log *logger.Logger) *TelemetrySender {
	return &TelemetrySender{
		client: client,
		logger: log,
	}
}

// IsConnected returns true if the gRPC client is connected
func (ts *TelemetrySender) IsConnected() bool {
	return ts.client.IsConnected()
}

// SendTelemetry sends telemetry data to the KVM VM
func (ts *TelemetrySender) SendTelemetry(ctx context.Context, data *edge.TelemetryData) error {
	client := ts.client.GetTelemetryClient()
	if client == nil {
		return fmt.Errorf("gRPC telemetry client not available")
	}

	// Create request
	req := &edge.SendTelemetryRequest{
		Telemetry: data,
	}

	// Send with timeout
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.SendTelemetry(sendCtx, req)
	if err != nil {
		return fmt.Errorf("failed to send telemetry: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("KVM VM rejected telemetry: %s", resp.ErrorMessage)
	}

	cameraCount := 0
	if data.Cameras != nil {
		cameraCount = len(data.Cameras)
	}
	ts.logger.Debug("Telemetry sent successfully",
		"cameras", cameraCount,
	)

	return nil
}

// Heartbeat sends a heartbeat to the KVM VM
func (ts *TelemetrySender) Heartbeat(ctx context.Context, req *edge.HeartbeatRequest) error {
	client := ts.client.GetTelemetryClient()
	if client == nil {
		return fmt.Errorf("gRPC telemetry client not available")
	}

	// Send with timeout
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := client.Heartbeat(sendCtx, req)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("KVM VM rejected heartbeat")
	}

	ts.logger.Debug("Heartbeat sent successfully",
		"edge_id", req.EdgeId,
		"server_timestamp", resp.ServerTimestamp,
	)

	return nil
}

