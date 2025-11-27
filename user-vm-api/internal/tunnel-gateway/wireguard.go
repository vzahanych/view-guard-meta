package tunnelgateway

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/service"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// WireGuardServer manages the WireGuard server interface and peers
type WireGuardServer struct {
	config     *config.Config
	logger     *logging.Logger
	db         *database.DB
	client     *wgctrl.Client
	iface      string
	listenPort int
	privateKey wgtypes.Key
	publicKey  wgtypes.Key
	mu         sync.RWMutex
	peers      map[string]*PeerInfo
	eventBus   *service.EventBus
	ctx        context.Context
	cancel     context.CancelFunc
}

// PeerInfo contains information about a WireGuard peer
type PeerInfo struct {
	PublicKey      wgtypes.Key
	AllowedIPs     []net.IPNet
	Endpoint       *net.UDPAddr
	LastHandshake  time.Time
	Connected      bool
	Latency        time.Duration // Measured latency
	LastPingTime   time.Time     // Last ping sent
	LastPongTime   time.Time     // Last pong received
	PingCount      int64         // Total ping count
	PongCount      int64         // Total pong count
	BytesReceived  uint64        // Total bytes received
	BytesSent      uint64        // Total bytes sent
	mu             sync.RWMutex
}

// NewWireGuardServer creates a new WireGuard server instance
func NewWireGuardServer(cfg *config.Config, log *logging.Logger, db *database.DB) (*WireGuardServer, error) {
	if !cfg.UserVMAPI.WireGuardServer.Enabled {
		return nil, fmt.Errorf("wireguard server is disabled")
	}

	// Create wgctrl client
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create wgctrl client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	server := &WireGuardServer{
		config:     cfg,
		logger:     log,
		db:         db,
		client:     client,
		iface:      cfg.UserVMAPI.WireGuardServer.Interface,
		listenPort: cfg.UserVMAPI.WireGuardServer.ListenPort,
		peers:      make(map[string]*PeerInfo),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Load or generate server keys
	if err := server.loadOrGenerateKeys(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to load or generate keys: %w", err)
	}

	return server, nil
}

// Name returns the service name
func (w *WireGuardServer) Name() string {
	return "wireguard-server"
}

// SetEventBus sets the event bus for publishing events
func (w *WireGuardServer) SetEventBus(bus *service.EventBus) {
	w.eventBus = bus
}

// Start starts the WireGuard server
func (w *WireGuardServer) Start(ctx context.Context) error {
	w.logger.Info("Starting WireGuard server",
		zap.String("interface", w.iface),
		zap.Int("listen_port", w.listenPort),
		zap.String("public_key", w.publicKey.String()))

	// Configure WireGuard interface
	if err := w.configureInterface(); err != nil {
		return fmt.Errorf("failed to configure interface: %w", err)
	}

	// Load existing peers from database
	if err := w.loadPeersFromDatabase(ctx); err != nil {
		w.logger.Warn("Failed to load peers from database", zap.Error(err))
	}

	// Start peer monitoring
	go w.monitorPeers(ctx)

	w.logger.Info("WireGuard server started successfully")
	return nil
}

// Stop stops the WireGuard server
func (w *WireGuardServer) Stop(ctx context.Context) error {
	w.logger.Info("Stopping WireGuard server")

	w.cancel()

	// Remove WireGuard interface (optional - may want to keep it running)
	// For now, we'll just close the client
	if w.client != nil {
		w.client.Close()
	}

	w.logger.Info("WireGuard server stopped")
	return nil
}

// loadOrGenerateKeys loads server keys from config file or generates new ones
func (w *WireGuardServer) loadOrGenerateKeys() error {
	cfg := w.config.UserVMAPI.WireGuardServer

	// Try to load from config file first (if it exists and contains PrivateKey)
	if cfg.ConfigPath != "" {
		if _, err := os.Stat(cfg.ConfigPath); err == nil {
			// Config file exists, try to load PrivateKey from it
			configData, err := os.ReadFile(cfg.ConfigPath)
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
								w.logger.Info("Loaded WireGuard keys from config file", zap.String("config_path", cfg.ConfigPath))
								return nil
							}
						}
					}
				}
			}
		}
	}

	// Try to load from separate key files if paths are specified
	if cfg.PrivateKey != "" && cfg.PublicKey != "" {
		privateKeyData, err := os.ReadFile(cfg.PrivateKey)
		if err == nil {
			privateKey, err := wgtypes.ParseKey(strings.TrimSpace(string(privateKeyData)))
			if err == nil {
				publicKeyData, err := os.ReadFile(cfg.PublicKey)
				if err == nil {
					publicKey, err := wgtypes.ParseKey(strings.TrimSpace(string(publicKeyData)))
					if err == nil {
						w.privateKey = privateKey
						w.publicKey = publicKey
						w.logger.Info("Loaded WireGuard keys from files")
						return nil
					}
				}
			}
		}
	}

	// Generate new keys (fallback)
	w.logger.Info("Generating new WireGuard server keys")
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	w.privateKey = privateKey
	w.publicKey = privateKey.PublicKey()

	// Don't try to save to config path if it's read-only (mounted volume)
	// The config file should be provided by the setup process

	return nil
}

// saveKeysToFile saves WireGuard keys to a config file
func (w *WireGuardServer) saveKeysToFile(configPath string) error {
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

	w.logger.Info("Saved WireGuard config", zap.String("path", configPath))
	return nil
}

// configureInterface configures the WireGuard interface
func (w *WireGuardServer) configureInterface() error {
	// Check if interface already exists
	dev, err := w.client.Device(w.iface)
	if err == nil && dev != nil {
		w.logger.Info("WireGuard interface already exists", zap.String("interface", w.iface))
		// If interface exists, try to reload config from file if available
		return w.loadConfigFromFile()
	}

	// If the device does not exist yet, create it using iproute2.
	w.logger.Info("Creating WireGuard interface", zap.String("interface", w.iface))

	cmd := exec.Command("ip", "link", "add", "dev", w.iface, "type", "wireguard")
	if output, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
		// Ignore "File exists" error
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to create WireGuard interface %s: %w (output: %s)", w.iface, cmdErr, string(output))
		}
	}

	// Load configuration from file if available (includes peer configuration)
	configPath := w.config.UserVMAPI.WireGuardServer.ConfigPath
	if configPath != "" {
		if err := w.loadConfigFromFile(); err != nil {
			w.logger.Warn("Failed to load config from file, using programmatic config", zap.Error(err), zap.String("config_path", configPath))
			// Fall back to programmatic configuration
			if err := w.updateInterfaceConfig(); err != nil {
				return err
			}
		} else {
			w.logger.Info("Loaded WireGuard config from file", zap.String("config_path", configPath))
		}
	} else {
		// No config file, use programmatic configuration
		w.logger.Info("No config file specified, using programmatic configuration")
		if err := w.updateInterfaceConfig(); err != nil {
			return err
		}
	}

	// Add IP address to the interface (wg setconf doesn't set Address)
	addrCmd := exec.Command("ip", "addr", "add", "10.0.0.1/24", "dev", w.iface)
	if output, cmdErr := addrCmd.CombinedOutput(); cmdErr != nil {
		// Ignore "File exists" error (address already assigned)
		if !strings.Contains(string(output), "File exists") && !strings.Contains(string(output), "already assigned") {
			return fmt.Errorf("failed to add IP address to WireGuard interface %s: %w (output: %s)", w.iface, cmdErr, string(output))
		}
	}

	// Bring the interface up so it can start accepting traffic.
	upCmd := exec.Command("ip", "link", "set", "up", "dev", w.iface)
	if output, cmdErr := upCmd.CombinedOutput(); cmdErr != nil {
		return fmt.Errorf("failed to bring WireGuard interface %s up: %w (output: %s)", w.iface, cmdErr, string(output))
	}

	w.logger.Info("WireGuard interface configured and up", zap.String("interface", w.iface), zap.String("address", "10.0.0.1/24"))
	return nil
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
func (w *WireGuardServer) loadConfigFromFile() error {
	configPath := w.config.UserVMAPI.WireGuardServer.ConfigPath
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

	w.logger.Info("Loaded WireGuard config from file", zap.String("path", configPath))
	return nil
}

// updateInterfaceConfig updates the WireGuard interface configuration
func (w *WireGuardServer) updateInterfaceConfig() error {
	cfg := wgtypes.Config{
		PrivateKey:   &w.privateKey,
		ListenPort:   &w.listenPort,
		ReplacePeers: false, // Don't replace existing peers
	}

	if err := w.client.ConfigureDevice(w.iface, cfg); err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	w.logger.Info("WireGuard interface configured", zap.String("interface", w.iface))
	return nil
}

// AddPeer adds a new peer to the WireGuard interface
func (w *WireGuardServer) AddPeer(publicKey wgtypes.Key, allowedIPs []net.IPNet) error {
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
	w.peers[publicKey.String()] = &PeerInfo{
		PublicKey:   publicKey,
		AllowedIPs:  allowedIPs,
		Connected:   false,
	}

	w.logger.Info("Added WireGuard peer", zap.String("public_key", publicKey.String()))
	return nil
}

// RemovePeer removes a peer from the WireGuard interface
func (w *WireGuardServer) RemovePeer(publicKey wgtypes.Key) error {
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

	w.logger.Info("Removed WireGuard peer", zap.String("public_key", publicKey.String()))
	return nil
}

// GetPublicKey returns the server's public key
func (w *WireGuardServer) GetPublicKey() wgtypes.Key {
	return w.publicKey
}

// GetListenPort returns the server's listen port
func (w *WireGuardServer) GetListenPort() int {
	return w.listenPort
}

// loadPeersFromDatabase loads peer information from the database
func (w *WireGuardServer) loadPeersFromDatabase(ctx context.Context) error {
	rows, err := w.db.QueryContext(ctx, "SELECT edge_id, wireguard_public_key FROM edges WHERE status = 'active'")
	if err != nil {
		return fmt.Errorf("failed to query edges: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var edgeID, publicKeyStr string
		if err := rows.Scan(&edgeID, &publicKeyStr); err != nil {
			w.logger.Warn("Failed to scan edge row", zap.Error(err))
			continue
		}

		publicKey, err := wgtypes.ParseKey(publicKeyStr)
		if err != nil {
			w.logger.Warn("Failed to parse public key", zap.String("edge_id", edgeID), zap.Error(err))
			continue
		}

		// Add peer with default allowed IPs (10.0.0.x/32)
		allowedIPs := []net.IPNet{
			{
				IP:   net.IPv4(10, 0, 0, 2), // Will be assigned per edge
				Mask: net.CIDRMask(32, 32),
			},
		}

		if err := w.AddPeer(publicKey, allowedIPs); err != nil {
			w.logger.Warn("Failed to add peer from database", zap.String("edge_id", edgeID), zap.Error(err))
		}
	}

	return nil
}

// monitorPeers periodically checks peer connection status
func (w *WireGuardServer) monitorPeers(ctx context.Context) {
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
func (w *WireGuardServer) updatePeerStatus() {
	dev, err := w.client.Device(w.iface)
	if err != nil {
		w.logger.Warn("Failed to get device status", zap.Error(err))
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, peer := range dev.Peers {
		peerKey := peer.PublicKey.String()
		peerInfo, exists := w.peers[peerKey]
		if !exists {
			continue
		}

		peerInfo.mu.Lock()
		// Update connection status based on last handshake
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
		peerInfo.mu.Unlock()

		// Publish events for connection state changes
		if w.eventBus != nil {
			if !wasConnected && peerInfo.Connected {
				w.eventBus.Publish(service.Event{
					Type:      service.EventTypeWireGuardClientConnected,
					Timestamp: time.Now().Unix(),
					Data: map[string]interface{}{
						"public_key":      peerKey,
						"last_handshake":  peer.LastHandshakeTime.Unix(),
						"bytes_received":  peer.ReceiveBytes,
						"bytes_sent":      peer.TransmitBytes,
					},
				})
			} else if wasConnected && !peerInfo.Connected {
				w.eventBus.Publish(service.Event{
					Type:      service.EventTypeWireGuardClientDisconnected,
					Timestamp: time.Now().Unix(),
					Data: map[string]interface{}{
						"public_key":     peerKey,
						"last_handshake": peer.LastHandshakeTime.Unix(),
					},
				})
			}
		}
	}
}

// GetPeerInfo returns connection information for a peer
func (w *WireGuardServer) GetPeerInfo(publicKey wgtypes.Key) (*PeerInfo, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	peerInfo, exists := w.peers[publicKey.String()]
	return peerInfo, exists
}

// GetConnectedPeers returns list of connected peer public keys
func (w *WireGuardServer) GetConnectedPeers() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	connected := make([]string, 0)
	for peerKey, peerInfo := range w.peers {
		peerInfo.mu.RLock()
		if peerInfo.Connected {
			connected = append(connected, peerKey)
		}
		peerInfo.mu.RUnlock()
	}
	return connected
}

// GetPeerLatency returns the measured latency for a peer
func (w *WireGuardServer) GetPeerLatency(publicKey wgtypes.Key) (time.Duration, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	peerInfo, exists := w.peers[publicKey.String()]
	if !exists {
		return 0, false
	}

	peerInfo.mu.RLock()
	defer peerInfo.mu.RUnlock()
	return peerInfo.Latency, true
}

// UpdatePeerLatency updates the latency measurement for a peer
func (w *WireGuardServer) UpdatePeerLatency(publicKey wgtypes.Key, latency time.Duration) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	peerInfo, exists := w.peers[publicKey.String()]
	if !exists {
		return
	}

	peerInfo.mu.Lock()
	peerInfo.Latency = latency
	peerInfo.LastPongTime = time.Now()
	peerInfo.PongCount++
	peerInfo.mu.Unlock()
}

// RecordPing records a ping sent to a peer
func (w *WireGuardServer) RecordPing(publicKey wgtypes.Key) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	peerInfo, exists := w.peers[publicKey.String()]
	if !exists {
		return
	}

	peerInfo.mu.Lock()
	peerInfo.LastPingTime = time.Now()
	peerInfo.PingCount++
	peerInfo.mu.Unlock()
}

// GenerateKeyPair generates a new WireGuard key pair for Edge clients
func GenerateKeyPair() (privateKey wgtypes.Key, publicKey wgtypes.Key, err error) {
	privateKey, err = wgtypes.GeneratePrivateKey()
	if err != nil {
		return wgtypes.Key{}, wgtypes.Key{}, fmt.Errorf("failed to generate private key: %w", err)
	}
	publicKey = privateKey.PublicKey()
	return privateKey, publicKey, nil
}

// GenerateBootstrapToken generates a bootstrap token for Edge registration
func GenerateBootstrapToken() (string, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(token), nil
}

// DeriveAllowedIP generates an allowed IP for a peer based on edge ID
func DeriveAllowedIP(edgeID string, index int) net.IPNet {
	// Simple hash-based IP assignment (for PoC)
	// In production, use a proper IPAM system
	ip := net.IPv4(10, 0, 0, byte(2+index))
	return net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(32, 32),
	}
}
