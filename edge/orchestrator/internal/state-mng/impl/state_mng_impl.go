package impl

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/config"
	aigateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway"
	aigwtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/types"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	eventbustypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-client-service/types"
	webgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway"
	"go.uber.org/zap"
)

// Known edge-related event types used by the state manager.
const (
	EventTypeWireGuardConnected    eventbustypes.EventType = "network.wireguard.connected"
	EventTypeWireGuardDisconnected eventbustypes.EventType = "network.wireguard.disconnected"
	EventTypeHTTPSConnected        eventbustypes.EventType = "network.https.connected"
	EventTypeHTTPSDisconnected     eventbustypes.EventType = "network.https.disconnected"
	EventTypeEdgeAuthenticated     eventbustypes.EventType = "edge.authenticated"
	EventTypeCapabilitiesReceived  eventbustypes.EventType = "edge.capabilities_received"
	EventTypeCameraDiscovered      eventbustypes.EventType = "camera.discovered"
	EventTypeCameraRegistered      eventbustypes.EventType = "camera.registered"
	EventTypeCameraConnected       eventbustypes.EventType = "camera.connected"
	EventTypeCameraDisconnected    eventbustypes.EventType = "camera.disconnected"
	EventTypeSnapshotRequested     eventbustypes.EventType = "snapshot.requested"
	EventTypeScreenshotSetReady    eventbustypes.EventType = "screenshot_set.ready"
	EventTypeFrameReceived         eventbustypes.EventType = "video.frame_received"
	EventTypeDetection             eventbustypes.EventType = "ai.detection"
	EventTypeInference             eventbustypes.EventType = "ai.inference"
	EventTypeScreenshotSaved       eventbustypes.EventType = "screenshot.saved"
	EventTypeClipRecorded          eventbustypes.EventType = "video.clip_recorded"
	EventTypeStorageFull           eventbustypes.EventType = "storage.full"
	EventTypeStorageWarning        eventbustypes.EventType = "storage.warning"
	EventTypeModelDeployed         eventbustypes.EventType = "model.deployed"
)

// EdgeStatus represents the current state of the edge appliance.
type EdgeStatus string

const (
	EdgeStatusInitializing         EdgeStatus = "initializing"
	EdgeStatusDisconnected         EdgeStatus = "disconnected"
	EdgeStatusWGConnecting         EdgeStatus = "wg_connecting"
	EdgeStatusHTTPConnecting       EdgeStatus = "http_connecting"
	EdgeStatusWireGuardConn        EdgeStatus = "wireguard_connected"
	EdgeStatusHTTPSConn            EdgeStatus = "https_connected"
	EdgeStatusAuthenticated        EdgeStatus = "authenticated"
	EdgeStatusCapabilitiesReceived EdgeStatus = "capabilities_received"
	EdgeStatusCameraDiscovered     EdgeStatus = "camera_discovered"
	EdgeStatusCameraSynced         EdgeStatus = "camera_synced"
	// Waiting for user to take and label snapshots for VM request
	EdgeStatusWaitingForSnapshots  EdgeStatus = "waiting_for_camera_screenshots"
	EdgeStatusScreenshotSetReady   EdgeStatus = "screenshot_set_ready"
	EdgeStatusModelDeployed        EdgeStatus = "model_deployed"
	EdgeStatusFrameProcessing       EdgeStatus = "frame_processing"
	EdgeStatusError                EdgeStatus = "error"
	EdgeStatusMetaStorageError     EdgeStatus = "meta_storage_error"
	EdgeStatusObjectStorageError   EdgeStatus = "object_storage_error"
	EdgeStatusCCTVServiceError     EdgeStatus = "cctv_service_error"
	EdgeStatusAIGatewayError       EdgeStatus = "ai_gateway_error"
	EdgeStatusVMGatewayError       EdgeStatus = "vm_gateway_error"
	EdgeStatusWebGatewayError      EdgeStatus = "web_gateway_error"
	EdgeStatusWGConnectionError    EdgeStatus = "wg_connection_error"
	EdgeStatusHTTPConnectionError  EdgeStatus = "http_connection_error"
)

// EdgeState represents the complete state of the edge appliance
type EdgeState struct {
	Status             EdgeStatus
	NetworkConnected   bool
	VMAuthenticated    bool
	CamerasEnabled     int
	AIProcessingActive bool
	StorageHealth      string // "healthy", "warning", "full"
	LastUpdated        time.Time
}

// stateManager implements the StateManager interface.
type StateManagerImpl struct {
	eventBus eventbus.EventBus
	logger   *zap.Logger
	config   *config.Config

	// Service dependencies
	aiGateway     aigateway.AIGateway
	cctvService   cctv.CCTVService
	metaStorage   metastorage.MetaDataStore
	objectStorage objectstorage.ObjectStorageService
	vmGateway     vmgateway.VMGateway
	webGateway    webgateway.WebGateway

	// Capability sync state
	minSnapshots int
	syncInterval time.Duration
	syncTrigger  chan struct{} // Channel to trigger immediate sync
	pendingSync  bool          // Flag to indicate cameras are waiting to sync

	// Dataset upload state
	edgeID     string // Edge appliance identifier
	httpClient *http.Client
	vmEndpoint string

	// Security event queue state
	maxQueueSize int // Maximum queue size for security events

	// Pending snapshot request state (replaces deprecated @snapshot_request package)
	// Note: Now stored in database, but kept in memory for fast access
	pendingSnapshotRequests map[string]*types.PendingSnapshotRequest // cameraID -> request

	// Frame processing state (for continuous monitoring when model is deployed)
	frameProcessingInterval time.Duration // Interval between frame captures (default: 30s)
	frameProcessingActive   map[string]context.CancelFunc // cameraID -> cancel function for frame processing
	frameProcessingMu       sync.RWMutex                  // Mutex for frame processing state

	// Current edge state
	state EdgeState
	mu    sync.RWMutex

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once
}

// NewStateManager creates a new StateManager implementation.
//
// - bus: application-wide event bus implementation (in-memory, NATS, etc.)
// - log: optional logger (if nil, a development logger is created)
func NewStateManagerImpl(bus eventbus.EventBus, log *zap.Logger) (*StateManagerImpl, error) {
	if bus == nil {
		return nil, fmt.Errorf("event bus is required")
	}

	// Create a development logger if none is provided
	if log == nil {
		var err error
		log, err = zap.NewDevelopment()
		if err != nil {
			return nil, fmt.Errorf("failed to create logger: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &StateManagerImpl{
		eventBus: bus,
		logger:   log,
		state: EdgeState{
			Status:        EdgeStatusInitializing,
			StorageHealth: "healthy",
			LastUpdated:   time.Now(),
		},
		syncInterval:            5 * time.Minute, // Default sync interval
		syncTrigger:             make(chan struct{}, 1),
		pendingSnapshotRequests: make(map[string]*types.PendingSnapshotRequest),
		frameProcessingInterval: 30 * time.Second, // Default: 30 seconds between frames
		frameProcessingActive:   make(map[string]context.CancelFunc),
		ctx:                     ctx,
		cancel:                  cancel,
	}, nil
}

// Name returns the service name.
func (m *StateManagerImpl) Name() string {
	return "edge-state-manager"
}

// SetAIGateway sets the AI gateway service dependency
func (m *StateManagerImpl) SetAIGateway(aiGateway interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gw, ok := aiGateway.(aigateway.AIGateway); ok {
		m.aiGateway = gw
	}
}

// SetCCTVService sets the CCTV service dependency
func (m *StateManagerImpl) SetCCTVService(cctvService interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if svc, ok := cctvService.(cctv.CCTVService); ok {
		m.cctvService = svc
	}
}

// SetMetaStorage sets the metadata storage service dependency
func (m *StateManagerImpl) SetMetaStorage(metaStorage interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if store, ok := metaStorage.(metastorage.MetaDataStore); ok {
		m.metaStorage = store
	}
}

// SetObjectStorage sets the object storage service dependency
func (m *StateManagerImpl) SetObjectStorage(objectStorage interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if store, ok := objectStorage.(objectstorage.ObjectStorageService); ok {
		m.objectStorage = store
	}
}

// SetVMGateway sets the VM gateway service dependency
func (m *StateManagerImpl) SetVMGateway(vmGateway interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gw, ok := vmGateway.(vmgateway.VMGateway); ok {
		m.vmGateway = gw
	}
}

// SetWebGateway sets the web gateway service dependency
func (m *StateManagerImpl) SetWebGateway(webGateway interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gw, ok := webGateway.(webgateway.WebGateway); ok {
		m.webGateway = gw
	}
}

// SetConfig sets the configuration
func (m *StateManagerImpl) SetConfig(cfg *config.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = cfg
	if cfg != nil {
		// Minimum number of "normal" snapshots required for dataset readiness.
		// Currently not configurable via config struct, so use a sensible default.
		if m.minSnapshots <= 0 {
			m.minSnapshots = 50 // Default
		}

		// Security event queue size (default if not set elsewhere)
		if m.maxQueueSize <= 0 {
			m.maxQueueSize = 1000 // Default
		}

		// Initialize HTTP client for dataset uploads
		m.httpClient = &http.Client{
			Timeout: 10 * time.Minute, // Allow up to 10 minutes for large dataset uploads
		}

		// Determine VM endpoint from config (HTTPS client config)
		m.vmEndpoint = cfg.VMGateway.HTTPSClientConfig.VMEndpoint
		if m.vmEndpoint == "" {
			// Default to localhost for PoC (when WireGuard is not configured)
			m.vmEndpoint = "http://localhost:8080"
		}

		// Ensure endpoint has http:// or https:// prefix
		if !hasProtocol(m.vmEndpoint) {
			m.vmEndpoint = "http://" + m.vmEndpoint
		}

		// Database removed - use @meta-storage and @object-storage instead
	}
}

// SetEdgeID sets the edge ID for dataset uploads
func (m *StateManagerImpl) SetEdgeID(edgeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.edgeID = edgeID
}

// Start begins processing events from the event bus.
func (m *StateManagerImpl) Start(ctx context.Context) error {
	var startErr error
	m.startOnce.Do(func() {
		// Initialize and log initial state
		m.mu.Lock()
		m.state.Status = EdgeStatusInitializing
		m.state.LastUpdated = time.Now()
		initialState := m.state
		m.mu.Unlock()

		// Persist initial state
		m.persistStateToStorage(ctx, initialState)

		m.logger.Info("Edge state initialized",
			zap.String("status", string(initialState.Status)),
			zap.Bool("network_connected", initialState.NetworkConnected),
			zap.Bool("vm_authenticated", initialState.VMAuthenticated),
			zap.Int("cameras_enabled", initialState.CamerasEnabled),
			zap.String("storage_health", initialState.StorageHealth))

		// Check health of all services after setting initializing status
		m.checkServicesHealth(ctx)

		m.logger.Info("Starting edge state manager, subscribing to all events")

		events := m.eventBus.SubscribeAll()

		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.run(events)
		}()

		// Start capability sync loop if VM gateway is available
		if m.vmGateway != nil && m.cctvService != nil {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.capabilitySyncLoop(ctx)
			}()
			m.logger.Info("Capability sync loop started")
		}

		// Connection initiation is now handled in checkServicesHealth -> initiateConnection
		// This ensures health checks pass before attempting connection

		m.logger.Info("Edge state manager started, listening for events from all services")
	})

	return startErr
}

// Stop stops processing events and waits for in-flight tasks to complete.
func (m *StateManagerImpl) Stop(ctx context.Context) error {
	var stopErr error
	m.stopOnce.Do(func() {
		m.logger.Info("Stopping edge state manager")
		m.cancel()

		done := make(chan struct{})
		go func() {
			m.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			stopErr = ctx.Err()
		}

		// Database removed - use @meta-storage and @object-storage instead

		m.logger.Info("Edge state manager stopped")
	})

	return stopErr
}

// run listens for events from the event bus and processes them.
func (m *StateManagerImpl) run(events <-chan eventbustypes.Event) {
	for {
		select {
		case <-m.ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				// Event bus closed.
				return
			}
			m.handleEvent(ev)
		}
	}
}

// handleEvent processes a single event from the bus.
func (m *StateManagerImpl) handleEvent(ev eventbustypes.Event) {
	m.logger.Debug("edge-state-manager: handling event",
		zap.String("event_type", string(ev.Type)),
		zap.String("source", ev.Source),
	)

	// Update state based on event
	newState, err := m.updateStateForEvent(ev)
	if err != nil {
		m.logger.Warn("edge-state-manager: failed to update state for event",
			zap.String("event_type", string(ev.Type)),
			zap.Error(err),
		)
		return
	}

	// Execute workflow based on the new state
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.executeWorkflow(m.ctx, ev, newState)
	}()
}

// updateStateForEvent updates the edge state based on the event.
func (m *StateManagerImpl) updateStateForEvent(ev eventbustypes.Event) (EdgeState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldState := m.state
	newState := m.state
	newState.LastUpdated = time.Now()

	// Update state based on event type
	switch ev.Type {
	// Network events
	case EventTypeWireGuardConnected:
		newState.NetworkConnected = true
		if newState.Status == EdgeStatusDisconnected || newState.Status == EdgeStatusInitializing {
			newState.Status = EdgeStatusWireGuardConn
		}
	case EventTypeWireGuardDisconnected:
		newState.NetworkConnected = false
		newState.VMAuthenticated = false
		newState.Status = EdgeStatusDisconnected
	case EventTypeHTTPSConnected:
		if newState.Status == EdgeStatusWireGuardConn {
			newState.Status = EdgeStatusHTTPSConn
		}
	case EventTypeHTTPSDisconnected:
		if newState.Status == EdgeStatusHTTPSConn || newState.Status == EdgeStatusAuthenticated || newState.Status == EdgeStatusScreenshotSetReady {
			newState.Status = EdgeStatusWireGuardConn
			newState.VMAuthenticated = false
		}
	case EventTypeEdgeAuthenticated:
		if newState.Status == EdgeStatusHTTPSConn {
			newState.Status = EdgeStatusAuthenticated
			newState.VMAuthenticated = true
		}
	case EventTypeCapabilitiesReceived:
		if newState.Status == EdgeStatusAuthenticated || newState.Status == EdgeStatusHTTPSConn {
			newState.Status = EdgeStatusCapabilitiesReceived
		}

	// Camera events
	case EventTypeCameraDiscovered:
		if newState.Status == EdgeStatusCapabilitiesReceived {
			newState.Status = EdgeStatusCameraDiscovered
		}

	// Snapshot request events (VM asks Edge/user for labeled screenshots)
	case EventTypeSnapshotRequested:
		// When VM requests labeled screenshots, we enter waiting_for_camera_screenshots
		// (unless we're already in an error state)
		if newState.Status != EdgeStatusError &&
			newState.Status != EdgeStatusMetaStorageError &&
			newState.Status != EdgeStatusObjectStorageError &&
			newState.Status != EdgeStatusCCTVServiceError &&
			newState.Status != EdgeStatusAIGatewayError &&
			newState.Status != EdgeStatusVMGatewayError &&
			newState.Status != EdgeStatusWebGatewayError {

			newState.Status = EdgeStatusWaitingForSnapshots
		}
	case EventTypeScreenshotSetReady:
		// When user marks screenshot set as ready, transition from waiting_for_camera_screenshots to screenshot_set_ready
		if newState.Status == EdgeStatusWaitingForSnapshots {
			newState.Status = EdgeStatusScreenshotSetReady
		}

	case EventTypeCameraRegistered:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Info("Camera registered", zap.String("camera_id", cameraID))
			// State updated via workflow
		}
	case EventTypeCameraConnected:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Info("Camera connected", zap.String("camera_id", cameraID))
			// State updated via workflow
		}
	case EventTypeCameraDisconnected:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Info("Camera disconnected", zap.String("camera_id", cameraID))
			// State updated via workflow
		}

	// AI events
	case EventTypeDetection:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Debug("AI detection event", zap.String("camera_id", cameraID))
			// State updated via workflow
		}
	case EventTypeInference:
		// Track AI processing activity
		newState.AIProcessingActive = true

	// Storage events
	case EventTypeStorageWarning:
		newState.StorageHealth = "warning"
	case EventTypeStorageFull:
		newState.StorageHealth = "full"
		m.logger.Warn("Storage full, may need cleanup")

	// Video events
	case EventTypeFrameReceived:
		// Track frame processing
	case EventTypeClipRecorded:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Debug("Clip recorded", zap.String("camera_id", cameraID))
		}

	// Screenshot events
	case EventTypeScreenshotSaved:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Debug("Screenshot saved", zap.String("camera_id", cameraID))
		}
	}

	// Note: "ready" state has been removed as it was misleading.
	// Edge operates in specific states: camera_synced, waiting_for_camera_screenshots, screenshot_set_ready, etc.
	// Each state represents a specific operational phase.

	// Persist state to meta-storage (use background context since we're in an event handler)
	m.persistStateToStorage(context.Background(), newState)

	// Log state transition if status changed
	if oldState.Status != newState.Status {
		m.logger.Info("edge-state-manager: state transition",
			zap.String("old_status", string(oldState.Status)),
			zap.String("new_status", string(newState.Status)),
			zap.String("event_type", string(ev.Type)),
		)

		// Stop frame processing if transitioning away from model_deployed or frame_processing
		if (oldState.Status == EdgeStatusModelDeployed || oldState.Status == EdgeStatusFrameProcessing) &&
			newState.Status != EdgeStatusModelDeployed && newState.Status != EdgeStatusFrameProcessing {
			m.stopAllFrameProcessing()
		}
	}

	m.state = newState
	return newState, nil
}

// executeWorkflow determines and executes the workflow based on the event and current state.
func (m *StateManagerImpl) executeWorkflow(ctx context.Context, ev eventbustypes.Event, state EdgeState) {
	switch ev.Type {
	// Network workflow
	case EventTypeWireGuardConnected:
		m.logger.Info("WireGuard connected, waiting for HTTPS connection")
		// Workflow: WireGuard -> HTTPS -> Auth -> Capabilities -> Camera Discovery -> Camera Sync -> Screenshot Set Ready

	case EventTypeHTTPSConnected:
		m.logger.Info("HTTPS connected, initiating authentication")
		// Workflow: Trigger authentication if needed

	case EventTypeEdgeAuthenticated:
		m.logger.Info("Edge authenticated, initializing services")
		// Workflow: After authentication, ensure cameras are discovered and AI is ready
		m.initializeServicesAfterAuth(ctx)

	case EventTypeCapabilitiesReceived:
		m.logger.Info("Capabilities received from VM, checking for camera capability")
		// Workflow: Check capabilities and initiate camera discovery if CCTV capability is present
		m.handleCapabilitiesReceived(ctx, ev)

	// Camera workflow
	case EventTypeCameraDiscovered:
		m.logger.Info("Cameras discovered, updating state")
		m.handleCameraDiscovered(ctx, ev)

	// Snapshot request workflow (VM asks Edge/user for labeled screenshots)
	case EventTypeSnapshotRequested:
		m.logger.Info("Snapshot request received from VM, saving to meta-storage and updating state")
		m.handleSnapshotRequested(ctx, ev)

	case EventTypeScreenshotSetReady:
		m.logger.Info("Screenshot set marked as ready by user")
		m.handleScreenshotSetReady(ctx, ev)

	case EventTypeCameraRegistered:
		m.handleCameraRegistered(ctx, ev)
	case EventTypeCameraConnected:
		m.handleCameraConnected(ctx, ev)

	// AI workflow
	case EventTypeDetection:
		m.handleAIDetection(ctx, ev)
	case EventTypeInference:
		// Track inference activity
		m.state.AIProcessingActive = true

	// Storage workflow
	case EventTypeStorageWarning:
		m.handleStorageWarning(ctx, ev)
	case EventTypeStorageFull:
		m.handleStorageFull(ctx, ev)

	// Video workflow
	case EventTypeFrameReceived:
		// Frames are handled by AI gateway automatically
	case EventTypeClipRecorded:
		m.handleClipRecorded(ctx, ev)

	// Screenshot workflow
	case EventTypeScreenshotSaved:
		m.handleScreenshotSaved(ctx, ev)

	// Model deployment workflow
	case EventTypeModelDeployed:
		m.handleModelDeployed(ctx, ev)
	}

	// General state-based workflows
	switch state.Status {
	case EdgeStatusScreenshotSetReady:
		m.executeScreenshotSetReadyWorkflow(ctx, state)
	case EdgeStatusModelDeployed:
		m.executeModelDeployedWorkflow(ctx, state)
	case EdgeStatusFrameProcessing:
		m.executeFrameProcessingWorkflow(ctx, state)
	case EdgeStatusError:
		m.handleErrorState(ctx, state)
	}
}

// initializeServicesAfterAuth initializes services after authentication
func (m *StateManagerImpl) initializeServicesAfterAuth(ctx context.Context) {
	m.logger.Info("Initializing services after authentication")

	// Trigger camera discovery if CCTV service is available
	if m.cctvService != nil {
		// Type assertion would be done here if we had the interface
		// For now, we'll publish an event to trigger discovery
		m.eventBus.Publish(eventbustypes.Event{
			Type:      "workflow.camera.discover",
			Source:    "edge-state-manager",
			Timestamp: time.Now(),
			Data:      make(map[string]interface{}),
		})
	}

		// Ensure AI gateway is available
		if m.aiGateway != nil {
			m.logger.Debug("AI gateway available for processing")
		}
}

// handleCapabilitiesReceived handles capabilities received from VM
func (m *StateManagerImpl) handleCapabilitiesReceived(ctx context.Context, ev eventbustypes.Event) {
	capabilities, ok := ev.Data["capabilities"].(map[string]interface{})
	if !ok {
		m.logger.Warn("Capabilities data not found in event")
		return
	}

	m.logger.Info("Processing capabilities", zap.Any("capabilities", capabilities))

	// Check if CCTV camera capability is present
	if cctvCap, ok := capabilities["cctv_camera"].(bool); ok && cctvCap {
		m.logger.Info("CCTV camera capability detected, initiating camera discovery")
		// Initiate camera discovery through CCTV service
		if m.cctvService != nil {
			if err := m.cctvService.DiscoverCameras(ctx); err != nil {
				m.logger.Error("Failed to initiate camera discovery", zap.Error(err))
				// Update state to error
				m.mu.Lock()
				m.state.Status = EdgeStatusCCTVServiceError
				m.persistStateToStorage(ctx, m.state)
				m.mu.Unlock()
				return
			}
			m.logger.Info("Camera discovery initiated")
		} else {
			m.logger.Warn("CCTV service not available for camera discovery")
		}
	} else {
		m.logger.Info("No CCTV camera capability found in capabilities")
	}
}

// handleCameraDiscovered handles camera discovery events
func (m *StateManagerImpl) handleCameraDiscovered(ctx context.Context, ev eventbustypes.Event) {
	cameraID, _ := ev.Data["camera_id"].(string)
	m.logger.Info("Camera discovered, workflow: sync with VM",
		zap.String("camera_id", cameraID),
	)

	// Check current state and trigger sync if we're in camera_discovered state
	// (state was already updated to camera_discovered by updateState before this handler runs)
	m.mu.Lock()
	currentStatus := m.state.Status
	m.mu.Unlock()

	// If we're in camera_discovered state, sync cameras with VM
	// This ensures we sync after transitioning to camera_discovered
	if currentStatus == EdgeStatusCameraDiscovered {
		m.logger.Info("State is camera_discovered, syncing cameras with VM")
		m.syncCamerasWithVM(ctx)
	} else {
		m.logger.Debug("State is not camera_discovered, skipping sync",
			zap.String("current_status", string(currentStatus)),
		)
	}
}

// handleSnapshotRequested handles snapshot capture requests from VM.
// VM is effectively asking this Edge (and its user) to take labeled screenshots from a camera.
// We persist this request in meta-storage (via state-mng helpers) and move state to
// waiting_for_camera_screenshots so that UI can show pending actions.
func (m *StateManagerImpl) handleSnapshotRequested(ctx context.Context, ev eventbustypes.Event) {
	cameraID, _ := ev.Data["camera_id"].(string)
	label, _ := ev.Data["label"].(string)
	customLabel, _ := ev.Data["custom_label"].(string)

	var count int32
	if v, ok := ev.Data["count"].(int32); ok {
		count = v
	} else if v, ok := ev.Data["count"].(float64); ok {
		count = int32(v)
	}
	if count <= 0 {
		count = 1
	}

	m.logger.Info("Handling snapshot request from VM",
		zap.String("camera_id", cameraID),
		zap.String("label", label),
		zap.String("custom_label", customLabel),
		zap.Int32("count", count),
	)

	// Persist pending snapshot request metadata via state-manager helper
	if err := m.SavePendingSnapshotRequest(ctx, cameraID, label, customLabel, count); err != nil {
		m.logger.Error("Failed to save pending snapshot request",
			zap.String("camera_id", cameraID),
			zap.Error(err),
		)
		return
	}

	// Update state to waiting_for_camera_screenshots (unless we're already in an error state)
	m.mu.Lock()
	if m.state.Status != EdgeStatusError &&
		m.state.Status != EdgeStatusMetaStorageError &&
		m.state.Status != EdgeStatusObjectStorageError &&
		m.state.Status != EdgeStatusCCTVServiceError &&
		m.state.Status != EdgeStatusAIGatewayError &&
		m.state.Status != EdgeStatusVMGatewayError &&
		m.state.Status != EdgeStatusWebGatewayError {

		m.state.Status = EdgeStatusWaitingForSnapshots
		m.persistStateToStorage(ctx, m.state)
		m.logger.Info("State updated to waiting_for_camera_screenshots")
	}
	m.mu.Unlock()
}

// handleScreenshotSetReady handles user marking screenshot set as ready
func (m *StateManagerImpl) handleScreenshotSetReady(ctx context.Context, ev eventbustypes.Event) {
	cameraID, _ := ev.Data["camera_id"].(string)
	labeledCount, _ := ev.Data["labeled_count"].(int)
	minRequired, _ := ev.Data["min_required"].(int32)

	m.logger.Info("Screenshot set marked as ready by user",
		zap.String("camera_id", cameraID),
		zap.Int("labeled_count", labeledCount),
		zap.Int32("min_required", minRequired),
	)

	// Update state to screenshot_set_ready
	m.mu.Lock()
	if m.state.Status == EdgeStatusWaitingForSnapshots {
		m.state.Status = EdgeStatusScreenshotSetReady
		m.persistStateToStorage(ctx, m.state)
		m.logger.Info("State updated to screenshot_set_ready")
	}
	m.mu.Unlock()

	// Sync screenshots to VM for model training
	if cameraID != "" && m.vmGateway != nil && m.metaStorage != nil {
		m.syncScreenshotsToVM(ctx, cameraID)
	}

	// Clear the pending snapshot request since user has completed it
	if cameraID != "" {
		if err := m.ClearPendingSnapshotRequest(ctx, cameraID); err != nil {
			m.logger.Warn("Failed to clear pending snapshot request",
				zap.String("camera_id", cameraID),
				zap.Error(err),
			)
		} else {
			m.logger.Info("Pending snapshot request cleared",
				zap.String("camera_id", cameraID),
			)
		}
	}
}

// syncScreenshotsToVM syncs labeled screenshots to VM for model training
func (m *StateManagerImpl) syncScreenshotsToVM(ctx context.Context, cameraID string) {
	m.logger.Info("Syncing labeled screenshots to VM for model training",
		zap.String("camera_id", cameraID),
	)

	// Get all labeled screenshots for this camera from meta-storage
	filters := map[string]interface{}{
		"camera_id": cameraID,
	}
	screenshots, err := m.metaStorage.ListScreenshots(ctx, filters)
	if err != nil {
		m.logger.Error("Failed to list screenshots from meta-storage",
			zap.String("camera_id", cameraID),
			zap.Error(err),
		)
		return
	}

	// Filter to only labeled screenshots and fetch image data
	labeledScreenshots := make([]*httpsclienttypes.ScreenshotInfo, 0)
	for _, ss := range screenshots {
		if ss.Label == "" {
			continue // Skip unlabeled screenshots
		}

		// Convert to ScreenshotInfo format
		screenshotInfo := &httpsclienttypes.ScreenshotInfo{
			ScreenshotID: ss.ID,
			CameraID:    ss.CameraID,
			ObjectKey:   ss.ObjectKey, // Path to image in object storage
			Label:       ss.Label,
			CustomLabel: ss.CustomLabel,
			Description: ss.Description,
			CreatedAt:   ss.CreatedAt.Unix(),
		}

		// Add metadata if available
		if ss.Metadata != nil {
			screenshotInfo.Metadata = ss.Metadata
		}

		// Fetch image data from object storage
		if ss.ObjectKey != "" && m.objectStorage != nil {
			reader, err := m.objectStorage.LoadSnapshot(ctx, ss.ObjectKey)
			if err != nil {
				m.logger.Warn("Failed to load screenshot image from object storage",
					zap.String("screenshot_id", ss.ID),
					zap.String("object_key", ss.ObjectKey),
					zap.Error(err),
				)
			} else {
				// Read image data
				imageData, err := io.ReadAll(reader)
				reader.Close()
				if err != nil {
					m.logger.Warn("Failed to read screenshot image data",
						zap.String("screenshot_id", ss.ID),
						zap.Error(err),
					)
				} else {
					// Encode as base64
					screenshotInfo.ImageData = base64.StdEncoding.EncodeToString(imageData)
					
					// Determine image format from object key extension
					ext := strings.ToLower(filepath.Ext(ss.ObjectKey))
					switch ext {
					case ".jpg", ".jpeg":
						screenshotInfo.ImageFormat = "jpeg"
					case ".png":
						screenshotInfo.ImageFormat = "png"
					case ".gif":
						screenshotInfo.ImageFormat = "gif"
					case ".webp":
						screenshotInfo.ImageFormat = "webp"
					default:
						screenshotInfo.ImageFormat = "jpeg" // Default to JPEG
					}
				}
			}
		}

		labeledScreenshots = append(labeledScreenshots, screenshotInfo)
	}

	if len(labeledScreenshots) == 0 {
		m.logger.Warn("No labeled screenshots found for camera",
			zap.String("camera_id", cameraID),
		)
		return
	}

	m.logger.Info("Prepared labeled screenshots for sync",
		zap.String("camera_id", cameraID),
		zap.Int("count", len(labeledScreenshots)),
	)

	// Get edge ID from state manager
	edgeID := m.edgeID
	if edgeID == "" {
		m.logger.Warn("Edge ID not available, cannot sync screenshots")
		return
	}

	// Create sync request
	req := &httpsclienttypes.SyncScreenshotsRequest{
		EdgeID:      edgeID,
		CameraID:     cameraID,
		Screenshots: labeledScreenshots,
	}

	// Call VM gateway to sync screenshots
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := m.vmGateway.SyncScreenshots(callCtx, req)
	if err != nil {
		m.logger.Error("Failed to sync screenshots to VM",
			zap.String("camera_id", cameraID),
			zap.Error(err),
		)
		// On error, set state to VM gateway error
		m.mu.Lock()
		m.state.Status = EdgeStatusVMGatewayError
		m.persistStateToStorage(ctx, m.state)
		m.mu.Unlock()
		return
	}

	if !resp.Success {
		m.logger.Error("VM rejected screenshot sync",
			zap.String("camera_id", cameraID),
			zap.String("error", resp.ErrorMessage),
		)
		// On error, set state to VM gateway error
		m.mu.Lock()
		m.state.Status = EdgeStatusVMGatewayError
		m.persistStateToStorage(ctx, m.state)
		m.mu.Unlock()
		return
	}

	m.logger.Info("Screenshots synced to VM successfully",
		zap.String("camera_id", cameraID),
		zap.Int("count", len(labeledScreenshots)),
		zap.String("message", resp.Message),
	)
}

// syncCamerasWithVM syncs discovered cameras with VM and enables cameras that VM decides to enable
func (m *StateManagerImpl) syncCamerasWithVM(ctx context.Context) {
	m.logger.Info("Syncing cameras with VM")

	// Get all discovered cameras
	cameras, err := m.cctvService.GetDiscoveredCameras(ctx)
	if err != nil {
		m.logger.Error("Failed to get discovered cameras", zap.Error(err))
		return
	}

	if len(cameras) == 0 {
		m.logger.Info("No cameras to sync")
		return
	}

	// Convert cameras to sync request format
	cameraInfos := make([]*httpsclienttypes.CameraInfo, 0, len(cameras))
	for _, cam := range cameras {
		// Determine source based on camera type
		source := ""
		if cam.DevicePath != "" {
			source = cam.DevicePath
		} else if cam.ONVIFEndpoint != "" {
			source = cam.ONVIFEndpoint
		} else if cam.IPAddress != "" {
			source = cam.IPAddress
		}

		cameraInfos = append(cameraInfos, &httpsclienttypes.CameraInfo{
			CameraID: cam.ID,
			Name:     cam.Name,
			Type:     string(cam.Type),
			Source:   source,
			Enabled:  cam.Enabled,
		})
	}

	// Get edge ID from config or state
	edgeID := m.edgeID
	if edgeID == "" {
		edgeID = "edge-dev-001" // fallback
	}

	// Sync cameras with VM
	syncReq := &httpsclienttypes.SyncCamerasRequest{
		EdgeID:  edgeID,
		Cameras: cameraInfos,
	}

	syncResp, err := m.vmGateway.SyncCameras(ctx, syncReq)
	if err != nil {
		m.logger.Error("Failed to sync cameras with VM", zap.Error(err))
		// On error, set state to error (not ready)
		m.mu.Lock()
		if m.state.Status == EdgeStatusCameraDiscovered {
			m.state.Status = EdgeStatusVMGatewayError
			m.persistStateToStorage(ctx, m.state)
			m.logger.Error("State updated to vm_gateway_error due to camera sync failure")
		}
		m.mu.Unlock()
		return
	}

	if !syncResp.Success {
		m.logger.Error("Camera sync failed", zap.String("error", syncResp.ErrorMessage))
		// On sync failure, set state to error (not ready)
		m.mu.Lock()
		if m.state.Status == EdgeStatusCameraDiscovered {
			m.state.Status = EdgeStatusVMGatewayError
			m.persistStateToStorage(ctx, m.state)
			m.logger.Error("State updated to vm_gateway_error due to camera sync failure")
		}
		m.mu.Unlock()
		return
	}

	m.logger.Info("Cameras synced with VM successfully",
		zap.Int("total_cameras", len(cameras)),
		zap.Int("enabled_cameras", len(syncResp.EnabledCameras)),
	)

	// Enable cameras that VM decided to enable
	for _, enabledCam := range syncResp.EnabledCameras {
		if enabledCam.Enabled {
			if err := m.cctvService.EnableCamera(ctx, enabledCam.CameraID); err != nil {
				m.logger.Warn("Failed to enable camera",
					zap.String("camera_id", enabledCam.CameraID),
					zap.Error(err),
				)
			} else {
				m.logger.Info("Camera enabled by VM decision",
					zap.String("camera_id", enabledCam.CameraID),
				)
			}
		}
	}

	// Update state to camera_synced
	m.mu.Lock()
	if m.state.Status == EdgeStatusCameraDiscovered {
		m.state.Status = EdgeStatusCameraSynced
		m.persistStateToStorage(ctx, m.state)
		m.logger.Info("State updated to camera_synced")
	}
	m.mu.Unlock()
}

// handleCameraRegistered handles camera registration events
func (m *StateManagerImpl) handleCameraRegistered(ctx context.Context, ev eventbustypes.Event) {
	cameraID, _ := ev.Data["camera_id"].(string)
	m.logger.Info("Camera registered, workflow: enable and start processing",
		zap.String("camera_id", cameraID),
	)

	// Workflow: After registration, enable camera and start AI processing
	// 1. Enable camera via CCTV service
	// 2. Start frame capture
	// 3. Start AI processing for this camera
	// 4. Mark as pending and attempt capability sync
	m.pendingSync = true
	select {
	case m.syncTrigger <- struct{}{}:
		// Sync attempt triggered
	default:
		// Channel is full, sync already queued
	}
}

// handleCameraConnected handles camera connection events
func (m *StateManagerImpl) handleCameraConnected(ctx context.Context, ev eventbustypes.Event) {
	cameraID, _ := ev.Data["camera_id"].(string)
	m.logger.Info("Camera connected, workflow: start AI processing",
		zap.String("camera_id", cameraID),
	)

	// Workflow: Start AI frame processing for connected camera
	if m.aiGateway != nil {
		// Type assertion and call would be: aiGateway.StartFrameProcessing(ctx, cameraID)
		m.eventBus.Publish(eventbustypes.Event{
			Type:      "workflow.ai.start_processing",
			Source:    "edge-state-manager",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"camera_id": cameraID,
			},
		})
	}
}

// handleAIDetection handles AI detection events
func (m *StateManagerImpl) handleAIDetection(ctx context.Context, ev eventbustypes.Event) {
	cameraID, _ := ev.Data["camera_id"].(string)
	eventID, _ := ev.Data["event_id"].(string)
	m.logger.Info("AI detection, workflow: save screenshot and record clip",
		zap.String("camera_id", cameraID),
		zap.String("event_id", eventID),
	)

	// Workflow: When AI detects something:
	// 1. Save screenshot via CCTV service
	// 2. Record video clip
	// 3. Store metadata via meta-storage
	// 4. Upload to object-storage if needed
}

// handleStorageWarning handles storage warning events
func (m *StateManagerImpl) handleStorageWarning(ctx context.Context, ev eventbustypes.Event) {
	m.logger.Warn("Storage warning, workflow: trigger cleanup")

	// Workflow: When storage is getting full:
	// 1. Check old clips/snapshots
	// 2. Trigger cleanup via object-storage
	// 3. Update metadata via meta-storage
}

// handleStorageFull handles storage full events
func (m *StateManagerImpl) handleStorageFull(ctx context.Context, ev eventbustypes.Event) {
	m.logger.Error("Storage full, workflow: emergency cleanup and stop recording")

	// Workflow: When storage is full:
	// 1. Stop new recordings
	// 2. Emergency cleanup
	// 3. Alert via VM gateway
}

// handleClipRecorded handles clip recording events
func (m *StateManagerImpl) handleClipRecorded(ctx context.Context, ev eventbustypes.Event) {
	cameraID, _ := ev.Data["camera_id"].(string)
	clipID, _ := ev.Data["clip_id"].(string)
	m.logger.Debug("Clip recorded, workflow: store metadata and upload if needed",
		zap.String("camera_id", cameraID),
		zap.String("clip_id", clipID),
	)

	// Workflow: After clip is recorded:
	// 1. Store metadata via meta-storage
	// 2. Upload to object-storage if configured
	// 3. Sync with VM if connected
}

// handleModelDeployed handles model deployment events from VM → Edge API
func (m *StateManagerImpl) handleModelDeployed(ctx context.Context, ev eventbustypes.Event) {
	// Extract event data
	modelID, ok := ev.Data["model_id"].(string)
	if !ok || modelID == "" {
		m.logger.Warn("Model deployment event missing model_id")
		return
	}

	m.logger.Info("Handling model deployment event",
		zap.String("model_id", modelID),
	)

	// Transition state from screenshot_set_ready to model_deployed
	m.mu.Lock()
	currentStatus := m.state.Status
	if currentStatus == EdgeStatusScreenshotSetReady {
		m.state.Status = EdgeStatusModelDeployed
		m.state.LastUpdated = time.Now()
		m.logger.Info("State transition: screenshot_set_ready → model_deployed",
			zap.String("model_id", modelID),
		)
	} else {
		m.logger.Debug("Model deployed but state is not screenshot_set_ready, not transitioning",
			zap.String("current_status", string(currentStatus)),
			zap.String("model_id", modelID),
		)
	}
	newState := m.state
	m.mu.Unlock()

	// Persist state update
	m.persistStateToStorage(ctx, newState)

	// Build ModelMetadata from event data
	aiMetadata := &aigwtypes.ModelMetadata{
		ModelID: modelID,
	}

	// Extract optional fields
	if version, ok := ev.Data["version"].(string); ok {
		aiMetadata.Version = version
	}
	if modelType, ok := ev.Data["model_type"].(string); ok {
		aiMetadata.ModelType = modelType
	}
	if cameraID, ok := ev.Data["camera_id"].(string); ok && cameraID != "" {
		aiMetadata.CameraID = &cameraID
	}
	if framework, ok := ev.Data["framework"].(string); ok {
		aiMetadata.Framework = framework
	}
	if modelPath, ok := ev.Data["model_path"].(string); ok {
		aiMetadata.ModelPath = modelPath
	}
	if metadataPath, ok := ev.Data["metadata_path"].(string); ok {
		aiMetadata.MetadataPath = metadataPath
	}

	// Notify AI gateway about model deployment
	if m.aiGateway != nil {
		if err := m.aiGateway.NotifyModelDeployment(ctx, aiMetadata); err != nil {
			m.logger.Warn("Failed to notify AI gateway about model deployment",
				zap.String("model_id", modelID),
				zap.Error(err),
			)
		} else {
			m.logger.Info("AI gateway notified about model deployment",
				zap.String("model_id", modelID),
			)
		}
	} else {
		m.logger.Warn("AI gateway not available, cannot notify about model deployment")
	}
}

// executeFrameProcessingWorkflow executes workflows when frame processing is active
// This is the final operational state where Edge is actively monitoring cameras
func (m *StateManagerImpl) executeFrameProcessingWorkflow(ctx context.Context, state EdgeState) {
	m.logger.Debug("Edge in frame_processing state - actively monitoring cameras",
		zap.String("status", string(state.Status)),
	)
	// Frame processing is active, no additional workflow needed
	// The frame processing loops are already running
}

// executeModelDeployedWorkflow executes workflows when model is deployed
// This starts continuous frame processing for all enabled cameras
func (m *StateManagerImpl) executeModelDeployedWorkflow(ctx context.Context, state EdgeState) {
	m.logger.Info("Starting continuous frame processing for model_deployed state")

	// Get all enabled cameras
	if m.cctvService == nil {
		m.logger.Warn("CCTV service not available, cannot start frame processing")
		return
	}

	cameras, err := m.cctvService.ListCameras(ctx, true) // enabledOnly = true
	if err != nil {
		m.logger.Error("Failed to list enabled cameras for frame processing",
			zap.Error(err),
		)
		return
	}

	if len(cameras) == 0 {
		m.logger.Info("No enabled cameras found, frame processing not started")
		return
	}

	// Start frame processing for each enabled camera
	startedCount := 0
	for _, camera := range cameras {
		if err := m.startFrameProcessingForCamera(ctx, camera.ID); err == nil {
			startedCount++
		}
	}

	if startedCount > 0 {
		// Transition to frame_processing state when frame processing successfully starts
		m.mu.Lock()
		if m.state.Status == EdgeStatusModelDeployed {
			m.state.Status = EdgeStatusFrameProcessing
			m.state.LastUpdated = time.Now()
			m.logger.Info("State transition: model_deployed → frame_processing",
				zap.Int("cameras_processing", startedCount),
			)
			newState := m.state
			m.mu.Unlock()

			// Persist state update
			m.persistStateToStorage(ctx, newState)
		} else {
			m.mu.Unlock()
		}

		m.logger.Info("Frame processing started for all enabled cameras",
			zap.Int("camera_count", startedCount),
			zap.Duration("interval", m.frameProcessingInterval),
		)
	} else {
		m.logger.Warn("Failed to start frame processing for any cameras")
	}
}

// startFrameProcessingForCamera starts periodic frame capture and processing for a camera
// Returns error if frame processing cannot be started
func (m *StateManagerImpl) startFrameProcessingForCamera(ctx context.Context, cameraID string) error {
	m.frameProcessingMu.Lock()
	defer m.frameProcessingMu.Unlock()

	// Check if already processing for this camera
	if _, exists := m.frameProcessingActive[cameraID]; exists {
		m.logger.Debug("Frame processing already active for camera",
			zap.String("camera_id", cameraID),
		)
		return nil // Already processing, not an error
	}

	// Verify camera exists and is enabled
	if m.cctvService == nil {
		return fmt.Errorf("CCTV service not available")
	}

	camera, err := m.cctvService.GetCamera(ctx, cameraID)
	if err != nil {
		return fmt.Errorf("failed to get camera: %w", err)
	}

	if !camera.Enabled {
		return fmt.Errorf("camera is not enabled")
	}

	// Create cancel context for this camera's frame processing
	frameCtx, cancel := context.WithCancel(ctx)
	m.frameProcessingActive[cameraID] = cancel

	// Start frame processing goroutine
	m.wg.Add(1)
	go m.frameProcessingLoop(frameCtx, cameraID)

	m.logger.Info("Started frame processing for camera",
		zap.String("camera_id", cameraID),
		zap.Duration("interval", m.frameProcessingInterval),
	)

	return nil
}

// stopFrameProcessingForCamera stops frame processing for a camera
func (m *StateManagerImpl) stopFrameProcessingForCamera(cameraID string) {
	m.frameProcessingMu.Lock()
	defer m.frameProcessingMu.Unlock()

	cancel, exists := m.frameProcessingActive[cameraID]
	if !exists {
		return
	}

	cancel()
	delete(m.frameProcessingActive, cameraID)

	m.logger.Info("Stopped frame processing for camera",
		zap.String("camera_id", cameraID),
	)
}

// stopAllFrameProcessing stops frame processing for all cameras
func (m *StateManagerImpl) stopAllFrameProcessing() {
	m.frameProcessingMu.Lock()
	defer m.frameProcessingMu.Unlock()

	for cameraID, cancel := range m.frameProcessingActive {
		cancel()
		m.logger.Info("Stopped frame processing for camera",
			zap.String("camera_id", cameraID),
		)
	}

	// Clear the map
	m.frameProcessingActive = make(map[string]context.CancelFunc)

	m.logger.Info("Stopped frame processing for all cameras")
}

// frameProcessingLoop continuously captures frames from a camera and processes them
func (m *StateManagerImpl) frameProcessingLoop(ctx context.Context, cameraID string) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.frameProcessingInterval)
	defer ticker.Stop()

	m.logger.Info("Frame processing loop started",
		zap.String("camera_id", cameraID),
		zap.Duration("interval", m.frameProcessingInterval),
	)

	// Process first frame immediately
	m.processFrameForCamera(ctx, cameraID)

	// Then process frames at regular intervals
	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Frame processing loop stopped",
				zap.String("camera_id", cameraID),
			)
			return
		case <-ticker.C:
			m.processFrameForCamera(ctx, cameraID)
		}
	}
}

// processFrameForCamera captures a frame, stores it in object storage, and sends to AI gateway
func (m *StateManagerImpl) processFrameForCamera(ctx context.Context, cameraID string) {
	if m.cctvService == nil {
		return
	}

	// Capture frame from camera
	frame, err := m.cctvService.CaptureFrame(ctx, cameraID)
	if err != nil {
		m.logger.Warn("Failed to capture frame from camera",
			zap.String("camera_id", cameraID),
			zap.Error(err),
		)
		return
	}

	// Store frame in object storage (frames bucket)
	frameID := fmt.Sprintf("frame-%s-%d", cameraID, time.Now().Unix())
	frameKey := fmt.Sprintf("frames/%s/%s/%s.jpg", cameraID, time.Now().Format("2006-01-02"), frameID)

	if m.objectStorage != nil {
		// Store frame image data
		err = m.objectStorage.StoreFrame(ctx, frameKey, frame.Data)
		if err != nil {
			m.logger.Warn("Failed to store frame in object storage",
				zap.String("camera_id", cameraID),
				zap.String("frame_key", frameKey),
				zap.Error(err),
			)
			return
		}

		m.logger.Debug("Frame stored in object storage",
			zap.String("camera_id", cameraID),
			zap.String("frame_key", frameKey),
		)
	}

	// Send frame to AI gateway for processing
	if m.aiGateway != nil {
		// Use AI gateway to process the frame
		// The AI gateway will:
		// 1. Load the model from MinIO (if not already loaded)
		// 2. Process the frame
		// 3. Determine if it's similar to training set or has anomalies
		// 4. Delete normal frames or move suspicious ones to security event bucket
		err = m.aiGateway.ProcessFrame(ctx, cameraID, frameKey, frame.Data)
		if err != nil {
			m.logger.Warn("Failed to process frame with AI gateway",
				zap.String("camera_id", cameraID),
				zap.String("frame_key", frameKey),
				zap.Error(err),
			)
		}
	} else {
		m.logger.Debug("AI gateway not available, frame stored but not processed",
			zap.String("camera_id", cameraID),
			zap.String("frame_key", frameKey),
		)
	}
}

// handleScreenshotSaved handles screenshot save events
func (m *StateManagerImpl) handleScreenshotSaved(ctx context.Context, ev eventbustypes.Event) {
	cameraID, _ := ev.Data["camera_id"].(string)
	screenshotID, _ := ev.Data["screenshot_id"].(string)
	m.logger.Debug("Screenshot saved, workflow: update metadata",
		zap.String("camera_id", cameraID),
		zap.String("screenshot_id", screenshotID),
	)

	// Workflow: After screenshot is saved:
	// 1. Metadata already stored by CCTV service
	// 2. Update dataset status if needed
	// 3. Trigger capability sync to VM (if connected)
	if m.vmGateway != nil {
		select {
		case m.syncTrigger <- struct{}{}:
			// Sync triggered successfully
		default:
			// Channel is full, sync already queued
		}
	}
}

// executeScreenshotSetReadyWorkflow executes workflows when screenshot set is ready
// This syncs screenshots and their labels to VM for model training
func (m *StateManagerImpl) executeScreenshotSetReadyWorkflow(ctx context.Context, state EdgeState) {
	m.logger.Debug("Edge in screenshot_set_ready state")
	// Screenshot sync is handled in handleScreenshotSetReady when the event is received
}

// initiateWireGuardConnection initiates WireGuard connection in production mode
func (m *StateManagerImpl) initiateWireGuardConnection(ctx context.Context) {
	if m.vmGateway == nil {
		m.logger.Error("VM gateway not available, cannot establish WireGuard connection")
		m.mu.Lock()
		m.state.Status = EdgeStatusVMGatewayError
		m.state.LastUpdated = time.Now()
		m.persistStateToStorage(ctx, m.state)
		m.mu.Unlock()
		return
	}

	m.logger.Info("Initiating WireGuard connection to VM")

	// Check if already connected
	if m.vmGateway.IsConnected() {
		m.logger.Info("WireGuard already connected")
		// Update state to wireguard_connected
		m.mu.Lock()
		if m.state.Status == EdgeStatusWGConnecting || m.state.Status == EdgeStatusInitializing || m.state.Status == EdgeStatusDisconnected {
			m.state.Status = EdgeStatusWireGuardConn
			m.state.NetworkConnected = true
			m.state.LastUpdated = time.Now()
		}
		newState := m.state
		m.mu.Unlock()

		// Persist state update
		m.persistStateToStorage(ctx, newState)

		m.logger.Info("Edge state updated",
			zap.String("status", string(newState.Status)),
			zap.Bool("network_connected", newState.NetworkConnected))

		// WireGuard is started via fx lifecycle, we just monitor connection status
		return
	}

	// Poll for WireGuard connection status
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.NewTimer(60 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout.C:
			m.logger.Error("WireGuard connection timeout - connection not established within 60 seconds")
			m.mu.Lock()
			if m.state.Status == EdgeStatusWGConnecting {
				m.state.Status = EdgeStatusWGConnectionError
				m.state.LastUpdated = time.Now()
				m.persistStateToStorage(ctx, m.state)
			}
			m.mu.Unlock()
			return
		case <-ticker.C:
			if m.vmGateway.IsConnected() {
				m.logger.Info("WireGuard connection established")
				// Update state to wireguard_connected
				m.mu.Lock()
				if m.state.Status == EdgeStatusWGConnecting {
					m.state.Status = EdgeStatusWireGuardConn
					m.state.NetworkConnected = true
					m.state.LastUpdated = time.Now()
				}
				newState := m.state
				m.mu.Unlock()

				// Persist state update
				m.persistStateToStorage(ctx, newState)

				m.logger.Info("Edge state updated",
					zap.String("status", string(newState.Status)),
					zap.Bool("network_connected", newState.NetworkConnected))
				return
			}
		}
	}
}

// initiateHTTPConnection initiates HTTP connection in dev mode (WireGuard disabled)
func (m *StateManagerImpl) initiateHTTPConnection(ctx context.Context) {
	if m.vmGateway == nil {
		m.logger.Error("VM gateway not available, cannot establish HTTP connection")
		m.mu.Lock()
		m.state.Status = EdgeStatusVMGatewayError
		m.state.LastUpdated = time.Now()
		m.persistStateToStorage(ctx, m.state)
		m.mu.Unlock()
		return
	}

	m.logger.Info("Initiating HTTP connection to VM (dev mode, WireGuard disabled)")

	// Get edge ID
	edgeID := m.edgeID
	if edgeID == "" && m.config != nil {
		edgeID = m.config.VMGateway.EdgeID
	}
	if edgeID == "" {
		edgeID = "edge-dev-001" // Fallback default
	}

	// Try to authenticate with VM using the VM gateway interface
	// This will fail if VM is not running or unreachable
	m.logger.Info("Testing HTTP connection to VM by attempting authentication")
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err := m.vmGateway.Authenticate(testCtx, edgeID)
	if err != nil {
		m.logger.Error("HTTP connection test failed - VM is not reachable or not running",
			zap.Error(err))
		m.mu.Lock()
		if m.state.Status == EdgeStatusHTTPConnecting {
			m.state.Status = EdgeStatusHTTPConnectionError
			m.state.LastUpdated = time.Now()
			m.persistStateToStorage(ctx, m.state)
		}
		m.mu.Unlock()
		return
	}

	// Connection successful
	m.logger.Info("HTTP connection established and authenticated (dev mode)")
	m.mu.Lock()
	if m.state.Status == EdgeStatusHTTPConnecting {
		m.state.Status = EdgeStatusHTTPSConn
		m.state.NetworkConnected = true
		m.state.VMAuthenticated = true
		m.state.LastUpdated = time.Now()
	}
	newState := m.state
	m.mu.Unlock()

	// Persist state update
	m.persistStateToStorage(ctx, newState)

	m.logger.Info("Edge state updated",
		zap.String("status", string(newState.Status)),
		zap.Bool("network_connected", newState.NetworkConnected),
		zap.Bool("vm_authenticated", newState.VMAuthenticated))
}

// handleErrorState handles error state
func (m *StateManagerImpl) handleErrorState(ctx context.Context, state EdgeState) {
	m.logger.Warn("Edge in error state, attempting recovery")

	// Workflow: Error recovery
	// 1. Identify error source
	// 2. Attempt recovery
	// 3. Alert if recovery fails
}

// GetStatus returns the current edge status (thread-safe).
func (m *StateManagerImpl) GetStatus() EdgeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.Status
}

// GetState returns the current edge state (thread-safe).
func (m *StateManagerImpl) GetState() EdgeState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// checkServicesHealth checks the health of all services and updates status if any are unhealthy
func (m *StateManagerImpl) checkServicesHealth(ctx context.Context) {
	m.mu.Lock()

	// Only check health if status is initializing
	if m.state.Status != EdgeStatusInitializing {
		m.mu.Unlock()
		return
	}

	// Check meta-storage health (required service)
	if m.metaStorage == nil {
		m.logger.Error("Meta-storage is not available")
		m.state.Status = EdgeStatusMetaStorageError
		m.state.LastUpdated = time.Now()
		m.persistStateToStorage(ctx, m.state)
		return
	}
	_, err := m.metaStorage.GetStorageStats(ctx)
	if err != nil {
		m.logger.Error("Meta-storage health check failed", zap.Error(err))
		m.state.Status = EdgeStatusMetaStorageError
		m.state.LastUpdated = time.Now()
		m.persistStateToStorage(ctx, m.state)
		return
	}
	m.logger.Debug("Meta-storage health check passed")

	// Check object-storage health (optional service - only check if available)
	if m.objectStorage != nil {
		// Object-storage doesn't have a direct health check method
		// If it's initialized, we assume it's healthy (Start() would have failed if there was an issue)
		m.logger.Debug("Object-storage is available")
	} else {
		m.logger.Debug("Object-storage is not available (optional service)")
	}

	// Check CCTV service health (required service)
	if m.cctvService == nil {
		m.logger.Error("CCTV service is not available")
		m.state.Status = EdgeStatusCCTVServiceError
		m.state.LastUpdated = time.Now()
		m.persistStateToStorage(ctx, m.state)
		return
	}
	m.logger.Debug("CCTV service is available")

	// Check AI gateway health (required service)
	if m.aiGateway == nil {
		m.logger.Error("AI gateway is not available")
		m.state.Status = EdgeStatusAIGatewayError
		m.state.LastUpdated = time.Now()
		m.persistStateToStorage(ctx, m.state)
		return
	}
	m.logger.Debug("AI gateway is available")

	// Check VM gateway health (optional service - only check if available)
	if m.vmGateway != nil {
		// VM gateway is available, check if it's properly initialized
		// We can check WireGuard connection status, but in dev mode it might not be connected
		m.logger.Debug("VM gateway is available")
	} else {
		m.logger.Debug("VM gateway is not available (optional in dev mode)")
	}

	// Check web gateway health (required service)
	if m.webGateway == nil {
		m.logger.Error("Web gateway is not available")
		m.state.Status = EdgeStatusWebGatewayError
		m.state.LastUpdated = time.Now()
		m.persistStateToStorage(ctx, m.state)
		return
	}
	m.logger.Debug("Web gateway is available")

	// All required services are healthy, now initiate connection
	m.logger.Info("All services health checks passed, initiating connection")

	// Unlock mutex before calling initiateConnection (it will lock its own mutex)
	m.mu.Unlock()

	// Initiate connection based on WireGuard configuration
	m.initiateConnection(ctx)
}

// initiateConnection initiates WireGuard or HTTP connection based on configuration
func (m *StateManagerImpl) initiateConnection(ctx context.Context) {
	m.logger.Info("initiateConnection called")
	m.mu.Lock()

	// Only proceed if status is still initializing
	currentStatus := m.state.Status
	m.logger.Info("Current status check", zap.String("status", string(currentStatus)))
	if currentStatus != EdgeStatusInitializing {
		m.logger.Debug("Status is not initializing, skipping connection initiation", zap.String("status", string(currentStatus)))
		m.mu.Unlock()
		return
	}

	// Check if VM gateway is available
	if m.vmGateway == nil {
		m.logger.Warn("VM gateway is not available, cannot establish connection")
		m.state.Status = EdgeStatusVMGatewayError
		m.state.LastUpdated = time.Now()
		m.persistStateToStorage(ctx, m.state)
		m.mu.Unlock()
		return
	}
	m.logger.Info("VM gateway is available")

	// Check WireGuard enabled status from config
	wireGuardEnabled := false
	if m.config != nil {
		wireGuardEnabled = m.config.VMGateway.WireGuard.Enabled
		m.logger.Info("Checking connection configuration",
			zap.Bool("wireguard_enabled", wireGuardEnabled),
			zap.String("environment", m.config.Environment))
	} else {
		m.logger.Warn("Config is nil, cannot determine WireGuard status")
	}

	if wireGuardEnabled {
		// WireGuard is enabled - initiate WireGuard connection
		m.logger.Info("WireGuard is enabled, initiating WireGuard connection")
		m.state.Status = EdgeStatusWGConnecting
		m.state.LastUpdated = time.Now()
		m.persistStateToStorage(ctx, m.state)
		m.mu.Unlock()

		// Start WireGuard connection in a goroutine
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.initiateWireGuardConnection(ctx)
		}()
	} else {
		// WireGuard is disabled (dev mode) - connect via HTTP directly
		m.logger.Info("WireGuard is disabled, connecting via HTTP (localhost/dev mode)")
		m.state.Status = EdgeStatusHTTPConnecting
		m.state.LastUpdated = time.Now()
		m.persistStateToStorage(ctx, m.state)
		m.mu.Unlock()

		// Call HTTP connection synchronously so we can update status based on result
		m.initiateHTTPConnection(ctx)
	}
}

// persistStateToStorage persists the edge state to meta-storage
func (m *StateManagerImpl) persistStateToStorage(ctx context.Context, state EdgeState) {
	if m.metaStorage == nil {
		return
	}

	// Convert EdgeState to map for storage
	stateMap := map[string]interface{}{
		"status":               string(state.Status),
		"network_connected":    state.NetworkConnected,
		"vm_authenticated":     state.VMAuthenticated,
		"cameras_enabled":      state.CamerasEnabled,
		"ai_processing_active": state.AIProcessingActive,
		"storage_health":       state.StorageHealth,
		"last_updated":         state.LastUpdated.Format(time.RFC3339),
	}

	if err := m.metaStorage.SaveEdgeState(ctx, stateMap); err != nil {
		m.logger.Warn("failed to persist edge state to storage", zap.Error(err))
	}
}

// SyncCameraCapabilities syncs capabilities for a single camera to the VM
func (m *StateManagerImpl) SyncCameraCapabilities(ctx context.Context, cameraID string) error {
	if m.vmGateway == nil {
		return fmt.Errorf("VM gateway not available")
	}

	if m.vmGateway == nil || !m.vmGateway.IsHTTPConnected() {
		return fmt.Errorf("HTTPS not connected")
	}

	// Get camera
	if m.cctvService == nil {
		return fmt.Errorf("CCTV service not available")
	}

	cam, err := m.cctvService.GetCamera(ctx, cameraID)
	if err != nil {
		return fmt.Errorf("failed to get camera: %w", err)
	}

	// Build dataset status for this camera
	status := m.buildDatasetStatus(ctx, cam)
	if status == nil {
		return fmt.Errorf("failed to build dataset status")
	}

	// Build sync request with single camera
	req := &httpsclienttypes.SyncCapabilitiesRequest{
		SyncedAt: time.Now().UnixNano(),
		Cameras:  []*httpsclienttypes.CameraCapability{m.toCameraCapability(cam, status)},
	}

	// Call SyncCapabilities with timeout
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := m.vmGateway.SyncCapabilities(callCtx, req)
	if err != nil {
		return fmt.Errorf("capability sync failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("capability sync rejected: %s", resp.ErrorMessage)
	}

	m.logger.Info("Camera capability synced",
		zap.String("camera_id", cameraID),
	)
	return nil
}

// capabilitySyncLoop runs the periodic capability sync loop
func (m *StateManagerImpl) capabilitySyncLoop(ctx context.Context) {
	ticker := time.NewTicker(m.syncInterval)
	defer ticker.Stop()

	// Retry ticker for pending syncs (check every 5 seconds if we have pending cameras)
	retryTicker := time.NewTicker(5 * time.Second)
	defer retryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Periodic sync
			m.syncOnce(ctx)
		case <-retryTicker.C:
			// Retry pending syncs if HTTPS connection becomes ready
			if m.pendingSync {
				m.logger.Debug("Retrying capability sync for pending cameras")
				m.syncOnce(ctx)
			}
		case <-m.syncTrigger:
			// Immediate sync triggered by camera registration or WireGuard connection
			m.syncOnce(ctx)
		}
	}
}

// syncOnce performs a single capability sync for all cameras
func (m *StateManagerImpl) syncOnce(ctx context.Context) {
	// Check HTTPS connection first
	if m.vmGateway == nil || !m.vmGateway.IsHTTPConnected() {
		m.logger.Debug("Skipping capability sync - HTTPS not connected (will retry when connection is ready)")
		m.pendingSync = true // Mark as pending so we retry when connection is ready
		return
	}

	// List all cameras
	if m.cctvService == nil {
		m.logger.Debug("Skipping capability sync - CCTV service not available")
		return
	}

	cameras, err := m.cctvService.ListCameras(ctx, false)
	if err != nil {
		m.logger.Warn("Failed to list cameras for capability sync",
			zap.Error(err),
		)
		return
	}

	if len(cameras) == 0 {
		m.logger.Debug("Skipping capability sync - no cameras discovered yet")
		m.pendingSync = false
		return
	}

	m.logger.Debug("Starting capability sync",
		zap.Int("camera_count", len(cameras)),
	)

	req := &httpsclienttypes.SyncCapabilitiesRequest{
		SyncedAt: time.Now().UnixNano(),
	}

	for _, cam := range cameras {
		status := m.buildDatasetStatus(ctx, cam)
		if status != nil {
			req.Cameras = append(req.Cameras, m.toCameraCapability(cam, status))
		}
	}

	if len(req.Cameras) == 0 {
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if m.vmGateway == nil {
		m.logger.Debug("Skipping capability sync - VM gateway not available")
		m.pendingSync = true
		return
	}

	resp, err := m.vmGateway.SyncCapabilities(callCtx, req)
	if err != nil {
		// Check if it's an authentication error - if so, mark as pending and retry later
		if isAuthError(err) {
			m.logger.Debug("Capability sync failed - authentication not ready, will retry",
				zap.Error(err),
			)
			m.pendingSync = true
			return
		}
		m.logger.Warn("Capability sync failed",
			zap.Error(err),
		)
		m.pendingSync = true // Retry on other errors too
		return
	}

	if !resp.Success {
		// Check if it's an authentication-related rejection
		if resp.ErrorMessage != "" && (contains(resp.ErrorMessage, "not registered") || contains(resp.ErrorMessage, "authentication")) {
			m.logger.Debug("Capability sync rejected - authentication not ready, will retry",
				zap.String("error", resp.ErrorMessage),
			)
			m.pendingSync = true
			return
		}
		m.logger.Info("Capability sync rejected",
			zap.String("error", resp.ErrorMessage),
		)
		m.pendingSync = true
		return
	}

	// Success - clear pending flag
	m.pendingSync = false
	m.logger.Info("Capability sync sent successfully",
		zap.Int("cameras", len(req.Cameras)),
	)
}

// buildDatasetStatus builds dataset status for a camera
func (m *StateManagerImpl) buildDatasetStatus(ctx context.Context, cam *cctvtypes.Camera) *cctvtypes.DatasetStatus {
	if m.cctvService == nil {
		return &cctvtypes.DatasetStatus{
			LabelCounts:           make(map[string]int),
			RequiredSnapshotCount: m.minSnapshots,
			SnapshotRequired:      true,
			LastSynced:            time.Now(),
		}
	}

	datasetStatus, err := m.cctvService.GetDatasetStatus(ctx, cam.ID, m.minSnapshots)
	if err != nil {
		m.logger.Info("Failed to get dataset status",
			zap.String("camera_id", cam.ID),
			zap.Error(err),
		)
		return &cctvtypes.DatasetStatus{
			LabelCounts:           make(map[string]int),
			RequiredSnapshotCount: m.minSnapshots,
			SnapshotRequired:      true,
			LastSynced:            time.Now(),
		}
	}

	return datasetStatus
}

// toCameraCapability converts a camera and dataset status to a DTO format
// used for capability sync over HTTPS.
func (m *StateManagerImpl) toCameraCapability(cam *cctvtypes.Camera, status *cctvtypes.DatasetStatus) *httpsclienttypes.CameraCapability {
	labelCounts := make(map[string]uint32, len(status.LabelCounts))
	for label, count := range status.LabelCounts {
		if count < 0 {
			continue
		}
		labelCounts[label] = uint32(count)
	}

	return &httpsclienttypes.CameraCapability{
		CameraID:              cam.ID,
		Name:                  cam.Name,
		Type:                  string(cam.Type),
		Enabled:               cam.Enabled,
		Status:                string(cam.Status),
		LabelCounts:           labelCounts,
		LabeledSnapshotCount:  uint32(status.LabeledSnapshotCount),
		RequiredSnapshotCount: uint32(status.RequiredSnapshotCount),
		SnapshotRequired:      status.SnapshotRequired,
	}
}

// isAuthError checks if an error is authentication-related
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "unauthenticated") ||
		strings.Contains(errStr, "not registered") ||
		strings.Contains(errStr, "authentication failed")
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// UploadDatasetForCamera packages and uploads all screenshots for a camera to the VM
func (m *StateManagerImpl) UploadDatasetForCamera(ctx context.Context, cameraID string, screenshotList []interface{}) (string, error) {
	m.mu.RLock()
	config := m.config
	edgeID := m.edgeID
	m.mu.RUnlock()

	// Convert screenshot list to CCTV screenshot type
	screenshots := make([]*cctvtypes.Screenshot, 0, len(screenshotList))
	for _, ss := range screenshotList {
		if screenshot, ok := ss.(*cctvtypes.Screenshot); ok {
			screenshots = append(screenshots, screenshot)
		}
	}

	if len(screenshots) == 0 {
		return "", fmt.Errorf("no screenshots to upload for camera %s", cameraID)
	}

	// Check CCTV service
	if m.cctvService == nil {
		return "", fmt.Errorf("CCTV service not available")
	}

	// Determine output directory for temporary archive.
	// Use meta-storage data directory (if configured) as a base, with a fallback.
	outputDir := filepath.Join(config.MetaStorage.DataDir, "exports")
	if outputDir == "" {
		outputDir = "exports"
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	// Package dataset
	m.logger.Info("Packaging dataset for upload",
		zap.String("camera_id", cameraID),
		zap.Int("screenshot_count", len(screenshots)),
		zap.String("edge_id", edgeID),
	)

	archivePath, checksum, err := m.packageDataset(ctx, edgeID, cameraID, screenshots, outputDir, m.cctvService)
	if err != nil {
		return "", fmt.Errorf("failed to package dataset: %w", err)
	}

	// Clean up archive file after upload (defer)
	defer func() {
		if err := os.Remove(archivePath); err != nil {
			m.logger.Warn("Failed to clean up archive file",
				zap.Error(err),
				zap.String("archive_path", archivePath),
			)
		}
	}()

	// Upload dataset to VM
	m.logger.Info("Uploading dataset to VM",
		zap.String("camera_id", cameraID),
		zap.String("archive_path", archivePath),
		zap.String("checksum", checksum),
	)

	datasetID, err := m.uploadDataset(ctx, archivePath, cameraID, checksum, edgeID)
	if err != nil {
		return "", fmt.Errorf("failed to upload dataset: %w", err)
	}

	m.logger.Info("Dataset uploaded successfully",
		zap.String("camera_id", cameraID),
		zap.String("dataset_id", datasetID),
		zap.String("checksum", checksum),
	)

	return datasetID, nil
}

// DatasetMetadata contains metadata about the dataset
type datasetMetadata struct {
	EdgeID         string         `json:"edge_id"`
	CameraID       string         `json:"camera_id"`
	TotalSnapshots int            `json:"total_snapshots"`
	LabelCounts    map[string]int `json:"label_counts"`
	SyncedAt       time.Time      `json:"synced_at"`
	Checksum       string         `json:"checksum,omitempty"`
}

// ManifestEntry represents a single screenshot in the manifest
type manifestEntry struct {
	ScreenshotID string    `json:"screenshot_id"`
	Label        string    `json:"label"`
	CustomLabel  string    `json:"custom_label,omitempty"`
	Description  string    `json:"description,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	FilePath     string    `json:"file_path"`
}

// Manifest contains the list of all screenshots in the dataset
type manifest struct {
	CameraID    string          `json:"camera_id"`
	SyncedAt    time.Time       `json:"synced_at"`
	Screenshots []manifestEntry `json:"screenshots"`
}

// packageDataset packages all screenshots for a camera into a tar.gz archive
func (m *StateManagerImpl) packageDataset(
	ctx context.Context,
	edgeID string,
	cameraID string,
	screenshotList []*cctvtypes.Screenshot,
	outputDir string,
	cctvService cctv.CCTVService,
) (archivePath string, checksum string, err error) {
	// Generate archive filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	archiveFilename := fmt.Sprintf("dataset_%s_%s_%s.tar.gz", edgeID, cameraID, timestamp)
	archivePath = filepath.Join(outputDir, archiveFilename)

	// Create tar.gz archive
	file, err := os.Create(archivePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create archive file: %w", err)
	}
	defer file.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Calculate label counts
	labelCounts := make(map[string]int)
	for _, ss := range screenshotList {
		label := ss.Label
		if ss.CustomLabel != "" {
			label = ss.CustomLabel
		}
		labelCounts[label]++
	}

	// Create manifest entries
	manifestEntries := make([]manifestEntry, 0, len(screenshotList))
	syncedAt := time.Now()

	// Add metadata.json
	metadata := datasetMetadata{
		EdgeID:         edgeID,
		CameraID:       cameraID,
		TotalSnapshots: len(screenshotList),
		LabelCounts:    labelCounts,
		SyncedAt:       syncedAt,
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write metadata.json to archive
	if err := m.writeFileToTar(tarWriter, "metadata.json", metadataJSON); err != nil {
		return "", "", fmt.Errorf("failed to write metadata.json: %w", err)
	}

	// Add all screenshot files to archive
	for _, ss := range screenshotList {
		// Load screenshot image from object storage
		frame, err := cctvService.GetScreenshotImage(ctx, ss.ID)
		if err != nil {
			m.logger.Warn("Failed to load screenshot image from object storage, skipping",
				zap.Error(err),
				zap.String("screenshot_id", ss.ID),
				zap.String("object_key", ss.ObjectKey),
			)
			continue
		}

		// Create relative path in archive
		archivePath := filepath.Join("screenshots", fmt.Sprintf("%s.jpg", ss.ID))

		// Write screenshot to archive
		if err := m.writeFileToTar(tarWriter, archivePath, frame.Data); err != nil {
			m.logger.Warn("Failed to write screenshot to archive, skipping",
				zap.Error(err),
				zap.String("screenshot_id", ss.ID),
			)
			continue
		}

		// Add to manifest
		customLabel := ss.CustomLabel
		description := ss.Description

		manifestEntries = append(manifestEntries, manifestEntry{
			ScreenshotID: ss.ID,
			Label:        ss.Label,
			CustomLabel:  customLabel,
			Description:  description,
			CreatedAt:    ss.CreatedAt,
			FilePath:     archivePath,
		})
	}

	// Create and write manifest.json
	manifest := manifest{
		CameraID:    cameraID,
		SyncedAt:    syncedAt,
		Screenshots: manifestEntries,
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := m.writeFileToTar(tarWriter, "manifest.json", manifestJSON); err != nil {
		return "", "", fmt.Errorf("failed to write manifest.json: %w", err)
	}

	// Close tar writer to flush data
	if err := tarWriter.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Close gzip writer to flush data
	if err := gzipWriter.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Close file
	if err := file.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close archive file: %w", err)
	}

	// Calculate checksum from the archive file
	checksumBytes, err := m.calculateFileChecksum(archivePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	checksum = fmt.Sprintf("%x", checksumBytes)

	m.logger.Info("Dataset packaged successfully",
		zap.String("archive_path", archivePath),
		zap.String("camera_id", cameraID),
		zap.Int("total_snapshots", len(screenshotList)),
		zap.String("checksum", checksum),
	)

	return archivePath, checksum, nil
}

// writeFileToTar writes a file to the tar archive
func (m *StateManagerImpl) writeFileToTar(tarWriter *tar.Writer, path string, data []byte) error {
	header := &tar.Header{
		Name:    path,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: time.Now(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	if _, err := tarWriter.Write(data); err != nil {
		return fmt.Errorf("failed to write file data: %w", err)
	}

	return nil
}

// calculateFileChecksum calculates SHA-256 checksum of a file
func (m *StateManagerImpl) calculateFileChecksum(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}

	return hasher.Sum(nil), nil
}

// uploadDataset uploads a dataset archive to the VM
func (m *StateManagerImpl) uploadDataset(ctx context.Context, archivePath string, cameraID string, checksum string, edgeID string) (string, error) {
	m.mu.RLock()
	httpClient := m.httpClient
	vmEndpoint := m.vmEndpoint
	m.mu.RUnlock()

	if httpClient == nil {
		return "", fmt.Errorf("HTTP client not initialized")
	}

	// Verify archive file exists
	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		return "", fmt.Errorf("archive file not found: %w", err)
	}
	fileSize := fileInfo.Size()

	m.logger.Info("Starting dataset upload",
		zap.String("archive_path", archivePath),
		zap.String("camera_id", cameraID),
		zap.Int64("file_size_bytes", fileSize),
		zap.String("checksum", checksum),
		zap.String("vm_endpoint", vmEndpoint),
	)

	// Execute upload with retry logic
	datasetID, err := m.uploadWithRetry(ctx, archivePath, cameraID, checksum, edgeID, fileSize, httpClient, vmEndpoint)
	if err != nil {
		return "", err
	}

	return datasetID, nil
}

// uploadWithRetry attempts to upload with exponential backoff retry
func (m *StateManagerImpl) uploadWithRetry(ctx context.Context, archivePath string, cameraID string, checksum string, edgeID string, fileSize int64, httpClient *http.Client, vmEndpoint string) (string, error) {
	maxRetries := 3
	baseDelay := 2 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			m.logger.Info("Retrying dataset upload",
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", maxRetries),
				zap.Float64("delay_seconds", delay.Seconds()),
				zap.String("camera_id", cameraID),
			)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		// Create multipart request
		req, err := m.createMultipartRequest(ctx, archivePath, cameraID, checksum, edgeID, vmEndpoint)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		// Execute request
		startTime := time.Now()
		resp, err := httpClient.Do(req)
		uploadDuration := time.Since(startTime)

		if err != nil {
			if attempt == maxRetries-1 {
				return "", fmt.Errorf("upload failed after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		// Read response body
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			if attempt == maxRetries-1 {
				return "", fmt.Errorf("failed to read response: %w", err)
			}
			continue
		}

		// Check response status
		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
			// Don't retry on client errors (4xx)
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return "", err
			}
			if attempt == maxRetries-1 {
				return "", err
			}
			continue
		}

		// Success! Parse JSON response
		var response struct {
			DatasetID string `json:"dataset_id"`
			Message   string `json:"message,omitempty"`
		}
		datasetID := ""
		if err := json.Unmarshal(body, &response); err == nil {
			datasetID = response.DatasetID
		} else {
			m.logger.Warn("Failed to parse dataset_id from response",
				zap.Error(err),
				zap.String("response_body", string(body)),
			)
			if len(body) > 0 {
				datasetID = string(body)
			}
		}

		m.logger.Info("Dataset upload successful",
			zap.Int("status_code", resp.StatusCode),
			zap.String("dataset_id", datasetID),
			zap.Int64("file_size_bytes", fileSize),
			zap.Int("attempt", attempt+1),
			zap.Float64("duration_seconds", uploadDuration.Seconds()),
		)

		return datasetID, nil
	}

	return "", fmt.Errorf("upload failed after %d attempts", maxRetries)
}

// createMultipartRequest creates a new multipart form request for dataset upload
func (m *StateManagerImpl) createMultipartRequest(ctx context.Context, archivePath string, cameraID string, checksum string, edgeID string, vmEndpoint string) (*http.Request, error) {
	// Open archive file
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add archive file
	part, err := writer.CreateFormFile("dataset", filepath.Base(archivePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	// Copy file to form
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file to form: %w", err)
	}

	// Add metadata fields
	if err := writer.WriteField("camera_id", cameraID); err != nil {
		return nil, fmt.Errorf("failed to write camera_id field: %w", err)
	}

	if err := writer.WriteField("checksum", checksum); err != nil {
		return nil, fmt.Errorf("failed to write checksum field: %w", err)
	}

	// Close multipart writer
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create HTTP request
	uploadURL := vmEndpoint + "/api/datasets/upload"
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, &requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Add Edge ID header for authentication
	if edgeID != "" {
		req.Header.Set("X-Edge-ID", edgeID)
	}

	return req, nil
}

// hasProtocol checks if a URL has a protocol prefix
func hasProtocol(url string) bool {
	return len(url) > 7 && (url[0:7] == "http://" || url[0:8] == "https://")
}

// ReportStatus reports deployment status to the VM
func (m *StateManagerImpl) ReportStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error {
	if m.vmGateway == nil {
		return fmt.Errorf("VM gateway not available")
	}

	if !m.vmGateway.IsHTTPConnected() {
		// Queue for retry when connection is available
		m.logger.Debug("HTTPS not connected, deployment status will be retried when connection is available",
			zap.String("deployment_id", deploymentID),
			zap.String("status", status),
		)
		// For now, return error - in the future we could add a queue here
		return fmt.Errorf("HTTPS not connected")
	}

	if m.vmGateway == nil {
		return fmt.Errorf("VM gateway not available")
	}
	err := m.vmGateway.ReportDeploymentStatus(ctx, deploymentID, status, errorMessage, modelPath)
	if err != nil {
		m.logger.Warn("Failed to report deployment status",
			zap.String("deployment_id", deploymentID),
			zap.String("status", status),
			zap.Error(err),
		)
		return err
	}

	m.logger.Info("Deployment status reported successfully",
		zap.String("deployment_id", deploymentID),
		zap.String("status", status),
	)

	return nil
}

// SetStateManager is deprecated - use @meta-storage and @object-storage instead
// This method is kept for backward compatibility but does nothing
func (m *StateManagerImpl) SetStateManager(stateManager interface{}) {
	m.logger.Warn("SetStateManager is deprecated - use @meta-storage and @object-storage instead")
}

// Security event storage methods are now in event_storage.go

// Snapshot Request Management methods are now in snapshot_request_storage.go
