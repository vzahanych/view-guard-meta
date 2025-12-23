package impl

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
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service/types"
	"go.uber.org/zap"
)

// wgClientService implements the WGClientService interface
type wgClientService struct {
	config        *types.WGClientConfig
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
}

// NewWGClientService creates a new WireGuard client service
func NewWGClientService(cfg *types.WGClientConfig, log *zap.Logger) *wgClientService {
	ctx, cancel := context.WithCancel(context.Background())

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

	return &wgClientService{
		config:        cfg,
		logger:        log,
		interfaceName: interfaceName,
		configPath:    configPath,
		connected:     false,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetEventBus sets the event bus for publishing events
func (w *wgClientService) SetEventBus(bus eventbus.EventBus) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.eventBus = bus
}

// Name returns the service name
func (w *wgClientService) Name() string {
	return "wireguard-client"
}

// Start starts the WireGuard client service
func (w *wgClientService) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.config.Enabled {
		w.logger.Info("WireGuard is disabled, skipping start")
		return nil
	}

	w.ctx, w.cancel = context.WithCancel(ctx)

	// Check if WireGuard is installed
	if !w.isWireGuardInstalled() {
		err := fmt.Errorf("wireguard tools not found (wg command not available)")
		w.logger.Error("WireGuard tools not available", zap.Error(err))
		return err
	}

	// Ensure config file exists
	if err := w.ensureConfigFile(); err != nil {
		w.logger.Error("Failed to ensure config file", zap.Error(err))
		return err
	}

	// Start tunnel
	if err := w.startTunnel(); err != nil {
		w.logger.Error("Failed to start tunnel", zap.Error(err))
		return err
	}

	w.connected = true

	// Start health monitoring
	w.startHealthMonitoring(ctx)

	w.logger.Info("WireGuard client started", zap.String("interface", w.interfaceName), zap.String("endpoint", w.config.KVMEndpoint))
	if w.eventBus != nil {
		w.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("network.wireguard.connected"),
			Source:    w.Name(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"interface": w.interfaceName,
				"endpoint":  w.config.KVMEndpoint,
			},
		})
	}

	return nil
}

// Stop stops the WireGuard client service
func (w *wgClientService) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.connected {
		return nil
	}

	// Stop health monitoring
	if w.healthTicker != nil {
		w.healthTicker.Stop()
	}

	// Stop tunnel
	if err := w.stopTunnel(); err != nil {
		w.logger.Error("Failed to stop tunnel", zap.Error(err))
		// Continue with shutdown even if stop fails
	}

	w.cancel()
	w.connected = false

	w.logger.Info("WireGuard client stopped")
	if w.eventBus != nil {
		w.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("network.wireguard.disconnected"),
			Source:    w.Name(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"interface": w.interfaceName,
			},
		})
	}

	return nil
}

// IsConnected returns whether the tunnel is connected
func (w *wgClientService) IsConnected() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.connected && w.isTunnelUp()
}

// GetInterfaceName returns the WireGuard interface name
func (w *wgClientService) GetInterfaceName() string {
	return w.interfaceName
}

// GetEndpoint returns the KVM VM endpoint
func (w *wgClientService) GetEndpoint() string {
	return w.config.KVMEndpoint
}

// GetLatency returns the last measured latency
func (w *wgClientService) GetLatency() time.Duration {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastLatency
}

// GetStats returns tunnel statistics
func (w *wgClientService) GetStats() (*TunnelStats, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	stats := &TunnelStats{
		InterfaceName: w.interfaceName,
		Connected:     w.connected && w.isTunnelUp(),
		Latency:       w.lastLatency,
		Endpoint:      w.config.KVMEndpoint,
	}

	// Get WireGuard statistics if tunnel is up
	if stats.Connected {
		cmd := exec.Command("wg", "show", w.interfaceName, "dump")
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
func (w *wgClientService) startTunnel() error {
	// Create WireGuard interface if it does not exist
	linkAdd := exec.CommandContext(w.ctx, "ip", "link", "add", "dev", w.interfaceName, "type", "wireguard")
	linkAdd.Env = os.Environ()
	if output, err := linkAdd.CombinedOutput(); err != nil {
		// If the interface already exists, ignore the error
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to add wireguard interface %s: %w, output: %s", w.interfaceName, err, string(output))
		}
	}

	// Read and parse config file
	configData, err := os.ReadFile(w.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	privateKey, peerPublicKey, peerAllowedIPs, peerEndpoint, peerPresharedKey, peerKeepalive, address, err := parseWireGuardConfig(string(configData))
	if err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Configure interface using individual wg set commands (more reliable than wg setconf)
	// Set private key
	cmd := exec.CommandContext(w.ctx, "wg", "set", w.interfaceName, "private-key", "/dev/stdin")
	cmd.Stdin = strings.NewReader(privateKey)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set private key: %w, output: %s", err, string(output))
	}

	// Add peer
	peerCmd := exec.CommandContext(w.ctx, "wg", "set", w.interfaceName, "peer", peerPublicKey)
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
	addrAdd := exec.CommandContext(w.ctx, "ip", "addr", "add", ipAddr, "dev", w.interfaceName)
	addrAdd.Env = os.Environ()
	if output, err := addrAdd.CombinedOutput(); err != nil {
		// Ignore "File exists" error (address already assigned)
		if !strings.Contains(string(output), "File exists") && !strings.Contains(string(output), "already assigned") {
			return fmt.Errorf("failed to add IP address to interface %s: %w, output: %s", w.interfaceName, err, string(output))
		}
	}

	// Bring interface up
	linkUp := exec.CommandContext(w.ctx, "ip", "link", "set", "up", "dev", w.interfaceName)
	linkUp.Env = os.Environ()
	if output, err := linkUp.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring interface up: %w, output: %s", err, string(output))
	}

	w.logger.Info("WireGuard tunnel started", zap.String("interface", w.interfaceName), zap.String("address", ipAddr))
	return nil
}

// stopTunnel stops the WireGuard tunnel
func (w *wgClientService) stopTunnel() error {
	// Bring interface down and delete it; ignore errors if it does not exist
	linkDown := exec.CommandContext(w.ctx, "ip", "link", "set", "down", "dev", w.interfaceName)
	linkDown.Env = os.Environ()
	if output, err := linkDown.CombinedOutput(); err != nil && !strings.Contains(string(output), "Cannot find device") {
		return fmt.Errorf("failed to bring interface down: %w, output: %s", err, string(output))
	}

	linkDel := exec.CommandContext(w.ctx, "ip", "link", "del", "dev", w.interfaceName)
	linkDel.Env = os.Environ()
	if output, err := linkDel.CombinedOutput(); err != nil && !strings.Contains(string(output), "Cannot find device") {
		return fmt.Errorf("failed to delete interface: %w, output: %s", err, string(output))
	}

	w.logger.Debug("Tunnel stopped", zap.String("interface", w.interfaceName))
	return nil
}

// isTunnelUp checks if the tunnel interface is up
func (w *wgClientService) isTunnelUp() bool {
	// Check if interface exists
	cmd := exec.CommandContext(w.ctx, "wg", "show", w.interfaceName)
	err := cmd.Run()
	return err == nil
}

// isWireGuardInstalled checks if WireGuard tools are installed
func (w *wgClientService) isWireGuardInstalled() bool {
	cmd := exec.Command("which", "wg")
	err := cmd.Run()
	return err == nil
}

// ensureConfigFile ensures the WireGuard config file exists
func (w *wgClientService) ensureConfigFile() error {
	// Check if config file exists
	if _, err := os.Stat(w.configPath); err == nil {
		// Config file exists
		return nil
	}

	// Create directory if it doesn't exist
	configDir := filepath.Dir(w.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// For PoC, we'll create a basic config file template
	// In production, this would be generated from bootstrap tokens or ISO configuration
	if w.config.KVMEndpoint == "" {
		return fmt.Errorf("kvm_endpoint is required but not configured")
	}

	// Generate a basic config (this is a placeholder - real config would come from ISO/bootstrap)
	configContent := w.generateConfigTemplate()
	if err := os.WriteFile(w.configPath, []byte(configContent), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	w.logger.Info("Created WireGuard config file", zap.String("path", w.configPath))
	return nil
}

// generateConfigTemplate generates a basic WireGuard config template
// In production, this would be populated from bootstrap tokens or ISO configuration
func (w *wgClientService) generateConfigTemplate() string {
	// This is a template - real implementation would use actual keys from bootstrap
	return fmt.Sprintf(`[Interface]
# PrivateKey = <generated-or-from-bootstrap>
# Address = <assigned-by-kvm-vm>

[Peer]
# PublicKey = <kvm-vm-public-key>
Endpoint = %s:51820
# AllowedIPs = <assigned-by-kvm-vm>
PersistentKeepalive = 25
`, w.config.KVMEndpoint)
}

// startHealthMonitoring starts monitoring tunnel health
func (w *wgClientService) startHealthMonitoring(ctx context.Context) {
	interval := w.config.HealthCheckInterval
	if interval == 0 {
		interval = 10 * time.Second // Default
	}
	w.healthTicker = time.NewTicker(interval)

	go func() {
		defer w.healthTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-w.ctx.Done():
				return
			case <-w.healthTicker.C:
				w.checkHealth()
			}
		}
	}()
}

// checkHealth checks tunnel health and latency
func (w *wgClientService) checkHealth() {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if tunnel is up
	if !w.isTunnelUp() {
		if w.connected {
			w.logger.Error("Tunnel is down", zap.Error(fmt.Errorf("interface %s is not up", w.interfaceName)))
			w.connected = false
			if w.eventBus != nil {
				w.eventBus.Publish(evtbusstypes.Event{
					Type:      evtbusstypes.EventType("network.wireguard.disconnected"),
					Source:    w.Name(),
					Timestamp: time.Now(),
					Data: map[string]interface{}{
						"interface": w.interfaceName,
						"reason":    "tunnel_down",
					},
				})
			}

			// Attempt reconnection
			go w.reconnect()
		}
		return
	}

	// Measure latency (ping KVM endpoint if configured)
	if w.config.KVMEndpoint != "" {
		latency := w.measureLatency()
		w.lastLatency = latency
		w.logger.Debug("Tunnel health check", zap.Duration("latency", latency), zap.Bool("connected", w.connected))
	}

	// If we were disconnected but tunnel is now up, mark as connected
	if !w.connected && w.isTunnelUp() {
		w.connected = true
		if w.eventBus != nil {
			w.eventBus.Publish(evtbusstypes.Event{
				Type:      evtbusstypes.EventType("network.wireguard.connected"),
				Source:    w.Name(),
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"interface": w.interfaceName,
					"endpoint":  w.config.KVMEndpoint,
				},
			})
		}
	}
}

// measureLatency measures latency to the KVM endpoint
func (w *wgClientService) measureLatency() time.Duration {
	// Extract host from endpoint (remove port)
	endpoint := w.config.KVMEndpoint
	if idx := strings.LastIndex(endpoint, ":"); idx > 0 {
		endpoint = endpoint[:idx]
	}

	// Use ping to measure latency
	timeout := w.config.PingTimeout
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
func (w *wgClientService) reconnect() {
	w.logger.Info("Attempting to reconnect tunnel", zap.String("interface", w.interfaceName))

	// Wait before reconnecting
	timeout := w.config.ReconnectTimeout
	if timeout == 0 {
		timeout = 5 * time.Second // Default
	}
	time.Sleep(timeout)

	// Try to restart the tunnel
	if err := w.startTunnel(); err != nil {
		w.logger.Error("Reconnection failed", zap.Error(err))
		return
	}

	w.mu.Lock()
	w.connected = true
	w.mu.Unlock()

	w.logger.Info("Tunnel reconnected", zap.String("interface", w.interfaceName))
	if w.eventBus != nil {
		w.eventBus.Publish(evtbusstypes.Event{
			Type:      evtbusstypes.EventType("network.wireguard.connected"),
			Source:    w.Name(),
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"interface":   w.interfaceName,
				"endpoint":    w.config.KVMEndpoint,
				"reconnected": true,
			},
		})
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
