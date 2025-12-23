package impl

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
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
	EventTypeCameraDiscovered      eventbustypes.EventType = "camera.discovered"
	EventTypeCameraRegistered      eventbustypes.EventType = "camera.registered"
	EventTypeCameraConnected       eventbustypes.EventType = "camera.connected"
	EventTypeCameraDisconnected    eventbustypes.EventType = "camera.disconnected"
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
	EdgeStatusDisconnected  EdgeStatus = "disconnected"
	EdgeStatusWireGuardConn EdgeStatus = "wireguard_connected"
	EdgeStatusHTTPSConn     EdgeStatus = "https_connected"
	EdgeStatusAuthenticated EdgeStatus = "authenticated"
	EdgeStatusReady         EdgeStatus = "ready"
	EdgeStatusError         EdgeStatus = "error"
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

// HTTPSClient interface for VM communication
type HTTPSClient interface {
	IsConnected() bool
	SyncCapabilities(ctx context.Context, req *httpsclienttypes.SyncCapabilitiesRequest) (*httpsclienttypes.SyncCapabilitiesResponse, error)
	ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error
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
	httpsClient   HTTPSClient // HTTPS client for VM communication

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

	ctx, cancel := context.WithCancel(context.Background())

	return &StateManagerImpl{
		eventBus: bus,
		logger:   log,
		state: EdgeState{
			Status:        EdgeStatusDisconnected,
			StorageHealth: "healthy",
			LastUpdated:   time.Now(),
		},
		syncInterval:            5 * time.Minute, // Default sync interval
		syncTrigger:             make(chan struct{}, 1),
		pendingSnapshotRequests: make(map[string]*types.PendingSnapshotRequest),
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

// SetHTTPSClient sets the HTTPS client for VM communication
func (m *StateManagerImpl) SetHTTPSClient(httpsClient interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if client, ok := httpsClient.(HTTPSClient); ok {
		m.httpsClient = client
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
		m.logger.Info("Starting edge state manager, subscribing to all events")

		events := m.eventBus.SubscribeAll()

		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.run(events)
		}()

		// Start capability sync loop if HTTPS client is available
		if m.httpsClient != nil && m.cctvService != nil {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.capabilitySyncLoop(ctx)
			}()
			m.logger.Info("Capability sync loop started")
		}

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
		if newState.Status == EdgeStatusDisconnected {
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
		if newState.Status == EdgeStatusHTTPSConn || newState.Status == EdgeStatusAuthenticated || newState.Status == EdgeStatusReady {
			newState.Status = EdgeStatusWireGuardConn
			newState.VMAuthenticated = false
		}
	case EventTypeEdgeAuthenticated:
		if newState.Status == EdgeStatusHTTPSConn {
			newState.Status = EdgeStatusAuthenticated
			newState.VMAuthenticated = true
		}

	// Camera events
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

	// Update ready status if all conditions are met
	if newState.Status == EdgeStatusAuthenticated && newState.NetworkConnected && newState.VMAuthenticated {
		if newState.Status != EdgeStatusReady {
			newState.Status = EdgeStatusReady
			m.logger.Info("Edge appliance is ready for operations")
		}
	}

	// Log state transition if status changed
	if oldState.Status != newState.Status {
		m.logger.Info("edge-state-manager: state transition",
			zap.String("old_status", string(oldState.Status)),
			zap.String("new_status", string(newState.Status)),
			zap.String("event_type", string(ev.Type)),
		)
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
		// Workflow: WireGuard -> HTTPS -> Auth -> Ready

	case EventTypeHTTPSConnected:
		m.logger.Info("HTTPS connected, initiating authentication")
		// Workflow: Trigger authentication if needed

	case EventTypeEdgeAuthenticated:
		m.logger.Info("Edge authenticated, initializing services")
		// Workflow: After authentication, ensure cameras are discovered and AI is ready
		m.initializeServicesAfterAuth(ctx)

	// Camera workflow
	case EventTypeCameraDiscovered:
		m.handleCameraDiscovered(ctx, ev)
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
	case EdgeStatusReady:
		m.executeReadyStateWorkflow(ctx, state)
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

	// Ensure AI gateway is ready
	if m.aiGateway != nil {
		m.logger.Debug("AI gateway available, ready for processing")
	}
}

// handleCameraDiscovered handles camera discovery events
func (m *StateManagerImpl) handleCameraDiscovered(ctx context.Context, ev eventbustypes.Event) {
	cameraID, _ := ev.Data["camera_id"].(string)
	m.logger.Info("Camera discovered, workflow: register if needed",
		zap.String("camera_id", cameraID),
	)

	// Workflow: Auto-register discovered cameras if configured
	// This would interact with CCTV service to register the camera
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
	if m.aiGateway == nil {
		m.logger.Warn("AI gateway not available, cannot notify about model deployment")
		return
	}

	// Extract event data
	modelID, ok := ev.Data["model_id"].(string)
	if !ok || modelID == "" {
		m.logger.Warn("Model deployment event missing model_id")
		return
	}

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
	if m.httpsClient != nil {
		select {
		case m.syncTrigger <- struct{}{}:
			// Sync triggered successfully
		default:
			// Channel is full, sync already queued
		}
	}
}

// executeReadyStateWorkflow executes workflows when edge is in ready state
func (m *StateManagerImpl) executeReadyStateWorkflow(ctx context.Context, state EdgeState) {
	// Continuous monitoring and coordination in ready state
	// This runs periodically to ensure all services are coordinated

	// Example: Monitor camera health, AI processing status, storage usage
	m.logger.Debug("Edge in ready state, monitoring services")
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

// SyncCameraCapabilities syncs capabilities for a single camera to the VM
func (m *StateManagerImpl) SyncCameraCapabilities(ctx context.Context, cameraID string) error {
	if m.httpsClient == nil {
		return fmt.Errorf("HTTPS client not available")
	}

	if !m.httpsClient.IsConnected() {
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

	resp, err := m.httpsClient.SyncCapabilities(callCtx, req)
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
	if m.httpsClient == nil || !m.httpsClient.IsConnected() {
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

	resp, err := m.httpsClient.SyncCapabilities(callCtx, req)
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
	if m.httpsClient == nil {
		return fmt.Errorf("HTTPS client not available")
	}

	if !m.httpsClient.IsConnected() {
		// Queue for retry when connection is available
		m.logger.Debug("HTTPS not connected, deployment status will be retried when connection is available",
			zap.String("deployment_id", deploymentID),
			zap.String("status", status),
		)
		// For now, return error - in the future we could add a queue here
		return fmt.Errorf("HTTPS not connected")
	}

	err := m.httpsClient.ReportDeploymentStatus(ctx, deploymentID, status, errorMessage, modelPath)
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
