package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"
)

func setupTestCollector(t *testing.T) (*Collector, *camera.Manager, *events.Queue, *storage.StorageService) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled:  true,
		Interval: 30 * time.Second,
	}

	// Create state manager
	stateMgr := setupTestManager(t)

	// Create camera manager (with nil discovery services for testing)
	cameraMgr := camera.NewManager(
		stateMgr,
		nil, // onvifDiscovery
		nil, // usbDiscovery
		30*time.Second, // statusInterval
		log,
	)

	// Create event queue
	eventQueue := events.NewQueue(events.QueueConfig{
		StateManager: stateMgr,
		MaxSize:      100,
	}, log)

	// Create event storage
	eventStorage := events.NewStorage(stateMgr, log)

	// Create storage service
	tmpDir := t.TempDir()
	storageSvc, err := storage.NewStorageService(storage.StorageConfig{
		ClipsDir:             tmpDir + "/clips",
		SnapshotsDir:         tmpDir + "/snapshots",
		RetentionDays:        7,
		MaxDiskUsagePercent:  80.0,
		StateManager:         nil, // No state manager for basic tests
	}, log)
	if err != nil {
		t.Fatalf("Failed to create storage service: %v", err)
	}

	cfgWG := &config.WireGuardConfig{
		Enabled: false,
	}
	wgClient := wireguard.NewClient(cfgWG, log)

	collector := NewCollector(
		cfg,
		log,
		cameraMgr,
		eventQueue,
		eventStorage,
		storageSvc,
		wgClient,
	)

	return collector, cameraMgr, eventQueue, storageSvc
}

func setupTestManager(t *testing.T) *state.Manager {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.Config{
		Edge: config.EdgeConfig{
			Orchestrator: config.OrchestratorConfig{
				DataDir: t.TempDir(),
			},
		},
	}

	stateMgr, err := state.NewManager(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create state manager: %v", err)
	}

	return stateMgr
}

func TestCollector_NewCollector(t *testing.T) {
	collector, _, _, _ := setupTestCollector(t)

	if collector == nil {
		t.Fatal("NewCollector returned nil")
	}

	if collector.Name() != "telemetry-collector" {
		t.Errorf("Expected service name 'telemetry-collector', got %s", collector.Name())
	}
}

func TestCollector_StartStop(t *testing.T) {
	collector, _, _, _ := setupTestCollector(t)

	ctx := context.Background()

	err := collector.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment
	time.Sleep(10 * time.Millisecond)

	err = collector.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestCollector_Start_Disabled(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled: false,
	}

	stateMgr := setupTestManager(t)
	eventStorage := events.NewStorage(stateMgr, log)

	cfgWG := &config.WireGuardConfig{
		Enabled: false,
	}
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

	ctx := context.Background()

	err := collector.Start(ctx)
	if err != nil {
		t.Fatalf("Start should succeed when disabled: %v", err)
	}
}

func TestCollector_Collect_SystemMetrics(t *testing.T) {
	collector, _, _, _ := setupTestCollector(t)

	ctx := context.Background()

	data, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if data == nil {
		t.Fatal("Expected telemetry data, got nil")
	}

	// Test system metrics
	if data.System == nil {
		t.Fatal("Expected system metrics, got nil")
	}

	// CPU usage should be a percentage (0-100)
	if data.System.CpuUsagePercent < 0 || data.System.CpuUsagePercent > 100 {
		t.Errorf("CPU usage should be between 0 and 100, got %f", data.System.CpuUsagePercent)
	}

	// Memory should be set
	if data.System.MemoryTotalBytes == 0 {
		t.Error("Expected memory total bytes to be set")
	}

	// Disk usage should be set (from storage service)
	if data.System.DiskTotalBytes == 0 {
		t.Error("Expected disk total bytes to be set")
	}
}

func TestCollector_Collect_ApplicationMetrics(t *testing.T) {
	collector, cameraMgr, eventQueue, _ := setupTestCollector(t)

	ctx := context.Background()

	// Create a camera first (required for foreign key)
	discovered := &camera.DiscoveredCamera{
		ID:           "test-camera",
		Manufacturer: "Test",
		Model:        "Model",
		IPAddress:    "192.168.1.100",
		RTSPURLs:     []string{"rtsp://192.168.1.100/stream"},
		Capabilities: camera.CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}
	err := cameraMgr.RegisterCamera(ctx, discovered)
	if err != nil {
		t.Fatalf("Failed to register camera: %v", err)
	}

	// Add some events to the queue
	testEvent := events.NewEvent()
	testEvent.CameraID = "test-camera"
	testEvent.EventType = events.EventTypePersonDetected
	testEvent.Timestamp = time.Now()
	err = eventQueue.Enqueue(ctx, testEvent, 0)
	if err != nil {
		t.Fatalf("Failed to enqueue event: %v", err)
	}

	data, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if data.Application == nil {
		t.Fatal("Expected application metrics, got nil")
	}

	// Event queue length should be at least 1
	if data.Application.EventQueueLength < 1 {
		t.Errorf("Expected event queue length >= 1, got %d", data.Application.EventQueueLength)
	}

	// Storage stats should be set
	if data.Application.StorageClipsCount < 0 {
		t.Error("Expected storage clips count to be set")
	}
}

func TestCollector_Collect_CameraStatuses(t *testing.T) {
	collector, cameraMgr, _, _ := setupTestCollector(t)

	ctx := context.Background()

	// Register a test camera
	discovered := &camera.DiscoveredCamera{
		ID:           "camera-1",
		Manufacturer: "Test",
		Model:        "Model",
		IPAddress:    "192.168.1.100",
		RTSPURLs:     []string{"rtsp://192.168.1.100/stream"},
		Capabilities: camera.CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := cameraMgr.RegisterCamera(ctx, discovered)
	if err != nil {
		t.Fatalf("Failed to register camera: %v", err)
	}

	data, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if data.Cameras == nil {
		t.Fatal("Expected camera statuses, got nil")
	}

	if len(data.Cameras) < 1 {
		t.Errorf("Expected at least 1 camera status, got %d", len(data.Cameras))
	}

	// Check camera status
	found := false
	for _, cam := range data.Cameras {
		if cam.CameraId == "camera-1" {
			found = true
			if cam.StatusMessage == "" {
				t.Error("Expected status message to be set")
			}
			break
		}
	}

	if !found {
		t.Error("Expected to find camera-1 in statuses")
	}
}

func TestCollector_Collect_NoCameras(t *testing.T) {
	collector, _, _, _ := setupTestCollector(t)

	ctx := context.Background()

	data, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if data.Cameras == nil {
		t.Fatal("Expected camera statuses (empty slice), got nil")
	}

	if len(data.Cameras) != 0 {
		t.Errorf("Expected 0 camera statuses, got %d", len(data.Cameras))
	}
}

func TestCollector_Collect_NoEventQueue(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled: true,
	}

	stateMgr := setupTestManager(t)
	eventStorage := events.NewStorage(stateMgr, log)

	cfgWG := &config.WireGuardConfig{
		Enabled: false,
	}
	wgClient := wireguard.NewClient(cfgWG, log)

	cameraMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)

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
		nil, // No event queue
		eventStorage,
		storageSvc,
		wgClient,
	)

	ctx := context.Background()

	data, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect should succeed even without event queue: %v", err)
	}

	if data.Application == nil {
		t.Fatal("Expected application metrics, got nil")
	}

	// Event queue length should be 0 when queue is nil
	if data.Application.EventQueueLength != 0 {
		t.Errorf("Expected event queue length 0, got %d", data.Application.EventQueueLength)
	}
}

func TestCollector_Collect_NoStorageService(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.TelemetryConfig{
		Enabled: true,
	}

	stateMgr := setupTestManager(t)
	eventStorage := events.NewStorage(stateMgr, log)

	cfgWG := &config.WireGuardConfig{
		Enabled: false,
	}
	wgClient := wireguard.NewClient(cfgWG, log)

	cameraMgr := camera.NewManager(stateMgr, nil, nil, 30*time.Second, log)
	eventQueue := events.NewQueue(events.QueueConfig{
		StateManager: stateMgr,
		MaxSize:      100,
	}, log)

	collector := NewCollector(
		cfg,
		log,
		cameraMgr,
		eventQueue,
		eventStorage,
		nil, // No storage service
		wgClient,
	)

	ctx := context.Background()

	data, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect should succeed even without storage service: %v", err)
	}

	if data.System == nil {
		t.Fatal("Expected system metrics, got nil")
	}

	// Disk usage should be 0 when storage service is nil
	if data.System.DiskUsedBytes != 0 {
		t.Errorf("Expected disk used bytes 0, got %d", data.System.DiskUsedBytes)
	}

	if data.Application == nil {
		t.Fatal("Expected application metrics, got nil")
	}

	// Storage stats should be 0 when storage service is nil
	if data.Application.StorageClipsCount != 0 {
		t.Errorf("Expected storage clips count 0, got %d", data.Application.StorageClipsCount)
	}
}

func TestCollector_GetLastMetrics(t *testing.T) {
	collector, _, _, _ := setupTestCollector(t)

	ctx := context.Background()

	// Initially, last metrics should be nil
	lastMetrics := collector.GetLastMetrics()
	if lastMetrics != nil {
		t.Error("Expected last metrics to be nil initially")
	}

	// Collect metrics
	data, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// Now last metrics should be set
	lastMetrics = collector.GetLastMetrics()
	if lastMetrics == nil {
		t.Fatal("Expected last metrics to be set after collection")
	}

	if lastMetrics.Timestamp != data.Timestamp {
		t.Errorf("Expected timestamp %d, got %d", data.Timestamp, lastMetrics.Timestamp)
	}
}

func TestCollector_Collect_Timestamp(t *testing.T) {
	collector, _, _, _ := setupTestCollector(t)

	ctx := context.Background()

	before := time.Now().UnixNano()

	data, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	after := time.Now().UnixNano()

	// Timestamp should be between before and after
	if data.Timestamp < before || data.Timestamp > after {
		t.Errorf("Timestamp %d should be between %d and %d", data.Timestamp, before, after)
	}
}

func TestCollector_Collect_EdgeID(t *testing.T) {
	collector, _, _, _ := setupTestCollector(t)

	ctx := context.Background()

	data, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// Edge ID should be set (currently hardcoded to "edge-001")
	if data.EdgeId == "" {
		t.Error("Expected EdgeId to be set")
	}
}
