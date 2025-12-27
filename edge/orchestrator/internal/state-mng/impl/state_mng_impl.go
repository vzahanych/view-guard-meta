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
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng/types"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	webgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway"
	"go.uber.org/zap"
)

// Known edge-related event types used by the state manager.
const (
	EventTypeTunnelConnected    eventbustypes.EventType = "network.tunnel.connected"
	EventTypeTunnelDisconnected eventbustypes.EventType = "network.tunnel.disconnected"
	EventTypeTransportConnected    eventbustypes.EventType = "network.transport.connected"
	EventTypeTransportDisconnected eventbustypes.EventType = "network.transport.disconnected"
	EventTypeEdgeAuthenticated     eventbustypes.EventType = "edge.authenticated"
	EventTypeCapabilitiesReceived  eventbustypes.EventType = "edge.capabilities_received"
	EventTypeDeviceDiscovered      eventbustypes.EventType = "device.discovered"
	EventTypeDeviceRegistered      eventbustypes.EventType = "device.registered"
	EventTypeDeviceConnected       eventbustypes.EventType = "device.connected"
	EventTypeDeviceDisconnected    eventbustypes.EventType = "device.disconnected"
	EventTypeDataUnitRequested          eventbustypes.EventType = "data_unit.requested"
	EventTypeDataUnitSetReady           eventbustypes.EventType = "data_unit.set_ready"
	EventTypeRawDeviceDataFrameReceived eventbustypes.EventType = "raw_device_data.frame_received"
	EventTypeDetection                  eventbustypes.EventType = "ai.detection"
	EventTypeInference                  eventbustypes.EventType = "ai.inference"
	EventTypeDataUnitSaved              eventbustypes.EventType = "data_unit.saved"
	EventTypeRawDeviceDataClipRecorded  eventbustypes.EventType = "raw_device_data.clip_recorded"
	EventTypeStorageFull           eventbustypes.EventType = "storage.full"
	EventTypeStorageWarning        eventbustypes.EventType = "storage.warning"
	EventTypeModelDeployed         eventbustypes.EventType = "model.deployed"
	EventTypeModelDeploymentStatus eventbustypes.EventType = "model.deployment.status"
)

// OperationalState represents operational metrics and health information
// that are not part of the state machines (connection or camera)
type OperationalState struct {
	CamerasEnabled     int
	AIProcessingActive bool
	StorageHealth      string // "healthy", "warning", "full"
	LastUpdated        time.Time
}

// PendingModelDeployment represents a model deployment event that arrived
// before the camera was ready (out-of-order event)
type PendingModelDeployment struct {
	ModelID    string
	CameraID   string
	EventData  map[string]interface{}
	ReceivedAt time.Time
}

// StateManagerImpl implements the StateManager interface.
//
// Architecture: Pure Workflow Orchestrator
// =========================================
// StateManagerImpl is a pure workflow orchestrator that does NOT own state machine implementations.
// Instead, it:
//   - Queries connection state from vm-gateway service (via VMGateway interface)
//   - Queries device state from iot service (via DeviceStateService interface)
//   - Orchestrates workflows based on observed state from these services
//   - Handles event processing and workflow triggering
//   - Coordinates cross-service operations and recovery
//
// State Machine Ownership:
//   - Connection state machines: Owned by vm-gateway service
//   - Device state machines: Owned by iot.DeviceStateService
//   - StateManagerImpl: Only queries state, never creates or manages state machines directly
//
// Fallback Behavior:
//   - For backward compatibility during migration, fallback paths exist that create
//     old-style state machines when services are not available. These should be removed
//     once all deployments are migrated to the new architecture.
type StateManagerImpl struct {
	eventBus eventbus.EventBus
	logger   *zap.Logger
	config   *config.Config

	// Service dependencies
	aiGateway        aigateway.AIGateway
	cctvService      cctv.CCTVService
	metaStorage      metastorage.MetaDataStore
	objectStorage    objectstorage.ObjectStorageService
	vmGateway        vmgateway.VMGateway
	webGateway       webgateway.WebGateway
	deviceStateService iot.DeviceStateService // Device state service (owns device state machines)

	// Capability sync state
	minSnapshots  int
	syncInterval  time.Duration
	syncTrigger   chan struct{} // Channel to trigger immediate sync
	pendingSync   bool          // Flag to indicate cameras are waiting to sync
	pendingSyncMu sync.RWMutex  // Mutex for pendingSync flag

	// Capability sync change detection
	lastCapabilitySync map[string]time.Time // cameraID -> last sync timestamp
	capabilitySyncMu   sync.RWMutex         // Mutex for lastCapabilitySync

	// Pending model deployments (for out-of-order events)
	pendingModelDeployments map[string]*PendingModelDeployment // cameraID -> deployment
	pendingModelDeployMu    sync.RWMutex                       // Mutex for pending model deployments

	// Frame capture error tracking
	frameCaptureErrors map[string]int // cameraID -> consecutive error count
	frameErrorMu       sync.RWMutex   // Mutex for frame capture errors

	// Workflow execution concurrency control
	workflowSemaphore chan struct{} // Semaphore to limit concurrent workflows (deprecated, kept for backward compatibility)
	
	// Worker pool for workflow execution
	workflowPoolQueue    chan *WorkflowTask // Global queue for workflow tasks
	workflowPoolWorkers  int                 // Number of worker goroutines in the pool
	workflowPoolWg       sync.WaitGroup      // WaitGroup for worker pool goroutines
	workflowPoolStarted  bool                 // Whether worker pool is started
	workflowPoolMu       sync.RWMutex        // Mutex for worker pool state
	
	// Per-source workflow queues for serialized execution
	workflowQueues     map[string]chan *WorkflowTask // source -> queue
	workflowQueuesMu   sync.RWMutex                  // Mutex for workflow queues map
	workflowWorkersWg  sync.WaitGroup                // WaitGroup for workflow queue workers
	serializeWorkflows bool                          // Whether to serialize workflows per source
	
	// Event deduplication for idempotency
	processedEvents    map[string]time.Time // eventKey -> timestamp (for cleanup)
	processedEventsMu  sync.RWMutex         // Mutex for processed events map
	eventDedupWindow   time.Duration         // Time window for event deduplication (default: 1 hour)
	
	// Workflow idempotency tracking
	lastScreenshotSync map[string]time.Time // cameraID -> last sync timestamp
	screenshotSyncMu   sync.RWMutex         // Mutex for screenshot sync tracking
	servicesInitialized bool                 // Whether services have been initialized after auth
	servicesInitMu      sync.RWMutex        // Mutex for services initialized flag

	// Configuration values
	frameCaptureErrorThreshold int // Threshold for consecutive frame capture failures before error state

	// State persistence retry configuration
	statePersistenceMaxRetries int           // Maximum retry attempts for persistence failures
	statePersistenceRetryBackoff time.Duration // Initial backoff duration for retries

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
	frameProcessingInterval time.Duration                 // Interval between frame captures (default: 30s)
	frameProcessingActive   map[string]context.CancelFunc // cameraID -> cancel function for frame processing
	frameProcessingMu       sync.RWMutex                  // Mutex for frame processing state
	frameProcessingWg       sync.WaitGroup                // WaitGroup specifically for frame processing goroutines

	// Camera state machine adapters cache (for backward compatibility)
	// Note: Camera state machines are owned by iot.DeviceStateService, not by StateManagerImpl.
	// We cache adapters here to maintain backward compatibility with the CameraStateMachine interface.
	// The adapters wrap DeviceStateMachine instances from the iot service.
	cameraStateMachineAdapters   map[string]types.CameraStateMachine // cameraID -> adapter wrapping DeviceStateMachine
	cameraStateMachinesMu        sync.RWMutex                        // Mutex for camera state machine adapters map

	// Operational state (metrics and health, not part of state machines)
	operationalState OperationalState
	operationalMu    sync.RWMutex

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

	// Initialize operational state
	operationalState := OperationalState{
		StorageHealth: "healthy",
		LastUpdated:   time.Now(),
	}

	return &StateManagerImpl{
		eventBus:                  bus,
		logger:                    log,
		cameraStateMachineAdapters: make(map[string]types.CameraStateMachine),
		operationalState:           operationalState,
		syncInterval:               5 * time.Minute, // Default sync interval
		syncTrigger:                make(chan struct{}, 1),
		lastCapabilitySync:         make(map[string]time.Time),
		pendingSnapshotRequests:    make(map[string]*types.PendingSnapshotRequest),
		pendingModelDeployments:    make(map[string]*PendingModelDeployment),
		frameProcessingInterval:    30 * time.Second, // Default: 30 seconds between frames
		frameProcessingActive:      make(map[string]context.CancelFunc),
		frameCaptureErrors:            make(map[string]int),
		frameCaptureErrorThreshold:    5,                       // Default: 5 consecutive failures
		workflowSemaphore:             make(chan struct{}, 10), // Default: 10 concurrent workflows (deprecated)
		workflowPoolQueue:             make(chan *WorkflowTask, 1000), // Global workflow queue (buffer: 1000)
		workflowPoolWorkers:           10,                     // Default: 10 worker goroutines
		workflowPoolStarted:           false,
		statePersistenceMaxRetries:   3,                       // Default: 3 retries
		statePersistenceRetryBackoff: 1 * time.Second,         // Default: 1 second backoff
		workflowQueues:                make(map[string]chan *WorkflowTask),
		serializeWorkflows:            true, // Default: serialize workflows per source (leverages event ordering)
		processedEvents:               make(map[string]time.Time),
		eventDedupWindow:              1 * time.Hour, // Default: 1 hour deduplication window
		lastScreenshotSync:            make(map[string]time.Time),
		servicesInitialized:           false,
		ctx:                           ctx,
		cancel:                        cancel,
	}, nil
}

// WorkflowTask represents a workflow execution task
type WorkflowTask struct {
	Event           eventbustypes.Event
	ConnectionState vmgatewaytypes.ConnectionState
	CameraStates    map[string]types.CameraState
}

// Name returns the service name.
func (m *StateManagerImpl) Name() string {
	return "edge-state-manager"
}

// SetAIGateway sets the AI gateway service dependency
func (m *StateManagerImpl) SetAIGateway(aiGateway interface{}) {
	// No mutex needed - these are set during initialization before Start() is called
	if gw, ok := aiGateway.(aigateway.AIGateway); ok {
		m.aiGateway = gw
	}
}

// SetCCTVService sets the CCTV service dependency
func (m *StateManagerImpl) SetCCTVService(cctvService interface{}) {
	// No mutex needed - these are set during initialization before Start() is called
	if svc, ok := cctvService.(cctv.CCTVService); ok {
		m.cctvService = svc
	}
}

// SetMetaStorage sets the metadata storage service dependency
func (m *StateManagerImpl) SetMetaStorage(metaStorage interface{}) {
	// No mutex needed - these are set during initialization before Start() is called
	if store, ok := metaStorage.(metastorage.MetaDataStore); ok {
		m.metaStorage = store
	}
}

// SetObjectStorage sets the object storage service dependency
func (m *StateManagerImpl) SetObjectStorage(objectStorage interface{}) {
	// No mutex needed - these are set during initialization before Start() is called
	if store, ok := objectStorage.(objectstorage.ObjectStorageService); ok {
		m.objectStorage = store
	}
}

// SetVMGateway sets the VM gateway service dependency
func (m *StateManagerImpl) SetVMGateway(vmGateway interface{}) {
	// No mutex needed - these are set during initialization before Start() is called
	if gw, ok := vmGateway.(vmgateway.VMGateway); ok {
		m.vmGateway = gw
	}
}

// SetDeviceStateService sets the device state service dependency
func (m *StateManagerImpl) SetDeviceStateService(deviceStateService interface{}) {
	// No mutex needed - these are set during initialization before Start() is called
	if svc, ok := deviceStateService.(iot.DeviceStateService); ok {
		m.deviceStateService = svc
	}
}

// SetWebGateway sets the web gateway service dependency
func (m *StateManagerImpl) SetWebGateway(webGateway interface{}) {
	// No mutex needed - these are set during initialization before Start() is called
	if gw, ok := webGateway.(webgateway.WebGateway); ok {
		m.webGateway = gw
	}
}

// SetConfig sets the configuration
func (m *StateManagerImpl) SetConfig(cfg *config.Config) {
	// No mutex needed - these are set during initialization before Start() is called
	m.config = cfg
	if cfg != nil {
		// Apply state manager config if provided
		stateMngConfig := cfg.StateManager
		stateMngConfig.ApplyDefaults()

		// Update frame processing interval if configured
		if stateMngConfig.FrameProcessingInterval > 0 {
			m.frameProcessingInterval = stateMngConfig.FrameProcessingInterval
			m.logger.Info("Frame processing interval configured",
				zap.Duration("interval", m.frameProcessingInterval),
			)
		}

		// Update capability sync interval if configured
		if stateMngConfig.CapabilitySyncInterval > 0 {
			m.syncInterval = stateMngConfig.CapabilitySyncInterval
			m.logger.Info("Capability sync interval configured",
				zap.Duration("interval", m.syncInterval),
			)
		}

		// Update max concurrent workflows if configured (worker pool size)
		if stateMngConfig.MaxConcurrentWorkflows > 0 {
			// Update worker pool size
			oldWorkers := m.workflowPoolWorkers
			m.workflowPoolWorkers = stateMngConfig.MaxConcurrentWorkflows
			
			// If pool is already started and worker count changed, log warning
			// (we don't support dynamic resizing, would require restart)
			m.workflowPoolMu.RLock()
			poolStarted := m.workflowPoolStarted
			m.workflowPoolMu.RUnlock()
			if poolStarted && oldWorkers != m.workflowPoolWorkers {
				m.logger.Warn("Workflow pool worker count changed but pool is already started",
					zap.Int("old_workers", oldWorkers),
					zap.Int("new_workers", m.workflowPoolWorkers),
					zap.String("note", "restart required to apply new worker count"))
			}
			
			// Also update semaphore for backward compatibility
			oldSemaphore := m.workflowSemaphore
			m.workflowSemaphore = make(chan struct{}, stateMngConfig.MaxConcurrentWorkflows)
			close(oldSemaphore) // Close old channel
			
			m.logger.Info("Max concurrent workflows configured",
				zap.Int("max_workflows", stateMngConfig.MaxConcurrentWorkflows),
			)
		}

		// Update frame capture error threshold if configured
		if stateMngConfig.FrameCaptureErrorThreshold > 0 {
			m.frameCaptureErrorThreshold = stateMngConfig.FrameCaptureErrorThreshold
			m.logger.Info("Frame capture error threshold configured",
				zap.Int("threshold", m.frameCaptureErrorThreshold),
			)
		}

		// Update state persistence retry configuration if configured
		if stateMngConfig.StatePersistenceMaxRetries >= 0 {
			m.statePersistenceMaxRetries = stateMngConfig.StatePersistenceMaxRetries
			m.logger.Info("State persistence max retries configured",
				zap.Int("max_retries", m.statePersistenceMaxRetries),
			)
		}
		if stateMngConfig.StatePersistenceRetryBackoff > 0 {
			m.statePersistenceRetryBackoff = stateMngConfig.StatePersistenceRetryBackoff
			m.logger.Info("State persistence retry backoff configured",
				zap.Duration("backoff", m.statePersistenceRetryBackoff),
			)
		}

		// Update serialize workflows setting (default: true)
		// Default to true if not explicitly set to false
		m.serializeWorkflows = true
		if !cfg.StateManager.SerializeWorkflows {
			// Explicitly set to false
			m.serializeWorkflows = false
		}
		m.logger.Info("Workflow serialization configured",
			zap.Bool("serialize_workflows", m.serializeWorkflows),
		)

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
	// No mutex needed - these are set during initialization before Start() is called
	m.edgeID = edgeID
}

// getOrCreateCameraStateMachine gets or creates a camera state machine for the given camera ID.
//
// Architecture: This method queries the iot.DeviceStateService (which owns device state machines)
// and wraps the result in a CameraStateMachineAdapter.
// StateManagerImpl does NOT own or create state machines directly - it only queries from services.
//
// Requires: deviceStateService must be set via SetDeviceStateService() before calling this method.
func (m *StateManagerImpl) getOrCreateCameraStateMachine(cameraID string) types.CameraStateMachine {
	// Check cache first
	m.cameraStateMachinesMu.RLock()
	if adapter, exists := m.cameraStateMachineAdapters[cameraID]; exists {
		m.cameraStateMachinesMu.RUnlock()
		return adapter
	}
	m.cameraStateMachinesMu.RUnlock()

	// Device state service is required
	if m.deviceStateService == nil {
		m.logger.Error("DeviceStateService is not set, cannot create camera state machine",
			zap.String("camera_id", cameraID),
		)
		return nil
	}

	// Get or create device state machine from iot service
	ctx := context.Background()
	deviceSM, err := m.deviceStateService.GetOrCreateStateMachine(ctx, cameraID, iot.DeviceTypeCamera)
	if err != nil {
		m.logger.Error("Failed to get/create device state machine",
			zap.String("camera_id", cameraID),
			zap.Error(err),
		)
		return nil
	}

	// Wrap device state machine in adapter
	adapter := iot.NewCameraStateMachineAdapter(deviceSM)

	// Cache the adapter
	m.cameraStateMachinesMu.Lock()
	defer m.cameraStateMachinesMu.Unlock()
	
	// Double-check after acquiring write lock
	if existing, exists := m.cameraStateMachineAdapters[cameraID]; exists {
		return existing
	}
	
	m.cameraStateMachineAdapters[cameraID] = adapter
	return adapter
}

// getCameraStateMachine gets a camera state machine for the given camera ID (returns nil if not found)
func (m *StateManagerImpl) getCameraStateMachine(cameraID string) types.CameraStateMachine {
	// Check cache first
	m.cameraStateMachinesMu.RLock()
	if adapter, exists := m.cameraStateMachineAdapters[cameraID]; exists {
		m.cameraStateMachinesMu.RUnlock()
		return adapter
	}
	m.cameraStateMachinesMu.RUnlock()

	// If device state service is available, try to get from there
	if m.deviceStateService != nil {
		deviceSM, err := m.deviceStateService.GetStateMachine(cameraID)
		if err == nil {
			// Wrap in adapter and cache
			adapter := iot.NewCameraStateMachineAdapter(deviceSM)
			m.cameraStateMachinesMu.Lock()
			m.cameraStateMachineAdapters[cameraID] = adapter
			m.cameraStateMachinesMu.Unlock()
			return adapter
		}
	}

	return nil
}

// getAllCameraStateMachines returns all camera state machines
func (m *StateManagerImpl) getAllCameraStateMachines() map[string]types.CameraStateMachine {
	// If device state service is available, get all camera state machines from there
	if m.deviceStateService != nil {
		allDeviceSMs := m.deviceStateService.GetStateMachinesByType(iot.DeviceTypeCamera)
		
		m.cameraStateMachinesMu.Lock()
		defer m.cameraStateMachinesMu.Unlock()
		
		// Ensure all device state machines are wrapped and cached
		result := make(map[string]types.CameraStateMachine)
		for _, deviceSM := range allDeviceSMs {
			cameraID := deviceSM.GetDeviceID()
			
			// Check if we already have an adapter cached
			if adapter, exists := m.cameraStateMachineAdapters[cameraID]; exists {
				result[cameraID] = adapter
			} else {
				// Create and cache new adapter
				adapter := iot.NewCameraStateMachineAdapter(deviceSM)
				m.cameraStateMachineAdapters[cameraID] = adapter
				result[cameraID] = adapter
			}
		}
		
		// Also include any cached adapters that might not be in the service yet
		for k, v := range m.cameraStateMachineAdapters {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}
		
		return result
	}

	// Fallback to cached adapters only
	m.cameraStateMachinesMu.RLock()
	defer m.cameraStateMachinesMu.RUnlock()

	// Return a copy to avoid external modification
	result := make(map[string]types.CameraStateMachine)
	for k, v := range m.cameraStateMachineAdapters {
		result[k] = v
	}
	return result
}

// updateOperationalState updates operational metrics
func (m *StateManagerImpl) updateOperationalState(updateFn func(*OperationalState)) {
	m.operationalMu.Lock()
	defer m.operationalMu.Unlock()
	updateFn(&m.operationalState)
	m.operationalState.LastUpdated = time.Now()
}

// getOperationalState returns a copy of the operational state
func (m *StateManagerImpl) getOperationalState() OperationalState {
	m.operationalMu.RLock()
	defer m.operationalMu.RUnlock()
	return m.operationalState
}

// Start begins processing events from the event bus.
func (m *StateManagerImpl) Start(ctx context.Context) error {
	var startErr error
	m.startOnce.Do(func() {
		// Try to restore state from meta-storage first
		if err := m.restoreStateFromStorage(ctx); err != nil {
			m.logger.Warn("Failed to restore state from storage, initializing with defaults",
				zap.Error(err),
			)
			// Initialize connection state to disconnected (will be updated by events)
			if m.vmGateway != nil {
				_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")
			}

			// Initialize operational state
			m.updateOperationalState(func(op *OperationalState) {
				op.StorageHealth = "healthy"
			})
		}

		var connectionState vmgatewaytypes.ConnectionState
		if m.vmGateway != nil {
			connectionState = m.vmGateway.GetConnectionState()
		} else {
			connectionState = vmgatewaytypes.ConnectionStateDisconnected
		}
		operationalState := m.getOperationalState()

		// Persist current state (either restored or initialized)
		m.persistStateToStorage(ctx, connectionState, nil)

		m.logger.Info("Edge state initialized",
			zap.String("connection_state", string(connectionState)),
			zap.Int("cameras_enabled", operationalState.CamerasEnabled),
			zap.String("storage_health", operationalState.StorageHealth),
			zap.Int("camera_state_machines", len(m.getAllCameraStateMachines())),
		)

		// Check health of all services
		m.checkServicesHealth(ctx)

		m.logger.Info("Starting edge state manager, subscribing to all events")

		// Start workflow worker pool
		m.startWorkflowPool()

		events := m.eventBus.SubscribeAll()

		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.run(events)
		}()

		// Start capability sync loop if VM gateway is available
		// Use m.ctx (service lifetime) instead of Start(ctx) for long-lived goroutines
		if m.vmGateway != nil && m.cctvService != nil {
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				m.capabilitySyncLoop(m.ctx)
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
// It ensures proper shutdown ordering by stopping frame processing before canceling the main context.
func (m *StateManagerImpl) Stop(ctx context.Context) error {
	var stopErr error
	m.stopOnce.Do(func() {
		m.logger.Info("Stopping edge state manager")

		// Stop frame processing first to ensure proper shutdown ordering
		// This waits for all frame processing goroutines to finish before proceeding
		m.stopAllFrameProcessing()

		// Stop workflow worker pool first
		m.stopWorkflowPool()

		// Close workflow queues to stop workers
		m.workflowQueuesMu.Lock()
		for source, queue := range m.workflowQueues {
			close(queue)
			m.logger.Debug("Closed workflow queue",
				zap.String("source", source))
		}
		m.workflowQueues = make(map[string]chan *WorkflowTask)
		m.workflowQueuesMu.Unlock()

		// Now cancel the main context to signal other goroutines to stop
		m.cancel()
		
		// Wait for workflow queue workers to finish
		m.workflowWorkersWg.Wait()

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
		zap.Int64("sequence_number", ev.SequenceNumber),
	)

	// Check for duplicate event (idempotency)
	if m.isDuplicateEvent(ev) {
		m.logger.Debug("Duplicate event detected, skipping",
			zap.String("event_type", string(ev.Type)),
			zap.String("source", ev.Source),
			zap.Int64("sequence_number", ev.SequenceNumber))
		return
	}

	// Mark event as processed
	m.markEventProcessed(ev)

	// Update state based on event
	connectionState, cameraStates, err := m.updateStateForEvent(ev)
	if err != nil {
		m.logger.Warn("edge-state-manager: failed to update state for event",
			zap.String("event_type", string(ev.Type)),
			zap.Error(err),
		)
		return
	}

	// Create workflow task
	task := &WorkflowTask{
		Event:           ev,
		ConnectionState: connectionState,
		CameraStates:    cameraStates,
	}

	// Execute workflow based on serialization mode
	if m.serializeWorkflows && ev.SequenceNumber > 0 {
		// Serialized execution per source (respects event ordering)
		m.queueWorkflow(ev.Source, task)
	} else {
		// Concurrent execution via worker pool (backward compatible for events without sequence numbers)
		m.enqueueWorkflowToPool(task)
	}
}

// enqueueWorkflowToPool enqueues a workflow task to the global worker pool
func (m *StateManagerImpl) enqueueWorkflowToPool(task *WorkflowTask) {
	select {
	case m.workflowPoolQueue <- task:
		m.logger.Debug("Workflow enqueued to worker pool",
			zap.String("event_type", string(task.Event.Type)),
			zap.String("source", task.Event.Source))
	default:
		// Queue full - log warning and drop (or could use fallback)
		m.logger.Warn("Workflow pool queue full, dropping workflow",
			zap.String("event_type", string(task.Event.Type)),
			zap.String("source", task.Event.Source))
	}
}

// queueWorkflow queues a workflow task for serialized execution per source
func (m *StateManagerImpl) queueWorkflow(source string, task *WorkflowTask) {
	// Get or create workflow queue for this source
	queue := m.getOrCreateWorkflowQueue(source)
	
	// Queue the task (non-blocking, drop if queue is full)
	select {
	case queue <- task:
		m.logger.Debug("Workflow queued for serialized execution",
			zap.String("source", source),
			zap.String("event_type", string(task.Event.Type)),
			zap.Int64("sequence_number", task.Event.SequenceNumber))
		default:
			// Queue full - log warning and enqueue to worker pool (fallback)
			m.logger.Warn("Workflow queue full, enqueuing to worker pool",
				zap.String("source", source),
				zap.String("event_type", string(task.Event.Type)),
				zap.Int64("sequence_number", task.Event.SequenceNumber))
			m.enqueueWorkflowToPool(task)
		}
}

// getOrCreateWorkflowQueue gets or creates a workflow queue for a source
func (m *StateManagerImpl) getOrCreateWorkflowQueue(source string) chan *WorkflowTask {
	m.workflowQueuesMu.RLock()
	queue, exists := m.workflowQueues[source]
	m.workflowQueuesMu.RUnlock()
	
	if exists {
		return queue
	}
	
	m.workflowQueuesMu.Lock()
	defer m.workflowQueuesMu.Unlock()
	
	// Double-check after acquiring write lock
	if queue, exists := m.workflowQueues[source]; exists {
		return queue
	}
	
	// Create new queue and start worker
	queue = make(chan *WorkflowTask, 100) // Buffer size: 100
	m.workflowQueues[source] = queue
	
	// Start worker for this source
	m.workflowWorkersWg.Add(1)
	go m.workflowQueueWorker(source, queue)
	
	m.logger.Info("Created workflow queue for source",
		zap.String("source", source))
	
	return queue
}

// workflowQueueWorker processes workflows from a queue for a specific source
// This ensures workflows execute sequentially per source, respecting event ordering
func (m *StateManagerImpl) workflowQueueWorker(source string, queue chan *WorkflowTask) {
	defer m.workflowWorkersWg.Done()
	
	m.logger.Debug("Workflow queue worker started",
		zap.String("source", source))
	
	for {
		select {
		case <-m.ctx.Done():
			m.logger.Debug("Workflow queue worker stopping",
				zap.String("source", source))
			return
		case task, ok := <-queue:
			if !ok {
				// Queue closed
				m.logger.Debug("Workflow queue closed",
					zap.String("source", source))
				return
			}
			
			// Execute workflow sequentially (respects event ordering)
			m.logger.Debug("Executing workflow from queue",
				zap.String("source", source),
				zap.String("event_type", string(task.Event.Type)),
				zap.Int64("sequence_number", task.Event.SequenceNumber))
			
			// Enqueue to worker pool for execution (respects global concurrency limit)
			m.enqueueWorkflowToPool(task)
		}
	}
}

// startWorkflowPool starts the workflow worker pool
func (m *StateManagerImpl) startWorkflowPool() {
	m.workflowPoolMu.Lock()
	defer m.workflowPoolMu.Unlock()
	
	if m.workflowPoolStarted {
		return
	}
	
	m.logger.Info("Starting workflow worker pool",
		zap.Int("workers", m.workflowPoolWorkers),
		zap.Int("queue_size", cap(m.workflowPoolQueue)))
	
	// Start worker goroutines
	for i := 0; i < m.workflowPoolWorkers; i++ {
		m.workflowPoolWg.Add(1)
		go m.workflowPoolWorker(i)
	}
	
	m.workflowPoolStarted = true
}

// stopWorkflowPool stops the workflow worker pool
func (m *StateManagerImpl) stopWorkflowPool() {
	m.workflowPoolMu.Lock()
	defer m.workflowPoolMu.Unlock()
	
	if !m.workflowPoolStarted {
		return
	}
	
	m.logger.Info("Stopping workflow worker pool")
	
	// Close the queue to signal workers to stop
	close(m.workflowPoolQueue)
	
	// Wait for all workers to finish
	done := make(chan struct{})
	go func() {
		m.workflowPoolWg.Wait()
		close(done)
	}()
	
	// Wait with timeout
	select {
	case <-done:
		m.logger.Info("Workflow worker pool stopped")
	case <-time.After(30 * time.Second):
		m.logger.Warn("Workflow worker pool stop timeout")
	}
	
	m.workflowPoolStarted = false
	// Recreate queue for potential restart (though we don't support restart)
	m.workflowPoolQueue = make(chan *WorkflowTask, 1000)
}

// isDuplicateEvent checks if an event has already been processed (idempotency)
func (m *StateManagerImpl) isDuplicateEvent(ev eventbustypes.Event) bool {
	// Generate event key for deduplication
	eventKey := m.generateEventKey(ev)
	
	m.processedEventsMu.RLock()
	defer m.processedEventsMu.RUnlock()
	
	// Check if event was processed recently
	if processedTime, exists := m.processedEvents[eventKey]; exists {
		// Check if within deduplication window
		if time.Since(processedTime) < m.eventDedupWindow {
			return true
		}
		// Event is outside window, consider it new
	}
	
	return false
}

// markEventProcessed marks an event as processed for deduplication
func (m *StateManagerImpl) markEventProcessed(ev eventbustypes.Event) {
	eventKey := m.generateEventKey(ev)
	
	m.processedEventsMu.Lock()
	defer m.processedEventsMu.Unlock()
	
	m.processedEvents[eventKey] = time.Now()
	
	// Cleanup old events periodically (every 1000 events to avoid overhead)
	if len(m.processedEvents) > 1000 {
		m.cleanupOldProcessedEvents()
	}
}

// generateEventKey generates a unique key for event deduplication
func (m *StateManagerImpl) generateEventKey(ev eventbustypes.Event) string {
	// Use event type, source, sequence number, and key data fields for uniqueness
	key := fmt.Sprintf("%s:%s:%d", ev.Type, ev.Source, ev.SequenceNumber)
	
	// Add camera_id if present (for camera-specific events)
	if cameraID, ok := ev.Data["camera_id"].(string); ok && cameraID != "" {
		key += ":" + cameraID
	}
	
	// Add model_id if present (for model deployment events)
	if modelID, ok := ev.Data["model_id"].(string); ok && modelID != "" {
		key += ":" + modelID
	}
	
	// Add event_id if present (for events with explicit IDs)
	if eventID, ok := ev.Data["event_id"].(string); ok && eventID != "" {
		key += ":" + eventID
	}
	
	return key
}

// cleanupOldProcessedEvents removes events outside the deduplication window
func (m *StateManagerImpl) cleanupOldProcessedEvents() {
	now := time.Now()
	for key, processedTime := range m.processedEvents {
		if now.Sub(processedTime) > m.eventDedupWindow {
			delete(m.processedEvents, key)
		}
	}
}

// workflowPoolWorker is a worker in the workflow pool that processes tasks from the global queue
func (m *StateManagerImpl) workflowPoolWorker(workerID int) {
	defer m.workflowPoolWg.Done()
	
	m.logger.Debug("Workflow pool worker started",
		zap.Int("worker_id", workerID))
	
	for {
		select {
		case <-m.ctx.Done():
			m.logger.Debug("Workflow pool worker stopping",
				zap.Int("worker_id", workerID))
			return
		case task, ok := <-m.workflowPoolQueue:
			if !ok {
				// Queue closed
				m.logger.Debug("Workflow pool queue closed, worker stopping",
					zap.Int("worker_id", workerID))
				return
			}
			
			// Execute workflow
			m.logger.Debug("Workflow pool worker executing workflow",
				zap.Int("worker_id", workerID),
				zap.String("event_type", string(task.Event.Type)),
				zap.String("source", task.Event.Source),
				zap.Int64("sequence_number", task.Event.SequenceNumber))
			
			m.executeWorkflow(m.ctx, task.Event, task.ConnectionState, task.CameraStates)
		}
	}
}

// getConnectionState returns the current connection state from vm-gateway service.
//
// Architecture: StateManagerImpl does NOT own the connection state machine.
// Connection state is owned and managed by the vm-gateway service.
// This method queries the state from the service, it does not manage it.
func (m *StateManagerImpl) getConnectionState() vmgatewaytypes.ConnectionState {
	if m.vmGateway != nil {
		return m.vmGateway.GetConnectionState()
	}
	return vmgatewaytypes.ConnectionStateDisconnected
}

// updateStateForEvent updates the connection and camera state machines based on the event.
// Returns the updated connection state and a map of updated camera states.
func (m *StateManagerImpl) updateStateForEvent(ev eventbustypes.Event) (vmgatewaytypes.ConnectionState, map[string]types.CameraState, error) {
	var oldConnectionState vmgatewaytypes.ConnectionState
	if m.vmGateway != nil {
		oldConnectionState = m.vmGateway.GetConnectionState()
	} else {
		oldConnectionState = vmgatewaytypes.ConnectionStateDisconnected
	}
	updatedCameraStates := make(map[string]types.CameraState)

	// Update state based on event type
	switch ev.Type {
	// Network events - update connection state machine (via vm-gateway)
	case EventTypeTunnelConnected:
		if m.vmGateway != nil {
			currentState := m.vmGateway.GetConnectionState()
			if currentState == vmgatewaytypes.ConnectionStateDisconnected {
				if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateWGConnecting, ""); err != nil {
					m.logger.Warn("Failed to transition to wg_connecting", zap.Error(err))
				} else {
					if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateWireGuardConnected, ""); err != nil {
						m.logger.Warn("Failed to transition to wireguard_connected", zap.Error(err))
					}
				}
			}
		}
	case EventTypeTunnelDisconnected:
		// Note: Frame processing continues even when disconnected - Edge should monitor security zone independently
		// Security events will be queued and synced when connection is restored
		if m.vmGateway != nil {
			if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, ""); err != nil {
				m.logger.Warn("Failed to transition to disconnected", zap.Error(err))
			}
		}
	case EventTypeTransportConnected:
		if m.vmGateway != nil {
			currentState := m.vmGateway.GetConnectionState()
			if currentState == vmgatewaytypes.ConnectionStateWireGuardConnected {
				if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateHTTPConnecting, ""); err != nil {
					m.logger.Warn("Failed to transition to http_connecting", zap.Error(err))
				} else {
					if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateHTTPSConnected, ""); err != nil {
						m.logger.Warn("Failed to transition to https_connected", zap.Error(err))
					}
				}
			}
		}
	case EventTypeTransportDisconnected:
		if m.vmGateway != nil {
			currentState := m.vmGateway.GetConnectionState()
			if currentState == vmgatewaytypes.ConnectionStateHTTPSConnected || currentState == vmgatewaytypes.ConnectionStateAuthenticated {
				// Note: Frame processing continues even when HTTPS disconnects - Edge should monitor security zone independently
				// Security events will be queued and synced when connection is restored
				if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateWireGuardConnected, ""); err != nil {
					m.logger.Warn("Failed to transition to wireguard_connected", zap.Error(err))
				}
			}
		}
	case EventTypeEdgeAuthenticated:
		if m.vmGateway != nil {
			currentState := m.vmGateway.GetConnectionState()
			if currentState == vmgatewaytypes.ConnectionStateHTTPSConnected {
				if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateAuthenticated, ""); err != nil {
					m.logger.Warn("Failed to transition to authenticated", zap.Error(err))
				}
			}
		}
	case EventTypeCapabilitiesReceived:
		if m.vmGateway != nil {
			currentState := m.vmGateway.GetConnectionState()
			if currentState == vmgatewaytypes.ConnectionStateAuthenticated || currentState == vmgatewaytypes.ConnectionStateHTTPSConnected {
				if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateCapabilitiesReceived, ""); err != nil {
					m.logger.Warn("Failed to transition to capabilities_received", zap.Error(err))
				}
			}
		}

	// Camera events - update camera state machines
	case EventTypeDeviceDiscovered:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			cameraSM := m.getOrCreateCameraStateMachine(cameraID)
			if cameraSM == nil {
				m.logger.Error("Failed to get/create camera state machine",
					zap.String("camera_id", cameraID),
				)
				return oldConnectionState, updatedCameraStates, nil
			}
			currentState := cameraSM.GetState()
			if currentState == types.CameraStateUndiscovered {
				if err := cameraSM.Transition(types.CameraStateDiscovered, ""); err != nil {
					m.logger.Warn("Failed to transition camera to discovered", zap.String("camera_id", cameraID), zap.Error(err))
				} else {
					updatedCameraStates[cameraID] = cameraSM.GetState()
				}
			}
		}
	case EventTypeDeviceRegistered:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Info("Camera registered", zap.String("camera_id", cameraID))
			// State updated via workflow
		}
	case EventTypeDeviceConnected:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Info("Camera connected", zap.String("camera_id", cameraID))
			// State updated via workflow
		}
	case EventTypeDeviceDisconnected:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			cameraSM := m.getCameraStateMachine(cameraID)
			if cameraSM != nil {
				if err := cameraSM.Transition(types.CameraStateDisconnected, ""); err != nil {
					m.logger.Warn("Failed to transition camera to disconnected", zap.String("camera_id", cameraID), zap.Error(err))
				} else {
					updatedCameraStates[cameraID] = cameraSM.GetState()
				}
			}
			m.logger.Info("Camera disconnected", zap.String("camera_id", cameraID))
		}

	// Snapshot request events - update camera state machines
	case EventTypeDataUnitRequested:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			cameraSM := m.getOrCreateCameraStateMachine(cameraID)
			if cameraSM == nil {
				m.logger.Error("Failed to get/create camera state machine",
					zap.String("camera_id", cameraID),
				)
				return oldConnectionState, updatedCameraStates, nil
			}
			currentState := cameraSM.GetState()
			// Can transition to waiting_for_screenshots from synced or later states
			if currentState == types.CameraStateSynced || currentState == types.CameraStateWaitingForScreenshots {
				if err := cameraSM.Transition(types.CameraStateWaitingForScreenshots, ""); err != nil {
					m.logger.Warn("Failed to transition camera to waiting_for_screenshots", zap.String("camera_id", cameraID), zap.Error(err))
				} else {
					updatedCameraStates[cameraID] = cameraSM.GetState()
				}
			}
		}
	case EventTypeDataUnitSetReady:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			cameraSM := m.getOrCreateCameraStateMachine(cameraID)
			if cameraSM == nil {
				m.logger.Error("Failed to get/create camera state machine",
					zap.String("camera_id", cameraID),
				)
				return oldConnectionState, updatedCameraStates, nil
			}
			currentState := cameraSM.GetState()
			if currentState == types.CameraStateWaitingForScreenshots {
				if err := cameraSM.Transition(types.CameraStateScreenshotSetReady, ""); err != nil {
					m.logger.Warn("Failed to transition camera to screenshot_set_ready", zap.String("camera_id", cameraID), zap.Error(err))
				} else {
					updatedCameraStates[cameraID] = cameraSM.GetState()
				}
			}
		}

	// AI events
	case EventTypeDetection:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Debug("AI detection event", zap.String("camera_id", cameraID))
			// State updated via workflow
		}
	case EventTypeInference:
		// Track AI processing activity in operational state
		m.updateOperationalState(func(op *OperationalState) {
			op.AIProcessingActive = true
		})

	// Storage events - update operational state
	case EventTypeStorageWarning:
		m.updateOperationalState(func(op *OperationalState) {
			op.StorageHealth = "warning"
		})
	case EventTypeStorageFull:
		m.updateOperationalState(func(op *OperationalState) {
			op.StorageHealth = "full"
		})
		m.logger.Warn("Storage full, may need cleanup")

	// Video events
	case EventTypeRawDeviceDataFrameReceived:
		// Track frame processing
	case EventTypeRawDeviceDataClipRecorded:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Debug("Clip recorded", zap.String("camera_id", cameraID))
		}

	// Screenshot events
	case EventTypeDataUnitSaved:
		if cameraID, ok := ev.Data["camera_id"].(string); ok {
			m.logger.Debug("Screenshot saved", zap.String("camera_id", cameraID))
		}
	}

	// Get current connection state
	var newConnectionState vmgatewaytypes.ConnectionState
	if m.vmGateway != nil {
		newConnectionState = m.vmGateway.GetConnectionState()
	} else {
		newConnectionState = vmgatewaytypes.ConnectionStateDisconnected
	}

	// Log state transitions
	if oldConnectionState != newConnectionState {
		m.logger.Info("edge-state-manager: connection state transition",
			zap.String("old_state", string(oldConnectionState)),
			zap.String("new_state", string(newConnectionState)),
			zap.String("event_type", string(ev.Type)),
		)
	}

	// Log camera state transitions
	for cameraID, newState := range updatedCameraStates {
		m.logger.Info("edge-state-manager: camera state transition",
			zap.String("camera_id", cameraID),
			zap.String("new_state", string(newState)),
			zap.String("event_type", string(ev.Type)),
		)
	}

	// Persist state to meta-storage with timeout (use service context with timeout)
	timeout := 5 * time.Second
	if m.config != nil && m.config.StateManager.StatePersistenceTimeout > 0 {
		timeout = m.config.StateManager.StatePersistenceTimeout
	}
	persistCtx, cancel := context.WithTimeout(m.ctx, timeout)
	defer cancel()
	if err := m.persistStateToStorage(persistCtx, newConnectionState, updatedCameraStates); err != nil {
		m.logger.Warn("Failed to persist state to storage",
			zap.Error(err),
			zap.String("connection_state", string(newConnectionState)),
		)
	}

	return newConnectionState, updatedCameraStates, nil
}

// executeWorkflow determines and executes the workflow based on the event and current states.
func (m *StateManagerImpl) executeWorkflow(ctx context.Context, ev eventbustypes.Event, connectionState vmgatewaytypes.ConnectionState, cameraStates map[string]types.CameraState) {
	switch ev.Type {
	// Network workflow
	case EventTypeTunnelConnected:
		m.logger.Info("WireGuard connected, waiting for HTTPS connection")
		// Workflow: WireGuard -> HTTPS -> Auth -> Capabilities -> Camera Discovery -> Camera Sync -> Screenshot Set Ready

	case EventTypeTransportConnected:
		m.logger.Info("HTTPS connected, initiating authentication")
		// Workflow: Trigger authentication if needed

	case EventTypeEdgeAuthenticated:
		m.logger.Info("Edge authenticated, initializing services")
		// Workflow: After authentication, ensure cameras are discovered and AI is ready
		m.initializeServicesAfterAuth(ctx)
		// Sync pending security events that were queued during disconnection
		// This ensures all security events detected during disconnection are transmitted to VM
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.syncPendingSecurityEvents(m.ctx)
		}()

	case EventTypeCapabilitiesReceived:
		m.logger.Info("Capabilities received from VM, checking for camera capability")
		// Workflow: Check capabilities and initiate camera discovery if CCTV capability is present
		m.handleCapabilitiesReceived(ctx, ev)

	// Camera workflow
	case EventTypeDeviceDiscovered:
		m.logger.Info("Cameras discovered, updating state")
		m.handleCameraDiscovered(ctx, ev)

	// Snapshot request workflow (VM asks Edge/user for labeled screenshots)
	case EventTypeDataUnitRequested:
		m.logger.Info("Snapshot request received from VM, saving to meta-storage and updating state")
		m.handleSnapshotRequested(ctx, ev)

	case EventTypeDataUnitSetReady:
		m.logger.Info("Screenshot set marked as ready by user")
		m.handleScreenshotSetReady(ctx, ev)

	case EventTypeDeviceRegistered:
		m.handleCameraRegistered(ctx, ev)
	case EventTypeDeviceConnected:
		m.handleCameraConnected(ctx, ev)

	// AI workflow
	case EventTypeDetection:
		m.handleAIDetection(ctx, ev)
	case EventTypeInference:
		// Track inference activity in operational state
		m.updateOperationalState(func(op *OperationalState) {
			op.AIProcessingActive = true
		})

	// Storage workflow
	case EventTypeStorageWarning:
		m.handleStorageWarning(ctx, ev)
	case EventTypeStorageFull:
		m.handleStorageFull(ctx, ev)

	// Video workflow
	case EventTypeRawDeviceDataFrameReceived:
		// Frames are handled by AI gateway automatically
	case EventTypeRawDeviceDataClipRecorded:
		m.handleClipRecorded(ctx, ev)

	// Screenshot workflow
	case EventTypeDataUnitSaved:
		m.handleScreenshotSaved(ctx, ev)

	// Model deployment workflow
	case EventTypeModelDeployed:
		m.handleModelDeployed(ctx, ev)
	case EventTypeModelDeploymentStatus:
		m.handleModelDeploymentStatus(ctx, ev)
	default:
		// Unhandled event - log for observability
		m.logger.Warn("Unhandled event type in workflow execution",
			zap.String("event_type", string(ev.Type)),
			zap.String("source", ev.Source),
			zap.Int64("sequence_number", ev.SequenceNumber),
			zap.Time("timestamp", ev.Timestamp),
		)
	}

	// General state-based workflows - check connection state
	switch connectionState {
	case vmgatewaytypes.ConnectionStateCapabilitiesReceived:
		// After capabilities received, check for cameras that need workflows
		// Use all camera state machines, not just the delta from the event
		// This ensures workflows run even when only connection state changes
		allCameraSMs := m.getAllCameraStateMachines()
		for cameraID, cameraSM := range allCameraSMs {
			cameraState := cameraSM.GetState()
			switch cameraState {
			case types.CameraStateScreenshotSetReady:
				m.executeScreenshotSetReadyWorkflowForCamera(ctx, cameraID)
			case types.CameraStateModelDeployed:
				m.executeModelDeployedWorkflowForCamera(ctx, cameraID)
			case types.CameraStateFrameProcessing:
				m.executeFrameProcessingWorkflowForCamera(ctx, cameraID)
			}
		}
	case vmgatewaytypes.ConnectionStateError:
		m.handleConnectionErrorState(ctx, connectionState)
	}

	// Check all camera states for workflows
	for cameraID := range cameraStates {
		cameraSM := m.getCameraStateMachine(cameraID)
		if cameraSM == nil {
			continue
		}
		cameraState := cameraSM.GetState()
		switch cameraState {
		case types.CameraStateScreenshotSetReady:
			m.executeScreenshotSetReadyWorkflowForCamera(ctx, cameraID)
		case types.CameraStateModelDeployed:
			m.executeModelDeployedWorkflowForCamera(ctx, cameraID)
		case types.CameraStateFrameProcessing:
			m.executeFrameProcessingWorkflowForCamera(ctx, cameraID)
		case types.CameraStateError:
			m.handleCameraErrorState(ctx, cameraID, cameraState)
		}
	}
}

// initializeServicesAfterAuth initializes services after authentication
func (m *StateManagerImpl) initializeServicesAfterAuth(ctx context.Context) {
	// Idempotency check: only initialize once
	m.servicesInitMu.Lock()
	if m.servicesInitialized {
		m.servicesInitMu.Unlock()
		m.logger.Debug("Services already initialized after auth, skipping")
		return
	}
	m.servicesInitialized = true
	m.servicesInitMu.Unlock()

	m.logger.Info("Initializing services after authentication")

	// Trigger camera discovery if CCTV service is available
	if m.cctvService != nil {
		// Type assertion would be done here if we had the interface
		// For now, we'll publish an event to trigger discovery
		m.eventBus.Publish(eventbustypes.Event{
			Type:      eventbustypes.EventTypeWorkflowDeviceDiscover,
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
				// Update connection state to error
				if m.vmGateway != nil {
					_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, fmt.Sprintf("CCTV service error: %v", err))
				}
				connectionState := m.getConnectionState()
				m.persistStateToStorage(ctx, connectionState, nil)
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

	// Get camera state machine
	cameraSM := m.getOrCreateCameraStateMachine(cameraID)
	if cameraSM == nil {
		m.logger.Error("Failed to get/create camera state machine for camera discovery",
			zap.String("camera_id", cameraID),
		)
		return
	}
	currentCameraState := cameraSM.GetState()

	// Check connection state - need to be authenticated to sync
	connectionState := m.getConnectionState()
	if connectionState != vmgatewaytypes.ConnectionStateCapabilitiesReceived {
		m.logger.Debug("Connection not ready for camera sync",
			zap.String("camera_id", cameraID),
			zap.String("connection_state", string(connectionState)),
		)
		return
	}

	// If camera is in discovered state, sync with VM
	if currentCameraState == types.CameraStateDiscovered {
		m.logger.Info("Camera is in discovered state, syncing with VM",
			zap.String("camera_id", cameraID),
		)
		m.syncCamerasWithVM(ctx)
	} else {
		m.logger.Debug("Camera is not in discovered state, skipping sync",
			zap.String("camera_id", cameraID),
			zap.String("current_state", string(currentCameraState)),
		)
	}
}

// handleSnapshotRequested handles snapshot capture requests from VM.
// VM is effectively asking this Edge (and its user) to take labeled screenshots from a camera.
// We persist this request in meta-storage (via state-mng helpers) and move state to
// waiting_for_camera_screenshots so that UI can show pending actions.
// This function is idempotent: it checks if a request already exists before creating a new one.
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

	// Idempotency check: check if request already exists
	if existing, err := m.GetPendingSnapshotRequest(ctx, cameraID); err == nil && existing != nil {
		m.logger.Debug("Pending snapshot request already exists, skipping duplicate",
			zap.String("camera_id", cameraID),
			zap.String("existing_label", existing.Label),
			zap.String("existing_custom_label", existing.CustomLabel))
		// Request already exists, workflow is idempotent
		return
	}

	// Persist pending snapshot request metadata via state-manager helper
	if err := m.SavePendingSnapshotRequest(ctx, cameraID, label, customLabel, count); err != nil {
		m.logger.Error("Failed to save pending snapshot request",
			zap.String("camera_id", cameraID),
			zap.Error(err),
		)
		return
	}

	// Update camera state to waiting_for_screenshots (state transition handled in updateStateForEvent)
	// This handler just ensures the workflow is executed
	m.logger.Info("Snapshot request workflow executed",
		zap.String("camera_id", cameraID),
	)
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

	// Update camera state to screenshot_set_ready (state transition handled in updateStateForEvent)
	// This handler just ensures the workflow is executed
	m.logger.Info("Screenshot set ready workflow executed",
		zap.String("camera_id", cameraID),
	)

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

		// Check if there's a pending model deployment for this camera
		m.pendingModelDeployMu.Lock()
		pendingDeployment, hasPending := m.pendingModelDeployments[cameraID]
		if hasPending {
			delete(m.pendingModelDeployments, cameraID)
			m.pendingModelDeployMu.Unlock()
			// Process the pending model deployment
			m.logger.Info("Processing pending model deployment",
				zap.String("camera_id", cameraID),
				zap.String("model_id", pendingDeployment.ModelID),
			)
			// Create a synthetic event to trigger model deployment
			ev := eventbustypes.Event{
				Type:   EventTypeModelDeployed,
				Source: "state-manager",
				Data:   pendingDeployment.EventData,
			}
			m.handleModelDeployed(ctx, ev)
		} else {
			m.pendingModelDeployMu.Unlock()
		}
	}
}

// syncScreenshotsToVM syncs labeled screenshots to VM for model training
// This function is idempotent: it checks if screenshots were recently synced to avoid duplicate syncs
func (m *StateManagerImpl) syncScreenshotsToVM(ctx context.Context, cameraID string) {
	// Idempotency check: avoid syncing too frequently (within 5 minutes)
	m.screenshotSyncMu.RLock()
	lastSync, recentlySynced := m.lastScreenshotSync[cameraID]
	m.screenshotSyncMu.RUnlock()
	
	if recentlySynced && time.Since(lastSync) < 5*time.Minute {
		m.logger.Debug("Screenshots recently synced, skipping duplicate sync",
			zap.String("camera_id", cameraID),
			zap.Duration("time_since_sync", time.Since(lastSync)))
		return
	}

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

	// Filter to only labeled data units and fetch raw data
	// Note: Currently screenshots (images) from cameras, but device-agnostic for future IoT support
	labeledDataUnits := make([]*vmgatewaytypes.DataUnitInfo, 0)
	for _, ss := range screenshots {
		if ss.Label == "" {
			continue // Skip unlabeled data units
		}

		// Convert to DataUnitInfo format (device-agnostic)
		dataUnitInfo := &vmgatewaytypes.DataUnitInfo{
			DataUnitID:   ss.ID,
			DeviceID:     ss.CameraID, // Map CameraID to DeviceID for device-agnostic API
			ObjectKey:    ss.ObjectKey, // Path to data in object storage
			Label:        ss.Label,
			CustomLabel:  ss.CustomLabel,
			Description:  ss.Description,
			CreatedAt:    ss.CreatedAt.Unix(),
		}

		// Add metadata if available
		if ss.Metadata != nil {
			dataUnitInfo.Metadata = ss.Metadata
		}

		// Fetch raw data from object storage (currently images, but device-agnostic)
		if ss.ObjectKey != "" && m.objectStorage != nil {
			reader, err := m.objectStorage.LoadSnapshot(ctx, ss.ObjectKey)
			if err != nil {
				m.logger.Warn("Failed to load data unit from object storage",
					zap.String("data_unit_id", ss.ID),
					zap.String("object_key", ss.ObjectKey),
					zap.Error(err),
				)
			} else {
				// Read raw data
				rawData, err := io.ReadAll(reader)
				reader.Close()
				if err != nil {
					m.logger.Warn("Failed to read data unit raw data",
						zap.String("data_unit_id", ss.ID),
						zap.Error(err),
					)
				} else {
					// Encode as base64
					dataUnitInfo.RawData = base64.StdEncoding.EncodeToString(rawData)

					// Determine data format from object key extension (device-agnostic)
					ext := strings.ToLower(filepath.Ext(ss.ObjectKey))
					switch ext {
					case ".jpg", ".jpeg":
						dataUnitInfo.RawDataFormat = "jpeg"
					case ".png":
						dataUnitInfo.RawDataFormat = "png"
					case ".gif":
						dataUnitInfo.RawDataFormat = "gif"
					case ".webp":
						dataUnitInfo.RawDataFormat = "webp"
					case ".json":
						dataUnitInfo.RawDataFormat = "json"
					case ".wav":
						dataUnitInfo.RawDataFormat = "wav"
					default:
						dataUnitInfo.RawDataFormat = "jpeg" // Default to JPEG for cameras
					}
				}
			}
		}

		labeledDataUnits = append(labeledDataUnits, dataUnitInfo)
	}

	if len(labeledDataUnits) == 0 {
		m.logger.Warn("No labeled data units found for device",
			zap.String("device_id", cameraID),
		)
		return
	}

	m.logger.Info("Prepared labeled data units for sync",
		zap.String("device_id", cameraID),
		zap.Int("count", len(labeledDataUnits)),
	)

	// Get edge ID from state manager
	edgeID := m.edgeID
	if edgeID == "" {
		m.logger.Warn("Edge ID not available, cannot sync data units")
		return
	}

	// Batch data units to avoid loading all into memory at once
	// Process in batches of 20 data units
	batchSize := 20
	for i := 0; i < len(labeledDataUnits); i += batchSize {
		end := i + batchSize
		if end > len(labeledDataUnits) {
			end = len(labeledDataUnits)
		}
		batch := labeledDataUnits[i:end]

		// Create sync request for this batch (device-agnostic)
		req := &vmgatewaytypes.SyncDataUnitsRequest{
			EdgeID:    edgeID,
			DeviceID:  cameraID, // Device that produced the data units
			DataUnits: batch,
		}

		// Call VM gateway to sync data units batch
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		resp, err := m.vmGateway.SyncDataUnits(callCtx, req)
		cancel()

		if err != nil {
			m.logger.Error("Failed to sync data unit batch to VM",
				zap.String("device_id", cameraID),
				zap.Int("batch_start", i),
				zap.Int("batch_size", len(batch)),
				zap.Error(err),
			)
			// On error, set connection state to error
			if m.vmGateway != nil {
				_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, fmt.Sprintf("VM gateway error: %v", err))
			}
			connectionState := m.getConnectionState()
			timeout := 5 * time.Second
			if m.config != nil && m.config.StateManager.StatePersistenceTimeout > 0 {
				timeout = m.config.StateManager.StatePersistenceTimeout
			}
			persistCtx, cancelPersist := context.WithTimeout(ctx, timeout)
			_ = m.persistStateToStorage(persistCtx, connectionState, nil)
			cancelPersist()
			return
		}

		if !resp.Success {
			m.logger.Error("VM rejected data unit batch sync",
				zap.String("camera_id", cameraID),
				zap.Int("batch_start", i),
				zap.Int("batch_size", len(batch)),
				zap.String("error", resp.ErrorMessage),
			)
			// On error, set connection state to error
			if m.vmGateway != nil {
				_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, fmt.Sprintf("VM gateway error: %s", resp.ErrorMessage))
			}
			connectionState := m.getConnectionState()
			timeout := 5 * time.Second
			if m.config != nil && m.config.StateManager.StatePersistenceTimeout > 0 {
				timeout = m.config.StateManager.StatePersistenceTimeout
			}
			persistCtx, cancelPersist := context.WithTimeout(ctx, timeout)
			_ = m.persistStateToStorage(persistCtx, connectionState, nil)
			cancelPersist()
			return
		}

		m.logger.Info("Data unit batch synced to VM successfully",
			zap.String("device_id", cameraID),
			zap.Int("batch_start", i),
			zap.Int("batch_size", len(batch)),
		)
	}

	// Mark sync as complete (idempotency tracking)
	m.screenshotSyncMu.Lock()
	m.lastScreenshotSync[cameraID] = time.Now()
	m.screenshotSyncMu.Unlock()

	m.logger.Info("All data unit batches synced to VM successfully",
		zap.String("device_id", cameraID),
		zap.Int("total_count", len(labeledDataUnits)),
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
	// Build device-agnostic device info list (currently cameras, but supports other IoT devices)
	deviceInfos := make([]*vmgatewaytypes.DeviceInfo, 0, len(cameras))
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

		deviceInfos = append(deviceInfos, &vmgatewaytypes.DeviceInfo{
			DeviceID: cam.ID,
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

	// Sync devices with VM (device-agnostic, currently cameras)
	syncReq := &vmgatewaytypes.SyncDevicesRequest{
		EdgeID:  edgeID,
		Devices: deviceInfos,
	}

	syncResp, err := m.vmGateway.SyncDevices(ctx, syncReq)
	if err != nil {
		m.logger.Error("Failed to sync cameras with VM", zap.Error(err))
		// On error, set connection state to error
		if m.vmGateway != nil {
			_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, fmt.Sprintf("VM gateway error during camera sync: %v", err))
		}
		connectionState := m.getConnectionState()
		m.persistStateToStorage(ctx, connectionState, nil)
		m.logger.Error("Connection state updated to error due to camera sync failure")
		return
	}

	if !syncResp.Success {
		m.logger.Error("Device sync failed", zap.String("error", syncResp.ErrorMessage))
		// On sync failure, set connection state to error
		if m.vmGateway != nil {
			_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, fmt.Sprintf("VM gateway error: %s", syncResp.ErrorMessage))
		}
		connectionState := m.getConnectionState()
		m.persistStateToStorage(ctx, connectionState, nil)
		m.logger.Error("Connection state updated to error due to device sync failure")
		return
	}

	m.logger.Info("Devices synced with VM successfully",
		zap.Int("total_devices", len(cameras)),
		zap.Int("enabled_devices", len(syncResp.EnabledDevices)),
	)

	// Enable devices that VM decided to enable (currently cameras, but device-agnostic)
	for _, enabledDev := range syncResp.EnabledDevices {
		if enabledDev.Enabled {
			// For now, all devices are cameras, but this is device-agnostic for future IoT support
			if err := m.cctvService.EnableCamera(ctx, enabledDev.DeviceID); err != nil {
				m.logger.Warn("Failed to enable device",
					zap.String("device_id", enabledDev.DeviceID),
					zap.Error(err),
				)
			} else {
				m.logger.Info("Device enabled by VM decision",
					zap.String("device_id", enabledDev.DeviceID),
				)
			}
		}
	}

	// Update camera states to synced for all discovered cameras
	updatedCameraStates := make(map[string]types.CameraState)
	for _, cam := range cameras {
		cameraSM := m.getOrCreateCameraStateMachine(cam.ID)
		if cameraSM == nil {
			m.logger.Warn("Failed to get/create camera state machine for camera sync",
				zap.String("camera_id", cam.ID),
			)
			continue
		}
		currentState := cameraSM.GetState()
		if currentState == types.CameraStateDiscovered {
			if err := cameraSM.Transition(types.CameraStateSynced, ""); err != nil {
				m.logger.Warn("Failed to transition camera to synced",
					zap.String("camera_id", cam.ID),
					zap.Error(err),
				)
			} else {
				updatedCameraStates[cam.ID] = cameraSM.GetState()
				m.logger.Info("Camera state updated to synced",
					zap.String("camera_id", cam.ID),
				)
			}
		}
	}

	// Persist updated camera states
	if len(updatedCameraStates) > 0 {
		connectionState := m.getConnectionState()
		if err := m.persistStateToStorage(ctx, connectionState, updatedCameraStates); err != nil {
			m.logger.Warn("Failed to persist camera states after sync",
				zap.Error(err),
				zap.String("connection_state", string(connectionState)),
				zap.Int("camera_count", len(updatedCameraStates)),
			)
		} else {
			m.logger.Info("Camera states updated to synced", zap.Int("count", len(updatedCameraStates)))
		}
	}
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
	m.pendingSyncMu.Lock()
	m.pendingSync = true
	m.pendingSyncMu.Unlock()
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
// This function is idempotent: it checks if the model is already deployed before processing.
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

	// Get camera ID from event (model deployment is per-camera)
	cameraID, _ := ev.Data["camera_id"].(string)
	if cameraID == "" {
		m.logger.Warn("Model deployment event missing camera_id")
		return
	}

	// Transition camera state from screenshot_set_ready to model_deployed
	cameraSM := m.getOrCreateCameraStateMachine(cameraID)
	if cameraSM == nil {
		m.logger.Error("Failed to get/create camera state machine for model deployment",
			zap.String("camera_id", cameraID),
		)
		return
	}
	
	// Idempotency check: if model is already deployed and matches, skip
	if metadataSM, ok := cameraSM.(types.CameraStateMachineWithMetadata); ok {
		currentModelID := metadataSM.GetStateInfo().ModelID
		currentState := cameraSM.GetState()
		if currentModelID == modelID && (currentState == types.CameraStateModelDeployed || currentState == types.CameraStateFrameProcessing) {
			m.logger.Debug("Model already deployed for camera, skipping duplicate deployment",
				zap.String("camera_id", cameraID),
				zap.String("model_id", modelID),
				zap.String("current_state", string(currentState)))
			return
		}
	}
	
	currentCameraState := cameraSM.GetState()
	if currentCameraState == types.CameraStateScreenshotSetReady {
		// Set model ID if cameraSM supports metadata
		if metadataSM, ok := cameraSM.(types.CameraStateMachineWithMetadata); ok {
			metadataSM.SetModelID(modelID)
		} else {
			m.logger.Warn("Camera state machine does not support metadata, cannot set model ID",
				zap.String("camera_id", cameraID),
				zap.String("model_id", modelID),
			)
		}
		// Transition to model_deployed
		if err := cameraSM.Transition(types.CameraStateModelDeployed, ""); err != nil {
			m.logger.Warn("Failed to transition camera to model_deployed",
				zap.String("camera_id", cameraID),
				zap.String("model_id", modelID),
				zap.Error(err),
			)
		} else {
			m.logger.Info("Camera state transition: screenshot_set_ready → model_deployed",
				zap.String("camera_id", cameraID),
				zap.String("model_id", modelID),
			)
			// Persist state update
			cameraStates := map[string]types.CameraState{cameraID: cameraSM.GetState()}
			m.persistStateToStorage(ctx, m.getConnectionState(), cameraStates)
		}
	} else {
		// Queue model deployment for later processing when camera reaches screenshot_set_ready
		m.logger.Info("Model deployed but camera state is not screenshot_set_ready, queuing deployment",
			zap.String("camera_id", cameraID),
			zap.String("current_state", string(currentCameraState)),
			zap.String("model_id", modelID),
		)
		m.pendingModelDeployMu.Lock()
		m.pendingModelDeployments[cameraID] = &PendingModelDeployment{
			ModelID:    modelID,
			CameraID:   cameraID,
			EventData:  ev.Data,
			ReceivedAt: time.Now(),
		}
		m.pendingModelDeployMu.Unlock()
		m.logger.Info("Model deployment queued, will process when camera reaches screenshot_set_ready",
			zap.String("camera_id", cameraID),
			zap.String("model_id", modelID),
		)
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

// executeFrameProcessingWorkflowForCamera executes workflows when a camera is in frame_processing state
// This is the final operational state where Edge is actively monitoring a camera
func (m *StateManagerImpl) executeFrameProcessingWorkflowForCamera(ctx context.Context, cameraID string) {
	cameraSM := m.getCameraStateMachine(cameraID)
	if cameraSM == nil {
		return
	}

	m.logger.Debug("Camera in frame_processing state - actively monitoring",
		zap.String("camera_id", cameraID),
		zap.String("state", string(cameraSM.GetState())),
	)
	// Frame processing is active, no additional workflow needed
	// The frame processing loop is already running
}

// executeModelDeployedWorkflowForCamera executes workflows when a camera's model is deployed
// This starts continuous frame processing for the specific camera
func (m *StateManagerImpl) executeModelDeployedWorkflowForCamera(ctx context.Context, cameraID string) {
	cameraSM := m.getCameraStateMachine(cameraID)
	if cameraSM == nil {
		return
	}

	m.logger.Info("Starting continuous frame processing for camera with deployed model",
		zap.String("camera_id", cameraID),
	)

	// Verify camera exists and is enabled
	if m.cctvService == nil {
		m.logger.Warn("CCTV service not available, cannot start frame processing",
			zap.String("camera_id", cameraID),
		)
		return
	}

	camera, err := m.cctvService.GetCamera(ctx, cameraID)
	if err != nil {
		m.logger.Error("Failed to get camera for frame processing",
			zap.String("camera_id", cameraID),
			zap.Error(err),
		)
		return
	}

	if !camera.Enabled {
		m.logger.Warn("Camera is not enabled, skipping frame processing",
			zap.String("camera_id", cameraID),
		)
		return
	}

	// Start frame processing for this camera
	if err := m.startFrameProcessingForCamera(ctx, cameraID); err != nil {
		m.logger.Error("Failed to start frame processing for camera",
			zap.String("camera_id", cameraID),
			zap.Error(err),
		)
		return
	}

	// Transition camera to frame_processing state when frame processing successfully starts
	if cameraSM.CanTransition(types.CameraStateFrameProcessing) {
		if err := cameraSM.Transition(types.CameraStateFrameProcessing, ""); err != nil {
			m.logger.Warn("Failed to transition camera to frame_processing",
				zap.String("camera_id", cameraID),
				zap.Error(err),
			)
		} else {
			m.logger.Info("Camera state transition: model_deployed → frame_processing",
				zap.String("camera_id", cameraID),
			)
			// Persist state update
			cameraStates := map[string]types.CameraState{cameraID: cameraSM.GetState()}
			m.persistStateToStorage(ctx, m.getConnectionState(), cameraStates)
		}
	}

	m.logger.Info("Frame processing started for camera",
		zap.String("camera_id", cameraID),
		zap.Duration("interval", m.frameProcessingInterval),
	)
}

// handleModelDeploymentStatus handles model deployment status events
// These events are published when a model is deployed and need to be reported to the VM
func (m *StateManagerImpl) handleModelDeploymentStatus(ctx context.Context, ev eventbustypes.Event) {
	// Extract event data
	deploymentID, _ := ev.Data["deployment_id"].(string)
	status, _ := ev.Data["status"].(string)
	modelPath, _ := ev.Data["model_path"].(string)
	modelID, _ := ev.Data["model_id"].(string)

	if deploymentID == "" {
		m.logger.Warn("Model deployment status event missing deployment_id")
		return
	}

	if status == "" {
		m.logger.Warn("Model deployment status event missing status")
		return
	}

	m.logger.Info("Handling model deployment status event",
		zap.String("deployment_id", deploymentID),
		zap.String("status", status),
		zap.String("model_id", modelID),
		zap.String("model_path", modelPath),
	)

	// Report deployment status to VM via VM gateway
	if m.vmGateway == nil {
		m.logger.Warn("VM gateway not available, cannot report deployment status",
			zap.String("deployment_id", deploymentID),
		)
		return
	}

	// Check if transport is connected (required for reporting status)
	if !m.vmGateway.IsTransportConnected() {
		m.logger.Debug("Transport not connected, cannot report deployment status",
			zap.String("deployment_id", deploymentID),
		)
		// Don't log as warning - this is expected during disconnection
		return
	}

	// Prepare error message if status indicates failure
	var errorMsg *string
	if status == "failed" || status == "error" {
		if errMsg, ok := ev.Data["error"].(string); ok && errMsg != "" {
			errorMsg = &errMsg
		} else {
			msg := "Model deployment failed"
			errorMsg = &msg
		}
	}

	// Prepare model path pointer
	var modelPathPtr *string
	if modelPath != "" {
		modelPathPtr = &modelPath
	}

	// Report status to VM with timeout
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := m.vmGateway.ReportDeploymentStatus(callCtx, deploymentID, status, errorMsg, modelPathPtr); err != nil {
		m.logger.Error("Failed to report deployment status to VM",
			zap.String("deployment_id", deploymentID),
			zap.String("status", status),
			zap.Error(err),
		)
		return
	}

	m.logger.Info("Model deployment status reported to VM",
		zap.String("deployment_id", deploymentID),
		zap.String("status", status),
		zap.String("model_id", modelID),
	)
}

// startFrameProcessingForCamera starts periodic frame capture and processing for a camera
// Returns error if frame processing cannot be started
// This function is thread-safe and prevents duplicate goroutines for the same camera
func (m *StateManagerImpl) startFrameProcessingForCamera(ctx context.Context, cameraID string) error {
	// First, do a quick check with read lock to see if already processing
	m.frameProcessingMu.RLock()
	_, exists := m.frameProcessingActive[cameraID]
	m.frameProcessingMu.RUnlock()

	if exists {
		m.logger.Debug("Frame processing already active for camera",
			zap.String("camera_id", cameraID),
		)
		return nil // Already processing, not an error
	}

	// Verify camera exists and is enabled (do this outside the lock to avoid blocking)
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

	// Now acquire write lock and re-check existence to prevent race condition
	// Another goroutine might have started processing while we were validating
	m.frameProcessingMu.Lock()
	defer m.frameProcessingMu.Unlock()

	// Re-check existence inside critical section to prevent duplicate goroutines
	if _, exists := m.frameProcessingActive[cameraID]; exists {
		m.logger.Debug("Frame processing already active for camera (re-checked after validation)",
			zap.String("camera_id", cameraID),
		)
		return nil // Another goroutine started it, not an error
	}

	// Create cancel context for this camera's frame processing
	frameCtx, cancel := context.WithCancel(ctx)
	m.frameProcessingActive[cameraID] = cancel

	// Start frame processing goroutine
	m.wg.Add(1)
	m.frameProcessingWg.Add(1)
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
// It waits for all frame processing goroutines to finish before clearing the map
func (m *StateManagerImpl) stopAllFrameProcessing() {
	m.frameProcessingMu.Lock()

	// Count how many goroutines we're stopping
	activeCount := len(m.frameProcessingActive)

	// Cancel all contexts to signal goroutines to stop
	for cameraID, cancel := range m.frameProcessingActive {
		cancel()
		m.logger.Info("Stopped frame processing for camera",
			zap.String("camera_id", cameraID),
		)
	}

	// Unlock mutex to allow goroutines to finish and call frameProcessingWg.Done()
	m.frameProcessingMu.Unlock()

	// Wait for frame processing goroutines to finish with a timeout
	// Frame processing loops should exit quickly after context cancellation
	if activeCount > 0 {
		done := make(chan struct{})
		go func() {
			m.frameProcessingWg.Wait()
			close(done)
		}()

		// Wait with timeout (max 10 seconds) for goroutines to finish
		// Frame processing should exit quickly after context cancellation
		timeout := 10 * time.Second
		select {
		case <-done:
			// All frame processing goroutines have finished
			m.logger.Debug("All frame processing goroutines finished",
				zap.Int("stopped_count", activeCount),
			)
		case <-time.After(timeout):
			m.logger.Warn("Timeout waiting for frame processing goroutines to finish",
				zap.Int("active_count", activeCount),
				zap.Duration("timeout", timeout),
			)
		}
	}

	// Lock again to clear the map
	m.frameProcessingMu.Lock()
	defer m.frameProcessingMu.Unlock()

	// Clear the map - goroutines should have finished by now
	m.frameProcessingActive = make(map[string]context.CancelFunc)

	m.logger.Info("Stopped frame processing for all cameras",
		zap.Int("stopped_count", activeCount),
	)
}

// frameProcessingLoop continuously captures frames from a camera and processes them
func (m *StateManagerImpl) frameProcessingLoop(ctx context.Context, cameraID string) {
	defer m.wg.Done()
	defer m.frameProcessingWg.Done()

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

	// Capture frame from camera with timeout
	captureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	frame, err := m.cctvService.CaptureFrame(captureCtx, cameraID)
	if err != nil {
		m.logger.Warn("Failed to capture frame from camera",
			zap.String("camera_id", cameraID),
			zap.Error(err),
		)
		// Track consecutive failures
		m.frameErrorMu.Lock()
		m.frameCaptureErrors[cameraID]++
		errorCount := m.frameCaptureErrors[cameraID]
		m.frameErrorMu.Unlock()
		// If too many consecutive failures, transition to error state
		if errorCount >= m.frameCaptureErrorThreshold {
			m.logger.Error("Too many consecutive frame capture failures, transitioning camera to error state",
				zap.String("camera_id", cameraID),
				zap.Int("error_count", errorCount),
			)
			cameraSM := m.getCameraStateMachine(cameraID)
			if cameraSM != nil && cameraSM.CanTransition(types.CameraStateError) {
				_ = cameraSM.Transition(types.CameraStateError, fmt.Sprintf("Frame capture failed %d times", errorCount))
			}
		}
		return
	}
	// Reset error count on success
	m.frameErrorMu.Lock()
	delete(m.frameCaptureErrors, cameraID)
	m.frameErrorMu.Unlock()

	// Store frame in object storage (frames bucket)
	frameID := fmt.Sprintf("frame-%s-%d", cameraID, time.Now().Unix())
	frameKey := fmt.Sprintf("frames/%s/%s/%s.jpg", cameraID, time.Now().Format("2006-01-02"), frameID)

	if m.objectStorage != nil {
		// Store frame image data with timeout
		storeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		err = m.objectStorage.StoreFrame(storeCtx, frameKey, frame.Data)
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

	// Send frame to AI gateway for processing with timeout
	if m.aiGateway != nil {
		// Use AI gateway to process the frame
		// The AI gateway will:
		// 1. Load the model from MinIO (if not already loaded)
		// 2. Process the frame
		// 3. Determine if it's similar to training set or has anomalies
		// 4. Delete normal frames or move suspicious ones to security event bucket
		processCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		err = m.aiGateway.ProcessFrame(processCtx, cameraID, frameKey, frame.Data)
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
// executeScreenshotSetReadyWorkflowForCamera executes workflows when a camera's screenshot set is ready
func (m *StateManagerImpl) executeScreenshotSetReadyWorkflowForCamera(ctx context.Context, cameraID string) {
	cameraSM := m.getCameraStateMachine(cameraID)
	if cameraSM == nil {
		return
	}

	m.logger.Debug("Camera in screenshot_set_ready state",
		zap.String("camera_id", cameraID),
		zap.String("state", string(cameraSM.GetState())),
	)
	// Screenshot sync is handled in handleScreenshotSetReady when the event is received
}

// initiateWireGuardConnection initiates WireGuard connection in production mode
func (m *StateManagerImpl) initiateWireGuardConnection(ctx context.Context) {
	if m.vmGateway == nil {
		m.logger.Error("VM gateway not available, cannot establish WireGuard connection")
		return
	}

	m.logger.Info("Initiating WireGuard connection to VM")

	// Check if already connected
	if m.vmGateway.IsConnected() {
		m.logger.Info("WireGuard already connected")
		// Update connection state to wireguard_connected
		currentState := m.getConnectionState()
		if currentState == vmgatewaytypes.ConnectionStateDisconnected || currentState == vmgatewaytypes.ConnectionStateWGConnecting {
			if m.vmGateway != nil {
				if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateWireGuardConnected, ""); err != nil {
					m.logger.Warn("Failed to transition to wireguard_connected", zap.Error(err))
				} else {
					connectionState := m.getConnectionState()
					m.persistStateToStorageWithErrorHandling(ctx, connectionState, nil, "wireguard_connected")
				}
			}
		}

		m.logger.Info("Connection state updated",
			zap.String("connection_state", string(m.getConnectionState())),
		)

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
			currentState := m.getConnectionState()
			if currentState == vmgatewaytypes.ConnectionStateWGConnecting {
				if m.vmGateway != nil {
					_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateWGConnectionError, "Connection timeout")
					connectionState := m.getConnectionState()
					m.persistStateToStorageWithErrorHandling(ctx, connectionState, nil, "wireguard_timeout")
				}
			}
			return
		case <-ticker.C:
			if m.vmGateway.IsConnected() {
				m.logger.Info("WireGuard connection established")
				// Update connection state to wireguard_connected
				currentState := m.getConnectionState()
				if currentState == vmgatewaytypes.ConnectionStateWGConnecting {
					if m.vmGateway != nil {
						if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateWireGuardConnected, ""); err != nil {
							m.logger.Warn("Failed to transition to wireguard_connected", zap.Error(err))
						} else {
							connectionState := m.getConnectionState()
							m.persistStateToStorageWithErrorHandling(ctx, connectionState, nil, "wireguard_connected_after_retry")
						}
					}
				}

				m.logger.Info("Connection state updated",
					zap.String("connection_state", string(m.getConnectionState())),
				)
				return
			}
		}
	}
}

// initiateHTTPConnection initiates HTTP connection in dev mode (WireGuard disabled)
func (m *StateManagerImpl) initiateHTTPConnection(ctx context.Context) {
	if m.vmGateway == nil {
		m.logger.Error("VM gateway not available, cannot establish HTTP connection")
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
		currentState := m.getConnectionState()
		if currentState == vmgatewaytypes.ConnectionStateHTTPConnecting {
			if m.vmGateway != nil {
				_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateHTTPConnectionError, fmt.Sprintf("Authentication failed: %v", err))
				connectionState := m.getConnectionState()
				m.persistStateToStorageWithErrorHandling(ctx, connectionState, nil, "http_auth_error")
			}
		}
		return
	}

	// Connection successful
	m.logger.Info("HTTP connection established and authenticated (dev mode)")
	currentState := m.getConnectionState()
	if currentState == vmgatewaytypes.ConnectionStateHTTPConnecting {
		if m.vmGateway != nil {
			if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateHTTPSConnected, ""); err != nil {
				m.logger.Warn("Failed to transition to https_connected", zap.Error(err))
			} else {
				// After HTTPS connection, try to authenticate
				if m.vmGateway != nil {
					if err := m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateAuthenticated, ""); err != nil {
						m.logger.Warn("Failed to transition to authenticated", zap.Error(err))
					}
					connectionState := m.getConnectionState()
					m.persistStateToStorageWithErrorHandling(ctx, connectionState, nil, "http_authenticated")
				}
			}
		}
	}

	var connectionStateInfo vmgatewaytypes.ConnectionStateInfo
	if m.vmGateway != nil {
		connectionStateInfo = m.vmGateway.GetConnectionStateInfo()
	}
	m.logger.Info("Connection state updated",
		zap.String("connection_state", string(m.getConnectionState())),
		zap.Bool("vm_reachable", connectionStateInfo.VMReachable))
}

// handleErrorState handles error state
// handleConnectionErrorState handles connection-level error states
func (m *StateManagerImpl) handleConnectionErrorState(ctx context.Context, connectionState vmgatewaytypes.ConnectionState) {
	m.logger.Warn("Edge in connection error state, attempting recovery",
		zap.String("connection_state", string(connectionState)),
	)

	// Identify error source from connection state
	var errorMessage string
	if m.vmGateway != nil {
		connectionStateInfo := m.vmGateway.GetConnectionStateInfo()
		if connectionStateInfo.Error != "" {
			errorMessage = connectionStateInfo.Error
		}
	}

	// Attempt recovery based on error type
	switch connectionState {
	case vmgatewaytypes.ConnectionStateWGConnectionError:
		m.logger.Info("Attempting WireGuard connection recovery",
			zap.String("error", errorMessage),
		)
		// Retry WireGuard connection after a delay
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			time.Sleep(10 * time.Second) // Wait before retry
			if m.getConnectionState() == vmgatewaytypes.ConnectionStateWGConnectionError {
				m.logger.Info("Retrying WireGuard connection")
				if m.vmGateway != nil {
					_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")
					m.initiateWireGuardConnection(m.ctx)
				}
			}
		}()

	case vmgatewaytypes.ConnectionStateHTTPConnectionError:
		m.logger.Info("Attempting HTTP connection recovery",
			zap.String("error", errorMessage),
		)
		// Retry HTTP connection after a delay
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			time.Sleep(10 * time.Second) // Wait before retry
			if m.getConnectionState() == vmgatewaytypes.ConnectionStateHTTPConnectionError {
				m.logger.Info("Retrying HTTP connection")
				if m.vmGateway != nil {
					_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")
					m.initiateHTTPConnection(m.ctx)
				}
			}
		}()

	case vmgatewaytypes.ConnectionStateError:
		m.logger.Info("Attempting general error recovery",
			zap.String("error", errorMessage),
		)
		// Check service health and retry connection
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			time.Sleep(30 * time.Second) // Wait longer for general errors
			if m.getConnectionState() == vmgatewaytypes.ConnectionStateError {
				m.logger.Info("Retrying connection after error recovery")
				m.checkServicesHealth(m.ctx)
			}
		}()

	default:
		m.logger.Debug("No specific recovery strategy for connection state",
			zap.String("connection_state", string(connectionState)),
		)
	}
}

// handleCameraErrorState handles camera-level error states
func (m *StateManagerImpl) handleCameraErrorState(ctx context.Context, cameraID string, cameraState types.CameraState) {
	cameraSM := m.getCameraStateMachine(cameraID)
	if cameraSM == nil {
		return
	}

	stateInfo := cameraSM.GetStateInfo()
	m.logger.Warn("Camera in error state, attempting recovery",
		zap.String("camera_id", cameraID),
		zap.String("camera_state", string(cameraState)),
		zap.String("error", stateInfo.Error),
	)

	// Identify error source from error message
	errorMessage := stateInfo.Error

	// Attempt recovery based on error type
	if strings.Contains(errorMessage, "Frame capture failed") {
		// Frame capture error - try to recover by resetting error count and retrying discovery
		m.logger.Info("Attempting camera recovery from frame capture errors",
			zap.String("camera_id", cameraID),
		)

		// Reset frame capture error count
		m.frameErrorMu.Lock()
		delete(m.frameCaptureErrors, cameraID)
		m.frameErrorMu.Unlock()

		// Retry camera discovery after a delay
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			time.Sleep(30 * time.Second) // Wait before retry
			if cameraSM.GetState() == types.CameraStateError {
				m.logger.Info("Retrying camera discovery after error recovery",
					zap.String("camera_id", cameraID),
				)
				// Try to transition back to discovered state
				if cameraSM.CanTransition(types.CameraStateDiscovered) {
					_ = cameraSM.Transition(types.CameraStateDiscovered, "")
					// Trigger camera discovery workflow
					if m.cctvService != nil {
						// Camera should be rediscovered on next capability sync or discovery event
						m.logger.Info("Camera state reset to discovered, will be rediscovered",
							zap.String("camera_id", cameraID),
						)
					}
				}
			}
		}()
	} else {
		// Other camera errors - try to recover by resetting to discovered state
		m.logger.Info("Attempting general camera error recovery",
			zap.String("camera_id", cameraID),
			zap.String("error", errorMessage),
		)

		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			time.Sleep(30 * time.Second) // Wait before retry
			if cameraSM.GetState() == types.CameraStateError {
				m.logger.Info("Retrying camera recovery",
					zap.String("camera_id", cameraID),
				)
				// Try to transition back to discovered state
				if cameraSM.CanTransition(types.CameraStateDiscovered) {
					_ = cameraSM.Transition(types.CameraStateDiscovered, "")
					m.logger.Info("Camera state reset to discovered",
						zap.String("camera_id", cameraID),
					)
				}
			}
		}()
	}
}

// checkServicesHealth checks the health of all services and updates status if any are unhealthy
func (m *StateManagerImpl) checkServicesHealth(ctx context.Context) {
	// Check connection state - only check health if in disconnected state (initial state)
	connectionState := m.getConnectionState()
	if connectionState != vmgatewaytypes.ConnectionStateDisconnected {
		return
	}

	// Check meta-storage health (required service)
	if m.metaStorage == nil {
		m.logger.Error("Meta-storage is not available")
		if m.vmGateway != nil {
			_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, "Meta-storage is not available")
			connectionState := m.getConnectionState()
			m.persistStateToStorageWithErrorHandling(ctx, connectionState, nil, "health_check_meta_storage_unavailable")
		}
		return
	}
	_, err := m.metaStorage.GetStorageStats(ctx)
	if err != nil {
		m.logger.Error("Meta-storage health check failed", zap.Error(err))
		if m.vmGateway != nil {
			_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, fmt.Sprintf("Meta-storage health check failed: %v", err))
			connectionState := m.getConnectionState()
			m.persistStateToStorageWithErrorHandling(ctx, connectionState, nil, "health_check_meta_storage_failed")
		}
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
		if m.vmGateway != nil {
			_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, "CCTV service is not available")
			connectionState := m.getConnectionState()
			m.persistStateToStorageWithErrorHandling(ctx, connectionState, nil, "health_check_cctv_unavailable")
		}
		return
	}
	m.logger.Debug("CCTV service is available")

	// Check AI gateway health (required service)
	if m.aiGateway == nil {
		m.logger.Error("AI gateway is not available")
		if m.vmGateway != nil {
			_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, "AI gateway is not available")
			connectionState := m.getConnectionState()
			m.persistStateToStorageWithErrorHandling(ctx, connectionState, nil, "health_check_ai_gateway_unavailable")
		}
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
		if m.vmGateway != nil {
			_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateError, "Web gateway is not available")
			connectionState := m.getConnectionState()
			m.persistStateToStorageWithErrorHandling(ctx, connectionState, nil, "health_check_web_gateway_unavailable")
		}
		return
	}
	m.logger.Debug("Web gateway is available")

	// All required services are healthy, now initiate connection
	m.logger.Info("All services health checks passed, initiating connection")

	// Initiate connection based on WireGuard configuration
	m.initiateConnection(ctx)
}

// initiateConnection initiates WireGuard or HTTP connection based on configuration
func (m *StateManagerImpl) initiateConnection(ctx context.Context) {
	m.logger.Info("initiateConnection called")

	// Only proceed if connection state is still disconnected
	currentState := m.getConnectionState()
	m.logger.Info("Current connection state check", zap.String("connection_state", string(currentState)))
	if currentState != vmgatewaytypes.ConnectionStateDisconnected {
		m.logger.Debug("Connection state is not disconnected, skipping connection initiation", zap.String("connection_state", string(currentState)))
		return
	}

	// Check if VM gateway is available
	if m.vmGateway == nil {
		m.logger.Warn("VM gateway is not available, cannot establish connection")
		// Note: Cannot transition state if vmGateway is nil, so we just return
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
		if m.vmGateway != nil {
			_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateWGConnecting, "")
		}
		connectionState := m.getConnectionState()
		if err := m.persistStateToStorage(ctx, connectionState, nil); err != nil {
			m.logger.Warn("Failed to persist state during WireGuard connection initiation",
				zap.Error(err),
				zap.String("connection_state", string(connectionState)),
			)
		}

		// Start WireGuard connection in a goroutine
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.initiateWireGuardConnection(ctx)
		}()
	} else {
		// WireGuard is disabled (dev mode) - connect via HTTP directly
		m.logger.Info("WireGuard is disabled, connecting via HTTP (localhost/dev mode)")
		if m.vmGateway != nil {
			_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateHTTPConnecting, "")
		}
		connectionState := m.getConnectionState()
		if err := m.persistStateToStorage(ctx, connectionState, nil); err != nil {
			m.logger.Warn("Failed to persist state during HTTP connection initiation",
				zap.Error(err),
				zap.String("connection_state", string(connectionState)),
			)
		}

		// Call HTTP connection synchronously so we can update status based on result
		m.initiateHTTPConnection(ctx)
	}
}

// persistStateToStorageWithErrorHandling is a helper that calls persistStateToStorage and logs errors
// This is used in error paths where we want to persist state but don't want to fail the operation if persistence fails
func (m *StateManagerImpl) persistStateToStorageWithErrorHandling(ctx context.Context, connectionState vmgatewaytypes.ConnectionState, cameraStates map[string]types.CameraState, operation string) {
	if err := m.persistStateToStorage(ctx, connectionState, cameraStates); err != nil {
		m.logger.Warn("Failed to persist state",
			zap.Error(err),
			zap.String("operation", operation),
			zap.String("connection_state", string(connectionState)),
			zap.Int("camera_states_count", len(cameraStates)),
		)
	}
}

// persistStateToStorage persists the connection state and camera states to meta-storage
// This method saves both connection state and per-camera states separately for better state management
// Implements retry logic with exponential backoff for transient failures
// Returns error if persistence fails after all retry attempts
func (m *StateManagerImpl) persistStateToStorage(ctx context.Context, connectionState vmgatewaytypes.ConnectionState, cameraStates map[string]types.CameraState) error {
	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage not available")
	}

	// Get connection state info
	var connectionStateInfo vmgatewaytypes.ConnectionStateInfo
	if m.vmGateway != nil {
		connectionStateInfo = m.vmGateway.GetConnectionStateInfo()
	} else {
		connectionStateInfo = vmgatewaytypes.ConnectionStateInfo{
			State:         connectionState,
			LastUpdated:   time.Now(),
			VMReachable:   false,
			NetworkHealth: "unhealthy",
		}
	}
	operationalState := m.getOperationalState()

	// Build comprehensive state map with connection state and operational metrics
	stateMap := map[string]interface{}{
		"connection_state":         string(connectionState),
		"connection_error":         connectionStateInfo.Error,
		"network_health":           connectionStateInfo.NetworkHealth,
		"vm_reachable":             connectionStateInfo.VMReachable,
		"connection_last_updated":  connectionStateInfo.LastUpdated.Format(time.RFC3339),
		"cameras_enabled":          operationalState.CamerasEnabled,
		"ai_processing_active":     operationalState.AIProcessingActive,
		"storage_health":            operationalState.StorageHealth,
		"operational_last_updated": operationalState.LastUpdated.Format(time.RFC3339),
		"last_updated":             time.Now().Format(time.RFC3339), // Overall state timestamp
	}

	// Add camera states separately - each camera state is stored with its full info
	if cameraStates != nil && len(cameraStates) > 0 {
		cameraStatesMap := make(map[string]interface{})
		for cameraID, state := range cameraStates {
			// Get full camera state info if state machine exists
			cameraSM := m.getCameraStateMachine(cameraID)
			if cameraSM != nil {
				stateInfo := cameraSM.GetStateInfo()
				cameraStatesMap[cameraID] = map[string]interface{}{
					"state":         string(state),
					"error":         stateInfo.Error,
					"model_id":      stateInfo.ModelID,
					"dataset_id":    stateInfo.DatasetID,
					"is_processing": stateInfo.IsProcessing,
					"last_updated":  stateInfo.LastUpdated.Format(time.RFC3339),
				}
			} else {
				// Fallback: just store the state if state machine doesn't exist yet
				cameraStatesMap[cameraID] = map[string]interface{}{
					"state":        string(state),
					"last_updated": time.Now().Format(time.RFC3339),
				}
			}
		}
		stateMap["camera_states"] = cameraStatesMap
	} else {
		// If no camera states provided, persist all current camera states
		allCameraSMs := m.getAllCameraStateMachines()
		if len(allCameraSMs) > 0 {
			cameraStatesMap := make(map[string]interface{})
			for cameraID, cameraSM := range allCameraSMs {
				stateInfo := cameraSM.GetStateInfo()
				cameraStatesMap[cameraID] = map[string]interface{}{
					"state":         string(stateInfo.State),
					"error":         stateInfo.Error,
					"model_id":      stateInfo.ModelID,
					"dataset_id":    stateInfo.DatasetID,
					"is_processing": stateInfo.IsProcessing,
					"last_updated":  stateInfo.LastUpdated.Format(time.RFC3339),
				}
			}
			stateMap["camera_states"] = cameraStatesMap
		}
	}

	// Attempt persistence with retry logic
	maxRetries := m.statePersistenceMaxRetries
	backoff := m.statePersistenceRetryBackoff
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check context cancellation before each attempt
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during state persistence: %w", ctx.Err())
		default:
		}

		// Attempt to persist
		err := m.metaStorage.SaveEdgeState(ctx, stateMap)
		if err == nil {
			// Success - log if retry was needed
			if attempt > 0 {
				m.logger.Info("Edge state persisted successfully after retry",
					zap.Int("attempt", attempt+1),
					zap.String("connection_state", string(connectionState)),
					zap.Int("camera_states_count", len(cameraStates)),
				)
			} else {
				m.logger.Debug("Edge state persisted successfully",
					zap.String("connection_state", string(connectionState)),
					zap.Int("camera_states_count", len(cameraStates)),
				)
			}
			return nil
		}

		lastErr = err

		// If this was the last attempt, don't wait
		if attempt >= maxRetries {
			break
		}

		// Calculate exponential backoff: backoff * 2^attempt
		multiplier := 1 << attempt // 2^attempt
		waitTime := time.Duration(int64(backoff) * int64(multiplier))
		if waitTime > 10*time.Second {
			waitTime = 10 * time.Second // Cap at 10 seconds
		}

		m.logger.Warn("Failed to persist edge state to storage, retrying",
			zap.Error(err),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", maxRetries),
			zap.Duration("backoff", waitTime),
			zap.String("connection_state", string(connectionState)),
			zap.Int("camera_states_count", len(cameraStates)),
		)

		// Wait with exponential backoff, respecting context cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during state persistence retry: %w", ctx.Err())
		case <-time.After(waitTime):
			// Continue to next retry attempt
		}
	}

	// All retries exhausted - log error and return
	m.logger.Error("Failed to persist edge state to storage after all retry attempts",
		zap.Error(lastErr),
		zap.Int("total_attempts", maxRetries+1),
		zap.String("connection_state", string(connectionState)),
		zap.Int("camera_states_count", len(cameraStates)),
	)
	return fmt.Errorf("failed to persist state after %d attempts: %w", maxRetries+1, lastErr)
}

// restoreStateFromStorage restores connection state and camera states from meta-storage
// This is called on startup to recover the previous state
func (m *StateManagerImpl) restoreStateFromStorage(ctx context.Context) error {
	if m.metaStorage == nil {
		return fmt.Errorf("meta-storage not available")
	}

	// Get current edge state from meta-storage
	stateMap, exists := m.metaStorage.GetCurrentEdgeState(ctx)
	if !exists {
		m.logger.Info("No previous edge state found in storage, starting fresh")
		return fmt.Errorf("no previous state found")
	}

	// Restore connection state
	if connectionStateStr, ok := stateMap["connection_state"].(string); ok {
		connectionState := vmgatewaytypes.ConnectionState(connectionStateStr)
		// Validate connection state before restoring
		if connectionState == vmgatewaytypes.ConnectionStateDisconnected ||
			connectionState == vmgatewaytypes.ConnectionStateWGConnecting ||
			connectionState == vmgatewaytypes.ConnectionStateWireGuardConnected ||
			connectionState == vmgatewaytypes.ConnectionStateHTTPConnecting ||
			connectionState == vmgatewaytypes.ConnectionStateHTTPSConnected ||
			connectionState == vmgatewaytypes.ConnectionStateAuthenticated ||
			connectionState == vmgatewaytypes.ConnectionStateCapabilitiesReceived ||
			connectionState == vmgatewaytypes.ConnectionStateError ||
			connectionState == vmgatewaytypes.ConnectionStateWGConnectionError ||
			connectionState == vmgatewaytypes.ConnectionStateHTTPConnectionError {

			// Get error message if present
			errorMsg := ""
			if errStr, ok := stateMap["connection_error"].(string); ok {
				errorMsg = errStr
			}

			if m.vmGateway != nil {
				if err := m.vmGateway.TransitionConnectionState(connectionState, errorMsg); err != nil {
					m.logger.Warn("Failed to restore connection state, using disconnected",
						zap.String("restored_state", string(connectionState)),
						zap.Error(err),
					)
					_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")
				} else {
					m.logger.Info("Connection state restored from storage",
						zap.String("connection_state", string(connectionState)),
					)
				}
			}
		} else {
			m.logger.Warn("Invalid connection state in storage, using disconnected",
				zap.String("invalid_state", connectionStateStr),
			)
			if m.vmGateway != nil {
				_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")
			}
		}
	}

	// Restore operational state
	if storageHealth, ok := stateMap["storage_health"].(string); ok {
		m.updateOperationalState(func(op *OperationalState) {
			op.StorageHealth = storageHealth
		})
	}
	if camerasEnabled, ok := stateMap["cameras_enabled"].(int); ok {
		m.updateOperationalState(func(op *OperationalState) {
			op.CamerasEnabled = camerasEnabled
		})
	}
	if aiProcessingActive, ok := stateMap["ai_processing_active"].(bool); ok {
		m.updateOperationalState(func(op *OperationalState) {
			op.AIProcessingActive = aiProcessingActive
		})
	}

	// Restore camera states
	if cameraStatesRaw, ok := stateMap["camera_states"].(map[string]interface{}); ok {
		restoredCount := 0
		for cameraID, cameraStateRaw := range cameraStatesRaw {
			cameraStateMap, ok := cameraStateRaw.(map[string]interface{})
			if !ok {
				// Try string format (backward compatibility)
				if stateStr, ok := cameraStateRaw.(string); ok {
					cameraState := types.CameraState(stateStr)
					cameraSM := m.getOrCreateCameraStateMachine(cameraID)
					if cameraSM == nil {
						m.logger.Warn("Failed to get/create camera state machine for state restoration",
							zap.String("camera_id", cameraID),
						)
						continue
					}
					if err := cameraSM.Transition(cameraState, ""); err == nil {
						restoredCount++
					}
				}
				continue
			}

			// Restore camera state with full info
			if stateStr, ok := cameraStateMap["state"].(string); ok {
				cameraState := types.CameraState(stateStr)
				cameraSM := m.getOrCreateCameraStateMachine(cameraID)
				if cameraSM == nil {
					m.logger.Warn("Failed to get/create camera state machine for state restoration",
						zap.String("camera_id", cameraID),
					)
					continue
				}

				// Restore state
				errorMsg := ""
				if errStr, ok := cameraStateMap["error"].(string); ok {
					errorMsg = errStr
				}

				if err := cameraSM.Transition(cameraState, errorMsg); err != nil {
					m.logger.Warn("Failed to restore camera state",
						zap.String("camera_id", cameraID),
						zap.String("state", stateStr),
						zap.Error(err),
					)
					continue
				}

				// Restore model ID and dataset ID if present
				if metadataSM, ok := cameraSM.(types.CameraStateMachineWithMetadata); ok {
					if modelID, ok := cameraStateMap["model_id"].(string); ok && modelID != "" {
						metadataSM.SetModelID(modelID)
					}
					if datasetID, ok := cameraStateMap["dataset_id"].(string); ok && datasetID != "" {
						metadataSM.SetDatasetID(datasetID)
					}
				} else {
					m.logger.Warn("Camera state machine does not support metadata, cannot restore model/dataset ID",
						zap.String("camera_id", cameraID),
					)
				}

				restoredCount++
				m.logger.Debug("Camera state restored from storage",
					zap.String("camera_id", cameraID),
					zap.String("state", string(cameraState)),
				)
			}
		}
		m.logger.Info("Camera states restored from storage",
			zap.Int("restored_count", restoredCount),
			zap.Int("total_in_storage", len(cameraStatesRaw)),
		)
	}

	// After restoring states, recover active workflows
	m.recoverActiveWorkflows(ctx)

	return nil
}

// recoverActiveWorkflows recovers active workflows based on restored states
// This includes resuming frame processing for cameras that were in frame_processing state,
// and re-establishing connections if needed
func (m *StateManagerImpl) recoverActiveWorkflows(ctx context.Context) {
	connectionState := m.getConnectionState()

	// Recover connection workflows if needed
	if connectionState == vmgatewaytypes.ConnectionStateWGConnecting {
		m.logger.Info("Recovering WireGuard connection workflow")
		// Connection attempt was in progress, re-initiate
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.initiateWireGuardConnection(ctx)
		}()
	} else if connectionState == vmgatewaytypes.ConnectionStateHTTPConnecting {
		m.logger.Info("Recovering HTTP connection workflow")
		// HTTP connection attempt was in progress, re-initiate
		m.initiateHTTPConnection(ctx)
	} else if connectionState == vmgatewaytypes.ConnectionStateError ||
		connectionState == vmgatewaytypes.ConnectionStateWGConnectionError ||
		connectionState == vmgatewaytypes.ConnectionStateHTTPConnectionError {
		m.logger.Info("Recovering from connection error state",
			zap.String("connection_state", string(connectionState)),
		)
		// Transition to disconnected to allow retry
		if m.vmGateway != nil {
			_ = m.vmGateway.TransitionConnectionState(vmgatewaytypes.ConnectionStateDisconnected, "")
		}
		// Re-initiate connection
		m.initiateConnection(ctx)
	} else if connectionState == vmgatewaytypes.ConnectionStateCapabilitiesReceived {
		m.logger.Info("Recovering capabilities workflow - checking for cameras to sync")
		// Capabilities were received, check if we need to sync cameras
		if m.cctvService != nil {
			// Trigger camera discovery/sync if needed
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				// Check if cameras need to be synced
				cameras, err := m.cctvService.GetDiscoveredCameras(ctx)
				if err == nil && len(cameras) > 0 {
					m.syncCamerasWithVM(ctx)
				}
			}()
		}
	}

	// Recover camera workflows
	allCameraSMs := m.getAllCameraStateMachines()
	for cameraID, cameraSM := range allCameraSMs {
		cameraState := cameraSM.GetState()
		stateInfo := cameraSM.GetStateInfo()

		// Recover frame processing for cameras that were in frame_processing state
		if cameraState == types.CameraStateFrameProcessing {
			m.logger.Info("Recovering frame processing workflow for camera",
				zap.String("camera_id", cameraID),
			)
			// Resume frame processing for this camera
			if err := m.startFrameProcessingForCamera(ctx, cameraID); err != nil {
				m.logger.Warn("Failed to resume frame processing for camera",
					zap.String("camera_id", cameraID),
					zap.Error(err),
				)
				// Transition to error state if we can't resume
				_ = cameraSM.Transition(types.CameraStateError, fmt.Sprintf("Failed to resume frame processing: %v", err))
			}
		} else if cameraState == types.CameraStateModelDeployed {
			m.logger.Info("Recovering model deployment workflow for camera",
				zap.String("camera_id", cameraID),
				zap.String("model_id", stateInfo.ModelID),
			)
			// Model was deployed, start frame processing
			if err := m.startFrameProcessingForCamera(ctx, cameraID); err != nil {
				m.logger.Warn("Failed to start frame processing for camera after recovery",
					zap.String("camera_id", cameraID),
					zap.Error(err),
				)
			} else {
				// Transition to frame_processing if successful
				if cameraSM.CanTransition(types.CameraStateFrameProcessing) {
					_ = cameraSM.Transition(types.CameraStateFrameProcessing, "")
				}
			}
		} else if cameraState == types.CameraStateError {
			m.logger.Info("Recovering from camera error state",
				zap.String("camera_id", cameraID),
				zap.String("error", stateInfo.Error),
			)
			// Camera was in error state, attempt recovery by transitioning to discovered
			// This allows the camera to go through the normal workflow again
			if cameraSM.CanTransition(types.CameraStateDiscovered) {
				_ = cameraSM.Transition(types.CameraStateDiscovered, "")
				m.logger.Info("Camera error state cleared, reset to discovered",
					zap.String("camera_id", cameraID),
				)
			}
		} else if cameraState == types.CameraStateScreenshotSetReady {
			m.logger.Info("Recovering screenshot set ready workflow for camera",
				zap.String("camera_id", cameraID),
				zap.String("dataset_id", stateInfo.DatasetID),
			)
			// Screenshot set was ready, check if we need to sync to VM
			if m.vmGateway != nil && m.vmGateway.IsConnected() {
				m.wg.Add(1)
				go func() {
					defer m.wg.Done()
					m.syncScreenshotsToVM(ctx, cameraID)
				}()
			}
		}
	}

	m.logger.Info("Active workflows recovery completed",
		zap.String("connection_state", string(connectionState)),
		zap.Int("cameras_recovered", len(allCameraSMs)),
	)
}

// SyncCameraCapabilities syncs capabilities for a single camera to the VM
func (m *StateManagerImpl) SyncCameraCapabilities(ctx context.Context, cameraID string) error {
	if m.vmGateway == nil {
		return fmt.Errorf("VM gateway not available")
	}

	if m.vmGateway == nil || !m.vmGateway.IsTransportConnected() {
		return fmt.Errorf("transport not connected")
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

	// Build sync request with single device (camera)
	req := &vmgatewaytypes.SyncCapabilitiesRequest{
		SyncedAt: time.Now().UnixNano(),
		Devices:  []*vmgatewaytypes.DeviceCapability{m.toDeviceCapability(cam, status)},
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
			m.pendingSyncMu.RLock()
			shouldRetry := m.pendingSync
			m.pendingSyncMu.RUnlock()
			if shouldRetry {
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
	// Check transport connection first
	if m.vmGateway == nil || !m.vmGateway.IsTransportConnected() {
		m.logger.Debug("Skipping capability sync - transport not connected (will retry when connection is ready)")
		m.pendingSyncMu.Lock()
		m.pendingSync = true // Mark as pending so we retry when connection is ready
		m.pendingSyncMu.Unlock()
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
		m.pendingSyncMu.Lock()
		m.pendingSync = false
		m.pendingSyncMu.Unlock()
		return
	}

	m.logger.Debug("Starting capability sync",
		zap.Int("camera_count", len(cameras)),
	)

	req := &vmgatewaytypes.SyncCapabilitiesRequest{
		SyncedAt: time.Now().UnixNano(),
		Devices:  []*vmgatewaytypes.DeviceCapability{},
	}

	// Track which cameras have changed since last sync
	now := time.Now()
	changedCameras := make(map[string]bool)

	for _, cam := range cameras {
		// Check if camera has changed since last sync
		m.capabilitySyncMu.RLock()
		lastSync, hasLastSync := m.lastCapabilitySync[cam.ID]
		m.capabilitySyncMu.RUnlock()

		// Sync if:
		// 1. Never synced before
		// 2. Last sync was more than syncInterval ago
		// 3. Camera metadata changed (we'll sync all for now, can be optimized later)
		shouldSync := !hasLastSync || now.Sub(lastSync) >= m.syncInterval

		if shouldSync {
			status := m.buildDatasetStatus(ctx, cam)
			if status != nil {
				req.Devices = append(req.Devices, m.toDeviceCapability(cam, status))
				changedCameras[cam.ID] = true
			}
		}
	}

	if len(req.Devices) == 0 {
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if m.vmGateway == nil {
		m.logger.Debug("Skipping capability sync - VM gateway not available")
		m.pendingSyncMu.Lock()
		m.pendingSync = true
		m.pendingSyncMu.Unlock()
		return
	}

	resp, err := m.vmGateway.SyncCapabilities(callCtx, req)
	if err != nil {
		// Check if it's an authentication error - if so, mark as pending and retry later
		if isAuthError(err) {
			m.logger.Debug("Capability sync failed - authentication not ready, will retry",
				zap.Error(err),
			)
			m.pendingSyncMu.Lock()
			m.pendingSync = true
			m.pendingSyncMu.Unlock()
			return
		}
		m.logger.Warn("Capability sync failed",
			zap.Error(err),
		)
		m.pendingSyncMu.Lock()
		m.pendingSync = true // Retry on other errors too
		m.pendingSyncMu.Unlock()
		return
	}

	if !resp.Success {
		// Check if it's an authentication-related rejection
		if resp.ErrorMessage != "" && (contains(resp.ErrorMessage, "not registered") || contains(resp.ErrorMessage, "authentication")) {
			m.logger.Debug("Capability sync rejected - authentication not ready, will retry",
				zap.String("error", resp.ErrorMessage),
			)
			m.pendingSyncMu.Lock()
			m.pendingSync = true
			m.pendingSyncMu.Unlock()
			return
		}
		m.logger.Info("Capability sync rejected",
			zap.String("error", resp.ErrorMessage),
		)
		m.pendingSyncMu.Lock()
		m.pendingSync = true
		m.pendingSyncMu.Unlock()
		return
	}

	// Success - clear pending flag and update last sync timestamps
	m.pendingSyncMu.Lock()
	m.pendingSync = false
	m.pendingSyncMu.Unlock()

	// Update last sync timestamp for successfully synced cameras
	if len(changedCameras) > 0 {
		m.capabilitySyncMu.Lock()
		for cameraID := range changedCameras {
			m.lastCapabilitySync[cameraID] = now
		}
		m.capabilitySyncMu.Unlock()
	}

	m.logger.Info("Capability sync sent successfully",
		zap.Int("devices", len(req.Devices)),
		zap.Int("changed_devices", len(changedCameras)),
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

// toDeviceCapability converts a camera and dataset status to a device-agnostic DTO format
// used for capability sync over HTTPS.
// Note: Currently used for cameras, but designed to be device-agnostic for future IoT device support.
func (m *StateManagerImpl) toDeviceCapability(cam *cctvtypes.Camera, status *cctvtypes.DatasetStatus) *vmgatewaytypes.DeviceCapability {
	labelCounts := make(map[string]uint32, len(status.LabelCounts))
	for label, count := range status.LabelCounts {
		if count < 0 {
			continue
		}
		labelCounts[label] = uint32(count)
	}

	return &vmgatewaytypes.DeviceCapability{
		DeviceID:              cam.ID,
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
	m.operationalMu.RLock()
	config := m.config
	edgeID := m.edgeID
	m.operationalMu.RUnlock()

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
	m.operationalMu.RLock()
	httpClient := m.httpClient
	vmEndpoint := m.vmEndpoint
	m.operationalMu.RUnlock()

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

	if !m.vmGateway.IsTransportConnected() {
		// Queue for retry when connection is available
		m.logger.Debug("Transport not connected, deployment status will be retried when connection is available",
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
