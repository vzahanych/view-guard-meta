package tunnelgateway

import (
	"context"
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

	config       *config.Config
	logger       *logging.Logger
	db           *database.DB
	wgServer     *WireGuardServer
	auth         *EdgeAuth
	eventBus     *service.EventBus
	grpcServer   *grpc.Server
	listener     net.Listener
	connections  map[string]*EdgeConnection // edge_id -> connection info
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc

	// Service interfaces (will be set when services are available)
	eventReceiver    EventReceiver
	datasetReceiver  DatasetReceiver
	modelDistributor ModelDistributor
	telemetryHandler TelemetryHandler
}

// EdgeConnection tracks connection state for an Edge Appliance
type EdgeConnection struct {
	EdgeID           string
	PublicKey        string
	ConnectedAt      time.Time
	LastHeartbeat    time.Time
	LastTelemetry    time.Time
	Latency          time.Duration
	ConnectionCount  int64
	mu               sync.RWMutex
}

// EventReceiver interface for receiving events from Edge
type EventReceiver interface {
	ReceiveEvent(ctx context.Context, edgeID string, event *edge.Event) (string, error) // Returns event ID
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
	}

	return server, nil
}

// SetEventBus sets the event bus for publishing events
func (s *EdgeAPIServer) SetEventBus(bus *service.EventBus) {
	s.eventBus = bus
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

	// Start connection monitoring
	go s.monitorConnections(ctx)

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
		return "", fmt.Errorf("edge not found for WireGuard peer: %w", err)
	}

	// Update connection tracking
	s.updateConnection(edgeID, peerAddr)

	return edgeID, nil
}

// updateConnection updates connection tracking for an edge
func (s *EdgeAPIServer) updateConnection(edgeID, peerAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, exists := s.connections[edgeID]
	if !exists {
		conn = &EdgeConnection{
			EdgeID:      edgeID,
			ConnectedAt: time.Now(),
		}
		s.connections[edgeID] = conn
	}

	conn.mu.Lock()
	conn.ConnectionCount++
	conn.mu.Unlock()

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
func (s *EdgeAPIServer) GetConnection(edgeID string) (*EdgeConnection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conn, exists := s.connections[edgeID]
	return conn, exists
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
			Success:      false,
			ReceivedCount: 0,
			ErrorMessage: "event receiver not configured",
		}, nil
	}

	// Convert proto events and forward to event receiver
	eventIDs, err := s.eventReceiver.ReceiveEvents(ctx, edgeID, req.Events)
	if err != nil {
		s.logger.Error("Failed to receive events", zap.String("edge_id", edgeID), zap.Error(err))
		return &edge.SendEventsResponse{
			Success:      false,
			ReceivedCount: 0,
			ErrorMessage: err.Error(),
		}, nil
	}

	s.logger.Info("Received events from Edge",
		zap.String("edge_id", edgeID),
		zap.Int("count", len(req.Events)),
		zap.Int("received", len(eventIDs)))

	return &edge.SendEventsResponse{
		Success:      true,
		ReceivedCount: int32(len(eventIDs)),
		EventIds:     eventIDs,
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

	// Forward to event receiver
	eventID, err := s.eventReceiver.ReceiveEvent(ctx, edgeID, req.Event)
	if err != nil {
		s.logger.Error("Failed to receive event", zap.String("edge_id", edgeID), zap.Error(err))
		return &edge.SendEventResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

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

	return &edge.HeartbeatResponse{
		Success:        true,
		ServerTimestamp: time.Now().UnixNano(),
	}, nil
}

// ControlService implementation

// GetConfig retrieves Edge configuration (placeholder for future implementation)
func (s *EdgeAPIServer) GetConfig(ctx context.Context, req *edge.GetConfigRequest) (*edge.GetConfigResponse, error) {
	// TODO: Implement configuration retrieval
	// For now, return empty config
	return &edge.GetConfigResponse{
		Success:     true,
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

// wrappedServerStream wraps grpc.ServerStream to override context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
