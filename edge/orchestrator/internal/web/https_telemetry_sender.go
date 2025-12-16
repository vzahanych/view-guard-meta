package web

import (
	"context"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// TelemetryClient interface for sending telemetry (matches telemetry.TelemetryClient)
type TelemetryClient interface {
	IsConnected() bool
	SendTelemetry(ctx context.Context, data *edge.TelemetryData) error
	Heartbeat(ctx context.Context, req *edge.HeartbeatRequest) error
}

// httpSTelemetrySender adapts HTTPSClient to TelemetryClient interface
type httpSTelemetrySender struct {
	httpsClient *HTTPSClient
	logger      *logger.Logger
}

// NewHTTPSTelemetrySender creates a new HTTPS telemetry sender adapter
// Returns a TelemetryClient interface that can be used by telemetry.Sender
func NewHTTPSTelemetrySender(httpsClient *HTTPSClient, log *logger.Logger) interface {
	IsConnected() bool
	SendTelemetry(ctx context.Context, data *edge.TelemetryData) error
	Heartbeat(ctx context.Context, req *edge.HeartbeatRequest) error
} {
	return &httpSTelemetrySender{
		httpsClient: httpsClient,
		logger:      log,
	}
}

// IsConnected returns true if HTTPS client is connected
func (s *httpSTelemetrySender) IsConnected() bool {
	return s.httpsClient != nil && s.httpsClient.IsConnected()
}

// SendTelemetry sends telemetry data via HTTPS
func (s *httpSTelemetrySender) SendTelemetry(ctx context.Context, data *edge.TelemetryData) error {
	return s.httpsClient.SendTelemetry(ctx, data)
}

// Heartbeat sends a heartbeat via HTTPS
func (s *httpSTelemetrySender) Heartbeat(ctx context.Context, req *edge.HeartbeatRequest) error {
	return s.httpsClient.Heartbeat(ctx, req)
}
