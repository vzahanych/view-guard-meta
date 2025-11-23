package grpc

import (
	"context"
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"
)

func setupTestGRPCClient(t *testing.T) (*Client, *wireguard.Client) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.WireGuardConfig{
		Enabled:     true,
		KVMEndpoint: "localhost:50051",
	}

	wgClient := wireguard.NewClient(cfg, log)
	grpcClient := NewClient(cfg, wgClient, log)

	return grpcClient, wgClient
}

func TestClient_NewClient(t *testing.T) {
	client, _ := setupTestGRPCClient(t)

	if client.Name() != "grpc-client" {
		t.Errorf("Expected service name 'grpc-client', got %s", client.Name())
	}
}

func TestClient_IsConnected_NotStarted(t *testing.T) {
	client, _ := setupTestGRPCClient(t)

	// Client not started, should not be connected
	if client.IsConnected() {
		t.Error("Expected client to be disconnected when not started")
	}
}

func TestClient_GetEventClient_NotStarted(t *testing.T) {
	client, _ := setupTestGRPCClient(t)

	// Client not started, should return nil
	eventClient := client.GetEventClient()
	if eventClient != nil {
		t.Error("Expected event client to be nil when not started")
	}
}

func TestClient_Start_Disabled(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.WireGuardConfig{
		Enabled: false, // Disabled
	}

	wgClient := wireguard.NewClient(cfg, log)
	client := NewClient(cfg, wgClient, log)

	ctx := context.Background()

	// Start should succeed but do nothing
	err := client.Start(ctx)
	if err != nil {
		t.Fatalf("Start should succeed when disabled: %v", err)
	}
}

func TestClient_Stop_NotStarted(t *testing.T) {
	client, _ := setupTestGRPCClient(t)

	ctx := context.Background()

	// Stop should succeed even if not started
	err := client.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop should succeed: %v", err)
	}
}

