package tunnelgateway

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database/migrations"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/service"
)

// Mock implementations for testing

type mockEventReceiver struct {
	events []*edge.Event
	ids    []string
}

func (m *mockEventReceiver) ReceiveEvent(ctx context.Context, edgeID string, event *edge.Event) (string, error) {
	m.events = append(m.events, event)
	id := "event-" + edgeID + "-" + time.Now().Format("20060102150405")
	m.ids = append(m.ids, id)
	return id, nil
}

func (m *mockEventReceiver) ReceiveEvents(ctx context.Context, edgeID string, events []*edge.Event) ([]string, error) {
	m.events = append(m.events, events...)
	ids := make([]string, len(events))
	for i := range events {
		ids[i] = "event-" + edgeID + "-" + time.Now().Format("20060102150405")
		m.ids = append(m.ids, ids[i])
	}
	return ids, nil
}

type mockTelemetryHandler struct {
	telemetry []*edge.TelemetryData
	heartbeats []int64
}

func (m *mockTelemetryHandler) HandleTelemetry(ctx context.Context, edgeID string, telemetry *edge.TelemetryData) error {
	m.telemetry = append(m.telemetry, telemetry)
	return nil
}

func (m *mockTelemetryHandler) HandleHeartbeat(ctx context.Context, edgeID string, timestamp int64) error {
	m.heartbeats = append(m.heartbeats, timestamp)
	return nil
}

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()

	cfg := database.DefaultConfig(":memory:")
	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Apply migrations so required tables (edges, events, etc.) exist for tests.
	ctx := context.Background()
	migrator := migrations.NewMigrator(db)
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	return db
}

func setupTestServer(t *testing.T) (*EdgeAPIServer, *database.DB, *WireGuardServer, *EdgeAuth, func()) {
	// Setup test database
	db := setupTestDB(t)

	// Setup test config
	cfg := &config.Config{
		UserVMAPI: config.UserVMAPIConfig{
			WireGuardServer: config.WireGuardServerConfig{
				Enabled:    true,
				Interface:  "wg-test",
				ListenPort: 51820,
			},
		},
		Log: config.LogConfig{
			Level:  "debug",
			Format: "text",
			Output: "stdout",
		},
	}

	// Setup logger
	log, err := logging.New(logging.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Setup WireGuard server (mock)
	wgServer, err := NewWireGuardServer(cfg, log, db)
	if err != nil {
		t.Fatalf("Failed to create WireGuard server: %v", err)
	}

	// Setup Edge auth
	auth := NewEdgeAuth(cfg, log, db, wgServer)

	// Setup Edge API server
	apiServer, err := NewEdgeAPIServer(cfg, log, db, wgServer, auth)
	if err != nil {
		t.Fatalf("Failed to create Edge API server: %v", err)
	}

	// Setup event bus
	eventBus := service.NewEventBus(100)
	apiServer.SetEventBus(eventBus)
	wgServer.SetEventBus(eventBus)

	cleanup := func() {
		db.Close()
	}

	return apiServer, db, wgServer, auth, cleanup
}

func TestEdgeAPIServer_StartStop(t *testing.T) {
	server, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Start server
	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop server
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = server.Stop(stopCtx)
	if err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}
}

func TestEdgeAPIServer_SendEvent(t *testing.T) {
	// This test exercises real WireGuard peer configuration and requires
	// NET_ADMIN capabilities, so it is skipped in normal unit test runs.
	t.Skip("integration-style test requires WireGuard and NET_ADMIN; skipping in unit tests")

	server, _, wgServer, auth, cleanup := setupTestServer(t)
	defer cleanup()

	// Register an edge first
	_, publicKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	token, err := auth.GenerateBootstrapToken()
	if err != nil {
		t.Fatalf("Failed to generate bootstrap token: %v", err)
	}

	regReq := &EdgeRegistrationRequest{
		BootstrapToken: token,
		EdgeName:       "test-edge",
		PublicKey:      publicKey.String(),
	}

	regResp, err := auth.RegisterEdge(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register edge: %v", err)
	}
	edgeID := regResp.EdgeID

	// Add peer to WireGuard
	allowedIP := DeriveAllowedIP(edgeID, 0)
	err = wgServer.AddPeer(publicKey, []net.IPNet{allowedIP})
	if err != nil {
		t.Fatalf("Failed to add peer: %v", err)
	}

	// Setup mock event receiver
	mockReceiver := &mockEventReceiver{}
	server.SetEventReceiver(mockReceiver)

	// Start server
	ctx := context.Background()
	err = server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop(context.Background())

	// Create test event
	testEvent := &edge.Event{
		Id:        "test-event-1",
		CameraId:  "camera-1",
		EventType: "person_detected",
		Timestamp: time.Now().UnixNano(),
		Confidence: 0.95,
	}

	// Create gRPC client (simplified - in real test would use proper connection)
	// For now, test the handler directly
	req := &edge.SendEventRequest{
		Event: testEvent,
	}

	// Create context with edge_id
	testCtx := context.WithValue(context.Background(), "edge_id", edgeID)

	// Call handler
	resp, err := server.SendEvent(testCtx, req)
	if err != nil {
		t.Fatalf("Failed to send event: %v", err)
	}
	if !resp.Success {
		t.Errorf("Expected success, got error: %s", resp.ErrorMessage)
	}
	if resp.EventId == "" {
		t.Error("Expected event ID, got empty")
	}
	if len(mockReceiver.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(mockReceiver.events))
	}
}

func TestEdgeAPIServer_SendEvents(t *testing.T) {
	t.Skip("integration-style test requires WireGuard and NET_ADMIN; skipping in unit tests")

	server, _, wgServer, auth, cleanup := setupTestServer(t)
	defer cleanup()

	// Register an edge
	_, publicKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	token, err := auth.GenerateBootstrapToken()
	if err != nil {
		t.Fatalf("Failed to generate bootstrap token: %v", err)
	}

	regReq := &EdgeRegistrationRequest{
		BootstrapToken: token,
		EdgeName:       "test-edge",
		PublicKey:      publicKey.String(),
	}

	regResp, err := auth.RegisterEdge(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register edge: %v", err)
	}
	edgeID := regResp.EdgeID

	// Add peer to WireGuard
	allowedIP := DeriveAllowedIP(edgeID, 0)
	err = wgServer.AddPeer(publicKey, []net.IPNet{allowedIP})
	if err != nil {
		t.Fatalf("Failed to add peer: %v", err)
	}

	// Setup mock event receiver
	mockReceiver := &mockEventReceiver{}
	server.SetEventReceiver(mockReceiver)

	// Start server
	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop(context.Background())

	// Create test events
	events := []*edge.Event{
		{
			Id:        "test-event-1",
			CameraId:  "camera-1",
			EventType: "person_detected",
			Timestamp: time.Now().UnixNano(),
			Confidence: 0.95,
		},
		{
			Id:        "test-event-2",
			CameraId:  "camera-2",
			EventType: "vehicle_detected",
			Timestamp: time.Now().UnixNano(),
			Confidence: 0.87,
		},
	}

	req := &edge.SendEventsRequest{
		Events: events,
	}

	// Create context with edge_id
	testCtx := context.WithValue(context.Background(), "edge_id", edgeID)

	// Call handler
	resp, err := server.SendEvents(testCtx, req)
	if err != nil {
		t.Fatalf("Failed to send events: %v", err)
	}
	if !resp.Success {
		t.Errorf("Expected success, got error: %s", resp.ErrorMessage)
	}
	if resp.ReceivedCount != 2 {
		t.Errorf("Expected 2 received events, got %d", resp.ReceivedCount)
	}
	if len(resp.EventIds) != 2 {
		t.Errorf("Expected 2 event IDs, got %d", len(resp.EventIds))
	}
	if len(mockReceiver.events) != 2 {
		t.Errorf("Expected 2 events in receiver, got %d", len(mockReceiver.events))
	}
}

func TestEdgeAPIServer_Heartbeat(t *testing.T) {
	t.Skip("integration-style test requires WireGuard and NET_ADMIN; skipping in unit tests")

	server, _, wgServer, auth, cleanup := setupTestServer(t)
	defer cleanup()

	// Register an edge
	_, publicKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	token, err := auth.GenerateBootstrapToken()
	if err != nil {
		t.Fatalf("Failed to generate bootstrap token: %v", err)
	}

	regReq := &EdgeRegistrationRequest{
		BootstrapToken: token,
		EdgeName:       "test-edge",
		PublicKey:      publicKey.String(),
	}

	regResp, err := auth.RegisterEdge(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register edge: %v", err)
	}
	edgeID := regResp.EdgeID

	// Add peer to WireGuard
	allowedIP := DeriveAllowedIP(edgeID, 0)
	err = wgServer.AddPeer(publicKey, []net.IPNet{allowedIP})
	if err != nil {
		t.Fatalf("Failed to add peer: %v", err)
	}

	// Setup mock telemetry handler
	mockHandler := &mockTelemetryHandler{}
	server.SetTelemetryHandler(mockHandler)

	// Start server
	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop(context.Background())

	// Create heartbeat request
	req := &edge.HeartbeatRequest{
		Timestamp: time.Now().UnixNano(),
		EdgeId:     edgeID,
	}

	// Create context with edge_id
	testCtx := context.WithValue(context.Background(), "edge_id", edgeID)

	// Call handler
	resp, err := server.Heartbeat(testCtx, req)
	if err != nil {
		t.Fatalf("Failed to send heartbeat: %v", err)
	}
	if !resp.Success {
		t.Error("Expected success, got false")
	}
	if resp.ServerTimestamp <= 0 {
		t.Error("Expected server timestamp > 0")
	}
	if len(mockHandler.heartbeats) != 1 {
		t.Errorf("Expected 1 heartbeat, got %d", len(mockHandler.heartbeats))
	}

	// Check connection tracking
	conn, exists := server.GetConnection(edgeID)
	if !exists {
		t.Error("Expected connection to exist")
	}
	if conn.LastHeartbeat.IsZero() {
		t.Error("Expected last heartbeat to be set")
	}
}

func TestEdgeAPIServer_SendTelemetry(t *testing.T) {
	t.Skip("integration-style test requires WireGuard and NET_ADMIN; skipping in unit tests")

	server, _, wgServer, auth, cleanup := setupTestServer(t)
	defer cleanup()

	// Register an edge
	_, publicKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	token, err := auth.GenerateBootstrapToken()
	if err != nil {
		t.Fatalf("Failed to generate bootstrap token: %v", err)
	}

	regReq := &EdgeRegistrationRequest{
		BootstrapToken: token,
		EdgeName:       "test-edge",
		PublicKey:      publicKey.String(),
	}

	regResp, err := auth.RegisterEdge(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register edge: %v", err)
	}
	edgeID := regResp.EdgeID

	// Add peer to WireGuard
	allowedIP := DeriveAllowedIP(edgeID, 0)
	err = wgServer.AddPeer(publicKey, []net.IPNet{allowedIP})
	if err != nil {
		t.Fatalf("Failed to add peer: %v", err)
	}

	// Setup mock telemetry handler
	mockHandler := &mockTelemetryHandler{}
	server.SetTelemetryHandler(mockHandler)

	// Start server
	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop(context.Background())

	// Create telemetry request
	telemetry := &edge.TelemetryData{
		Timestamp: time.Now().UnixNano(),
		EdgeId:    edgeID,
		System: &edge.SystemMetrics{
			CpuUsagePercent: 45.5,
			MemoryUsedBytes: 1024 * 1024 * 512, // 512 MB
			MemoryTotalBytes: 1024 * 1024 * 2048, // 2 GB
		},
		Application: &edge.ApplicationMetrics{
			EventQueueLength: 10,
			ActiveCameras:    2,
		},
	}

	req := &edge.SendTelemetryRequest{
		Telemetry: telemetry,
	}

	// Create context with edge_id
	testCtx := context.WithValue(context.Background(), "edge_id", edgeID)

	// Call handler
	resp, err := server.SendTelemetry(testCtx, req)
	if err != nil {
		t.Fatalf("Failed to send telemetry: %v", err)
	}
	if !resp.Success {
		t.Errorf("Expected success, got error: %s", resp.ErrorMessage)
	}
	if len(mockHandler.telemetry) != 1 {
		t.Errorf("Expected 1 telemetry entry, got %d", len(mockHandler.telemetry))
	}
}

func TestEdgeAPIServer_ConnectionTracking(t *testing.T) {
	t.Skip("integration-style test requires WireGuard and NET_ADMIN; skipping in unit tests")

	server, _, wgServer, auth, cleanup := setupTestServer(t)
	defer cleanup()

	// Register an edge
	_, publicKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	token, err := auth.GenerateBootstrapToken()
	if err != nil {
		t.Fatalf("Failed to generate bootstrap token: %v", err)
	}

	regReq := &EdgeRegistrationRequest{
		BootstrapToken: token,
		EdgeName:       "test-edge",
		PublicKey:      publicKey.String(),
	}

	regResp, err := auth.RegisterEdge(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register edge: %v", err)
	}
	edgeID := regResp.EdgeID

	// Add peer to WireGuard
	allowedIP := DeriveAllowedIP(edgeID, 0)
	err = wgServer.AddPeer(publicKey, []net.IPNet{allowedIP})
	if err != nil {
		t.Fatalf("Failed to add peer: %v", err)
	}

	// Start server
	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop(context.Background())

	// Initially no connections
	edges := server.GetConnectedEdges()
	if len(edges) != 0 {
		t.Errorf("Expected 0 connections, got %d", len(edges))
	}

	// Simulate connection (would normally happen via authentication)
	server.updateConnection(edgeID, "10.0.0.2:50051")

	// Check connection exists
	conn, exists := server.GetConnection(edgeID)
	if !exists {
		t.Error("Expected connection to exist")
	}
	if conn.EdgeID != edgeID {
		t.Errorf("Expected edge ID %s, got %s", edgeID, conn.EdgeID)
	}

	// Check connected edges list
	edges = server.GetConnectedEdges()
	if len(edges) != 1 {
		t.Errorf("Expected 1 connection, got %d", len(edges))
	}
	if edges[0] != edgeID {
		t.Errorf("Expected edge ID %s in list, got %s", edgeID, edges[0])
	}
}

func TestEdgeAPIServer_DisconnectionDetection(t *testing.T) {
	t.Skip("integration-style test requires WireGuard and NET_ADMIN; skipping in unit tests")

	server, _, wgServer, auth, cleanup := setupTestServer(t)
	defer cleanup()

	// Register an edge
	_, publicKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	token, err := auth.GenerateBootstrapToken()
	if err != nil {
		t.Fatalf("Failed to generate bootstrap token: %v", err)
	}

	regReq := &EdgeRegistrationRequest{
		BootstrapToken: token,
		EdgeName:       "test-edge",
		PublicKey:      publicKey.String(),
	}

	regResp, err := auth.RegisterEdge(context.Background(), regReq)
	if err != nil {
		t.Fatalf("Failed to register edge: %v", err)
	}
	edgeID := regResp.EdgeID

	// Add peer to WireGuard
	allowedIP := DeriveAllowedIP(edgeID, 0)
	err = wgServer.AddPeer(publicKey, []net.IPNet{allowedIP})
	if err != nil {
		t.Fatalf("Failed to add peer: %v", err)
	}

	// Setup mock telemetry handler
	mockHandler := &mockTelemetryHandler{}
	server.SetTelemetryHandler(mockHandler)

	// Start server
	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop(context.Background())

	// Simulate connection
	server.updateConnection(edgeID, "10.0.0.2:50051")

	// Send heartbeat
	req := &edge.HeartbeatRequest{
		Timestamp: time.Now().UnixNano(),
		EdgeId:     edgeID,
	}
	testCtx := context.WithValue(context.Background(), "edge_id", edgeID)
	_, err = server.Heartbeat(testCtx, req)
	if err != nil {
		t.Fatalf("Failed to send heartbeat: %v", err)
	}

	// Connection should exist
	_, exists := server.GetConnection(edgeID)
	if !exists {
		t.Error("Expected connection to exist")
	}

	// Simulate disconnection by setting old heartbeat time
	server.mu.Lock()
	if conn, exists := server.connections[edgeID]; exists {
		conn.mu.Lock()
		conn.LastHeartbeat = time.Now().Add(-10 * time.Minute) // 10 minutes ago
		conn.mu.Unlock()
	}
	server.mu.Unlock()

	// Trigger connection check
	server.checkConnections()

	// Connection should be removed
	_, exists = server.GetConnection(edgeID)
	if exists {
		t.Error("Expected connection to be removed")
	}
}

func TestWireGuardServer_ConnectionMonitoring(t *testing.T) {
	// This test would require actual WireGuard interface setup
	// For now, test the helper functions
	_, publicKey, err := GenerateKeyPair()
	require.NoError(t, err)

	// Test peer info tracking
	wgServer := &WireGuardServer{
		peers: make(map[string]*PeerInfo),
	}

	// Add peer info
	wgServer.peers[publicKey.String()] = &PeerInfo{
		PublicKey: publicKey,
		Connected: true,
	}

	// Test GetPeerInfo
	peerInfo, exists := wgServer.GetPeerInfo(publicKey)
	if !exists {
		t.Error("Expected peer info to exist")
	}
	if peerInfo.PublicKey != publicKey {
		t.Errorf("Expected public key %s, got %s", publicKey.String(), peerInfo.PublicKey.String())
	}

	// Test GetConnectedPeers
	connected := wgServer.GetConnectedPeers()
	found := false
	for _, key := range connected {
		if key == publicKey.String() {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected peer to be in connected list")
	}

	// Test latency tracking
	wgServer.UpdatePeerLatency(publicKey, 50*time.Millisecond)
	latency, exists := wgServer.GetPeerLatency(publicKey)
	if !exists {
		t.Error("Expected latency to exist")
	}
	if latency != 50*time.Millisecond {
		t.Errorf("Expected latency 50ms, got %v", latency)
	}

	// Test ping recording
	wgServer.RecordPing(publicKey)
	peerInfo, _ = wgServer.GetPeerInfo(publicKey)
	peerInfo.mu.RLock()
	if peerInfo.PingCount <= 0 {
		t.Error("Expected ping count > 0")
	}
	peerInfo.mu.RUnlock()
}

