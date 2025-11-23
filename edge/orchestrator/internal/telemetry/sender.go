package telemetry

import (
	"context"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"

	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// Sender handles sending telemetry and heartbeats to KVM VM
type Sender struct {
	*service.ServiceBase
	collector     *Collector
	grpcClient    TelemetryClient
	logger        *logger.Logger
	config        *config.TelemetryConfig
	edgeID        string
	ctx           context.Context
	cancel        context.CancelFunc
	sending       bool
	mu            sync.RWMutex
}

// TelemetryClient interface for sending telemetry (implemented by gRPC client)
type TelemetryClient interface {
	IsConnected() bool
	SendTelemetry(ctx context.Context, data *edge.TelemetryData) error
	Heartbeat(ctx context.Context, req *edge.HeartbeatRequest) error
}

// NewSender creates a new telemetry sender
func NewSender(
	collector *Collector,
	grpcClient TelemetryClient,
	cfg *config.TelemetryConfig,
	edgeID string,
	log *logger.Logger,
) *Sender {
	ctx, cancel := context.WithCancel(context.Background())
	return &Sender{
		ServiceBase: service.NewServiceBase("telemetry-sender", log),
		collector:   collector,
		grpcClient:  grpcClient,
		logger:      log,
		config:      cfg,
		edgeID:      edgeID,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the telemetry sender service
func (s *Sender) Start(ctx context.Context) error {
	if !s.config.Enabled {
		s.LogInfo("Telemetry transmission is disabled")
		return nil
	}

	s.mu.Lock()
	s.sending = true
	s.mu.Unlock()

	s.GetStatus().SetStatus(service.StatusRunning)

	// Start heartbeat loop
	go s.heartbeatLoop(ctx)

	// Start telemetry reporting loop
	go s.telemetryLoop(ctx)

	s.LogInfo("Telemetry sender started")
	return nil
}

// Stop stops the telemetry sender service
func (s *Sender) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.sending = false
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	s.LogInfo("Telemetry sender stopped")
	s.GetStatus().SetStatus(service.StatusStopped)
	return nil
}

// heartbeatLoop sends periodic heartbeats to KVM VM
func (s *Sender) heartbeatLoop(ctx context.Context) {
	// Send heartbeat every 30 seconds (configurable)
	interval := 30 * time.Second
	if s.config.Interval > 0 {
		interval = s.config.Interval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Send initial heartbeat
	s.sendHeartbeat(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.sendHeartbeat(ctx)
		}
	}
}

// telemetryLoop sends periodic telemetry reports to KVM VM
func (s *Sender) telemetryLoop(ctx context.Context) {
	// Send telemetry every 5 minutes (configurable)
	interval := 5 * time.Minute
	if s.config.Interval > 0 {
		// Use same interval as heartbeat, but send less frequently
		interval = s.config.Interval * 10
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Send initial telemetry
	s.sendTelemetry(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.sendTelemetry(ctx)
		}
	}
}

// sendHeartbeat sends a heartbeat to KVM VM
func (s *Sender) sendHeartbeat(ctx context.Context) {
	if !s.isSending() {
		return
	}

	if s.grpcClient == nil || !s.grpcClient.IsConnected() {
		s.LogDebug("gRPC client not connected, skipping heartbeat")
		return
	}

	req := &edge.HeartbeatRequest{
		Timestamp: time.Now().UnixNano(),
		EdgeId:    s.edgeID,
	}

	err := s.grpcClient.Heartbeat(ctx, req)
	if err != nil {
		s.LogError("Failed to send heartbeat", err)
		return
	}

	s.LogDebug("Heartbeat sent successfully")
}

// sendTelemetry sends a telemetry report to KVM VM
func (s *Sender) sendTelemetry(ctx context.Context) {
	if !s.isSending() {
		return
	}

	if s.grpcClient == nil || !s.grpcClient.IsConnected() {
		s.LogDebug("gRPC client not connected, skipping telemetry")
		return
	}

	data, err := s.collector.Collect(ctx)
	if err != nil {
		s.LogError("Failed to collect telemetry data", err)
		return
	}

	data.EdgeId = s.edgeID

	err = s.grpcClient.SendTelemetry(ctx, data)
	if err != nil {
		s.LogError("Failed to send telemetry", err)
		return
	}

	cameraCount := 0
	if data.Cameras != nil {
		cameraCount = len(data.Cameras)
	}
	s.LogDebug("Telemetry sent successfully",
		"cameras", cameraCount,
	)
}

// isSending returns whether the sender is active
func (s *Sender) isSending() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sending
}

