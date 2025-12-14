package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
	config          *config.WireGuardConfig
	wgClient        *wireguard.Client
	logger          *logger.Logger
	conn            *grpc.ClientConn
	mu              sync.RWMutex
	eventClient     edge.EventServiceClient
	telemetryClient edge.TelemetryServiceClient
	controlClient   edge.ControlServiceClient
	streamingClient edge.StreamingServiceClient
	wg              sync.WaitGroup // For monitoring goroutine
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

	// Start continuous gRPC health monitoring: Edge monitors VM configuration status through gRPC every 30s
	// This ensures the bidirectional gRPC connection is alive and ready (security requirement)
	// All VM-Edge communication is gRPC-only (no HTTP)
	c.wg.Add(1)
	go c.monitorVMHealth(ctx)

	return nil
}

// Stop stops the gRPC client service
func (c *Client) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.GetStatus().SetStatus(service.StatusStopping)

	// Wait for monitoring goroutine to finish
	c.wg.Wait()

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

// monitorVMHealth continuously monitors VM configuration status through gRPC every 30s
// This ensures the bidirectional gRPC connection is alive and ready (security requirement)
// Edge monitors VM configuration status through gRPC (all VM-Edge communication is gRPC-only, no HTTP)
func (c *Client) monitorVMHealth(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Initial check
	c.checkVMHealth(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkVMHealth(ctx)
		}
	}
}

// checkVMHealth verifies VM is alive and checks configuration status via gRPC
func (c *Client) checkVMHealth(ctx context.Context) {
	c.mu.RLock()
	controlClient := c.controlClient
	conn := c.conn
	c.mu.RUnlock()

	if controlClient == nil || conn == nil {
		c.LogInfo("gRPC client not ready for VM health check")
		return
	}

	// Check connection state
	state := conn.GetState()
	if state.String() != "READY" && state.String() != "IDLE" {
		c.LogInfo("gRPC connection not ready for VM health check",
			"state", state.String(),
			"note", "Bidirectional gRPC connection health is critical for security")
		return
	}

	// Call VM's GetConfig to verify VM is alive and check configuration status
	healthCtx, healthCancel := context.WithTimeout(ctx, 10*time.Second)
	defer healthCancel()

	_, err := controlClient.GetConfig(healthCtx, &edge.GetConfigRequest{})
	if err != nil {
		c.LogInfo("VM gRPC health check failed",
			"error", err,
			"note", "Bidirectional gRPC connection health is critical for security - VM may be unreachable")
		return
	}

	c.LogDebug("VM gRPC health check passed - VM is alive and configuration status verified",
		"note", "Bidirectional gRPC connection is alive and ready")
}

// connect establishes a gRPC connection
func (c *Client) connect(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
	// Load TLS credentials for mTLS (zero-trust security)
	var creds credentials.TransportCredentials
	clientCertPath := "/etc/ssl/certs/edge-client.crt"
	clientKeyPath := "/etc/ssl/private/edge-client.key"
	caCertPath := "/etc/ssl/certs/ca.crt"

	// Try to load TLS credentials (certificates should be mounted from Epic 2.0)
	tlsCreds, err := LoadClientCredentials(clientCertPath, clientKeyPath, caCertPath)
	if err != nil {
		c.LogError("Failed to load TLS credentials for gRPC client, using insecure (not recommended for production)", err,
			"client_cert", clientCertPath,
			"client_key", clientKeyPath,
			"ca_cert", caCertPath,
			"note", "TLS certificates should be generated in Epic 2.0 and mounted to containers")
		// Fall back to insecure for development/testing
		creds = insecure.NewCredentials()
	} else {
		c.LogInfo("Loaded TLS credentials for gRPC client (mTLS enabled)",
			"client_cert", clientCertPath)
		creds = tlsCreds
	}

	// Fail fast if the port isn't reachable to avoid hanging in gRPC dial retries.
	tcpCtx, tcpCancel := context.WithTimeout(ctx, 5*time.Second)
	defer tcpCancel()
	if preflightConn, err := (&net.Dialer{}).DialContext(tcpCtx, "tcp", endpoint); err != nil {
		c.LogError("TCP preflight to VM failed", err, "endpoint", endpoint)
		return nil, fmt.Errorf("tcp preflight to %s failed: %w", endpoint, err)
	} else {
		_ = preflightConn.Close()
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second, // Reduced from 10s to avoid "too_many_pings" error
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithBlock(),
		grpc.WithReturnConnectionError(), // Surface connection errors immediately instead of waiting for the deadline
		// Add default call options for better connection handling
		grpc.WithDefaultCallOptions(grpc.WaitForReady(false)),
	}

	// Use context with timeout instead of deprecated grpc.WithTimeout
	// Use a shorter timeout so failures surface quickly during bring-up
	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	c.LogInfo("Attempting gRPC connection", "endpoint", endpoint, "timeout", "20s")
	conn, err := grpc.DialContext(connectCtx, endpoint, opts...)
	if err != nil {
		c.LogError("gRPC dial failed", err, "endpoint", endpoint)
		return nil, fmt.Errorf("failed to dial %s: %w", endpoint, err)
	}

	c.LogInfo("gRPC connection established", "endpoint", endpoint)
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
