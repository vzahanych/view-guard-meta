package telemetry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"

	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// mockTelemetryClient is a mock implementation of TelemetryClient
type mockTelemetryClient struct {
	connected      bool
	sendTelemetryErr error
	heartbeatErr    error
	telemetrySent   []*edge.TelemetryData
	heartbeatsSent  []*edge.HeartbeatRequest
	mu             sync.Mutex
}

func (m *mockTelemetryClient) IsConnected() bool {
	return m.connected
}

func (m *mockTelemetryClient) SendTelemetry(ctx context.Context, data *edge.TelemetryData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.telemetrySent = append(m.telemetrySent, data)
	return m.sendTelemetryErr
}

func (m *mockTelemetryClient) Heartbeat(ctx context.Context, req *edge.HeartbeatRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.heartbeatsSent = append(m.heartbeatsSent, req)
	return m.heartbeatErr
}

func (m *mockTelemetryClient) getTelemetrySent() []*edge.TelemetryData {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.telemetrySent
}

func (m *mockTelemetryClient) getHeartbeatsSent() []*edge.HeartbeatRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.heartbeatsSent
}

func (m *mockTelemetryClient) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.telemetrySent = nil
	m.heartbeatsSent = nil
}

func setupTestSender(t *testing.T) (*Sender, *mockTelemetryClient) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled:  true,
		Interval: 100 * time.Millisecond, // Short interval for testing
	}

	mockClient := &mockTelemetryClient{
		connected: true,
	}

	// Create a real collector
	stateMgr := setupTestManager(t)
	eventStorage := events.NewStorage(stateMgr, log)
	cfgWG := &config.WireGuardConfig{Enabled: false}
	wgClient := wireguard.NewClient(cfgWG, log)
	cameraMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)
	eventQueue := events.NewQueue(events.QueueConfig{
		StateManager: stateMgr,
		MaxSize:      100,
	}, log)
	tmpDir := t.TempDir()
	storageSvc, _ := storage.NewStorageService(storage.StorageConfig{
		ClipsDir:            tmpDir + "/clips",
		SnapshotsDir:        tmpDir + "/snapshots",
		RetentionDays:       7,
		MaxDiskUsagePercent: 80.0,
		StateManager:        nil,
	}, log)
	collector := NewCollector(cfg, log, cameraMgr, eventQueue, eventStorage, storageSvc, wgClient)

	sender := NewSender(
		collector,
		mockClient,
		cfg,
		"test-edge",
		log,
	)

	return sender, mockClient
}

func TestSender_NewSender(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled: true,
	}

	mockClient := &mockTelemetryClient{connected: true}
	
	// Create a real collector
	stateMgr := setupTestManager(t)
	eventStorage := events.NewStorage(stateMgr, log)
	cfgWG := &config.WireGuardConfig{Enabled: false}
	wgClient := wireguard.NewClient(cfgWG, log)
	cameraMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)
	eventQueue := events.NewQueue(events.QueueConfig{
		StateManager: stateMgr,
		MaxSize:      100,
	}, log)
	tmpDir := t.TempDir()
	storageSvc, _ := storage.NewStorageService(storage.StorageConfig{
		ClipsDir:            tmpDir + "/clips",
		SnapshotsDir:        tmpDir + "/snapshots",
		RetentionDays:       7,
		MaxDiskUsagePercent: 80.0,
		StateManager:        nil,
	}, log)
	collector := NewCollector(cfg, log, cameraMgr, eventQueue, eventStorage, storageSvc, wgClient)

	sender := NewSender(
		collector,
		mockClient,
		cfg,
		"test-edge",
		log,
	)

	if sender == nil {
		t.Fatal("NewSender returned nil")
	}

	if sender.Name() != "telemetry-sender" {
		t.Errorf("Expected service name 'telemetry-sender', got %s", sender.Name())
	}

	if sender.edgeID != "test-edge" {
		t.Errorf("Expected edge ID 'test-edge', got %s", sender.edgeID)
	}
}

func TestSender_StartStop(t *testing.T) {
	sender, _ := setupTestSender(t)

	ctx := context.Background()

	err := sender.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Verify running
	status := sender.GetStatus().GetStatus()
	if status != service.StatusRunning {
		t.Errorf("Expected status %s, got %s", service.StatusRunning, status)
	}

	err = sender.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Give it a moment to stop
	time.Sleep(50 * time.Millisecond)

	// Verify stopped
	status = sender.GetStatus().GetStatus()
	if status != service.StatusStopped {
		t.Errorf("Expected status %s, got %s", service.StatusStopped, status)
	}
}

func TestSender_Start_Disabled(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled: false,
	}

	mockClient := &mockTelemetryClient{connected: true}
	
	// Create a real collector
	stateMgr := setupTestManager(t)
	eventStorage := events.NewStorage(stateMgr, log)
	cfgWG := &config.WireGuardConfig{Enabled: false}
	wgClient := wireguard.NewClient(cfgWG, log)
	cameraMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)
	eventQueue := events.NewQueue(events.QueueConfig{
		StateManager: stateMgr,
		MaxSize:      100,
	}, log)
	tmpDir := t.TempDir()
	storageSvc, _ := storage.NewStorageService(storage.StorageConfig{
		ClipsDir:            tmpDir + "/clips",
		SnapshotsDir:        tmpDir + "/snapshots",
		RetentionDays:       7,
		MaxDiskUsagePercent: 80.0,
		StateManager:        nil,
	}, log)
	collector := NewCollector(cfg, log, cameraMgr, eventQueue, eventStorage, storageSvc, wgClient)

	sender := NewSender(
		collector,
		mockClient,
		cfg,
		"test-edge",
		log,
	)

	ctx := context.Background()

	err := sender.Start(ctx)
	if err != nil {
		t.Fatalf("Start should succeed when disabled: %v", err)
	}
}

func TestSender_Heartbeat_Connected(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled:  true,
		Interval: 50 * time.Millisecond,
	}

	mockClient := &mockTelemetryClient{
		connected: true,
	}

	collector := &Collector{}

	sender := NewSender(
		collector,
		mockClient,
		cfg,
		"test-edge",
		log,
	)

	ctx := context.Background()

	err := sender.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for at least one heartbeat
	time.Sleep(150 * time.Millisecond)

	heartbeats := mockClient.getHeartbeatsSent()
	if len(heartbeats) == 0 {
		t.Error("Expected at least one heartbeat to be sent")
	}

	// Check heartbeat content
	hb := heartbeats[0]
	if hb.EdgeId != "test-edge" {
		t.Errorf("Expected edge ID 'test-edge', got %s", hb.EdgeId)
	}

	if hb.Timestamp == 0 {
		t.Error("Expected timestamp to be set")
	}

	sender.Stop(ctx)
}

func TestSender_Heartbeat_NotConnected(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled:  true,
		Interval: 50 * time.Millisecond,
	}

	mockClient := &mockTelemetryClient{
		connected: false, // Not connected
	}

	collector := &Collector{}

	sender := NewSender(
		collector,
		mockClient,
		cfg,
		"test-edge",
		log,
	)

	ctx := context.Background()

	err := sender.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait a bit
	time.Sleep(150 * time.Millisecond)

	heartbeats := mockClient.getHeartbeatsSent()
	if len(heartbeats) > 0 {
		t.Error("Expected no heartbeats to be sent when not connected")
	}

	sender.Stop(ctx)
}

func TestSender_Heartbeat_Error(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled:  true,
		Interval: 50 * time.Millisecond,
	}

	mockClient := &mockTelemetryClient{
		connected:   true,
		heartbeatErr: errors.New("heartbeat error"),
	}

	collector := &Collector{}

	sender := NewSender(
		collector,
		mockClient,
		cfg,
		"test-edge",
		log,
	)

	ctx := context.Background()

	err := sender.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for heartbeat attempt
	time.Sleep(150 * time.Millisecond)

	// Heartbeat should still be attempted (error is logged but doesn't stop the loop)
	heartbeats := mockClient.getHeartbeatsSent()
	if len(heartbeats) == 0 {
		t.Error("Expected heartbeat to be attempted even on error")
	}

	sender.Stop(ctx)
}

func TestSender_Telemetry_Connected(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled:  true,
		Interval: 50 * time.Millisecond, // This is used for heartbeat, telemetry is 10x
	}

	mockClient := &mockTelemetryClient{
		connected: true,
	}

	// Create a real collector with minimal dependencies
	stateMgr := setupTestManager(t)
	eventStorage := events.NewStorage(stateMgr, log)

	cfgWG := &config.WireGuardConfig{Enabled: false}
	wgClient := wireguard.NewClient(cfgWG, log)

	cameraMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)
	eventQueue := events.NewQueue(events.QueueConfig{
		StateManager: stateMgr,
		MaxSize:      100,
	}, log)

	tmpDir := t.TempDir()
	storageSvc, _ := storage.NewStorageService(storage.StorageConfig{
		ClipsDir:            tmpDir + "/clips",
		SnapshotsDir:        tmpDir + "/snapshots",
		RetentionDays:       7,
		MaxDiskUsagePercent: 80.0,
		StateManager:        nil,
	}, log)

	collector := NewCollector(
		cfg,
		log,
		cameraMgr,
		eventQueue,
		eventStorage,
		storageSvc,
		wgClient,
	)

	sender := NewSender(
		collector,
		mockClient,
		cfg,
		"test-edge",
		log,
	)

	ctx := context.Background()

	err := sender.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for telemetry (5 minutes default, but we'll wait a bit to see if it sends)
	// Actually, telemetry interval is 10x heartbeat interval, so 500ms
	time.Sleep(600 * time.Millisecond)

	telemetrySent := mockClient.getTelemetrySent()
	if len(telemetrySent) == 0 {
		t.Error("Expected at least one telemetry report to be sent")
	}

	// Check telemetry content
	telemetry := telemetrySent[0]
	if telemetry.EdgeId != "test-edge" {
		t.Errorf("Expected edge ID 'test-edge', got %s", telemetry.EdgeId)
	}

	if telemetry.System == nil {
		t.Error("Expected system metrics to be set")
	}

	if telemetry.Application == nil {
		t.Error("Expected application metrics to be set")
	}

	sender.Stop(ctx)
}

func TestSender_Telemetry_NotConnected(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled:  true,
		Interval: 50 * time.Millisecond,
	}

	mockClient := &mockTelemetryClient{
		connected: false,
	}

	collector := &Collector{}

	sender := NewSender(
		collector,
		mockClient,
		cfg,
		"test-edge",
		log,
	)

	ctx := context.Background()

	err := sender.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait a bit
	time.Sleep(600 * time.Millisecond)

	telemetrySent := mockClient.getTelemetrySent()
	if len(telemetrySent) > 0 {
		t.Error("Expected no telemetry to be sent when not connected")
	}

	sender.Stop(ctx)
}

func TestSender_Telemetry_Error(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled:  true,
		Interval: 50 * time.Millisecond,
	}

	mockClient := &mockTelemetryClient{
		connected:       true,
		sendTelemetryErr: errors.New("telemetry error"),
	}

	stateMgr := setupTestManager(t)
	eventStorage := events.NewStorage(stateMgr, log)

	cfgWG := &config.WireGuardConfig{Enabled: false}
	wgClient := wireguard.NewClient(cfgWG, log)

	cameraMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)
	eventQueue := events.NewQueue(events.QueueConfig{
		StateManager: stateMgr,
		MaxSize:      100,
	}, log)
	tmpDir := t.TempDir()
	storageSvc, _ := storage.NewStorageService(storage.StorageConfig{
		ClipsDir:            tmpDir + "/clips",
		SnapshotsDir:        tmpDir + "/snapshots",
		RetentionDays:       7,
		MaxDiskUsagePercent: 80.0,
		StateManager:        nil,
	}, log)

	collector := NewCollector(
		cfg,
		log,
		cameraMgr,
		eventQueue,
		eventStorage,
		storageSvc,
		wgClient,
	)

	sender := NewSender(
		collector,
		mockClient,
		cfg,
		"test-edge",
		log,
	)

	ctx := context.Background()

	err := sender.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for telemetry attempt
	time.Sleep(600 * time.Millisecond)

	// Telemetry should still be attempted (error is logged but doesn't stop the loop)
	telemetrySent := mockClient.getTelemetrySent()
	if len(telemetrySent) == 0 {
		t.Error("Expected telemetry to be attempted even on error")
	}

	sender.Stop(ctx)
}

func TestSender_Stop_StopsLoops(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled:  true,
		Interval: 50 * time.Millisecond,
	}

	mockClient := &mockTelemetryClient{
		connected: true,
	}

	collector := &Collector{}

	sender := NewSender(
		collector,
		mockClient,
		cfg,
		"test-edge",
		log,
	)

	ctx := context.Background()

	err := sender.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for some heartbeats
	time.Sleep(200 * time.Millisecond)

	initialHeartbeats := len(mockClient.getHeartbeatsSent())

	// Stop the sender
	err = sender.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Wait a bit more
	time.Sleep(200 * time.Millisecond)

	// Heartbeats should not increase after stop
	finalHeartbeats := len(mockClient.getHeartbeatsSent())
	if finalHeartbeats > initialHeartbeats {
		t.Error("Expected heartbeats to stop after Stop()")
	}
}

func TestSender_IsSending(t *testing.T) {
	sender, _ := setupTestSender(t)

	ctx := context.Background()

	// Initially not sending
	if sender.isSending() {
		t.Error("Expected sender to not be sending initially")
	}

	err := sender.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment
	time.Sleep(10 * time.Millisecond)

	// Should be sending
	if !sender.isSending() {
		t.Error("Expected sender to be sending after Start()")
	}

	err = sender.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Give it a moment
	time.Sleep(10 * time.Millisecond)

	// Should not be sending
	if sender.isSending() {
		t.Error("Expected sender to not be sending after Stop()")
	}
}

