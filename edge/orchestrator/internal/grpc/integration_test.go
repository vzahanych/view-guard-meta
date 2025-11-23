package grpc

import (
	"context"
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"
)

func setupTestIntegration(t *testing.T) (*events.Transmitter, *Client, *logger.Logger) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.WireGuardConfig{
		Enabled:     true,
		KVMEndpoint: "localhost:50051",
	}

	// Setup state manager for event storage
	tmpDir := t.TempDir()
	stateMgr, err := state.NewManager(&config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: tmpDir,
			},
		},
	}, log)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	// Create event queue and storage
	eventQueue := events.NewQueue(events.QueueConfig{
		StateManager: stateMgr,
		MaxSize:      1000,
	}, log)
	eventStorage := events.NewStorage(stateMgr, log)

	// Create transmitter
	transmitterConfig := events.TransmitterConfig{
		Queue:              eventQueue,
		Storage:            eventStorage,
		BatchSize:          10,
		TransmissionInterval: 5,
		MaxRetries:         3,
		RetryDelay:         1,
	}
	transmitter := events.NewTransmitter(transmitterConfig, log)

	// Create gRPC client
	wgClient := wireguard.NewClient(cfg, log)
	grpcClient := NewClient(cfg, wgClient, log)

	return transmitter, grpcClient, log
}

func TestIntegrateEventTransmitter(t *testing.T) {
	transmitter, grpcClient, log := setupTestIntegration(t)

	// Integration should succeed even if gRPC client is not connected
	err := IntegrateEventTransmitter(transmitter, grpcClient, log)
	if err != nil {
		t.Fatalf("IntegrateEventTransmitter failed: %v", err)
	}

	// Verify that OnTransmit is set
	config := transmitter.GetConfig()
	if config.OnTransmit == nil {
		t.Error("OnTransmit callback not set after integration")
	}
}

func TestIntegrateEventTransmitter_TransmissionCallback(t *testing.T) {
	transmitter, grpcClient, log := setupTestIntegration(t)

	err := IntegrateEventTransmitter(transmitter, grpcClient, log)
	if err != nil {
		t.Fatalf("IntegrateEventTransmitter failed: %v", err)
	}

	// Get config and verify callback
	config := transmitter.GetConfig()
	if config.OnTransmit == nil {
		t.Fatal("OnTransmit callback is nil")
	}

	// Test that callback returns error when gRPC client is not connected
	ctx := context.Background()
	testEvents := []*events.Event{
		events.NewEvent(),
	}

	err = config.OnTransmit(ctx, testEvents)
	if err == nil {
		t.Error("Expected error when gRPC client is not connected, got nil")
	}
}

