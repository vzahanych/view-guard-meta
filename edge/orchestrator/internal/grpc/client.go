package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"

	// Import generated proto stubs (all in same package: edge)
	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// Client manages gRPC connections to KVM VM over WireGuard tunnel
type Client struct {
	*service.ServiceBase
	config        *config.WireGuardConfig
	wgClient      *wireguard.Client
	logger        *logger.Logger
	conn          *grpc.ClientConn
	mu            sync.RWMutex
	eventClient   edge.EventServiceClient
	telemetryClient edge.TelemetryServiceClient
	controlClient   edge.ControlServiceClient
	streamingClient edge.StreamingServiceClient
}

// NewClient creates a new gRPC client
func NewClient(cfg *config.WireGuardConfig, wgClient *wireguard.Client, log *logger.Logger) *Client {
	return &Client{
		ServiceBase: service.NewServiceBase("grpc-client", log),
		config:      cfg,
		wgClient:    wgClient,
		logger:      log,
	}
}

// Name returns the service name
func (c *Client) Name() string {
	return "grpc-client"
}

// Start starts the gRPC client service
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.config.Enabled {
		c.LogInfo("gRPC client disabled (WireGuard disabled)")
		return nil
	}

	c.GetStatus().SetStatus(service.StatusStarting)

	// Wait for WireGuard to be connected
	if !c.wgClient.IsConnected() {
		c.LogInfo("Waiting for WireGuard connection...")
		// Wait up to 30 seconds for WireGuard to connect
		timeout := time.After(30 * time.Second)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timeout:
				return fmt.Errorf("WireGuard not connected after 30 seconds")
			case <-ticker.C:
				if c.wgClient.IsConnected() {
					c.LogInfo("WireGuard connected, establishing gRPC connection")
					goto connected
				}
			}
		}
	}

connected:
	// Connect to KVM VM over WireGuard tunnel
	// KVM VM gRPC server should be accessible via WireGuard interface
	// For PoC, we'll use localhost or WireGuard interface IP
	endpoint := c.getEndpoint()
	
	c.LogInfo("Connecting to KVM VM", "endpoint", endpoint)

	conn, err := c.connect(ctx, endpoint)
	if err != nil {
		c.GetStatus().SetError(err)
		c.LogError("Failed to connect to KVM VM", err)
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.eventClient = edge.NewEventServiceClient(conn)
	c.telemetryClient = edge.NewTelemetryServiceClient(conn)
	c.controlClient = edge.NewControlServiceClient(conn)
	c.streamingClient = edge.NewStreamingServiceClient(conn)

	c.GetStatus().SetStatus(service.StatusRunning)
	c.LogInfo("gRPC client connected", "endpoint", endpoint)

	return nil
}

// Stop stops the gRPC client service
func (c *Client) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.GetStatus().SetStatus(service.StatusStopping)

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			c.LogError("Error closing gRPC connection", err)
		}
		c.conn = nil
	}

	// Clear proto clients
	// c.eventClient = nil
	// c.telemetryClient = nil
	// c.controlClient = nil
	// c.streamingClient = nil

	c.GetStatus().SetStatus(service.StatusStopped)
	c.LogInfo("gRPC client stopped")

	return nil
}

// connect establishes a gRPC connection
func (c *Client) connect(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
	// Use insecure credentials for PoC (WireGuard provides encryption)
	// In production, could use TLS over WireGuard for additional security
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithBlock(),
		grpc.WithTimeout(10 * time.Second),
	}

	conn, err := grpc.DialContext(ctx, endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", endpoint, err)
	}

	return conn, nil
}

// getEndpoint returns the gRPC endpoint address
func (c *Client) getEndpoint() string {
	// Connect to VM over WireGuard tunnel using the WireGuard interface IP
	// VM WireGuard IP is 10.0.0.1, Edge is 10.0.0.2
	// gRPC server runs on port 50051
	return "10.0.0.1:50051"
}

// GetEventClient returns the event service client
func (c *Client) GetEventClient() edge.EventServiceClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.eventClient
}

// GetTelemetryClient returns the telemetry service client
func (c *Client) GetTelemetryClient() edge.TelemetryServiceClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.telemetryClient
}

// GetControlClient returns the control service client
func (c *Client) GetControlClient() edge.ControlServiceClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.controlClient
}

// GetStreamingClient returns the streaming service client
func (c *Client) GetStreamingClient() edge.StreamingServiceClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.streamingClient
}

// IsConnected returns whether the gRPC connection is active
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && c.wgClient.IsConnected()
}

// Reconnect attempts to reconnect to the KVM VM
func (c *Client) Reconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	endpoint := c.getEndpoint()
	conn, err := c.connect(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("reconnection failed: %w", err)
	}

	c.conn = conn
	c.eventClient = edge.NewEventServiceClient(conn)
	c.telemetryClient = edge.NewTelemetryServiceClient(conn)
	c.controlClient = edge.NewControlServiceClient(conn)
	c.streamingClient = edge.NewStreamingServiceClient(conn)

	c.LogInfo("gRPC client reconnected", "endpoint", endpoint)
	return nil
}

