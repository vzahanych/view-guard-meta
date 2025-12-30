package wireguard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"
)

// service implements the TunnelClientService interface for WireGuard
type service struct {
	config        *Config
	logger        *zap.Logger
	eventBus      eventbus.EventBus // Optional: for publishing events
	interfaceName string
	configPath    string
	connected     bool
	lastLatency   time.Duration
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	healthTicker  *time.Ticker
	stopping      bool // Flag to prevent new reconnect attempts after Stop()
}

// NewService creates a new WireGuard tunnel client service.
// Context is not created here - it will be created in Start() from the caller's context.
// This ensures contexts flow from callers and are tied to the service lifecycle.
func NewService(cfg *Config, log *zap.Logger) *service {
	// Determine interface name
	interfaceName := cfg.InterfaceName
	if interfaceName == "" {
		interfaceName = "wg0" // Default
	}

	// Determine config path
	configPath := cfg.ConfigPath
	if configPath == "" {
		configPath = "/etc/wireguard/wg0.conf" // Default
	}

	return &service{
		config:        cfg,
		logger:        log,
		interfaceName: interfaceName,
		configPath:    configPath,
		connected:     false,
		ctx:           nil, // Will be set in Start() from caller's context
		cancel:        nil, // Will be set in Start() from caller's context
	}
}

// SetEventBus sets the event bus for publishing events
func (s *service) SetEventBus(bus eventbus.EventBus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventBus = bus
}

// Name returns the service name
func (s *service) Name() string {
	return "wireguard-client"
}

// Start starts the WireGuard client service
func (s *service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.config.Enabled {
		s.logger.Info("WireGuard is disabled, skipping start")
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Check if WireGuard is installed
	if !s.isWireGuardInstalled() {
		err := fmt.Errorf("wireguard tools not found (wg command not available)")
		s.logger.Error("WireGuard tools not available", zap.Error(err))
		return err
	}

	// Ensure config file exists
	if err := s.ensureConfigFile(); err != nil {
		s.logger.Error("Failed to ensure config file", zap.Error(err))
		return err
	}

	// Emit tunnel.connecting event
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.TunnelConnectingEventData]{
			Type:      evtbusstypes.EventTypeNetworkTunnelConnecting,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: evtbusstypes.TunnelConnectingEventData{
				Interface: s.interfaceName,
				Endpoint:  s.config.KVMEndpoint,
				Provider:  "wireguard",
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish tunnel connecting event", zap.Error(err))
		}
	}

	// Start tunnel
	if err := s.startTunnel(); err != nil {
		s.logger.Error("Failed to start tunnel", zap.Error(err))
		// Emit tunnel.connection_error event
		if s.eventBus != nil {
			event := evtbusstypes.Event[evtbusstypes.TunnelConnectionErrorEventData]{
				Type:      evtbusstypes.EventTypeNetworkTunnelConnectionError,
				Source:    s.Name(),
				Timestamp: time.Now(),
				Data: evtbusstypes.TunnelConnectionErrorEventData{
					Interface: s.interfaceName,
					Endpoint:  s.config.KVMEndpoint,
					Provider:  "wireguard",
					Error:     err.Error(),
					Retryable: true,
				},
			}
			if pubErr := eventbus.PublishTyped(s.eventBus, event); pubErr != nil {
				s.logger.Warn("Failed to publish tunnel connection error event", zap.Error(pubErr))
			}
		}
		return err
	}

	s.connected = true

	// Start health monitoring
	s.startHealthMonitoring(ctx)

	s.logger.Info("WireGuard client started", zap.String("interface", s.interfaceName), zap.String("endpoint", s.config.KVMEndpoint))
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.TunnelConnectedEventData]{
			Type:      evtbusstypes.EventTypeNetworkTunnelConnected,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: evtbusstypes.TunnelConnectedEventData{
				Interface: s.interfaceName,
				Endpoint:  s.config.KVMEndpoint,
				Provider:  "wireguard",
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish wireguard connected event", zap.Error(err))
		}
	}

	return nil
}

// Stop stops the WireGuard client service
func (s *service) Stop(ctx context.Context) error {
	s.mu.Lock()
	// Set stopping flag first to prevent new reconnect attempts
	s.stopping = true
	s.mu.Unlock()

	if !s.connected {
		s.mu.Lock()
		// Cancel context if it was created (in Start())
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		return nil
	}

	// Stop health monitoring
	s.mu.Lock()
	if s.healthTicker != nil {
		s.healthTicker.Stop()
	}
	s.mu.Unlock()

	// Stop tunnel
	if err := s.stopTunnel(); err != nil {
		s.logger.Error("Failed to stop tunnel", zap.Error(err))
		// Continue with shutdown even if stop fails
	}

	s.mu.Lock()
	// Cancel context if it was created (in Start())
	if s.cancel != nil {
		s.cancel()
	}
	s.connected = false
	s.mu.Unlock()

	s.logger.Info("WireGuard client stopped")
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.TunnelDisconnectedEventData]{
			Type:      evtbusstypes.EventTypeNetworkTunnelDisconnected,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: evtbusstypes.TunnelDisconnectedEventData{
				Interface: s.interfaceName,
				Endpoint:  s.config.KVMEndpoint,
				Provider:  "wireguard",
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish wireguard disconnected event", zap.Error(err))
		}
	}

	return nil
}

// IsConnected returns whether the tunnel is connected
func (s *service) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected && s.isTunnelUp()
}

// GetInterfaceName returns the WireGuard interface name
func (s *service) GetInterfaceName() string {
	return s.interfaceName
}

// GetEndpoint returns the KVM VM endpoint
func (s *service) GetEndpoint() string {
	return s.config.KVMEndpoint
}

// GetLatency returns the last measured latency
func (s *service) GetLatency() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastLatency
}

// GetStats returns tunnel statistics
func (s *service) GetStats() (*TunnelStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &TunnelStats{
		InterfaceName: s.interfaceName,
		Connected:     s.connected && s.isTunnelUp(),
		Latency:       s.lastLatency,
		Endpoint:      s.config.KVMEndpoint,
	}

	// Get WireGuard statistics if tunnel is up
	if stats.Connected {
		cmd := exec.Command("wg", "show", s.interfaceName, "dump")
		output, err := cmd.Output()
		if err == nil {
			stats.RawStats = string(output)
		}
	}

	return stats, nil
}

// parseWireGuardConfig parses a WireGuard config file and returns the configuration values
func parseWireGuardConfig(configData string) (privateKey string, peerPublicKey string, peerAllowedIPs string, peerEndpoint string, peerPresharedKey string, peerKeepalive string, address string, err error) {
	lines := strings.Split(configData, "\n")
	var inInterface, inPeer bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Check for section headers
		if trimmed == "[Interface]" {
			inInterface = true
			inPeer = false
			continue
		}
		if trimmed == "[Peer]" {
			inPeer = true
			inInterface = false
			continue
		}

		// Parse Interface section
		if inInterface {
			if strings.HasPrefix(trimmed, "PrivateKey") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					privateKey = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(trimmed, "Address") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					address = strings.TrimSpace(parts[1])
				}
			}
		}

		// Parse Peer section
		if inPeer {
			if strings.HasPrefix(trimmed, "PublicKey") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					peerPublicKey = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(trimmed, "AllowedIPs") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					peerAllowedIPs = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(trimmed, "Endpoint") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					peerEndpoint = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(trimmed, "PresharedKey") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					peerPresharedKey = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(trimmed, "PersistentKeepalive") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					peerKeepalive = strings.TrimSpace(parts[1])
				}
			}
		}
	}

	if privateKey == "" {
		return "", "", "", "", "", "", "", fmt.Errorf("PrivateKey not found in config")
	}
	if peerPublicKey == "" {
		return "", "", "", "", "", "", "", fmt.Errorf("Peer PublicKey not found in config")
	}

	return privateKey, peerPublicKey, peerAllowedIPs, peerEndpoint, peerPresharedKey, peerKeepalive, address, nil
}

// startTunnel starts the WireGuard tunnel
func (s *service) startTunnel() error {
	// Create WireGuard interface if it does not exist
	linkAdd := exec.CommandContext(s.ctx, "ip", "link", "add", "dev", s.interfaceName, "type", "wireguard")
	linkAdd.Env = os.Environ()
	if output, err := linkAdd.CombinedOutput(); err != nil {
		// If the interface already exists, ignore the error
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to add wireguard interface %s: %w, output: %s", s.interfaceName, err, string(output))
		}
	}

	// Read and parse config file
	configData, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	privateKey, peerPublicKey, peerAllowedIPs, peerEndpoint, peerPresharedKey, peerKeepalive, address, err := parseWireGuardConfig(string(configData))
	if err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Configure interface using individual wg set commands (more reliable than wg setconf)
	// Set private key
	cmd := exec.CommandContext(s.ctx, "wg", "set", s.interfaceName, "private-key", "/dev/stdin")
	cmd.Stdin = strings.NewReader(privateKey)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set private key: %w, output: %s", err, string(output))
	}

	// Add peer
	peerCmd := exec.CommandContext(s.ctx, "wg", "set", s.interfaceName, "peer", peerPublicKey)
	if peerAllowedIPs != "" {
		peerCmd.Args = append(peerCmd.Args, "allowed-ips", peerAllowedIPs)
	}
	if peerEndpoint != "" {
		peerCmd.Args = append(peerCmd.Args, "endpoint", peerEndpoint)
	}
	if peerPresharedKey != "" {
		peerCmd.Args = append(peerCmd.Args, "preshared-key", "/dev/stdin")
		peerCmd.Stdin = strings.NewReader(peerPresharedKey)
	}
	if peerKeepalive != "" {
		peerCmd.Args = append(peerCmd.Args, "persistent-keepalive", peerKeepalive)
	}
	peerCmd.Env = os.Environ()
	if output, err := peerCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add peer: %w, output: %s", err, string(output))
	}

	// Add IP address to the interface
	ipAddr := "10.0.0.2/24"
	if address != "" {
		ipAddr = address
	}
	addrAdd := exec.CommandContext(s.ctx, "ip", "addr", "add", ipAddr, "dev", s.interfaceName)
	addrAdd.Env = os.Environ()
	if output, err := addrAdd.CombinedOutput(); err != nil {
		// Ignore "File exists" error (address already assigned)
		if !strings.Contains(string(output), "File exists") && !strings.Contains(string(output), "already assigned") {
			return fmt.Errorf("failed to add IP address to interface %s: %w, output: %s", s.interfaceName, err, string(output))
		}
	}

	// Bring interface up
	linkUp := exec.CommandContext(s.ctx, "ip", "link", "set", "up", "dev", s.interfaceName)
	linkUp.Env = os.Environ()
	if output, err := linkUp.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring interface up: %w, output: %s", err, string(output))
	}

	s.logger.Info("WireGuard tunnel started", zap.String("interface", s.interfaceName), zap.String("address", ipAddr))
	return nil
}

// stopTunnel stops the WireGuard tunnel
func (s *service) stopTunnel() error {
	// Bring interface down and delete it; ignore errors if it does not exist
	linkDown := exec.CommandContext(s.ctx, "ip", "link", "set", "down", "dev", s.interfaceName)
	linkDown.Env = os.Environ()
	if output, err := linkDown.CombinedOutput(); err != nil && !strings.Contains(string(output), "Cannot find device") {
		return fmt.Errorf("failed to bring interface down: %w, output: %s", err, string(output))
	}

	linkDel := exec.CommandContext(s.ctx, "ip", "link", "del", "dev", s.interfaceName)
	linkDel.Env = os.Environ()
	if output, err := linkDel.CombinedOutput(); err != nil && !strings.Contains(string(output), "Cannot find device") {
		return fmt.Errorf("failed to delete interface: %w, output: %s", err, string(output))
	}

	s.logger.Debug("Tunnel stopped", zap.String("interface", s.interfaceName))
	return nil
}

// isTunnelUp checks if the tunnel interface is up
func (s *service) isTunnelUp() bool {
	// Check if interface exists
	cmd := exec.CommandContext(s.ctx, "wg", "show", s.interfaceName)
	err := cmd.Run()
	return err == nil
}

// isWireGuardInstalled checks if WireGuard tools are installed
func (s *service) isWireGuardInstalled() bool {
	cmd := exec.Command("which", "wg")
	err := cmd.Run()
	return err == nil
}

// ensureConfigFile ensures the WireGuard config file exists
func (s *service) ensureConfigFile() error {
	// Check if config file exists
	if _, err := os.Stat(s.configPath); err == nil {
		// Config file exists
		return nil
	}

	// Create directory if it doesn't exist
	configDir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// For PoC, we'll create a basic config file template
	// In production, this would be generated from bootstrap tokens or ISO configuration
	if s.config.KVMEndpoint == "" {
		return fmt.Errorf("kvm_endpoint is required but not configured")
	}

	// Generate a basic config (this is a placeholder - real config would come from ISO/bootstrap)
	configContent := s.generateConfigTemplate()
	if err := os.WriteFile(s.configPath, []byte(configContent), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	s.logger.Info("Created WireGuard config file", zap.String("path", s.configPath))
	return nil
}

// generateConfigTemplate generates a basic WireGuard config template
// In production, this would be populated from bootstrap tokens or ISO configuration
func (s *service) generateConfigTemplate() string {
	// This is a template - real implementation would use actual keys from bootstrap
	return fmt.Sprintf(`[Interface]
# PrivateKey = <generated-or-from-bootstrap>
# Address = <assigned-by-kvm-vm>

[Peer]
# PublicKey = <kvm-vm-public-key>
Endpoint = %s:51820
# AllowedIPs = <assigned-by-kvm-vm>
PersistentKeepalive = 25
`, s.config.KVMEndpoint)
}

// startHealthMonitoring starts monitoring tunnel health
func (s *service) startHealthMonitoring(ctx context.Context) {
	interval := s.config.HealthCheckInterval
	if interval == 0 {
		interval = 10 * time.Second // Default
	}
	s.healthTicker = time.NewTicker(interval)

	go func() {
		defer s.healthTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.ctx.Done():
				return
			case <-s.healthTicker.C:
				s.checkHealth()
			}
		}
	}()
}

// checkHealth checks tunnel health and latency
func (s *service) checkHealth() {
	s.mu.Lock()
	stopping := s.stopping
	s.mu.Unlock()

	// Don't attempt reconnection if service is stopping
	if stopping {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if tunnel is up
	if !s.isTunnelUp() {
		if s.connected {
			s.logger.Error("Tunnel is down", zap.Error(fmt.Errorf("interface %s is not up", s.interfaceName)))
			s.connected = false
			if s.eventBus != nil {
				event := evtbusstypes.Event[evtbusstypes.TunnelDisconnectedEventData]{
					Type:      evtbusstypes.EventTypeNetworkTunnelDisconnected,
					Source:    s.Name(),
					Timestamp: time.Now(),
					Data: evtbusstypes.TunnelDisconnectedEventData{
						Interface: s.interfaceName,
						Endpoint:  s.config.KVMEndpoint,
						Provider:  "wireguard",
						Reason:    "tunnel_down",
					},
				}
				if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
					s.logger.Warn("Failed to publish wireguard disconnected event", zap.Error(err))
				}
			}

			// Attempt reconnection only if not stopping
			if !s.stopping {
				go s.reconnect()
			}
		}
		return
	}

	// Measure latency (ping KVM endpoint if configured)
	if s.config.KVMEndpoint != "" {
		latency := s.measureLatency()
		s.lastLatency = latency
		s.logger.Debug("Tunnel health check", zap.Duration("latency", latency), zap.Bool("connected", s.connected))
	}

	// If we were disconnected but tunnel is now up, mark as connected
	if !s.connected && s.isTunnelUp() {
		s.connected = true
		if s.eventBus != nil {
			event := evtbusstypes.Event[evtbusstypes.TunnelConnectedEventData]{
				Type:      evtbusstypes.EventTypeNetworkTunnelConnected,
				Source:    s.Name(),
				Timestamp: time.Now(),
				Data: evtbusstypes.TunnelConnectedEventData{
					Interface: s.interfaceName,
					Endpoint:  s.config.KVMEndpoint,
					Provider:  "wireguard",
					Reconnected: true,
				},
			}
			if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
				s.logger.Warn("Failed to publish wireguard connected event", zap.Error(err))
			}
		}
	}
}

// measureLatency measures latency to the KVM endpoint
func (s *service) measureLatency() time.Duration {
	// Extract host from endpoint (remove port)
	endpoint := s.config.KVMEndpoint
	if idx := strings.LastIndex(endpoint, ":"); idx > 0 {
		endpoint = endpoint[:idx]
	}

	// Use ping to measure latency
	timeout := s.config.PingTimeout
	if timeout == 0 {
		timeout = 2 * time.Second // Default
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", endpoint)
	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		// Ping failed, return high latency
		return 5 * time.Second
	}

	return duration
}

// reconnect attempts to reconnect the tunnel
// This function respects context cancellation and will exit early if the service is stopping.
func (s *service) reconnect() {
	// Check if service is stopping before starting reconnection
	s.mu.RLock()
	stopping := s.stopping
	ctx := s.ctx
	s.mu.RUnlock()

	if stopping {
		s.logger.Debug("Skipping reconnection - service is stopping")
		return
	}

	s.logger.Info("Attempting to reconnect tunnel", zap.String("interface", s.interfaceName))

	// Wait before reconnecting, but respect context cancellation
	timeout := s.config.ReconnectTimeout
	if timeout == 0 {
		timeout = 5 * time.Second // Default
	}

	// Use context-aware sleep instead of time.Sleep
	select {
	case <-ctx.Done():
		s.logger.Debug("Reconnection cancelled - service is stopping")
		return
	case <-time.After(timeout):
		// Timeout expired, continue with reconnection
	}

	// Check again if service is stopping before attempting reconnection
	s.mu.RLock()
	stopping = s.stopping
	s.mu.RUnlock()

	if stopping {
		s.logger.Debug("Skipping reconnection - service stopped during wait")
		return
	}

	// Try to restart the tunnel
	if err := s.startTunnel(); err != nil {
		s.logger.Error("Reconnection failed", zap.Error(err))
		return
	}

	s.mu.Lock()
	s.connected = true
	s.mu.Unlock()

	s.logger.Info("Tunnel reconnected", zap.String("interface", s.interfaceName))
	if s.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.TunnelConnectedEventData]{
			Type:      evtbusstypes.EventTypeNetworkTunnelConnected,
			Source:    s.Name(),
			Timestamp: time.Now(),
			Data: evtbusstypes.TunnelConnectedEventData{
				Interface:   s.interfaceName,
				Endpoint:    s.config.KVMEndpoint,
				Provider:    "wireguard",
				Reconnected: true,
			},
		}
		if err := eventbus.PublishTyped(s.eventBus, event); err != nil {
			s.logger.Warn("Failed to publish wireguard reconnected event", zap.Error(err))
		}
	}
}

// TunnelStats contains tunnel statistics
type TunnelStats struct {
	InterfaceName string
	Connected     bool
	Latency       time.Duration
	Endpoint      string
	RawStats      string
}
