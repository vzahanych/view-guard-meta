package tunnelgateway

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	grpctls "github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/tls"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// EdgeClient handles VM → Edge gRPC calls (VM connects to Edge's gRPC server)
// Maintains persistent connections to Edge gRPC servers for reuse
type EdgeClient struct {
	wgServer      *WireGuardServer
	edgeAPIServer *EdgeAPIServer // Optional: for getting connection info
	logger        *logging.Logger
	// Connection pool: edgeID -> gRPC connection
	connections map[string]*grpc.ClientConn
	mu          sync.RWMutex // Protects connections map
}

// NewEdgeClient creates a new Edge client for VM → Edge calls
func NewEdgeClient(wgServer *WireGuardServer, log *logging.Logger) *EdgeClient {
	return &EdgeClient{
		wgServer:    wgServer,
		logger:      log,
		connections: make(map[string]*grpc.ClientConn),
	}
}

// SetEdgeAPIServer sets the EdgeAPIServer for better edge lookup
func (c *EdgeClient) SetEdgeAPIServer(server *EdgeAPIServer) {
	c.edgeAPIServer = server
}

// GetOrCreateConnection gets an existing connection or creates a new one
// Reuses connections to avoid creating new connections for each call
// Exported so ModelTransferService can use it
func (c *EdgeClient) GetOrCreateConnection(ctx context.Context, edgeID string) (*grpc.ClientConn, error) {
	// Check for existing connection
	c.mu.RLock()
	conn, exists := c.connections[edgeID]
	c.mu.RUnlock()

	if exists && conn != nil {
		// Check if connection is still valid
		state := conn.GetState()
		if state == connectivity.Ready || state == connectivity.Idle {
			c.logger.Debug("Reusing existing gRPC connection to Edge",
				zap.String("edge_id", edgeID),
				zap.String("state", state.String()))
			return conn, nil
		}
		// Connection is not ready, close it and create new one
		c.logger.Info("Existing connection not ready, creating new one",
			zap.String("edge_id", edgeID),
			zap.String("state", state.String()))
		c.mu.Lock()
		delete(c.connections, edgeID)
		c.mu.Unlock()
		conn.Close()
	}

	// Before attempting VM→Edge connection, verify Edge→VM connection is established and active
	// This ensures proper coordination: Edge must be connected and authenticated before VM tries to connect
	if c.edgeAPIServer != nil {
		edgeConn, edgeConnected := c.edgeAPIServer.GetConnection(edgeID)
		if !edgeConnected || edgeConn == nil {
			c.logger.Warn("Edge→VM connection not established, waiting before attempting VM→Edge connection",
				zap.String("edge_id", edgeID),
				zap.String("note", "VM should only establish VM→Edge connection after Edge has established Edge→VM connection"))
			return nil, fmt.Errorf("edge %s is not connected to VM (Edge→VM connection not established)", edgeID)
		}

		// Check if Edge is actively sending heartbeats (connection is alive and authenticated)
		edgeConn.mu.RLock()
		lastHeartbeat := edgeConn.LastHeartbeat
		edgeConn.mu.RUnlock()

		heartbeatTimeout := 5 * time.Minute // Same threshold as ConnectionMonitor
		if lastHeartbeat.IsZero() || time.Since(lastHeartbeat) > heartbeatTimeout {
			c.logger.Warn("Edge→VM connection exists but heartbeat is stale, waiting before attempting VM→Edge connection",
				zap.String("edge_id", edgeID),
				zap.Time("last_heartbeat", lastHeartbeat),
				zap.Duration("heartbeat_age", time.Since(lastHeartbeat)),
				zap.Duration("timeout", heartbeatTimeout),
				zap.String("note", "Edge must be actively authenticated and sending heartbeats before VM can connect"))
			return nil, fmt.Errorf("edge %s connection is stale (last heartbeat: %v, age: %v)", edgeID, lastHeartbeat, time.Since(lastHeartbeat))
		}

		c.logger.Info("Edge→VM connection verified, proceeding with VM→Edge connection",
			zap.String("edge_id", edgeID),
			zap.Time("last_heartbeat", lastHeartbeat),
			zap.Duration("heartbeat_age", time.Since(lastHeartbeat)))
	} else {
		c.logger.Warn("EdgeAPIServer not available, cannot verify Edge→VM connection before attempting VM→Edge connection",
			zap.String("edge_id", edgeID),
			zap.String("note", "Proceeding without connection verification (may fail if Edge is not ready)"))
	}

	// Get Edge's WireGuard IP
	edgeIP, err := c.GetEdgeWireGuardIP(edgeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get Edge WireGuard IP: %w", err)
	}

	// Connect to Edge's gRPC server (Edge listens on port 50052)
	edgeEndpoint := fmt.Sprintf("%s:50052", edgeIP)
	c.logger.Info("Creating gRPC connection to Edge",
		zap.String("edge_id", edgeID),
		zap.String("endpoint", edgeEndpoint))

	// Load TLS credentials for mTLS (zero-trust security)
	var creds credentials.TransportCredentials
	clientCertPath := "/etc/ssl/certs/vm-client.crt"
	clientKeyPath := "/etc/ssl/private/vm-client.key"
	caCertPath := "/etc/ssl/certs/ca.crt"
	// Edge server certificate has CN=edge-orchestrator and DNS:edge-orchestrator in SAN
	// Even though we connect via IP (10.0.0.2), we must set ServerName for TLS verification
	serverName := "edge-orchestrator"

	// Try to load TLS credentials (certificates should be mounted from Epic 2.0)
	tlsCreds, err := grpctls.LoadClientCredentials(clientCertPath, clientKeyPath, caCertPath, serverName)
	if err != nil {
		c.logger.Warn("Failed to load TLS credentials for gRPC client, using insecure (not recommended for production)",
			zap.String("edge_id", edgeID),
			zap.Error(err),
			zap.String("client_cert", clientCertPath),
			zap.String("client_key", clientKeyPath),
			zap.String("ca_cert", caCertPath),
			zap.String("server_name", serverName),
			zap.String("note", "TLS certificates should be generated in Epic 2.0 and mounted to containers"))
		// Fall back to insecure for development/testing
		creds = insecure.NewCredentials()
	} else {
		c.logger.Info("Loaded TLS credentials for gRPC client (mTLS enabled)",
			zap.String("edge_id", edgeID),
			zap.String("client_cert", clientCertPath),
			zap.String("server_name", serverName))
		creds = tlsCreds
	}

	// Fail fast if the port isn't reachable to avoid hanging in gRPC dial retries.
	tcpCtx, tcpCancel := context.WithTimeout(ctx, 5*time.Second)
	defer tcpCancel()
	if preflightConn, err := (&net.Dialer{}).DialContext(tcpCtx, "tcp", edgeEndpoint); err != nil {
		c.logger.Error("TCP preflight to Edge failed",
			zap.String("edge_id", edgeID),
			zap.String("endpoint", edgeEndpoint),
			zap.Error(err))
		return nil, fmt.Errorf("tcp preflight to %s failed: %w", edgeEndpoint, err)
	} else {
		_ = preflightConn.Close()
	}

	// Create gRPC connection to Edge (persistent connection)
	// Use a shorter timeout so failures surface quickly during bring-up
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithBlock(),
		grpc.WithReturnConnectionError(), // Surface connection errors immediately instead of waiting for the deadline
		grpc.WithDefaultCallOptions(grpc.WaitForReady(false)),
	}

	// Use context with timeout for connection establishment
	// Use a shorter timeout so failures surface quickly during bring-up
	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	c.logger.Info("Attempting gRPC connection to Edge",
		zap.String("edge_id", edgeID),
		zap.String("endpoint", edgeEndpoint),
		zap.String("timeout", "20s"))

	conn, err = grpc.DialContext(connectCtx, edgeEndpoint, opts...)
	if err != nil {
		// Check if error is due to context cancellation or timeout
		if err == context.Canceled || err == context.DeadlineExceeded {
			c.logger.Error("gRPC connection failed due to context issue",
				zap.String("edge_id", edgeID),
				zap.String("endpoint", edgeEndpoint),
				zap.Error(err),
				zap.String("note", "Context may have been canceled or timed out - this is a security concern for persistent connections"))
		} else {
			c.logger.Error("gRPC dial failed",
				zap.String("edge_id", edgeID),
				zap.String("endpoint", edgeEndpoint),
				zap.Error(err))
		}
		return nil, fmt.Errorf("failed to connect to Edge gRPC server: %w", err)
	}

	// Store connection for reuse
	c.mu.Lock()
	c.connections[edgeID] = conn
	c.mu.Unlock()

	c.logger.Info("Created and cached gRPC connection to Edge",
		zap.String("edge_id", edgeID),
		zap.String("endpoint", edgeEndpoint))

	return conn, nil
}

// VerifyConnectionHealth verifies that the gRPC connection to Edge is healthy and ready
// This is critical for security applications where connection health must be monitored
// The connection must be in Ready or Idle state to be considered healthy
// Uses a longer timeout to allow connection establishment if needed
func (c *EdgeClient) VerifyConnectionHealth(ctx context.Context, edgeID string) error {
	// First check if we have an existing connection
	c.mu.RLock()
	conn, exists := c.connections[edgeID]
	c.mu.RUnlock()

	if exists && conn != nil {
		// Check if existing connection is still valid
		state := conn.GetState()
		if state == connectivity.Ready || state == connectivity.Idle {
			c.logger.Info("gRPC connection health verified - existing connection is alive and ready",
				zap.String("edge_id", edgeID),
				zap.String("state", state.String()),
				zap.String("note", "Connection health monitoring is critical for security applications - connection exists and is healthy"))
			return nil
		}
		// Connection exists but not ready - will be recreated by GetOrCreateConnection
		c.logger.Warn("Existing gRPC connection not ready, will recreate",
			zap.String("edge_id", edgeID),
			zap.String("state", state.String()),
			zap.String("note", "Connection state changed - this may indicate a network issue or Edge restart"))
	} else {
		c.logger.Info("No existing gRPC connection found in pool - will create new connection",
			zap.String("edge_id", edgeID),
			zap.String("note", "Connection should exist from earlier operations (Epic 2.4) - creating new connection"))
	}

	// Get or create connection (this will reuse existing connection if available, or create new one)
	// Use background context with timeout for connection establishment (not tied to request context)
	// This ensures connection can be established even if the original context is canceled
	connCtx, connCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer connCancel() // Cancel when function returns (context is only used for connection establishment)

	conn, err := c.GetOrCreateConnection(connCtx, edgeID)
	if err != nil {
		return fmt.Errorf("failed to get or create connection: %w", err)
	}

	// Verify connection state - Ready or Idle means connection is healthy
	state := conn.GetState()
	if state != connectivity.Ready && state != connectivity.Idle {
		return fmt.Errorf("connection not ready (state: %s) - connection health is critical for security", state.String())
	}

	c.logger.Info("gRPC connection health verified - connection is alive and ready",
		zap.String("edge_id", edgeID),
		zap.String("state", state.String()),
		zap.String("note", "Connection health monitoring is critical for security applications"))

	return nil
}

// RequestSnapshotCapture requests Edge to capture labeled snapshots
// Reuses existing gRPC connection if available
func (c *EdgeClient) RequestSnapshotCapture(ctx context.Context, edgeID string, cameraID string, label string, customLabel string, count int32, autoCapture bool) (*edge.RequestSnapshotCaptureResponse, error) {
	// Small retry helps if the Edge gRPC server is still starting or the tunnel just came up.
	maxAttempts := 3 // Increased to allow for Edge→VM connection establishment wait
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Get or create connection (reuses existing connection)
		// This will check if Edge→VM connection is established before attempting VM→Edge
		conn, err := c.GetOrCreateConnection(ctx, edgeID)
		if err != nil {
			// Check if error is because Edge→VM connection is not established
			errStr := err.Error()
			if strings.Contains(errStr, "not connected to VM") || strings.Contains(errStr, "connection is stale") {
				c.logger.Info("Edge→VM connection not ready, waiting before retry",
					zap.String("edge_id", edgeID),
					zap.Int("attempt", attempt),
					zap.Error(err),
					zap.String("note", "VM will wait for Edge to establish Edge→VM connection before attempting VM→Edge"))

				// Wait for Edge→VM connection to be established (with timeout)
				if attempt < maxAttempts {
					waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
					edgeConnected := c.waitForEdgeConnection(waitCtx, edgeID)
					waitCancel()

					if !edgeConnected {
						c.logger.Warn("Edge→VM connection not established within timeout, will retry",
							zap.String("edge_id", edgeID),
							zap.Int("attempt", attempt))
						lastErr = err

						select {
						case <-ctx.Done():
							return &edge.RequestSnapshotCaptureResponse{
								Accepted: false,
								Message:  ctx.Err().Error(),
							}, nil
						case <-time.After(2 * time.Second):
							continue // Retry after waiting
						}
					} else {
						c.logger.Info("Edge→VM connection established, retrying VM→Edge connection",
							zap.String("edge_id", edgeID),
							zap.Int("attempt", attempt))
						continue // Retry immediately since Edge is now connected
					}
				}
			}

			c.logger.Warn("Failed to get Edge gRPC connection",
				zap.String("edge_id", edgeID),
				zap.Int("attempt", attempt),
				zap.Error(err))
			lastErr = err
		} else {
			// Create ControlService client
			client := edge.NewControlServiceClient(conn)

			// Call RequestSnapshotCapture
			req := &edge.RequestSnapshotCaptureRequest{
				CameraId:    cameraID,
				Label:       label,
				CustomLabel: customLabel,
				Count:       count,
				AutoCapture: autoCapture,
			}

			resp, err := client.RequestSnapshotCapture(ctx, req)
			if err == nil {
				c.logger.Info("RequestSnapshotCapture completed",
					zap.String("edge_id", edgeID),
					zap.String("camera_id", cameraID),
					zap.Bool("accepted", resp.Accepted),
					zap.String("message", resp.Message))
				return resp, nil
			}

			c.logger.Error("Failed to call RequestSnapshotCapture on Edge",
				zap.String("edge_id", edgeID),
				zap.String("camera_id", cameraID),
				zap.Int("attempt", attempt),
				zap.Error(err))
			lastErr = fmt.Errorf("Failed to call Edge: %w", err)
		}

		// Retry only on transient connection issues
		if attempt >= maxAttempts {
			break
		}
		if s, ok := status.FromError(lastErr); ok {
			if s.Code() != codes.Unavailable && s.Code() != codes.DeadlineExceeded {
				break
			}
		}

		c.logger.Warn("RequestSnapshotCapture will retry after transient error",
			zap.String("edge_id", edgeID),
			zap.String("camera_id", cameraID),
			zap.Int("attempt", attempt),
			zap.Error(lastErr),
			zap.String("note", "Retry helps when Edge gRPC server is still coming up"))

		// Drop the cached connection in case it's stale
		c.CloseConnection(edgeID)

		select {
		case <-ctx.Done():
			return &edge.RequestSnapshotCaptureResponse{
				Accepted: false,
				Message:  ctx.Err().Error(),
			}, nil
		case <-time.After(2 * time.Second):
		}
	}

	return &edge.RequestSnapshotCaptureResponse{
		Accepted: false,
		Message:  fmt.Sprintf("Failed to call Edge: %v", lastErr),
	}, nil
}

// waitForEdgeConnection waits for Edge→VM connection to be established and active
// Returns true if Edge is connected and authenticated, false if timeout or context canceled
func (c *EdgeClient) waitForEdgeConnection(ctx context.Context, edgeID string) bool {
	if c.edgeAPIServer == nil {
		return false
	}

	heartbeatTimeout := 5 * time.Minute
	ticker := time.NewTicker(500 * time.Millisecond) // Check every 500ms
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			edgeConn, edgeConnected := c.edgeAPIServer.GetConnection(edgeID)
			if !edgeConnected || edgeConn == nil {
				continue // Keep waiting
			}

			// Check if Edge is actively sending heartbeats
			edgeConn.mu.RLock()
			lastHeartbeat := edgeConn.LastHeartbeat
			edgeConn.mu.RUnlock()

			if !lastHeartbeat.IsZero() && time.Since(lastHeartbeat) <= heartbeatTimeout {
				c.logger.Info("Edge→VM connection established and active",
					zap.String("edge_id", edgeID),
					zap.Time("last_heartbeat", lastHeartbeat),
					zap.Duration("heartbeat_age", time.Since(lastHeartbeat)))
				return true
			}
		}
	}
}

// GetEdgeWireGuardIP gets the WireGuard IP address for an Edge
func (c *EdgeClient) GetEdgeWireGuardIP(edgeID string) (string, error) {
	if c.wgServer == nil {
		return "", fmt.Errorf("WireGuard server not available")
	}

	// First, try to get public key from EdgeAPIServer's connection map (most reliable)
	// This works even if edge isn't in database yet, as long as it's connected via gRPC
	if c.edgeAPIServer != nil {
		c.edgeAPIServer.mu.RLock()
		conn, exists := c.edgeAPIServer.connections[edgeID]
		c.edgeAPIServer.mu.RUnlock()

		if exists && conn != nil && conn.PublicKey != "" {
			publicKey, err := wgtypes.ParseKey(conn.PublicKey)
			if err == nil {
				peerInfo, exists := c.wgServer.GetPeerInfo(publicKey)
				if exists && peerInfo != nil && len(peerInfo.AllowedIPs) > 0 {
					edgeIP := peerInfo.AllowedIPs[0].IP.String()
					c.logger.Info("Found edge IP from EdgeAPIServer connection map",
						zap.String("edge_id", edgeID),
						zap.String("ip", edgeIP))
					return edgeIP, nil
				}
			}
		} else {
			c.logger.Debug("Edge not found in EdgeAPIServer connection map, trying database",
				zap.String("edge_id", edgeID),
				zap.Bool("exists", exists))
		}
	}

	// Fallback: Query database to get public key for edge ID
	var publicKeyStr string
	query := `SELECT wireguard_public_key FROM edges WHERE edge_id = ?`
	row := c.wgServer.db.QueryRowContext(context.Background(), query, edgeID)
	err := row.Scan(&publicKeyStr)

	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("edge %s not found in database", edgeID)
		}
		return "", fmt.Errorf("failed to get public key for edge %s: %w", edgeID, err)
	}

	publicKey, err := wgtypes.ParseKey(publicKeyStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse public key: %w", err)
	}

	// Get peer info using public key
	peerInfo, exists := c.wgServer.GetPeerInfo(publicKey)
	if !exists || peerInfo == nil {
		return "", fmt.Errorf("peer not found for edge %s", edgeID)
	}

	if len(peerInfo.AllowedIPs) == 0 {
		return "", fmt.Errorf("no allowed IPs for edge %s", edgeID)
	}

	// Return first allowed IP (Edge's WireGuard IP)
	edgeIP := peerInfo.AllowedIPs[0].IP.String()
	return edgeIP, nil
}

// CloseConnection closes the gRPC connection for an Edge (cleanup)
func (c *EdgeClient) CloseConnection(edgeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if conn, exists := c.connections[edgeID]; exists {
		conn.Close()
		delete(c.connections, edgeID)
		c.logger.Info("Closed gRPC connection to Edge",
			zap.String("edge_id", edgeID))
	}
}

// CloseAllConnections closes all gRPC connections (cleanup on shutdown)
func (c *EdgeClient) CloseAllConnections() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for edgeID, conn := range c.connections {
		conn.Close()
		c.logger.Info("Closed gRPC connection to Edge",
			zap.String("edge_id", edgeID))
	}
	c.connections = make(map[string]*grpc.ClientConn)
}
