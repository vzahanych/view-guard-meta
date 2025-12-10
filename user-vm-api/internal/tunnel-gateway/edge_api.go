package tunnelgateway

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/service"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// EdgeAPIServer implements Edge-facing gRPC APIs
type EdgeAPIServer struct {
	edge.UnimplementedEventServiceServer
	edge.UnimplementedTelemetryServiceServer
	edge.UnimplementedControlServiceServer

	config      *config.Config
	logger      *logging.Logger
	db          *database.DB
	wgServer    *WireGuardServer
	auth        *EdgeAuth
	eventBus    *service.EventBus
	grpcServer  *grpc.Server
	listener    net.Listener
	connections map[string]*EdgeConnection // edge_id -> connection info
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc

	capStore *CapabilityStore

	// Connection monitor for continuous status monitoring
	connectionMonitor *ConnectionMonitor

	// Service interfaces (will be set when services are available)
	eventReceiver    EventReceiver
	datasetReceiver  DatasetReceiver
	modelDistributor ModelDistributor
	telemetryHandler TelemetryHandler
}

// EdgeConnection tracks connection state for an Edge Appliance
type EdgeConnection struct {
	EdgeID          string
	PublicKey       string
	ConnectedAt     time.Time
	LastHeartbeat   time.Time
	LastTelemetry   time.Time
	Latency         time.Duration
	ConnectionCount int64
	mu              sync.RWMutex
}

// EventReceiver interface for receiving events from Edge
type EventReceiver interface {
	ReceiveEvent(ctx context.Context, edgeID string, event *edge.Event) (string, error)       // Returns event ID
	ReceiveEvents(ctx context.Context, edgeID string, events []*edge.Event) ([]string, error) // Returns event IDs
}

// DatasetReceiver interface for receiving dataset uploads from Edge
type DatasetReceiver interface {
	ReceiveDataset(ctx context.Context, edgeID string, datasetPath string, metadata map[string]string) error
}

// ModelDistributor interface for distributing models to Edge
type ModelDistributor interface {
	GetModel(ctx context.Context, edgeID string, modelID string) ([]byte, error)
	ListModels(ctx context.Context, edgeID string) ([]string, error)
}

// TelemetryHandler interface for handling telemetry from Edge
type TelemetryHandler interface {
	HandleTelemetry(ctx context.Context, edgeID string, telemetry *edge.TelemetryData) error
	HandleHeartbeat(ctx context.Context, edgeID string, timestamp int64) error
}

// NewEdgeAPIServer creates a new Edge API server
func NewEdgeAPIServer(cfg *config.Config, log *logging.Logger, db *database.DB, wgServer *WireGuardServer, auth *EdgeAuth) (*EdgeAPIServer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	server := &EdgeAPIServer{
		config:      cfg,
		logger:      log,
		db:          db,
		wgServer:    wgServer,
		auth:        auth,
		connections: make(map[string]*EdgeConnection),
		ctx:         ctx,
		cancel:      cancel,
		capStore:    NewCapabilityStore(db),
	}

	// Create connection monitor
	server.connectionMonitor = NewConnectionMonitor(cfg, log, db, server, wgServer)

	return server, nil
}

// SetEventBus sets the event bus for publishing events
func (s *EdgeAPIServer) SetEventBus(bus *service.EventBus) {
	s.eventBus = bus
	// Also set event bus on capability store for state transition events
	if s.capStore != nil {
		s.capStore.SetEventBus(bus)
	}
	// Set event bus on connection monitor
	if s.connectionMonitor != nil {
		s.connectionMonitor.SetEventBus(bus)
	}
}

// GetCapabilityStore returns the capability store instance
func (s *EdgeAPIServer) GetCapabilityStore() *CapabilityStore {
	return s.capStore
}

// GetConnectionMonitor returns the connection monitor instance
func (s *EdgeAPIServer) GetConnectionMonitor() *ConnectionMonitor {
	return s.connectionMonitor
}

// GetDB returns the database instance
func (s *EdgeAPIServer) GetDB() *database.DB {
	return s.db
}

// GetWireGuardServer returns the WireGuard server instance
func (s *EdgeAPIServer) GetWireGuardServer() *WireGuardServer {
	return s.wgServer
}

// SetEventReceiver sets the event receiver service
func (s *EdgeAPIServer) SetEventReceiver(receiver EventReceiver) {
	s.eventReceiver = receiver
}

// SetDatasetReceiver sets the dataset receiver service
func (s *EdgeAPIServer) SetDatasetReceiver(receiver DatasetReceiver) {
	s.datasetReceiver = receiver
}

// SetModelDistributor sets the model distributor service
func (s *EdgeAPIServer) SetModelDistributor(distributor ModelDistributor) {
	s.modelDistributor = distributor
}

// SetTelemetryHandler sets the telemetry handler service
func (s *EdgeAPIServer) SetTelemetryHandler(handler TelemetryHandler) {
	s.telemetryHandler = handler
}

// Name returns the service name
func (s *EdgeAPIServer) Name() string {
	return "edge-api-server"
}

// Start starts the Edge API gRPC server
func (s *EdgeAPIServer) Start(ctx context.Context) error {
	// Determine listen address (default: listen on WireGuard interface)
	listenAddr := ":50051" // Default gRPC port
	if s.config.UserVMAPI.WireGuardServer.Enabled {
		// Listen on WireGuard interface IP
		// For PoC, we'll listen on all interfaces and rely on WireGuard routing
		listenAddr = fmt.Sprintf(":%d", 50051)
	}

	s.logger.Info("Starting Edge API gRPC server", zap.String("address", listenAddr))

	// Create listener
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = listener

	// Create gRPC server with authentication interceptor
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(s.authInterceptor),
		grpc.StreamInterceptor(s.authStreamInterceptor),
		// Note: For production, add TLS credentials here
		// grpc.Creds(credentials.NewTLS(tlsConfig))
	}

	s.grpcServer = grpc.NewServer(opts...)

	// Register services
	edge.RegisterEventServiceServer(s.grpcServer, s)
	edge.RegisterTelemetryServiceServer(s.grpcServer, s)
	edge.RegisterControlServiceServer(s.grpcServer, s)

	// Start server in goroutine
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			s.logger.Error("Edge API gRPC server error", zap.Error(err))
		}
	}()

	// Start legacy connection monitoring (for backward compatibility)
	go s.monitorConnections(ctx)

	// Start comprehensive connection monitor service
	if s.connectionMonitor != nil {
		if err := s.connectionMonitor.Start(ctx); err != nil {
			s.logger.Error("Failed to start connection monitor", zap.Error(err))
			// Don't fail server start if monitor fails
		}
	}

	s.logger.Info("Edge API gRPC server started", zap.String("address", listenAddr))
	return nil
}

// Stop stops the Edge API gRPC server
func (s *EdgeAPIServer) Stop(ctx context.Context) error {
	s.logger.Info("Stopping Edge API gRPC server")

	s.cancel()

	if s.grpcServer != nil {
		// Graceful shutdown
		stopped := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
			s.logger.Info("Edge API gRPC server stopped gracefully")
		case <-ctx.Done():
			s.grpcServer.Stop()
			s.logger.Warn("Edge API gRPC server force stopped")
		}
	}

	if s.listener != nil {
		s.listener.Close()
	}

	// Stop connection monitor
	if s.connectionMonitor != nil {
		if err := s.connectionMonitor.Stop(ctx); err != nil {
			s.logger.Warn("Error stopping connection monitor", zap.Error(err))
		}
	}

	return nil
}

// authInterceptor authenticates Edge connections using WireGuard peer info
func (s *EdgeAPIServer) authInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	edgeID, err := s.authenticateConnection(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
	}

	// Add edge_id to context
	ctx = context.WithValue(ctx, "edge_id", edgeID)

	return handler(ctx, req)
}

// authStreamInterceptor authenticates Edge connections for streaming RPCs
func (s *EdgeAPIServer) authStreamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx := ss.Context()
	edgeID, err := s.authenticateConnection(ctx)
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
	}

	// Add edge_id to context
	ctx = context.WithValue(ctx, "edge_id", edgeID)
	wrappedStream := &wrappedServerStream{ServerStream: ss, ctx: ctx}

	return handler(srv, wrappedStream)
}

// authenticateConnection authenticates a connection and returns the edge ID
func (s *EdgeAPIServer) authenticateConnection(ctx context.Context) (string, error) {
	// Get peer information from gRPC context
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("no peer information in context")
	}

	peerAddr := p.Addr.String()
	s.logger.Debug("Authenticating connection", zap.String("peer_addr", peerAddr))

	// Extract IP address
	host, _, err := net.SplitHostPort(peerAddr)
	if err != nil {
		return "", fmt.Errorf("invalid peer address: %w", err)
	}

	// For WireGuard connections, identify peer by IP and match to WireGuard peer
	// Get WireGuard device to find peer by allowed IPs
	dev, err := s.wgServer.client.Device(s.wgServer.iface)
	if err != nil {
		return "", fmt.Errorf("failed to get WireGuard device: %w", err)
	}

	peerIP := net.ParseIP(host)
	if peerIP == nil {
		return "", fmt.Errorf("invalid peer IP: %s", host)
	}

	// Find matching WireGuard peer by allowed IPs
	var matchedPublicKey string
	for _, wgPeer := range dev.Peers {
		for _, allowedIP := range wgPeer.AllowedIPs {
			if allowedIP.Contains(peerIP) {
				matchedPublicKey = wgPeer.PublicKey.String()
				break
			}
		}
		if matchedPublicKey != "" {
			break
		}
	}

	if matchedPublicKey == "" {
		return "", fmt.Errorf("no WireGuard peer found for IP: %s", host)
	}

	// Look up edge by WireGuard public key
	var edgeID string
	err = s.db.QueryRowContext(ctx,
		"SELECT edge_id FROM edges WHERE wireguard_public_key = ? AND status = 'active'",
		matchedPublicKey).Scan(&edgeID)

	if err != nil {
		// Edge not found for this WireGuard public key
		// In production: Edge ID should be pre-registered by SaaS components
		// For local test: Edge ID should be registered by test script before edge connects
		if err == sql.ErrNoRows {
			// Try to find edge by checking if there's a known edge_id that should match this public key
			// For local test environment, we might need to look up by a default edge_id
			// But in production, this should never happen - edge must be pre-registered
			s.logger.Warn("Edge not found for WireGuard public key - edge must be pre-registered",
				zap.String("public_key", matchedPublicKey))
			return "", fmt.Errorf("edge not registered: WireGuard public key not found in edge management system. Edge must be pre-registered before connecting")
		} else {
			return "", fmt.Errorf("failed to lookup edge for WireGuard peer: %w", err)
		}
	}

	// Validate that the edge exists and is active
	var status string
	checkErr := s.db.QueryRowContext(ctx,
		"SELECT status FROM edges WHERE edge_id = ?", edgeID).Scan(&status)
	if checkErr == sql.ErrNoRows {
		// Edge ID doesn't exist in database - this should not happen if edge was properly registered
		s.logger.Error("Edge ID not found in database - registration inconsistency",
			zap.String("edge_id", edgeID),
			zap.String("public_key", matchedPublicKey))
		return "", fmt.Errorf("edge ID %s not found in edge management system - edge must be pre-registered", edgeID)
	} else if checkErr != nil {
		return "", fmt.Errorf("failed to validate edge status: %w", checkErr)
	}

	// Update edge status to active and update WireGuard public key if needed
	if status != "active" {
		now := time.Now().Unix()
		_, updateErr := s.db.ExecContext(ctx,
			"UPDATE edges SET status = 'active', last_seen = ?, updated_at = ?, wireguard_public_key = ? WHERE edge_id = ?",
			now, now, matchedPublicKey, edgeID)
		if updateErr != nil {
			return "", fmt.Errorf("failed to update edge status: %w", updateErr)
		}
		s.logger.Info("Updated edge status to active",
			zap.String("edge_id", edgeID),
			zap.String("old_status", status),
			zap.String("public_key", matchedPublicKey))
	} else {
		// Update WireGuard public key if it changed
		var existingKey string
		keyErr := s.db.QueryRowContext(ctx,
			"SELECT wireguard_public_key FROM edges WHERE edge_id = ?", edgeID).Scan(&existingKey)
		if keyErr == nil && existingKey != matchedPublicKey {
			now := time.Now().Unix()
			_, updateErr := s.db.ExecContext(ctx,
				"UPDATE edges SET wireguard_public_key = ?, updated_at = ? WHERE edge_id = ?",
				matchedPublicKey, now, edgeID)
			if updateErr != nil {
				s.logger.Warn("Failed to update WireGuard public key",
					zap.String("edge_id", edgeID),
					zap.Error(updateErr))
			} else {
				s.logger.Info("Updated WireGuard public key for edge",
					zap.String("edge_id", edgeID),
					zap.String("new_public_key", matchedPublicKey))
			}
		}
	}

	// Ensure peer is in WireGuard server
	if s.wgServer != nil {
		publicKey, parseErr := wgtypes.ParseKey(matchedPublicKey)
		if parseErr == nil {
			_, exists := s.wgServer.GetPeerInfo(publicKey)
			if !exists {
				allowedIP := DeriveAllowedIP(edgeID, 0)
				allowedIPs := []net.IPNet{allowedIP}
				if addErr := s.wgServer.AddPeer(publicKey, allowedIPs); addErr != nil {
					s.logger.Warn("Failed to add peer to WireGuard server",
						zap.String("edge_id", edgeID),
						zap.Error(addErr))
				} else {
					s.logger.Info("Added peer to WireGuard server",
						zap.String("edge_id", edgeID),
						zap.String("public_key", matchedPublicKey),
						zap.String("allowed_ip", allowedIP.String()))
				}
			}
		}
	}

	// Update connection tracking
	s.updateConnection(edgeID, peerAddr)

	// Update connection monitor state
	if s.connectionMonitor != nil {
		s.connectionMonitor.UpdateConnectionState(edgeID, time.Now())
	}

	return edgeID, nil
}

// updateConnection updates connection tracking for an edge
func (s *EdgeAPIServer) updateConnection(edgeID, peerAddr string) {
	now := time.Now()

	s.mu.Lock()
	conn, exists := s.connections[edgeID]
	if !exists {
		conn = &EdgeConnection{
			EdgeID:      edgeID,
			ConnectedAt: now,
		}
		s.connections[edgeID] = conn
	}

	conn.mu.Lock()
	conn.ConnectionCount++
	conn.mu.Unlock()
	s.mu.Unlock()

	// Update connection monitor state machine
	if s.connectionMonitor != nil {
		s.connectionMonitor.UpdateConnectionState(edgeID, now)
	}

	// Publish connection event
	if s.eventBus != nil {
		s.eventBus.Publish(service.Event{
			Type:      "edge.connected",
			Timestamp: time.Now().Unix(),
			Data: map[string]interface{}{
				"edge_id":   edgeID,
				"peer_addr": peerAddr,
			},
		})
	}
}

// monitorConnections monitors Edge connections and detects disconnections
func (s *EdgeAPIServer) monitorConnections(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkConnections()
		}
	}
}

// checkConnections checks all connections and detects disconnections
func (s *EdgeAPIServer) checkConnections() {
	s.mu.RLock()
	connections := make(map[string]*EdgeConnection)
	for k, v := range s.connections {
		connections[k] = v
	}
	s.mu.RUnlock()

	now := time.Now()
	for edgeID, conn := range connections {
		conn.mu.RLock()
		lastHeartbeat := conn.LastHeartbeat
		conn.mu.RUnlock()

		// Consider disconnected if no heartbeat for 5 minutes
		if !lastHeartbeat.IsZero() && now.Sub(lastHeartbeat) > 5*time.Minute {
			s.handleDisconnection(edgeID)
		}
	}
}

// handleDisconnection handles Edge disconnection
func (s *EdgeAPIServer) handleDisconnection(edgeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.connections[edgeID]; !exists {
		return
	}

	delete(s.connections, edgeID)

	// Update database
	_, err := s.db.ExecContext(context.Background(),
		"UPDATE edges SET last_seen = ?, updated_at = ? WHERE edge_id = ?",
		time.Now().Unix(), time.Now().Unix(), edgeID)
	if err != nil {
		s.logger.Warn("Failed to update edge last_seen", zap.String("edge_id", edgeID), zap.Error(err))
	}

	// Publish disconnection event
	if s.eventBus != nil {
		s.eventBus.Publish(service.Event{
			Type:      "edge.disconnected",
			Timestamp: time.Now().Unix(),
			Data: map[string]interface{}{
				"edge_id": edgeID,
			},
		})
	}

	s.logger.Info("Edge disconnected", zap.String("edge_id", edgeID))
}

// GetConnection returns connection info for an edge
// This method maintains backward compatibility while ConnectionMonitor provides comprehensive state tracking
func (s *EdgeAPIServer) GetConnection(edgeID string) (*EdgeConnection, bool) {
	s.mu.RLock()
	conn, exists := s.connections[edgeID]
	s.mu.RUnlock()

	// If connection exists in map, return it
	// ConnectionMonitor tracks more comprehensive state via GetConnectionState()
	if exists {
		return conn, true
	}

	// If not in map but ConnectionMonitor has state info, Edge might be reconnecting
	// Return nil to indicate not actively connected via gRPC
	return nil, false
}

// GetConnectedEdges returns list of connected edge IDs
func (s *EdgeAPIServer) GetConnectedEdges() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	edges := make([]string, 0, len(s.connections))
	for edgeID := range s.connections {
		edges = append(edges, edgeID)
	}
	return edges
}

// EventService implementation

// SendEvents handles batch event upload from Edge
func (s *EdgeAPIServer) SendEvents(ctx context.Context, req *edge.SendEventsRequest) (*edge.SendEventsResponse, error) {
	edgeID := ctx.Value("edge_id").(string)

	if s.eventReceiver == nil {
		return &edge.SendEventsResponse{
			Success:       false,
			ReceivedCount: 0,
			ErrorMessage:  "event receiver not configured",
		}, nil
	}

	// Record gRPC call for metrics
	success := false
	defer func() {
		if s.connectionMonitor != nil {
			s.connectionMonitor.RecordGRPCCall(edgeID, success)
		}
	}()

	// Convert proto events and forward to event receiver
	eventIDs, err := s.eventReceiver.ReceiveEvents(ctx, edgeID, req.Events)
	if err != nil {
		s.logger.Error("Failed to receive events", zap.String("edge_id", edgeID), zap.Error(err))
		return &edge.SendEventsResponse{
			Success:       false,
			ReceivedCount: 0,
			ErrorMessage:  err.Error(),
		}, nil
	}

	s.logger.Info("Received events from Edge",
		zap.String("edge_id", edgeID),
		zap.Int("count", len(req.Events)),
		zap.Int("received", len(eventIDs)))

	success = true
	return &edge.SendEventsResponse{
		Success:       true,
		ReceivedCount: int32(len(eventIDs)),
		EventIds:      eventIDs,
	}, nil
}

// SendEvent handles single event upload from Edge
func (s *EdgeAPIServer) SendEvent(ctx context.Context, req *edge.SendEventRequest) (*edge.SendEventResponse, error) {
	edgeID := ctx.Value("edge_id").(string)

	if s.eventReceiver == nil {
		return &edge.SendEventResponse{
			Success:      false,
			ErrorMessage: "event receiver not configured",
		}, nil
	}

	// Record gRPC call for metrics
	success := false
	defer func() {
		if s.connectionMonitor != nil {
			s.connectionMonitor.RecordGRPCCall(edgeID, success)
		}
	}()

	// Forward to event receiver
	eventID, err := s.eventReceiver.ReceiveEvent(ctx, edgeID, req.Event)
	if err != nil {
		s.logger.Error("Failed to receive event", zap.String("edge_id", edgeID), zap.Error(err))
		return &edge.SendEventResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	success = true

	s.logger.Debug("Received event from Edge",
		zap.String("edge_id", edgeID),
		zap.String("event_id", eventID))

	return &edge.SendEventResponse{
		Success: true,
		EventId: eventID,
	}, nil
}

// TelemetryService implementation

// SendTelemetry handles telemetry data from Edge
func (s *EdgeAPIServer) SendTelemetry(ctx context.Context, req *edge.SendTelemetryRequest) (*edge.SendTelemetryResponse, error) {
	edgeID := ctx.Value("edge_id").(string)

	if s.telemetryHandler == nil {
		return &edge.SendTelemetryResponse{
			Success:      false,
			ErrorMessage: "telemetry handler not configured",
		}, nil
	}

	// Forward to telemetry handler
	if err := s.telemetryHandler.HandleTelemetry(ctx, edgeID, req.Telemetry); err != nil {
		s.logger.Error("Failed to handle telemetry", zap.String("edge_id", edgeID), zap.Error(err))
		return &edge.SendTelemetryResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Update connection tracking
	s.mu.Lock()
	if conn, exists := s.connections[edgeID]; exists {
		conn.mu.Lock()
		conn.LastTelemetry = time.Now()
		conn.mu.Unlock()
	}
	s.mu.Unlock()

	s.logger.Debug("Received telemetry from Edge", zap.String("edge_id", edgeID))

	return &edge.SendTelemetryResponse{
		Success: true,
	}, nil
}

// Heartbeat handles heartbeat from Edge
func (s *EdgeAPIServer) Heartbeat(ctx context.Context, req *edge.HeartbeatRequest) (*edge.HeartbeatResponse, error) {
	edgeID := ctx.Value("edge_id").(string)

	// Record gRPC call for metrics
	success := false
	defer func() {
		if s.connectionMonitor != nil {
			s.connectionMonitor.RecordGRPCCall(edgeID, success)
		}
	}()

	if s.telemetryHandler == nil {
		return &edge.HeartbeatResponse{
			Success: false,
		}, status.Errorf(codes.Internal, "telemetry handler not configured")
	}

	// Forward to telemetry handler
	if err := s.telemetryHandler.HandleHeartbeat(ctx, edgeID, req.Timestamp); err != nil {
		s.logger.Error("Failed to handle heartbeat", zap.String("edge_id", edgeID), zap.Error(err))
		return &edge.HeartbeatResponse{
			Success: false,
		}, status.Errorf(codes.Internal, "failed to handle heartbeat: %v", err)
	}

	// Update connection tracking
	s.mu.Lock()
	if conn, exists := s.connections[edgeID]; exists {
		conn.mu.Lock()
		conn.LastHeartbeat = time.Now()
		// Calculate latency if timestamp provided
		if req.Timestamp > 0 {
			clientTime := time.Unix(0, req.Timestamp)
			conn.Latency = time.Since(clientTime) / 2 // Approximate one-way latency
		}
		conn.mu.Unlock()
	}
	s.mu.Unlock()

	// Update connection monitor heartbeat
	if s.connectionMonitor != nil {
		s.connectionMonitor.UpdateConnectionState(edgeID, time.Now())
	}

	success = true
	return &edge.HeartbeatResponse{
		Success:         true,
		ServerTimestamp: time.Now().UnixNano(),
	}, nil
}

// ControlService implementation

// GetConfig retrieves Edge configuration (placeholder for future implementation)
func (s *EdgeAPIServer) GetConfig(ctx context.Context, req *edge.GetConfigRequest) (*edge.GetConfigResponse, error) {
	// TODO: Implement configuration retrieval
	// For now, return empty config
	return &edge.GetConfigResponse{
		Success:    true,
		ConfigJson: "{}",
	}, nil
}

// UpdateConfig updates Edge configuration (placeholder for future implementation)
func (s *EdgeAPIServer) UpdateConfig(ctx context.Context, req *edge.UpdateConfigRequest) (*edge.UpdateConfigResponse, error) {
	edgeID := ctx.Value("edge_id").(string)

	// TODO: Implement configuration update
	s.logger.Info("Config update requested", zap.String("edge_id", edgeID))

	return &edge.UpdateConfigResponse{
		Success: true,
	}, nil
}

// RestartService restarts a service on Edge (placeholder for future implementation)
func (s *EdgeAPIServer) RestartService(ctx context.Context, req *edge.RestartServiceRequest) (*edge.RestartServiceResponse, error) {
	edgeID := ctx.Value("edge_id").(string)

	// TODO: Implement service restart command
	s.logger.Info("Service restart requested",
		zap.String("edge_id", edgeID),
		zap.String("service", req.ServiceName))

	return &edge.RestartServiceResponse{
		Success: true,
	}, nil
}

// SyncCapabilities stores camera/dataset readiness data from Edge
func (s *EdgeAPIServer) SyncCapabilities(ctx context.Context, req *edge.SyncCapabilitiesRequest) (*edge.SyncCapabilitiesResponse, error) {
	edgeID := ctx.Value("edge_id").(string)

	// Record gRPC call for metrics
	success := false
	defer func() {
		if s.connectionMonitor != nil {
			s.connectionMonitor.RecordGRPCCall(edgeID, success)
		}
	}()

	// Check connection status before processing sync
	if s.connectionMonitor != nil {
		stateInfo, exists := s.connectionMonitor.GetConnectionState(edgeID)
		if exists && stateInfo != nil {
			state := stateInfo.State
			if state == StateDisconnected {
				s.logger.Warn("Edge is disconnected, rejecting capability sync",
					zap.String("edge_id", edgeID),
					zap.String("state", string(state)),
				)
				return &edge.SyncCapabilitiesResponse{
					Success:      false,
					ErrorMessage: "Edge is disconnected",
				}, nil
			}
		}
	}

	if s.capStore == nil {
		return &edge.SyncCapabilitiesResponse{
			Success:      false,
			ErrorMessage: "capability store not configured",
		}, nil
	}

	syncedAt := time.Unix(0, req.SyncedAt)
	if syncedAt.IsZero() {
		syncedAt = time.Now()
	}

	// Persist camera capabilities and dataset status
	if err := s.capStore.UpsertCapabilities(ctx, edgeID, req.Cameras, syncedAt); err != nil {
		s.logger.Error("Failed to persist capability sync", zap.String("edge_id", edgeID), zap.Error(err))
		return &edge.SyncCapabilitiesResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Count cameras needing snapshots
	camerasNeedingSnapshots := 0
	for _, cam := range req.Cameras {
		if cam.SnapshotRequired {
			camerasNeedingSnapshots++
		}
	}

	s.logger.Info("Capability sync received and persisted",
		zap.String("edge_id", edgeID),
		zap.Int("cameras", len(req.Cameras)),
		zap.Int("cameras_needing_snapshots", camerasNeedingSnapshots),
		zap.Time("synced_at", syncedAt),
	)

	// Publish event for capability sync completion
	if s.eventBus != nil {
		s.eventBus.Publish(service.Event{
			Type:      "edge.capability_sync",
			Timestamp: time.Now().Unix(),
			Data: map[string]interface{}{
				"edge_id":                   edgeID,
				"camera_count":              len(req.Cameras),
				"cameras_needing_snapshots": camerasNeedingSnapshots,
				"synced_at":                 syncedAt.Unix(),
			},
		})
	}

	success = true
	return &edge.SyncCapabilitiesResponse{Success: true}, nil
}

// wrappedServerStream wraps grpc.ServerStream to override context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
