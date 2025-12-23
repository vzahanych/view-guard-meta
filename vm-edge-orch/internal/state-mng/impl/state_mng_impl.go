package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	edgegateway "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway"
	eventbus "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/event-bus"
	metastorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage"
	statemng "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/state-mng"
)

// Known edge-related event types used by the state manager.
const (
	EventTypeWireGuardConnected    eventbus.EventType = "network.wireguard.connected"
	EventTypeWireGuardDisconnected eventbus.EventType = "network.wireguard.disconnected"
	EventTypeHTTPSConnected        eventbus.EventType = "network.https.connected"
	EventTypeHTTPSDisconnected     eventbus.EventType = "network.https.disconnected"
	EventTypeEdgeAuthenticated     eventbus.EventType = "edge.authenticated"
	EventTypeIOTSynced             eventbus.EventType = "edge.iot.synced"
	EventTypeIOTTrainDataReady     eventbus.EventType = "edge.iot.train_data_ready"
	EventTypeIOTModelTrained       eventbus.EventType = "edge.iot.model_trained"
	EventTypeIOTModelDeployed      eventbus.EventType = "edge.iot.model_deployed"
)

// stateManager implements the StateManager interface.
type stateManager struct {
	eventBus eventbus.EventBus
	store    metastorage.MetaDataStore
	logger   *zap.Logger

	// edgeGateway is used to make requests to Edge devices (IoT device sync, etc.)
	edgeGateway interface{} // edgegateway.EdgeGateway

	// edgeIPResolver provides thread-safe Edge IP address resolution
	edgeIPResolver *EdgeIPResolver

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once
}

// NewStateManager creates a new StateManager implementation.
//
// - bus: application-wide event bus implementation (in-memory, NATS, etc.)
// - store: meta-storage implementation (edge state, events, datasets, etc.)
// - log: optional zap.Logger (if nil, a development logger is created)
// - edgeGateway: optional EdgeGateway for making requests to Edge devices (IoT sync, etc.)
func NewStateManager(bus eventbus.EventBus, store metastorage.MetaDataStore, log *zap.Logger, edgeGateway interface{}) (statemng.StateManager, error) {
	if bus == nil {
		return nil, fmt.Errorf("event bus is required")
	}
	if store == nil {
		return nil, fmt.Errorf("meta-storage is required")
	}

	var logger *zap.Logger
	if log != nil {
		logger = log
	} else {
		var err error
		logger, err = zap.NewDevelopment()
		if err != nil {
			return nil, fmt.Errorf("failed to create default logger: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create Edge IP resolver if gateway is available
	var edgeIPResolver *EdgeIPResolver
	if edgeGateway != nil {
		if gateway, ok := edgeGateway.(edgegateway.EdgeGateway); ok {
			edgeIPResolver = NewEdgeIPResolver(gateway)
		}
	}

	return &stateManager{
		eventBus:       bus,
		store:          store,
		logger:         logger,
		edgeGateway:    edgeGateway,
		edgeIPResolver: edgeIPResolver,
		ctx:            ctx,
		cancel:         cancel,
	}, nil
}

// Name returns the service name.
func (m *stateManager) Name() string {
	return "state-manager"
}

// Start begins processing events from the event bus.
func (m *stateManager) Start(ctx context.Context) error {
	var startErr error
	m.startOnce.Do(func() {
		m.logger.Info("Starting state manager, subscribing to edge events")

		events := m.eventBus.SubscribeAll()

		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.run(events)
		}()
	})

	return startErr
}

// Stop stops processing events and waits for in-flight tasks to complete.
func (m *stateManager) Stop(ctx context.Context) error {
	var stopErr error
	m.stopOnce.Do(func() {
		m.logger.Info("Stopping state manager")
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

		m.logger.Info("State manager stopped")
	})

	return stopErr
}

// run listens for events from the event bus and processes them.
func (m *stateManager) run(events <-chan eventbus.Event) {
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
func (m *stateManager) handleEvent(ev eventbus.Event) {
	edgeIDStr, ok := ev.Data["edge_id"].(string)
	if !ok || edgeIDStr == "" {
		// Not an edge event we understand.
		return
	}

	edgeUUID, err := uuid.Parse(edgeIDStr)
	if err != nil {
		m.logger.Warn("state-manager: invalid edge_id in event",
			zap.String("edge_id", edgeIDStr),
			zap.String("event_type", string(ev.Type)),
			zap.Error(err))
		return
	}

	m.logger.Debug("state-manager: handling edge event",
		zap.String("event_type", string(ev.Type)),
		zap.String("edge_id", edgeUUID.String()))

	newState, err := m.updateEdgeStateForEvent(m.ctx, edgeUUID, ev)
	if err != nil {
		m.logger.Warn("state-manager: failed to update edge state for event",
			zap.String("edge_id", edgeUUID.String()),
			zap.String("event_type", string(ev.Type)),
			zap.Error(err))
		return
	}

	// Schedule next task based on the new state in a separate goroutine.
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.executeNextTask(m.ctx, edgeUUID, newState)
	}()
}

// updateEdgeStateForEvent updates the edge state in meta-storage based on the event.
func (m *stateManager) updateEdgeStateForEvent(ctx context.Context, edgeID uuid.UUID, ev eventbus.Event) (metastorage.EdgeState, error) {
	updated, err := m.store.UpdateEdge(edgeID, func(s metastorage.EdgeState) metastorage.EdgeState {
		// If this is a new edge, initialize basic state.
		if s.UUID == uuid.Nil {
			s.UUID = edgeID
			now := time.Now()
			s.CreatedAt = now
			s.UpdatedAt = now
			if s.Metadata == nil {
				s.Metadata = make(map[string]string)
			}
		}

		// Update status based on event type.
		switch ev.Type {
		case EventTypeWireGuardConnected:
			s.Status = metastorage.EdgeStatusConnected
		case EventTypeWireGuardDisconnected:
			s.Status = metastorage.EdgeStatusError
		case EventTypeHTTPSConnected:
			s.Status = metastorage.EdgeStatusHTTPSConnected
		case EventTypeHTTPSDisconnected:
			s.Status = metastorage.EdgeStatusConnected
		case EventTypeEdgeAuthenticated:
			s.Status = metastorage.EdgeStatusAuthenticated
		case EventTypeIOTSynced:
			s.Status = metastorage.EdgeStatusIOTSynced
		case EventTypeIOTTrainDataReady:
			s.Status = metastorage.EdgeStatusIOTTrainDataSynced
		case EventTypeIOTModelTrained:
			s.Status = metastorage.EdgeStatusIOTTrainModelTrained
		case EventTypeIOTModelDeployed:
			s.Status = metastorage.EdgeStatusIOTTrainModelDeployed
		default:
			// Unknown event; leave status unchanged.
		}

		// Record last event type and timestamp in metadata for debugging.
		if s.Metadata == nil {
			s.Metadata = make(map[string]string)
		}

		s.Metadata["last_event_type"] = string(ev.Type)
		s.Metadata["last_event_source"] = ev.Source
		s.UpdatedAt = time.Now()

		return s
	})

	if err != nil {
		return metastorage.EdgeState{}, err
	}

	return updated, nil
}

// executeNextTask determines the next task for the edge based on its state
// and executes it asynchronously. For now, this is a set of placeholders
// that log intent; concrete task implementations can be plugged in later.
func (m *stateManager) executeNextTask(ctx context.Context, edgeID uuid.UUID, state metastorage.EdgeState) {
	switch state.Status {
	case metastorage.EdgeStatusConnected:
		m.logger.Info("state-manager: edge connected via WireGuard; waiting for HTTPS and authentication",
			zap.String("edge_id", edgeID.String()))
	case metastorage.EdgeStatusHTTPSConnected:
		m.logger.Info("state-manager: edge HTTPS connected; waiting for authentication",
			zap.String("edge_id", edgeID.String()))
	case metastorage.EdgeStatusAuthenticated:
		m.logger.Info("state-manager: edge authenticated; requesting IoT device sync",
			zap.String("edge_id", edgeID.String()))
		// Request Edge to sync IoT devices (cameras)
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := m.requestIoTDeviceSync(ctx, edgeID, state); err != nil {
				m.logger.Warn("state-manager: failed to request IoT device sync",
					zap.String("edge_id", edgeID.String()),
					zap.Error(err))
			}
		}()
	case metastorage.EdgeStatusIOTSynced:
		m.logger.Info("state-manager: IoT capabilities synced; ready to request training data",
			zap.String("edge_id", edgeID.String()))
		// TODO: Trigger dataset collection / training-data request task.
	case metastorage.EdgeStatusIOTTrainDataSynced:
		m.logger.Info("state-manager: training data synced; should trigger model training",
			zap.String("edge_id", edgeID.String()))
		// TODO: Trigger model training task.
	case metastorage.EdgeStatusIOTTrainModelTrained:
		m.logger.Info("state-manager: model trained; should trigger deployment",
			zap.String("edge_id", edgeID.String()))
		// TODO: Trigger model deployment task.
	case metastorage.EdgeStatusIOTTrainModelDeployed:
		m.logger.Info("state-manager: model deployed; steady-state monitoring",
			zap.String("edge_id", edgeID.String()))
	case metastorage.EdgeStatusError:
		m.logger.Warn("state-manager: edge in error state; consider emitting alert",
			zap.String("edge_id", edgeID.String()))
	default:
		m.logger.Debug("state-manager: no task for current edge state",
			zap.String("edge_id", edgeID.String()),
			zap.String("status", string(state.Status)))
	}

	// For now, tasks are just log messages. When concrete tasks are defined,
	// they can use ctx for cancellation and respect deadlines.
	_ = ctx
}

// requestIoTDeviceSync requests the Edge device to discover and sync IoT devices (cameras).
func (m *stateManager) requestIoTDeviceSync(ctx context.Context, edgeID uuid.UUID, state metastorage.EdgeState) error {
	if m.edgeIPResolver == nil {
		m.logger.Debug("state-manager: edge IP resolver not available, skipping IoT device sync",
			zap.String("edge_id", edgeID.String()))
		return nil
	}

	// Get Edge's WireGuard public key from state
	if state.WGPublicKey == "" {
		return fmt.Errorf("edge WireGuard public key not found in state")
	}

	// Resolve Edge's IP address using the helper function
	edgeIP, err := m.edgeIPResolver.GetEdgeIP(state.WGPublicKey)
	if err != nil {
		return fmt.Errorf("failed to resolve edge IP address: %w", err)
	}

	// Construct Edge API URL (use HTTP in dev mode, HTTPS in prod)
	// For now, we'll use HTTPS and let the client handle TLS
	edgeURL := fmt.Sprintf("https://%s:8443/api/v1/iot/devices", edgeIP)

	if m.logger != nil {
		m.logger.Info("state-manager: requesting IoT devices from Edge",
			zap.String("edge_id", edgeID.String()),
			zap.String("edge_ip", edgeIP),
			zap.String("url", edgeURL))
	}

	// Get HTTPS client service from edge gateway
	if m.edgeGateway == nil {
		return fmt.Errorf("edge gateway not available")
	}

	gateway, ok := m.edgeGateway.(edgegateway.EdgeGateway)
	if !ok {
		return fmt.Errorf("edge gateway does not implement EdgeGateway interface")
	}

	httpsClientInterface := gateway.GetHTTPSClientService()
	if httpsClientInterface == nil {
		return fmt.Errorf("https client service not available")
	}

	// For now, create a simple HTTP client to make the request
	// TODO: Use the HTTPS client service's HTTP client properly
	// The HTTPS client service should expose a method to make requests
	client := &http.Client{
		Timeout: 10 * time.Second,
		// In dev mode, we might need to skip TLS verification
		// This should be configured based on environment
	}

	req, err := http.NewRequestWithContext(ctx, "GET", edgeURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add edge_id header for identification
	req.Header.Set("X-Edge-ID", edgeID.String())

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to request IoT devices: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && m.logger != nil {
			m.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("edge returned status %d", resp.StatusCode)
	}

	// Parse response
	var devicesResponse struct {
		Devices []metastorage.IoTDevice `json:"devices"`
	}

	if decodeErr := json.NewDecoder(resp.Body).Decode(&devicesResponse); decodeErr != nil {
		return fmt.Errorf("failed to decode IoT devices response: %w", decodeErr)
	}

	// Update edge state with discovered IoT devices
	_, err = m.store.UpdateEdge(edgeID, func(s metastorage.EdgeState) metastorage.EdgeState {
		s.Devices = devicesResponse.Devices
		return s
	})

	if err != nil {
		return fmt.Errorf("failed to update edge state with IoT devices: %w", err)
	}

	// Publish IoT devices synced event
	if m.eventBus != nil {
		m.eventBus.Publish(eventbus.Event{
			Type:      EventTypeIOTSynced,
			Source:    "state-manager",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"edge_id":      edgeID.String(),
				"device_count": len(devicesResponse.Devices),
			},
		})
	}

	if m.logger != nil {
		m.logger.Info("state-manager: successfully synced IoT devices from Edge",
			zap.String("edge_id", edgeID.String()),
			zap.Int("device_count", len(devicesResponse.Devices)))
	}

	return nil
}
