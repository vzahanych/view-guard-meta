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

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

// Client manages WireGuard tunnel connection to KVM VM
type Client struct {
	*service.ServiceBase
	config       *config.WireGuardConfig
	logger       *logger.Logger
	interfaceName string
	configPath    string
	connected     bool
	lastLatency   time.Duration
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	healthTicker  *time.Ticker
}

// NewClient creates a new WireGuard client
func NewClient(cfg *config.WireGuardConfig, log *logger.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	// Generate interface name (wg0, wg1, etc.)
	interfaceName := "wg0" // Default, can be configured

	// Determine config path
	configPath := cfg.ConfigPath
	if configPath == "" {
		// Default config path
		configPath = "/etc/wireguard/wg0.conf"
	}

	return &Client{
		ServiceBase:  service.NewServiceBase("wireguard-client", log),
		config:       cfg,
		logger:       log,
		interfaceName: interfaceName,
		configPath:    configPath,
		connected:     false,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Name returns the service name
func (c *Client) Name() string {
	return "wireguard-client"
}

// Start starts the WireGuard client service
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.config.Enabled {
		c.LogInfo("WireGuard is disabled, skipping start")
		return nil
	}

	c.GetStatus().SetStatus(service.StatusStarting)
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Check if WireGuard is installed
	if !c.isWireGuardInstalled() {
		err := fmt.Errorf("wireguard tools not found (wg command not available)")
		c.GetStatus().SetError(err)
		c.LogError("WireGuard tools not available", err)
		return err
	}

	// Ensure config file exists
	if err := c.ensureConfigFile(); err != nil {
		c.GetStatus().SetError(err)
		c.LogError("Failed to ensure config file", err)
		return err
	}

	// Start tunnel
	if err := c.startTunnel(); err != nil {
		c.GetStatus().SetError(err)
		c.LogError("Failed to start tunnel", err)
		return err
	}

	c.connected = true
	c.GetStatus().SetStatus(service.StatusRunning)

	// Start health monitoring
	c.startHealthMonitoring(ctx)

	c.LogInfo("WireGuard client started", "interface", c.interfaceName, "endpoint", c.config.KVMEndpoint)
	c.PublishEvent(service.EventTypeWireGuardConnected, map[string]interface{}{
		"interface": c.interfaceName,
		"endpoint":  c.config.KVMEndpoint,
	})

	return nil
}

// Stop stops the WireGuard client service
func (c *Client) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.GetStatus().SetStatus(service.StatusStopping)

	// Stop health monitoring
	if c.healthTicker != nil {
		c.healthTicker.Stop()
	}

	// Stop tunnel
	if err := c.stopTunnel(); err != nil {
		c.LogError("Failed to stop tunnel", err)
		// Continue with shutdown even if stop fails
	}

	c.cancel()
	c.connected = false
	c.GetStatus().SetStatus(service.StatusStopped)

	c.LogInfo("WireGuard client stopped")
	c.PublishEvent(service.EventTypeWireGuardDisconnected, map[string]interface{}{
		"interface": c.interfaceName,
	})

	return nil
}

// IsConnected returns whether the tunnel is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.isTunnelUp()
}

// GetLatency returns the last measured latency
func (c *Client) GetLatency() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastLatency
}

// GetInterfaceName returns the WireGuard interface name
func (c *Client) GetInterfaceName() string {
	return c.interfaceName
}

// GetEndpoint returns the KVM VM endpoint
func (c *Client) GetEndpoint() string {
	return c.config.KVMEndpoint
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
func (c *Client) startTunnel() error {
	// Create WireGuard interface if it does not exist
	linkAdd := exec.CommandContext(c.ctx, "ip", "link", "add", "dev", c.interfaceName, "type", "wireguard")
	linkAdd.Env = os.Environ()
	if output, err := linkAdd.CombinedOutput(); err != nil {
		// If the interface already exists, ignore the error
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to add wireguard interface %s: %w, output: %s", c.interfaceName, err, string(output))
		}
	}

	// Read and parse config file
	configData, err := os.ReadFile(c.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	privateKey, peerPublicKey, peerAllowedIPs, peerEndpoint, peerPresharedKey, peerKeepalive, address, err := parseWireGuardConfig(string(configData))
	if err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Configure interface using individual wg set commands (more reliable than wg setconf)
	// Set private key
	cmd := exec.CommandContext(c.ctx, "wg", "set", c.interfaceName, "private-key", "/dev/stdin")
	cmd.Stdin = strings.NewReader(privateKey)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set private key: %w, output: %s", err, string(output))
	}

	// Add peer
	peerCmd := exec.CommandContext(c.ctx, "wg", "set", c.interfaceName, "peer", peerPublicKey)
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
	addrAdd := exec.CommandContext(c.ctx, "ip", "addr", "add", ipAddr, "dev", c.interfaceName)
	addrAdd.Env = os.Environ()
	if output, err := addrAdd.CombinedOutput(); err != nil {
		// Ignore "File exists" error (address already assigned)
		if !strings.Contains(string(output), "File exists") && !strings.Contains(string(output), "already assigned") {
			return fmt.Errorf("failed to add IP address to interface %s: %w, output: %s", c.interfaceName, err, string(output))
		}
	}

	// Bring interface up
	linkUp := exec.CommandContext(c.ctx, "ip", "link", "set", "up", "dev", c.interfaceName)
	linkUp.Env = os.Environ()
	if output, err := linkUp.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring interface up: %w, output: %s", err, string(output))
	}

	c.LogInfo("WireGuard tunnel started", "interface", c.interfaceName, "address", ipAddr)
	return nil
}

// stopTunnel stops the WireGuard tunnel
func (c *Client) stopTunnel() error {
	// Bring interface down and delete it; ignore errors if it does not exist
	linkDown := exec.CommandContext(c.ctx, "ip", "link", "set", "down", "dev", c.interfaceName)
	linkDown.Env = os.Environ()
	if output, err := linkDown.CombinedOutput(); err != nil && !strings.Contains(string(output), "Cannot find device") {
		return fmt.Errorf("failed to bring interface down: %w, output: %s", err, string(output))
	}

	linkDel := exec.CommandContext(c.ctx, "ip", "link", "del", "dev", c.interfaceName)
	linkDel.Env = os.Environ()
	if output, err := linkDel.CombinedOutput(); err != nil && !strings.Contains(string(output), "Cannot find device") {
		return fmt.Errorf("failed to delete interface: %w, output: %s", err, string(output))
	}

	c.LogDebug("Tunnel stopped", "interface", c.interfaceName)
	return nil
}

// isTunnelUp checks if the tunnel interface is up
func (c *Client) isTunnelUp() bool {
	// Check if interface exists
	cmd := exec.CommandContext(c.ctx, "wg", "show", c.interfaceName)
	err := cmd.Run()
	return err == nil
}

// isWireGuardInstalled checks if WireGuard tools are installed
func (c *Client) isWireGuardInstalled() bool {
	cmd := exec.Command("which", "wg")
	err := cmd.Run()
	return err == nil
}

// ensureConfigFile ensures the WireGuard config file exists
func (c *Client) ensureConfigFile() error {
	// Check if config file exists
	if _, err := os.Stat(c.configPath); err == nil {
		// Config file exists
		return nil
	}

	// Create directory if it doesn't exist
	configDir := filepath.Dir(c.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// For PoC, we'll create a basic config file template
	// In production, this would be generated from bootstrap tokens or ISO configuration
	if c.config.KVMEndpoint == "" {
		return fmt.Errorf("kvm_endpoint is required but not configured")
	}

	// Generate a basic config (this is a placeholder - real config would come from ISO/bootstrap)
	configContent := c.generateConfigTemplate()
	if err := os.WriteFile(c.configPath, []byte(configContent), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	c.LogInfo("Created WireGuard config file", "path", c.configPath)
	return nil
}

// generateConfigTemplate generates a basic WireGuard config template
// In production, this would be populated from bootstrap tokens or ISO configuration
func (c *Client) generateConfigTemplate() string {
	// This is a template - real implementation would use actual keys from bootstrap
	return fmt.Sprintf(`[Interface]
# PrivateKey = <generated-or-from-bootstrap>
# Address = <assigned-by-kvm-vm>

[Peer]
# PublicKey = <kvm-vm-public-key>
Endpoint = %s:51820
# AllowedIPs = <assigned-by-kvm-vm>
PersistentKeepalive = 25
`, c.config.KVMEndpoint)
}

// startHealthMonitoring starts monitoring tunnel health
func (c *Client) startHealthMonitoring(ctx context.Context) {
	c.healthTicker = time.NewTicker(10 * time.Second) // Check every 10 seconds

	go func() {
		defer c.healthTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-c.ctx.Done():
				return
			case <-c.healthTicker.C:
				c.checkHealth()
			}
		}
	}()
}

// checkHealth checks tunnel health and latency
func (c *Client) checkHealth() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if tunnel is up
	if !c.isTunnelUp() {
		if c.connected {
			c.LogError("Tunnel is down", fmt.Errorf("interface %s is not up", c.interfaceName))
			c.connected = false
			c.GetStatus().SetError(fmt.Errorf("tunnel is down"))
			c.PublishEvent(service.EventTypeWireGuardDisconnected, map[string]interface{}{
				"interface": c.interfaceName,
				"reason":    "tunnel_down",
			})

			// Attempt reconnection
			go c.reconnect()
		}
		return
	}

	// Measure latency (ping KVM endpoint if configured)
	if c.config.KVMEndpoint != "" {
		latency := c.measureLatency()
		c.lastLatency = latency
		c.LogDebug("Tunnel health check", "latency", latency, "connected", c.connected)
	}

	// If we were disconnected but tunnel is now up, mark as connected
	if !c.connected && c.isTunnelUp() {
		c.connected = true
		c.GetStatus().SetStatus(service.StatusRunning)
		c.PublishEvent(service.EventTypeWireGuardConnected, map[string]interface{}{
			"interface": c.interfaceName,
			"endpoint":  c.config.KVMEndpoint,
		})
	}
}

// measureLatency measures latency to the KVM endpoint
func (c *Client) measureLatency() time.Duration {
	// Extract host from endpoint (remove port)
	endpoint := c.config.KVMEndpoint
	if idx := strings.LastIndex(endpoint, ":"); idx > 0 {
		endpoint = endpoint[:idx]
	}

	// Use ping to measure latency (1 ping, timeout 2 seconds)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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
func (c *Client) reconnect() {
	c.LogInfo("Attempting to reconnect tunnel", "interface", c.interfaceName)

	// Wait a bit before reconnecting
	time.Sleep(5 * time.Second)

	// Try to restart the tunnel
	if err := c.startTunnel(); err != nil {
		c.LogError("Reconnection failed", err)
		return
	}

	c.mu.Lock()
	c.connected = true
	c.GetStatus().SetStatus(service.StatusRunning)
	c.mu.Unlock()

	c.LogInfo("Tunnel reconnected", "interface", c.interfaceName)
	c.PublishEvent(service.EventTypeWireGuardConnected, map[string]interface{}{
		"interface": c.interfaceName,
		"endpoint":  c.config.KVMEndpoint,
		"reconnected": true,
	})
}

// GetStats returns tunnel statistics
func (c *Client) GetStats() (*TunnelStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := &TunnelStats{
		InterfaceName: c.interfaceName,
		Connected:     c.connected && c.isTunnelUp(),
		Latency:       c.lastLatency,
		Endpoint:      c.config.KVMEndpoint,
	}

	// Get WireGuard statistics if tunnel is up
	if stats.Connected {
		cmd := exec.Command("wg", "show", c.interfaceName, "dump")
		output, err := cmd.Output()
		if err == nil {
			stats.RawStats = string(output)
		}
	}

	return stats, nil
}

// TunnelStats contains tunnel statistics
type TunnelStats struct {
	InterfaceName string
	Connected     bool
	Latency       time.Duration
	Endpoint      string
	RawStats      string
}

