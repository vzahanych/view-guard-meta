package impl

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/vzahanych/view-guard-meta/vm-edge-orch/config"
	wgserver "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/wg-server-service"
	"github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/wg-server-service/types"
)

// wgServerService implements the WGServerService interface
type wgServerService struct {
	client     *wgctrl.Client
	iface      string
	listenPort int
	configPath string
	privateKey wgtypes.Key
	publicKey  wgtypes.Key
	mu         sync.RWMutex
	peers      map[string]*types.PeerInfo
	eventBus   interface{} // EventBus for publishing events (optional)
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *zap.Logger // Simple logger, can be nil
	disabled   bool        // When true, skips WireGuard OS interactions (dev env)
}

// NewWGServerService creates a new WireGuard server service implementation.
// cfg, log, and db are interface{} to avoid dependencies on non-existent packages.
// For now, we use simple defaults: interface "wg0", port 51820.
func NewWGServerService(cfg interface{}, log interface{}, db interface{}) (wgserver.WGServerService, error) {
	// Create wgctrl client
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create wgctrl client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Try to extract logger if it's a zap.Logger
	var logger *zap.Logger
	if zapLogger, ok := log.(*zap.Logger); ok {
		logger = zapLogger
	} else {
		// Create a simple logger if none provided
		logger, _ = zap.NewDevelopment()
	}

	iface := "wg0"      // Default interface name
	listenPort := 51820 // Default WireGuard port
	configPath := "/etc/wireguard/wg0.conf"

	// Try to extract Wireguard configuration from application config, if provided.
	if appCfg, ok := cfg.(*config.Config); ok && appCfg != nil {
		if appCfg.Wireguard.Interface != "" {
			iface = appCfg.Wireguard.Interface
		}
		if appCfg.Wireguard.ListenPort != 0 {
			listenPort = appCfg.Wireguard.ListenPort
		}
		if appCfg.Wireguard.ConfigPath != "" {
			configPath = appCfg.Wireguard.ConfigPath
		}
	}

	server := &wgServerService{
		client:     client,
		iface:      iface,
		listenPort: listenPort,
		configPath: configPath,
		peers:      make(map[string]*types.PeerInfo),
		ctx:        ctx,
		cancel:     cancel,
		logger:     logger,
	}

	// Detect dev environment and disable WireGuard OS interactions if so.
	if appCfg, ok := cfg.(*config.Config); ok && appCfg != nil && appCfg.Env == "dev" {
		server.disabled = true
		if server.logger != nil {
			server.logger.Info("WireGuard server service disabled for dev environment; skipping OS configuration")
		}
	} else {
		// Load or generate server keys only when not disabled.
		if err := server.loadOrGenerateKeys(); err != nil {
			client.Close()
			return nil, fmt.Errorf("failed to load or generate keys: %w", err)
		}
	}

	return server, nil
}

// Name returns the service name
func (w *wgServerService) Name() string {
	return "wireguard-server"
}

// SetEventBus sets the event bus for publishing events
func (w *wgServerService) SetEventBus(bus interface{}) {
	w.eventBus = bus
}

// Start starts the WireGuard server
func (w *wgServerService) Start(ctx context.Context) error {
	if w.disabled {
		if w.logger != nil {
			w.logger.Info("WireGuard server service is disabled in this environment; skipping start")
		}
		return nil
	}

	if w.logger != nil {
		w.logger.Info("Starting WireGuard server",
			zap.String("interface", w.iface),
			zap.Int("listen_port", w.listenPort),
			zap.String("public_key", w.publicKey.String()))
	}

	// Configure WireGuard interface
	if err := w.configureInterface(); err != nil {
		return fmt.Errorf("failed to configure interface: %w", err)
	}

	// Start peer monitoring
	go w.monitorPeers(ctx)

	if w.logger != nil {
		w.logger.Info("WireGuard server started successfully")
	}
	return nil
}

// Stop stops the WireGuard server
func (w *wgServerService) Stop(ctx context.Context) error {
	if w.logger != nil {
		w.logger.Info("Stopping WireGuard server")
	}

	w.cancel()

	// Remove WireGuard interface (optional - may want to keep it running)
	// For now, we'll just close the client
	if w.client != nil {
		w.client.Close()
	}

	if w.logger != nil {
		w.logger.Info("WireGuard server stopped")
	}
	return nil
}

// loadOrGenerateKeys loads server keys from config file or generates new ones
func (w *wgServerService) loadOrGenerateKeys() error {
	// Try to load from configured config file location (if any), falling back to default.
	configPath := w.configPath
	if configPath == "" {
		configPath = "/etc/wireguard/wg0.conf"
	}
	if _, err := os.Stat(configPath); err == nil {
		configData, err := os.ReadFile(configPath)
		if err == nil {
			// Parse config file to extract PrivateKey
			lines := strings.Split(string(configData), "\n")
			var inInterface bool
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "[Interface]" {
					inInterface = true
					continue
				}
				if strings.HasPrefix(trimmed, "[") {
					inInterface = false
					continue
				}
				if inInterface && strings.HasPrefix(trimmed, "PrivateKey") {
					parts := strings.SplitN(trimmed, "=", 2)
					if len(parts) == 2 {
						keyStr := strings.TrimSpace(parts[1])
						privateKey, err := wgtypes.ParseKey(keyStr)
						if err == nil {
							w.privateKey = privateKey
							w.publicKey = privateKey.PublicKey()
							if w.logger != nil {
								w.logger.Info("Loaded WireGuard keys from config file", zap.String("config_path", configPath))
							}
							return nil
						}
					}
				}
			}
		}
	}

	// Generate new keys (fallback)
	if w.logger != nil {
		w.logger.Info("Generating new WireGuard server keys")
	}
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	w.privateKey = privateKey
	w.publicKey = privateKey.PublicKey()

	return nil
}

// saveKeysToFile saves WireGuard keys to a config file
func (w *wgServerService) saveKeysToFile(configPath string) error {
	// Create directory if needed
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config file
	config := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = 10.0.0.1/24
ListenPort = %d

`, w.privateKey.String(), w.listenPort)

	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if w.logger != nil {
		w.logger.Info("Saved WireGuard config", zap.String("path", configPath))
	}
	return nil
}

// configureInterface configures the WireGuard interface
func (w *wgServerService) configureInterface() error {
	// Check if interface already exists
	dev, err := w.client.Device(w.iface)
	if err != nil {
		// In dev we might not have CAP_NET_ADMIN; treat lack of access as
		// "assume preconfigured" instead of a hard failure.
		if w.logger != nil {
			w.logger.Warn("WireGuard device not accessible, assuming preconfigured interface",
				zap.String("interface", w.iface),
				zap.Error(err))
		}
		return nil
	}

	if dev != nil {
		if w.logger != nil {
			w.logger.Info("WireGuard interface already exists", zap.String("interface", w.iface))
		}
		// If interface exists, try to reload config from file if available
		configPath := w.configPath
		if configPath == "" {
			configPath = "/etc/wireguard/wg0.conf"
		}
		if _, err := os.Stat(configPath); err == nil {
			if err := w.loadConfigFromFile(configPath); err != nil {
				if w.logger != nil {
					w.logger.Warn("Failed to load config from file, using programmatic config", zap.Error(err))
				}
				return w.updateInterfaceConfig()
			}
			return nil
		}
		return w.updateInterfaceConfig()
	}

	// Device does not exist: this is considered a critical VM configuration error.
	if w.logger != nil {
		w.logger.Error("WireGuard interface not found",
			zap.String("interface", w.iface),
			zap.Error(err))
	}
	return fmt.Errorf("wireguard interface %s not found; ensure it is created before starting the orchestrator", w.iface)
}

// parseWireGuardConfig parses a WireGuard config file and returns the configuration values
func parseWireGuardConfig(configData string) (privateKey string, listenPort string, peerPublicKey string, peerAllowedIPs string, peerPresharedKey string, err error) {
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
			} else if strings.HasPrefix(trimmed, "ListenPort") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					listenPort = strings.TrimSpace(parts[1])
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
			} else if strings.HasPrefix(trimmed, "PresharedKey") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					peerPresharedKey = strings.TrimSpace(parts[1])
				}
			}
		}
	}

	if privateKey == "" {
		return "", "", "", "", "", fmt.Errorf("PrivateKey not found in config")
	}

	return privateKey, listenPort, peerPublicKey, peerAllowedIPs, peerPresharedKey, nil
}

// loadConfigFromFile loads WireGuard configuration from the config file using wg set commands
func (w *wgServerService) loadConfigFromFile(configPath string) error {
	if configPath == "" {
		return fmt.Errorf("config path not specified")
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", configPath)
	}

	// Read config file
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse config file
	privateKey, listenPort, peerPublicKey, peerAllowedIPs, peerPresharedKey, err := parseWireGuardConfig(string(configData))
	if err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Configure interface using individual wg set commands (more reliable than wg setconf)
	// Set private key
	cmd := exec.Command("wg", "set", w.iface, "private-key", "/dev/stdin")
	cmd.Stdin = strings.NewReader(privateKey)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set private key: %w, output: %s", err, string(output))
	}

	// Set listen port if specified
	if listenPort != "" {
		portCmd := exec.Command("wg", "set", w.iface, "listen-port", listenPort)
		if output, err := portCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set listen port: %w, output: %s", err, string(output))
		}
	}

	// Add peer if configured
	if peerPublicKey != "" {
		peerCmd := exec.Command("wg", "set", w.iface, "peer", peerPublicKey)
		if peerAllowedIPs != "" {
			peerCmd.Args = append(peerCmd.Args, "allowed-ips", peerAllowedIPs)
		}
		if peerPresharedKey != "" {
			peerCmd.Args = append(peerCmd.Args, "preshared-key", "/dev/stdin")
			peerCmd.Stdin = strings.NewReader(peerPresharedKey)
		}
		if output, err := peerCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add peer: %w, output: %s", err, string(output))
		}
	}

	if w.logger != nil {
		w.logger.Info("Loaded WireGuard config from file", zap.String("path", configPath))
	}
	return nil
}

// updateInterfaceConfig updates the WireGuard interface configuration
func (w *wgServerService) updateInterfaceConfig() error {
	cfg := wgtypes.Config{
		PrivateKey:   &w.privateKey,
		ListenPort:   &w.listenPort,
		ReplacePeers: false, // Don't replace existing peers
	}

	if err := w.client.ConfigureDevice(w.iface, cfg); err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	if w.logger != nil {
		w.logger.Info("WireGuard interface configured", zap.String("interface", w.iface))
	}
	return nil
}

// AddPeer adds a new peer to the WireGuard interface
func (w *wgServerService) AddPeer(publicKey wgtypes.Key, allowedIPs []net.IPNet) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	cfg := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey:  publicKey,
				AllowedIPs: allowedIPs,
			},
		},
	}

	if err := w.client.ConfigureDevice(w.iface, cfg); err != nil {
		return fmt.Errorf("failed to add peer: %w", err)
	}

	// Store peer info
	w.peers[publicKey.String()] = &types.PeerInfo{
		PublicKey:  publicKey,
		AllowedIPs: allowedIPs,
		Connected:  false,
	}

	if w.logger != nil {
		w.logger.Info("Added WireGuard peer", zap.String("public_key", publicKey.String()))
	}
	return nil
}

// RemovePeer removes a peer from the WireGuard interface
func (w *wgServerService) RemovePeer(publicKey wgtypes.Key) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	cfg := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey: publicKey,
				Remove:    true,
			},
		},
	}

	if err := w.client.ConfigureDevice(w.iface, cfg); err != nil {
		return fmt.Errorf("failed to remove peer: %w", err)
	}

	// Remove from peers map
	delete(w.peers, publicKey.String())

	if w.logger != nil {
		w.logger.Info("Removed WireGuard peer", zap.String("public_key", publicKey.String()))
	}
	return nil
}

// GetPublicKey returns the server's public key
func (w *wgServerService) GetPublicKey() wgtypes.Key {
	return w.publicKey
}

// GetListenPort returns the server's listen port
func (w *wgServerService) GetListenPort() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.listenPort
}

// GetInterface returns the WireGuard interface name
func (w *wgServerService) GetInterface() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.iface
}

// loadPeersFromDatabase is removed - database functionality not available during refactoring

// monitorPeers periodically checks peer connection status
func (w *wgServerService) monitorPeers(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.updatePeerStatus()
		}
	}
}

// updatePeerStatus updates the connection status of all peers
func (w *wgServerService) updatePeerStatus() {
	dev, err := w.client.Device(w.iface)
	if err != nil {
		if w.logger != nil {
			w.logger.Warn("Failed to get device status", zap.Error(err))
		}
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, peer := range dev.Peers {
		peerKey := peer.PublicKey.String()
		peerInfo, exists := w.peers[peerKey]

		// If peer doesn't exist in our map, add it (it might be configured in WireGuard config file)
		if !exists {
			peerInfo = &types.PeerInfo{
				PublicKey:     peer.PublicKey,
				AllowedIPs:    peer.AllowedIPs,
				Endpoint:      peer.Endpoint,
				Connected:     !peer.LastHandshakeTime.IsZero() && time.Since(peer.LastHandshakeTime) < 3*time.Minute,
				LastHandshake: peer.LastHandshakeTime,
				BytesReceived: uint64(peer.ReceiveBytes),
				BytesSent:     uint64(peer.TransmitBytes),
			}
			w.peers[peerKey] = peerInfo
			if w.logger != nil {
				w.logger.Info("Discovered WireGuard peer from interface",
					zap.String("public_key", peerKey),
					zap.Bool("connected", peerInfo.Connected))
			}
		}

		// Update connection status based on last handshake
		// Note: We can't access peerInfo.mu directly (it's unexported), but we're already
		// holding w.mu, so updates are safe. The peerInfo struct will need methods for
		// thread-safe access if needed later.
		wasConnected := peerInfo.Connected
		peerInfo.Connected = !peer.LastHandshakeTime.IsZero() && time.Since(peer.LastHandshakeTime) < 3*time.Minute
		peerInfo.LastHandshake = peer.LastHandshakeTime

		// Update transfer statistics
		if peer.ReceiveBytes >= 0 {
			peerInfo.BytesReceived = uint64(peer.ReceiveBytes)
		}
		if peer.TransmitBytes >= 0 {
			peerInfo.BytesSent = uint64(peer.TransmitBytes)
		}

		// Check for disconnection (no handshake for 3 minutes)
		if !peer.LastHandshakeTime.IsZero() && time.Since(peer.LastHandshakeTime) > 3*time.Minute {
			peerInfo.Connected = false
		}

		// Log connection state changes
		if w.logger != nil {
			if !wasConnected && peerInfo.Connected {
				w.logger.Info("WireGuard peer connected",
					zap.String("public_key", peerKey),
					zap.Time("last_handshake", peer.LastHandshakeTime))
			}
		}
	}
}

// GetPeerInfo returns connection information for a peer
func (w *wgServerService) GetPeerInfo(publicKey wgtypes.Key) (*types.PeerInfo, bool) {
	w.mu.RLock()
	peerInfo, exists := w.peers[publicKey.String()]
	w.mu.RUnlock()

	// If not found in map, try to get from WireGuard interface directly
	if !exists || peerInfo == nil {
		dev, err := w.client.Device(w.iface)
		if err == nil {
			for _, peer := range dev.Peers {
				if peer.PublicKey.String() == publicKey.String() {
					// Found in interface, add to map
					w.mu.Lock()
					peerInfo = &types.PeerInfo{
						PublicKey:     peer.PublicKey,
						AllowedIPs:    peer.AllowedIPs,
						Endpoint:      peer.Endpoint,
						Connected:     !peer.LastHandshakeTime.IsZero() && time.Since(peer.LastHandshakeTime) < 3*time.Minute,
						LastHandshake: peer.LastHandshakeTime,
						BytesReceived: uint64(peer.ReceiveBytes),
						BytesSent:     uint64(peer.TransmitBytes),
					}
					w.peers[publicKey.String()] = peerInfo
					w.mu.Unlock()
					if w.logger != nil {
						w.logger.Debug("Found WireGuard peer from interface",
							zap.String("public_key", publicKey.String()),
							zap.Bool("connected", peerInfo.Connected))
					}
					return peerInfo, true
				}
			}
		}
	}

	return peerInfo, exists
}

// GetConnectedPeers returns list of connected peer public keys
func (w *wgServerService) GetConnectedPeers() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	connected := make([]string, 0)
	for peerKey, peerInfo := range w.peers {
		// Note: We're already holding w.mu.RLock, so accessing peerInfo.Connected is safe
		// The peerInfo.mu is unexported, so we can't use it here
		if peerInfo.Connected {
			connected = append(connected, peerKey)
		}
	}
	return connected
}
