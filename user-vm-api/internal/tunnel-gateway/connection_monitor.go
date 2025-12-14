package tunnelgateway

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/service"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// ConnectionState represents the state of an Edge connection
type ConnectionState string

const (
	StateRegistered   ConnectionState = "registered"   // Edge is registered but not yet connected
	StateConnecting   ConnectionState = "connecting"   // Edge is attempting to connect
	StateConnected    ConnectionState = "connected"    // Edge is actively connected
	StateDisconnected ConnectionState = "disconnected" // Edge is disconnected
	StateStale        ConnectionState = "stale"        // Edge connection is stale (no recent activity)
	StateReconnecting ConnectionState = "reconnecting" // Edge is attempting to reconnect
)

// ConnectionMonitor continuously monitors Edge connection status
type ConnectionMonitor struct {
	config        *config.Config
	logger        *logging.Logger
	db            *database.DB
	edgeAPIServer *EdgeAPIServer
	wgServer      *WireGuardServer
	edgeClient    *EdgeClient // For active gRPC health checks (VM → Edge)
	eventBus      *service.EventBus

	// Connection state tracking
	states   map[string]*ConnectionStateInfo // edge_id -> state info
	statesMu sync.RWMutex

	// Monitoring intervals
	checkInterval     time.Duration // How often to check connections
	heartbeatTimeout  time.Duration // Timeout for heartbeat (5 minutes)
	staleThreshold    time.Duration // Threshold for stale connections (10 minutes)
	reconnectInterval time.Duration // Interval between reconnection attempts
	keepaliveInterval time.Duration // Interval for keepalive pings (30 seconds)

	// Reconnection tracking
	reconnectBackoffs           map[string]time.Duration // edge_id -> current backoff duration
	reconnectMu                 sync.RWMutex
	maxRetries                  int           // Maximum reconnection attempts before giving up (default: 10)
	extendedDisconnectThreshold time.Duration // Threshold for extended disconnection alert (default: 30 minutes)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ConnectionStateInfo tracks detailed state information for an Edge
type ConnectionStateInfo struct {
	EdgeID               string
	State                ConnectionState
	LastSeen             time.Time
	LastHeartbeat        time.Time
	LastHandshake        time.Time
	ConnectionCount      int64
	ReconnectAttempts    int
	LastReconnectAttempt time.Time
	Latency              time.Duration
	BytesReceived        uint64
	BytesSent            uint64
	StateChangedAt       time.Time

	// Health metrics
	FirstConnectedAt     time.Time     // First time connection was established
	LastConnectedAt      time.Time     // Last time connection was established
	TotalUptime          time.Duration // Cumulative uptime
	TotalDowntime        time.Duration // Cumulative downtime
	CurrentSessionStart  time.Time     // Start of current connected session
	CurrentSessionUptime time.Duration // Uptime of current session

	// gRPC call metrics
	GRPCCallCount    int64     // Total gRPC calls
	GRPCSuccessCount int64     // Successful gRPC calls
	GRPCFailureCount int64     // Failed gRPC calls
	LastGRPCCallTime time.Time // Last gRPC call timestamp

	// Packet loss metrics (calculated from ping/pong)
	PingCount          uint64    // Total pings sent
	PongCount          uint64    // Total pongs received
	LastPacketLossCalc time.Time // Last time packet loss was calculated

	mu sync.RWMutex
}

// NewConnectionMonitor creates a new connection monitor service
func NewConnectionMonitor(
	cfg *config.Config,
	log *logging.Logger,
	db *database.DB,
	edgeAPIServer *EdgeAPIServer,
	wgServer *WireGuardServer,
	edgeClient *EdgeClient, // For active gRPC health checks (VM → Edge)
) *ConnectionMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &ConnectionMonitor{
		config:                      cfg,
		logger:                      log,
		db:                          db,
		edgeAPIServer:               edgeAPIServer,
		wgServer:                    wgServer,
		edgeClient:                  edgeClient,
		states:                      make(map[string]*ConnectionStateInfo),
		checkInterval:               30 * time.Second,
		heartbeatTimeout:            5 * time.Minute,
		staleThreshold:              10 * time.Minute,
		reconnectInterval:           30 * time.Second,
		keepaliveInterval:           30 * time.Second,
		reconnectBackoffs:           make(map[string]time.Duration),
		maxRetries:                  10,               // Maximum 10 reconnection attempts
		extendedDisconnectThreshold: 30 * time.Minute, // Alert after 30 minutes of disconnection
		ctx:                         ctx,
		cancel:                      cancel,
	}
}

// SetEventBus sets the event bus for publishing events
func (cm *ConnectionMonitor) SetEventBus(bus *service.EventBus) {
	cm.eventBus = bus
}

// Start starts the connection monitor service
func (cm *ConnectionMonitor) Start(ctx context.Context) error {
	cm.logger.Info("Starting connection monitor service")

	// Load all registered Edges from database
	if err := cm.loadRegisteredEdges(); err != nil {
		return fmt.Errorf("failed to load registered edges: %w", err)
	}

	// Start monitoring goroutine
	cm.wg.Add(1)
	go cm.monitorLoop(ctx)

	// Start keepalive goroutine
	cm.wg.Add(1)
	go cm.keepaliveLoop(ctx)

	cm.logger.Info("Connection monitor service started")
	return nil
}

// Stop stops the connection monitor service
func (cm *ConnectionMonitor) Stop(ctx context.Context) error {
	cm.logger.Info("Stopping connection monitor service")
	cm.cancel()
	cm.wg.Wait()
	cm.logger.Info("Connection monitor service stopped")
	return nil
}

// Name returns the service name
func (cm *ConnectionMonitor) Name() string {
	return "connection-monitor"
}

// loadRegisteredEdges loads all registered Edges from the database
func (cm *ConnectionMonitor) loadRegisteredEdges() error {
	rows, err := cm.db.QueryContext(context.Background(),
		"SELECT edge_id, wireguard_public_key, last_seen, status FROM edges WHERE status = 'active'")
	if err != nil {
		return fmt.Errorf("failed to query edges: %w", err)
	}
	defer rows.Close()

	cm.statesMu.Lock()
	defer cm.statesMu.Unlock()

	for rows.Next() {
		var edgeID, publicKey, dbStatus string
		var lastSeen int64

		if err := rows.Scan(&edgeID, &publicKey, &lastSeen, &dbStatus); err != nil {
			cm.logger.Warn("Failed to scan edge row", zap.Error(err))
			continue
		}

		// Initialize state info for this Edge
		stateInfo := &ConnectionStateInfo{
			EdgeID:         edgeID,
			State:          StateRegistered,
			LastSeen:       time.Unix(lastSeen, 0),
			StateChangedAt: time.Now(),
		}

		cm.states[edgeID] = stateInfo
		cm.logger.Debug("Loaded registered edge", zap.String("edge_id", edgeID))
	}

	return nil
}

// monitorLoop is the main monitoring loop
func (cm *ConnectionMonitor) monitorLoop(ctx context.Context) {
	defer cm.wg.Done()

	ticker := time.NewTicker(cm.checkInterval)
	defer ticker.Stop()

	// Initial check
	cm.checkAllConnections()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.checkAllConnections()
		}
	}
}

// checkAllConnections checks the status of all registered Edges
func (cm *ConnectionMonitor) checkAllConnections() {
	// Get all registered Edges from database
	rows, err := cm.db.QueryContext(context.Background(),
		"SELECT edge_id, wireguard_public_key, last_seen, status FROM edges WHERE status = 'active'")
	if err != nil {
		cm.logger.Error("Failed to query edges", zap.Error(err))
		return
	}
	defer rows.Close()

	edgeIDs := make(map[string]bool)

	// Check each registered Edge
	for rows.Next() {
		var edgeID, publicKey, dbStatus string
		var lastSeen int64

		if err := rows.Scan(&edgeID, &publicKey, &lastSeen, &dbStatus); err != nil {
			cm.logger.Warn("Failed to scan edge row", zap.Error(err))
			continue
		}

		edgeIDs[edgeID] = true
		cm.checkEdgeConnection(edgeID, publicKey, time.Unix(lastSeen, 0))
	}

	// Remove Edges that are no longer in the database
	cm.statesMu.Lock()
	for edgeID := range cm.states {
		if !edgeIDs[edgeID] {
			delete(cm.states, edgeID)
			cm.logger.Debug("Removed edge from monitoring", zap.String("edge_id", edgeID))
		}
	}
	cm.statesMu.Unlock()
}

// checkEdgeConnection checks the connection status for a specific Edge
func (cm *ConnectionMonitor) checkEdgeConnection(edgeID, publicKey string, dbLastSeen time.Time) {
	now := time.Now()

	// Get current state info
	cm.statesMu.Lock()
	stateInfo, exists := cm.states[edgeID]
	if !exists {
		stateInfo = &ConnectionStateInfo{
			EdgeID:         edgeID,
			State:          StateRegistered,
			LastSeen:       dbLastSeen,
			StateChangedAt: now,
		}
		cm.states[edgeID] = stateInfo
	}
	cm.statesMu.Unlock()

	stateInfo.mu.Lock()
	defer stateInfo.mu.Unlock()

	// Check gRPC connection status
	grpcConn, grpcConnected := cm.edgeAPIServer.GetConnection(edgeID)
	var lastHeartbeat time.Time
	if grpcConnected && grpcConn != nil {
		grpcConn.mu.RLock()
		lastHeartbeat = grpcConn.LastHeartbeat
		grpcConn.mu.RUnlock()
		stateInfo.LastHeartbeat = lastHeartbeat
	}

	// Check WireGuard peer status
	wgPeer, wgConnected := cm.getWireGuardPeer(publicKey)
	var lastHandshake time.Time
	if wgConnected && wgPeer != nil {
		wgPeer.mu.RLock()
		lastHandshake = wgPeer.LastHandshake
		stateInfo.Latency = wgPeer.Latency
		stateInfo.BytesReceived = wgPeer.BytesReceived
		stateInfo.BytesSent = wgPeer.BytesSent
		wgPeer.mu.RUnlock()
		stateInfo.LastHandshake = lastHandshake
	}

	// Active gRPC health check: VM monitors Edge status through gRPC (security requirement)
	// This verifies the bidirectional gRPC connection is alive and ready
	// All VM-Edge communication is gRPC-only (no HTTP)
	if cm.edgeClient != nil && (grpcConnected || wgConnected) {
		healthCtx, healthCancel := context.WithTimeout(context.Background(), 10*time.Second)
		grpcHealthErr := cm.edgeClient.VerifyConnectionHealth(healthCtx, edgeID)
		healthCancel()

		if grpcHealthErr != nil {
			cm.logger.Warn("gRPC health check failed for Edge",
				zap.String("edge_id", edgeID),
				zap.Error(grpcHealthErr),
				zap.String("note", "Bidirectional gRPC connection health is critical for security"))
			// Mark gRPC as not healthy even if we have a connection object
			grpcConnected = false
		} else {
			cm.logger.Debug("gRPC health check passed for Edge",
				zap.String("edge_id", edgeID),
				zap.String("note", "Bidirectional gRPC connection is alive and ready"))
			stateInfo.LastGRPCCallTime = now
			stateInfo.GRPCCallCount++
			stateInfo.GRPCSuccessCount++
		}
	}

	// Determine new state based on connection status
	newState := cm.determineState(stateInfo.State, grpcConnected, wgConnected, lastHeartbeat, lastHandshake, now)

	// Update state if changed
	if newState != stateInfo.State {
		oldState := stateInfo.State
		stateInfo.State = newState
		stateInfo.StateChangedAt = now

		// Update uptime/downtime metrics
		cm.updateUptimeDowntime(stateInfo, oldState, newState, now)

		// Update database
		cm.updateEdgeStatus(edgeID, string(newState), now)

		// Publish state change event
		cm.publishStateChange(edgeID, oldState, newState)

		cm.logger.Info("Edge connection state changed",
			zap.String("edge_id", edgeID),
			zap.String("old_state", string(oldState)),
			zap.String("new_state", string(newState)),
			zap.Time("last_heartbeat", lastHeartbeat),
			zap.Time("last_handshake", lastHandshake),
		)
	}

	// Update current session uptime if connected
	if newState == StateConnected {
		if !stateInfo.CurrentSessionStart.IsZero() {
			stateInfo.CurrentSessionUptime = now.Sub(stateInfo.CurrentSessionStart)
		}
	}

	// Update ping/pong counts from WireGuard peer
	if wgPeer != nil {
		wgPeer.mu.RLock()
		stateInfo.PingCount = uint64(wgPeer.PingCount)
		stateInfo.PongCount = uint64(wgPeer.PongCount)
		wgPeer.mu.RUnlock()
	}

	// Update last_seen in database if connected
	if newState == StateConnected {
		cm.updateEdgeLastSeen(edgeID, now)
	}

	// Check for extended disconnection and alert if needed
	if newState == StateDisconnected || newState == StateStale {
		disconnectDuration := now.Sub(stateInfo.StateChangedAt)
		if disconnectDuration >= cm.extendedDisconnectThreshold {
			// Check if we've already alerted for this extended disconnection
			// (to avoid spamming alerts)
			lastAlertTime := stateInfo.LastReconnectAttempt
			if lastAlertTime.IsZero() || now.Sub(lastAlertTime) >= cm.extendedDisconnectThreshold {
				cm.publishConnectionFailed(edgeID, stateInfo.ReconnectAttempts)
				// Update last alert time (reusing LastReconnectAttempt field for this)
				stateInfo.LastReconnectAttempt = now
			}
		}
	}
}

// determineState determines the connection state based on various factors
func (cm *ConnectionMonitor) determineState(
	currentState ConnectionState,
	grpcConnected bool,
	wgConnected bool,
	lastHeartbeat time.Time,
	lastHandshake time.Time,
	now time.Time,
) ConnectionState {
	// If both gRPC and WireGuard are connected and recent, state is connected
	if grpcConnected && wgConnected {
		heartbeatAge := now.Sub(lastHeartbeat)
		handshakeAge := now.Sub(lastHandshake)

		// Check if heartbeat is recent (within timeout)
		if !lastHeartbeat.IsZero() && heartbeatAge < cm.heartbeatTimeout {
			// Check if handshake is recent (within stale threshold)
			if !lastHandshake.IsZero() && handshakeAge < cm.staleThreshold {
				return StateConnected
			}
			// Handshake is stale but heartbeat is recent
			return StateStale
		}
		// Heartbeat is stale
		if heartbeatAge >= cm.heartbeatTimeout && heartbeatAge < cm.staleThreshold {
			return StateStale
		}
		// Both are very stale
		return StateDisconnected
	}

	// If WireGuard is connected but gRPC is not, state is connecting
	if wgConnected && !grpcConnected {
		if currentState == StateDisconnected || currentState == StateStale {
			return StateReconnecting
		}
		return StateConnecting
	}

	// If neither is connected
	if !grpcConnected && !wgConnected {
		if currentState == StateConnected || currentState == StateStale {
			return StateDisconnected
		}
		if currentState == StateReconnecting {
			// Check if we should continue reconnecting or give up
			return StateReconnecting
		}
		return StateDisconnected
	}

	// Default: maintain current state or go to registered
	if currentState == StateRegistered || currentState == StateConnecting {
		return currentState
	}

	return StateDisconnected
}

// getWireGuardPeer gets WireGuard peer information by public key
func (cm *ConnectionMonitor) getWireGuardPeer(publicKey string) (*PeerInfo, bool) {
	if cm.wgServer == nil {
		return nil, false
	}

	// Parse public key
	key, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return nil, false
	}

	// Get peer from WireGuard server
	cm.wgServer.mu.RLock()
	defer cm.wgServer.mu.RUnlock()

	peer, exists := cm.wgServer.peers[key.String()]
	return peer, exists
}

// updateEdgeStatus updates the Edge status in the database
func (cm *ConnectionMonitor) updateEdgeStatus(edgeID string, status string, timestamp time.Time) {
	_, err := cm.db.ExecContext(context.Background(),
		"UPDATE edges SET status = ?, last_seen = ?, updated_at = ? WHERE edge_id = ?",
		status, timestamp.Unix(), timestamp.Unix(), edgeID)
	if err != nil {
		cm.logger.Warn("Failed to update edge status",
			zap.String("edge_id", edgeID),
			zap.String("status", status),
			zap.Error(err),
		)
	}
}

// updateEdgeLastSeen updates the last_seen timestamp for an Edge
func (cm *ConnectionMonitor) updateEdgeLastSeen(edgeID string, timestamp time.Time) {
	_, err := cm.db.ExecContext(context.Background(),
		"UPDATE edges SET last_seen = ?, updated_at = ? WHERE edge_id = ?",
		timestamp.Unix(), timestamp.Unix(), edgeID)
	if err != nil {
		cm.logger.Warn("Failed to update edge last_seen",
			zap.String("edge_id", edgeID),
			zap.Error(err),
		)
	}
}

// publishStateChange publishes a state change event
func (cm *ConnectionMonitor) publishStateChange(edgeID string, oldState, newState ConnectionState) {
	if cm.eventBus == nil {
		return
	}

	var eventType service.EventType = "edge.state_changed"
	if newState == StateDisconnected {
		eventType = "edge.disconnected"
	} else if newState == StateConnected && oldState != StateConnected {
		eventType = "edge.connected"
	} else if newState == StateReconnecting {
		eventType = "edge.reconnecting"
	}

	cm.eventBus.Publish(service.Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"edge_id":   edgeID,
			"old_state": string(oldState),
			"new_state": string(newState),
		},
	})
}

// updateUptimeDowntime updates uptime and downtime metrics when state changes
func (cm *ConnectionMonitor) updateUptimeDowntime(stateInfo *ConnectionStateInfo, oldState, newState ConnectionState, now time.Time) {
	// Track first connection
	if newState == StateConnected && stateInfo.FirstConnectedAt.IsZero() {
		stateInfo.FirstConnectedAt = now
	}

	// Transitioning to connected
	if newState == StateConnected && oldState != StateConnected {
		stateInfo.LastConnectedAt = now
		stateInfo.CurrentSessionStart = now
		stateInfo.CurrentSessionUptime = 0

		// If we were disconnected, add to downtime
		if oldState == StateDisconnected || oldState == StateStale {
			if !stateInfo.LastConnectedAt.IsZero() && !stateInfo.StateChangedAt.IsZero() {
				downtime := now.Sub(stateInfo.StateChangedAt)
				stateInfo.TotalDowntime += downtime
			}
		}
	}

	// Transitioning from connected to disconnected/stale
	if (newState == StateDisconnected || newState == StateStale) && oldState == StateConnected {
		// Add current session uptime to total uptime
		if !stateInfo.CurrentSessionStart.IsZero() {
			sessionUptime := now.Sub(stateInfo.CurrentSessionStart)
			stateInfo.TotalUptime += sessionUptime
			stateInfo.CurrentSessionStart = time.Time{} // Reset
			stateInfo.CurrentSessionUptime = 0
		}
	}
}

// RecordGRPCCall records a gRPC call for metrics tracking
func (cm *ConnectionMonitor) RecordGRPCCall(edgeID string, success bool) {
	cm.statesMu.Lock()
	stateInfo, exists := cm.states[edgeID]
	if !exists {
		cm.statesMu.Unlock()
		return
	}
	cm.statesMu.Unlock()

	stateInfo.mu.Lock()
	defer stateInfo.mu.Unlock()

	stateInfo.GRPCCallCount++
	stateInfo.LastGRPCCallTime = time.Now()
	if success {
		stateInfo.GRPCSuccessCount++
	} else {
		stateInfo.GRPCFailureCount++
	}
}

// GetPacketLoss calculates packet loss percentage from ping/pong counts
func (cm *ConnectionMonitor) GetPacketLoss(edgeID string) float64 {
	cm.statesMu.RLock()
	stateInfo, exists := cm.states[edgeID]
	if !exists {
		cm.statesMu.RUnlock()
		return 0.0
	}
	cm.statesMu.RUnlock()

	stateInfo.mu.RLock()
	defer stateInfo.mu.RUnlock()

	if stateInfo.PingCount == 0 {
		return 0.0
	}

	// Packet loss = (pings - pongs) / pings * 100
	packetLoss := float64(stateInfo.PingCount-stateInfo.PongCount) / float64(stateInfo.PingCount) * 100.0
	if packetLoss < 0 {
		packetLoss = 0.0
	}
	if packetLoss > 100 {
		packetLoss = 100.0
	}
	return packetLoss
}

// publishConnectionFailed publishes a connection failed event for alerting
func (cm *ConnectionMonitor) publishConnectionFailed(edgeID string, reconnectAttempts int) {
	if cm.eventBus == nil {
		return
	}

	cm.statesMu.RLock()
	stateInfo, exists := cm.states[edgeID]
	var disconnectDuration time.Duration
	if exists {
		stateInfo.mu.RLock()
		disconnectDuration = time.Since(stateInfo.StateChangedAt)
		stateInfo.mu.RUnlock()
	}
	cm.statesMu.RUnlock()

	cm.logger.Warn("Edge connection failed - publishing alert",
		zap.String("edge_id", edgeID),
		zap.Int("reconnect_attempts", reconnectAttempts),
		zap.Duration("disconnect_duration", disconnectDuration),
	)

	cm.eventBus.Publish(service.Event{
		Type:      service.EventType("edge.connection_failed"),
		Timestamp: time.Now().Unix(),
		Data: map[string]interface{}{
			"edge_id":             edgeID,
			"reconnect_attempts":  reconnectAttempts,
			"disconnect_duration": disconnectDuration.String(),
			"max_retries":         cm.maxRetries,
		},
	})
}

// GetConnectionState returns the current connection state for an Edge
func (cm *ConnectionMonitor) GetConnectionState(edgeID string) (*ConnectionStateInfo, bool) {
	cm.statesMu.RLock()
	defer cm.statesMu.RUnlock()

	stateInfo, exists := cm.states[edgeID]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	stateInfo.mu.RLock()
	defer stateInfo.mu.RUnlock()

	return &ConnectionStateInfo{
		EdgeID:               stateInfo.EdgeID,
		State:                stateInfo.State,
		LastSeen:             stateInfo.LastSeen,
		LastHeartbeat:        stateInfo.LastHeartbeat,
		LastHandshake:        stateInfo.LastHandshake,
		ConnectionCount:      stateInfo.ConnectionCount,
		ReconnectAttempts:    stateInfo.ReconnectAttempts,
		LastReconnectAttempt: stateInfo.LastReconnectAttempt,
		Latency:              stateInfo.Latency,
		BytesReceived:        stateInfo.BytesReceived,
		BytesSent:            stateInfo.BytesSent,
		StateChangedAt:       stateInfo.StateChangedAt,
		FirstConnectedAt:     stateInfo.FirstConnectedAt,
		LastConnectedAt:      stateInfo.LastConnectedAt,
		TotalUptime:          stateInfo.TotalUptime,
		TotalDowntime:        stateInfo.TotalDowntime,
		CurrentSessionStart:  stateInfo.CurrentSessionStart,
		CurrentSessionUptime: stateInfo.CurrentSessionUptime,
		GRPCCallCount:        stateInfo.GRPCCallCount,
		GRPCSuccessCount:     stateInfo.GRPCSuccessCount,
		GRPCFailureCount:     stateInfo.GRPCFailureCount,
		LastGRPCCallTime:     stateInfo.LastGRPCCallTime,
		PingCount:            stateInfo.PingCount,
		PongCount:            stateInfo.PongCount,
		LastPacketLossCalc:   stateInfo.LastPacketLossCalc,
	}, true
}

// GetAllConnectionStates returns all connection states
func (cm *ConnectionMonitor) GetAllConnectionStates() map[string]*ConnectionStateInfo {
	cm.statesMu.RLock()
	defer cm.statesMu.RUnlock()

	result := make(map[string]*ConnectionStateInfo)
	for edgeID, stateInfo := range cm.states {
		stateInfo.mu.RLock()
		result[edgeID] = &ConnectionStateInfo{
			EdgeID:               stateInfo.EdgeID,
			State:                stateInfo.State,
			LastSeen:             stateInfo.LastSeen,
			LastHeartbeat:        stateInfo.LastHeartbeat,
			LastHandshake:        stateInfo.LastHandshake,
			ConnectionCount:      stateInfo.ConnectionCount,
			ReconnectAttempts:    stateInfo.ReconnectAttempts,
			LastReconnectAttempt: stateInfo.LastReconnectAttempt,
			Latency:              stateInfo.Latency,
			BytesReceived:        stateInfo.BytesReceived,
			BytesSent:            stateInfo.BytesSent,
			StateChangedAt:       stateInfo.StateChangedAt,
			FirstConnectedAt:     stateInfo.FirstConnectedAt,
			LastConnectedAt:      stateInfo.LastConnectedAt,
			TotalUptime:          stateInfo.TotalUptime,
			TotalDowntime:        stateInfo.TotalDowntime,
			CurrentSessionStart:  stateInfo.CurrentSessionStart,
			CurrentSessionUptime: stateInfo.CurrentSessionUptime,
			GRPCCallCount:        stateInfo.GRPCCallCount,
			GRPCSuccessCount:     stateInfo.GRPCSuccessCount,
			GRPCFailureCount:     stateInfo.GRPCFailureCount,
			LastGRPCCallTime:     stateInfo.LastGRPCCallTime,
			PingCount:            stateInfo.PingCount,
			PongCount:            stateInfo.PongCount,
			LastPacketLossCalc:   stateInfo.LastPacketLossCalc,
		}
		stateInfo.mu.RUnlock()
	}

	return result
}

// keepaliveLoop sends periodic keepalive pings to maintain WireGuard tunnels
func (cm *ConnectionMonitor) keepaliveLoop(ctx context.Context) {
	defer cm.wg.Done()

	ticker := time.NewTicker(cm.keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.sendKeepalivePings()
		}
	}
}

// sendKeepalivePings sends keepalive pings to all connected WireGuard peers
func (cm *ConnectionMonitor) sendKeepalivePings() {
	if cm.wgServer == nil {
		return
	}

	// Get all registered Edges from database
	rows, err := cm.db.QueryContext(context.Background(),
		"SELECT edge_id, wireguard_public_key FROM edges WHERE status = 'active'")
	if err != nil {
		cm.logger.Error("Failed to query edges for keepalive", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var edgeID, publicKey string
		if err := rows.Scan(&edgeID, &publicKey); err != nil {
			cm.logger.Warn("Failed to scan edge row for keepalive", zap.Error(err))
			continue
		}

		// Get connection state
		stateInfo, exists := cm.GetConnectionState(edgeID)
		if !exists || stateInfo.State != StateConnected {
			// Only send keepalive to connected edges
			continue
		}

		// Parse public key
		key, err := wgtypes.ParseKey(publicKey)
		if err != nil {
			cm.logger.Warn("Failed to parse public key for keepalive",
				zap.String("edge_id", edgeID),
				zap.Error(err),
			)
			continue
		}

		// Send keepalive ping via WireGuard
		cm.sendWireGuardKeepalive(key, edgeID)
	}
}

// sendWireGuardKeepalive sends a keepalive ping to a WireGuard peer
func (cm *ConnectionMonitor) sendWireGuardKeepalive(publicKey wgtypes.Key, edgeID string) {
	if cm.wgServer == nil {
		return
	}

	// Record ping in WireGuard server
	cm.wgServer.RecordPing(publicKey)

	// Check if handshake is stale and needs reconnection
	cm.checkAndHandleTunnelDrop(publicKey, edgeID)
}

// checkAndHandleTunnelDrop checks if tunnel has dropped and handles reconnection
func (cm *ConnectionMonitor) checkAndHandleTunnelDrop(publicKey wgtypes.Key, edgeID string) {
	if cm.wgServer == nil {
		return
	}

	// Get peer info
	wgPeer, exists := cm.getWireGuardPeer(publicKey.String())
	if !exists {
		return
	}

	wgPeer.mu.RLock()
	lastHandshake := wgPeer.LastHandshake
	wasConnected := wgPeer.Connected
	wgPeer.mu.RUnlock()

	now := time.Now()
	handshakeAge := now.Sub(lastHandshake)

	// Check if handshake is stale (no handshake for more than stale threshold)
	if lastHandshake.IsZero() || handshakeAge > cm.staleThreshold {
		// Tunnel has dropped
		if wasConnected {
			cm.handleTunnelDrop(edgeID, publicKey)
		}
	} else {
		// Tunnel is healthy
		if !wasConnected && !lastHandshake.IsZero() {
			cm.handleTunnelReconnect(edgeID, publicKey)
		}

		// Update peer as connected
		cm.wgServer.mu.Lock()
		if peer, exists := cm.wgServer.peers[publicKey.String()]; exists {
			peer.mu.Lock()
			peer.Connected = true
			peer.mu.Unlock()
		}
		cm.wgServer.mu.Unlock()
	}
}

// handleTunnelDrop handles when a WireGuard tunnel drops
func (cm *ConnectionMonitor) handleTunnelDrop(edgeID string, publicKey wgtypes.Key) {
	cm.logger.Warn("WireGuard tunnel dropped",
		zap.String("edge_id", edgeID),
		zap.String("public_key", publicKey.String()),
	)

	// Update peer as disconnected
	if cm.wgServer != nil {
		cm.wgServer.mu.Lock()
		if peer, exists := cm.wgServer.peers[publicKey.String()]; exists {
			peer.mu.Lock()
			peer.Connected = false
			peer.mu.Unlock()
		}
		cm.wgServer.mu.Unlock()
	}

	// Publish tunnel dropped event
	if cm.eventBus != nil {
		cm.eventBus.Publish(service.Event{
			Type:      "edge.tunnel_dropped",
			Timestamp: time.Now().Unix(),
			Data: map[string]interface{}{
				"edge_id": edgeID,
			},
		})
	}

	// Trigger reconnection attempt with exponential backoff
	cm.scheduleReconnection(edgeID)
}

// handleTunnelReconnect handles when a WireGuard tunnel reconnects
func (cm *ConnectionMonitor) handleTunnelReconnect(edgeID string, publicKey wgtypes.Key) {
	cm.logger.Info("WireGuard tunnel reconnected",
		zap.String("edge_id", edgeID),
		zap.String("public_key", publicKey.String()),
	)

	// Reset reconnection backoff
	cm.reconnectMu.Lock()
	delete(cm.reconnectBackoffs, edgeID)
	cm.reconnectMu.Unlock()

	// Update state info
	cm.statesMu.Lock()
	if stateInfo, exists := cm.states[edgeID]; exists {
		stateInfo.mu.Lock()
		stateInfo.ReconnectAttempts = 0
		stateInfo.mu.Unlock()
	}
	cm.statesMu.Unlock()

	// Publish tunnel reconnected event
	if cm.eventBus != nil {
		cm.eventBus.Publish(service.Event{
			Type:      "edge.tunnel_reconnected",
			Timestamp: time.Now().Unix(),
			Data: map[string]interface{}{
				"edge_id": edgeID,
			},
		})
	}
}

// scheduleReconnection schedules a reconnection attempt with exponential backoff
func (cm *ConnectionMonitor) scheduleReconnection(edgeID string) {
	// Check current reconnect attempts
	cm.statesMu.Lock()
	stateInfo, exists := cm.states[edgeID]
	var reconnectAttempts int
	if exists {
		stateInfo.mu.Lock()
		reconnectAttempts = stateInfo.ReconnectAttempts
		stateInfo.mu.Unlock()
	}
	cm.statesMu.Unlock()

	// Check if max retries exceeded (circuit breaker pattern)
	if reconnectAttempts >= cm.maxRetries {
		cm.logger.Warn("Max reconnection attempts exceeded, giving up",
			zap.String("edge_id", edgeID),
			zap.Int("attempts", reconnectAttempts),
			zap.Int("max_retries", cm.maxRetries),
		)

		// Update state to disconnected and stop retrying
		cm.statesMu.Lock()
		if stateInfo, exists := cm.states[edgeID]; exists {
			stateInfo.mu.Lock()
			oldState := stateInfo.State
			stateInfo.State = StateDisconnected
			stateInfo.StateChangedAt = time.Now()
			stateInfo.mu.Unlock()

			// Publish state change
			cm.publishStateChange(edgeID, oldState, StateDisconnected)
		}
		cm.statesMu.Unlock()

		// Publish connection failed event for alerting
		cm.publishConnectionFailed(edgeID, reconnectAttempts)

		// Clean up reconnection backoff
		cm.reconnectMu.Lock()
		delete(cm.reconnectBackoffs, edgeID)
		cm.reconnectMu.Unlock()

		// Update database
		cm.updateEdgeStatus(edgeID, string(StateDisconnected), time.Now())

		return
	}

	cm.reconnectMu.Lock()
	currentBackoff, exists := cm.reconnectBackoffs[edgeID]
	if !exists {
		currentBackoff = 5 * time.Second // Initial backoff: 5 seconds
	} else {
		// Exponential backoff: double the current backoff, max 5 minutes
		currentBackoff = currentBackoff * 2
		if currentBackoff > 5*time.Minute {
			currentBackoff = 5 * time.Minute
		}
	}
	cm.reconnectBackoffs[edgeID] = currentBackoff
	cm.reconnectMu.Unlock()

	// Update state info
	cm.statesMu.Lock()
	if stateInfo, exists := cm.states[edgeID]; exists {
		stateInfo.mu.Lock()
		stateInfo.ReconnectAttempts++
		stateInfo.LastReconnectAttempt = time.Now()
		if stateInfo.State != StateReconnecting {
			oldState := stateInfo.State
			stateInfo.State = StateReconnecting
			stateInfo.StateChangedAt = time.Now()
			stateInfo.mu.Unlock()

			// Publish state change
			cm.publishStateChange(edgeID, oldState, StateReconnecting)
		} else {
			stateInfo.mu.Unlock()
		}
	}
	cm.statesMu.Unlock()

	cm.logger.Info("Scheduled reconnection attempt",
		zap.String("edge_id", edgeID),
		zap.Int("attempt", reconnectAttempts+1),
		zap.Int("max_retries", cm.maxRetries),
		zap.Duration("backoff", currentBackoff),
	)

	// Schedule reconnection attempt
	go func() {
		select {
		case <-cm.ctx.Done():
			return
		case <-time.After(currentBackoff):
			cm.attemptReconnection(edgeID)
		}
	}()
}

// attemptReconnection attempts to reconnect to an Edge
func (cm *ConnectionMonitor) attemptReconnection(edgeID string) {
	cm.logger.Info("Attempting to reconnect Edge",
		zap.String("edge_id", edgeID),
	)

	// Get Edge info from database
	var publicKey string
	err := cm.db.QueryRowContext(context.Background(),
		"SELECT wireguard_public_key FROM edges WHERE edge_id = ? AND status = 'active'",
		edgeID).Scan(&publicKey)
	if err != nil {
		cm.logger.Warn("Failed to get Edge public key for reconnection",
			zap.String("edge_id", edgeID),
			zap.Error(err),
		)
		return
	}

	// Parse public key
	key, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		cm.logger.Warn("Failed to parse public key for reconnection",
			zap.String("edge_id", edgeID),
			zap.Error(err),
		)
		return
	}

	// Check if peer exists and try to trigger handshake
	// WireGuard will automatically attempt to reconnect when traffic is sent
	// We can trigger this by sending a ping
	cm.sendWireGuardKeepalive(key, edgeID)

	// Check handshake after a short delay
	time.Sleep(2 * time.Second)
	wgPeer, exists := cm.getWireGuardPeer(publicKey)
	if exists && wgPeer != nil {
		wgPeer.mu.RLock()
		lastHandshake := wgPeer.LastHandshake
		wgPeer.mu.RUnlock()

		if !lastHandshake.IsZero() && time.Since(lastHandshake) < cm.staleThreshold {
			// Reconnection successful
			cm.handleTunnelReconnect(edgeID, key)
		} else {
			// Reconnection failed, schedule another attempt
			cm.scheduleReconnection(edgeID)
		}
	} else {
		// Peer not found, schedule another attempt
		cm.scheduleReconnection(edgeID)
	}
}

// UpdateConnectionState updates the connection state when a new connection is established
// This is called by EdgeAPIServer when a gRPC connection is established
func (cm *ConnectionMonitor) UpdateConnectionState(edgeID string, connectedAt time.Time) {
	cm.statesMu.Lock()
	stateInfo, exists := cm.states[edgeID]
	if !exists {
		// Create new state info if it doesn't exist
		stateInfo = &ConnectionStateInfo{
			EdgeID:         edgeID,
			State:          StateConnecting,
			StateChangedAt: time.Now(),
		}
		cm.states[edgeID] = stateInfo
	}
	cm.statesMu.Unlock()

	stateInfo.mu.Lock()
	oldState := stateInfo.State
	now := time.Now()

	// Update connection info
	stateInfo.LastSeen = now
	stateInfo.LastHeartbeat = now
	stateInfo.ConnectionCount++

	// Update state to connected if it was connecting or reconnecting
	if stateInfo.State == StateConnecting || stateInfo.State == StateReconnecting {
		// Track first connection
		if stateInfo.FirstConnectedAt.IsZero() {
			stateInfo.FirstConnectedAt = now
		}
		stateInfo.State = StateConnected
		stateInfo.StateChangedAt = now
		stateInfo.LastConnectedAt = now
		if stateInfo.CurrentSessionStart.IsZero() {
			stateInfo.CurrentSessionStart = now
		}
		stateInfo.mu.Unlock()

		// Update uptime/downtime metrics
		cm.updateUptimeDowntime(stateInfo, oldState, StateConnected, now)

		// Update database
		cm.updateEdgeStatus(edgeID, string(StateConnected), now)

		// Publish state change
		if oldState != StateConnected {
			cm.publishStateChange(edgeID, oldState, StateConnected)
		}
	} else {
		stateInfo.mu.Unlock()
	}
}

// HandleDisconnection handles when an Edge disconnects
// This is called by EdgeAPIServer when a gRPC connection is lost
func (cm *ConnectionMonitor) HandleDisconnection(edgeID string) {
	cm.statesMu.Lock()
	stateInfo, exists := cm.states[edgeID]
	if !exists {
		cm.statesMu.Unlock()
		return
	}
	cm.statesMu.Unlock()

	stateInfo.mu.Lock()
	oldState := stateInfo.State
	now := time.Now()

	// Only update if currently connected or connecting
	if stateInfo.State == StateConnected || stateInfo.State == StateConnecting {
		// Update uptime before changing state
		if stateInfo.State == StateConnected && !stateInfo.CurrentSessionStart.IsZero() {
			sessionUptime := now.Sub(stateInfo.CurrentSessionStart)
			stateInfo.TotalUptime += sessionUptime
			stateInfo.CurrentSessionStart = time.Time{}
			stateInfo.CurrentSessionUptime = 0
		}

		stateInfo.State = StateDisconnected
		stateInfo.StateChangedAt = now
		stateInfo.mu.Unlock()

		// Update database
		cm.updateEdgeStatus(edgeID, string(StateDisconnected), now)

		// Publish state change
		cm.publishStateChange(edgeID, oldState, StateDisconnected)

		// Trigger reconnection attempt
		cm.scheduleReconnection(edgeID)
	} else {
		stateInfo.mu.Unlock()
	}
}
