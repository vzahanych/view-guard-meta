package grpc

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"
)

// IntegrateEventTransmitter integrates the gRPC event sender with the event transmitter
// This should be called after both the transmitter and gRPC client are started
func IntegrateEventTransmitter(
	transmitter *events.Transmitter,
	grpcClient *Client,
	log *logger.Logger,
) error {
	// Create event sender
	eventSender := NewEventSender(grpcClient, log)

	// Configure transmitter to use gRPC sender
	transmitterConfig := transmitter.GetConfig()
	transmitterConfig.OnTransmit = func(ctx context.Context, eventList []*events.Event) error {
		// Check if gRPC client is connected
		if !grpcClient.IsConnected() {
			return fmt.Errorf("gRPC client not connected")
		}
		return eventSender.SendEvents(ctx, eventList)
	}
	transmitter.SetConfig(transmitterConfig)

	log.Info("Event transmitter integrated with gRPC client")
	return nil
}

// SetupGRPCServices sets up all gRPC-related services
func SetupGRPCServices(
	cfg *config.Config,
	log *logger.Logger,
	wgClient *wireguard.Client,
	eventQueue *events.Queue,
	eventStorage *events.Storage,
) (*Client, *StreamingService, error) {
	// Create gRPC client
	grpcClient := NewClient(&cfg.Edge.WireGuard, wgClient, log)

	// Create streaming service
	streamingService := NewStreamingService(grpcClient, log)

	// Note: Event transmitter integration happens after services are started
	// because it requires the gRPC client to be connected

	return grpcClient, streamingService, nil
}

