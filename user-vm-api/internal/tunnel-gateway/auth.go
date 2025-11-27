package tunnelgateway

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// EdgeAuth handles Edge Appliance authentication and authorization
type EdgeAuth struct {
	config       *config.Config
	logger       *logging.Logger
	db           *database.DB
	wgServer     *WireGuardServer
	bootstrapTokens map[string]time.Time // token -> expiration time
	mu           sync.RWMutex
}

// EdgeRegistrationRequest represents a request to register a new Edge Appliance
type EdgeRegistrationRequest struct {
	BootstrapToken string
	EdgeName       string
	PublicKey      string
}

// EdgeRegistrationResponse represents the response to an Edge registration
type EdgeRegistrationResponse struct {
	EdgeID      string
	ServerPublicKey string
	ServerEndpoint   string
	AllowedIPs  []string
	Config      string // WireGuard client configuration
}

// NewEdgeAuth creates a new Edge authentication manager
func NewEdgeAuth(cfg *config.Config, log *logging.Logger, db *database.DB, wgServer *WireGuardServer) *EdgeAuth {
	return &EdgeAuth{
		config:          cfg,
		logger:          log,
		db:              db,
		wgServer:        wgServer,
		bootstrapTokens: make(map[string]time.Time),
	}
}

// RegisterEdge registers a new Edge Appliance
func (e *EdgeAuth) RegisterEdge(ctx context.Context, req *EdgeRegistrationRequest) (*EdgeRegistrationResponse, error) {
	// Validate bootstrap token
	if !e.validateBootstrapToken(req.BootstrapToken) {
		return nil, fmt.Errorf("invalid or expired bootstrap token")
	}

	// Parse public key
	publicKey, err := wgtypes.ParseKey(req.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	// Check if edge already exists
	var existingEdgeID string
	err = e.db.QueryRowContext(ctx,
		"SELECT edge_id FROM edges WHERE wireguard_public_key = ?",
		req.PublicKey).Scan(&existingEdgeID)

	if err == nil {
		// Edge already exists, return existing registration
		return e.getEdgeRegistration(ctx, existingEdgeID)
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check existing edge: %w", err)
	}

	// Generate edge ID
	edgeID, err := e.generateEdgeID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate edge ID: %w", err)
	}

	// Derive allowed IP for this edge
	allowedIP := DeriveAllowedIP(edgeID, 0)
	allowedIPs := []net.IPNet{allowedIP}

	// Add peer to WireGuard
	if err := e.wgServer.AddPeer(publicKey, allowedIPs); err != nil {
		return nil, fmt.Errorf("failed to add peer: %w", err)
	}

	// Store edge in database
	now := time.Now().Unix()
	_, err = e.db.ExecContext(ctx,
		`INSERT INTO edges (edge_id, name, wireguard_public_key, last_seen, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		edgeID, req.EdgeName, req.PublicKey, now, "active", now, now)
	if err != nil {
		// Rollback peer addition
		e.wgServer.RemovePeer(publicKey)
		return nil, fmt.Errorf("failed to store edge: %w", err)
	}

	// Remove bootstrap token (single use)
	e.removeBootstrapToken(req.BootstrapToken)

	e.logger.Info("Registered new Edge Appliance",
		zap.String("edge_id", edgeID),
		zap.String("name", req.EdgeName),
		zap.String("public_key", req.PublicKey))

	// Generate client configuration
	config := e.generateClientConfig(edgeID, publicKey, allowedIP)

	return &EdgeRegistrationResponse{
		EdgeID:          edgeID,
		ServerPublicKey: e.wgServer.GetPublicKey().String(),
		ServerEndpoint:  fmt.Sprintf("%s:%d", e.getServerEndpoint(), e.wgServer.GetListenPort()),
		AllowedIPs:      []string{allowedIP.String()},
		Config:          config,
	}, nil
}

// AuthenticateEdge authenticates an existing Edge Appliance
func (e *EdgeAuth) AuthenticateEdge(ctx context.Context, publicKey string) (string, error) {
	var edgeID string
	var status string
	err := e.db.QueryRowContext(ctx,
		"SELECT edge_id, status FROM edges WHERE wireguard_public_key = ?",
		publicKey).Scan(&edgeID, &status)

	if err == sql.ErrNoRows {
		return "", fmt.Errorf("edge not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to query edge: %w", err)
	}

	if status != "active" {
		return "", fmt.Errorf("edge is not active")
	}

	// Update last seen
	now := time.Now().Unix()
	_, err = e.db.ExecContext(ctx,
		"UPDATE edges SET last_seen = ?, updated_at = ? WHERE edge_id = ?",
		now, now, edgeID)
	if err != nil {
		e.logger.Warn("Failed to update last_seen", zap.String("edge_id", edgeID), zap.Error(err))
	}

	return edgeID, nil
}

// GetEdgeRegistration retrieves registration information for an existing edge
func (e *EdgeAuth) GetEdgeRegistration(ctx context.Context, edgeID string) (*EdgeRegistrationResponse, error) {
	return e.getEdgeRegistration(ctx, edgeID)
}

func (e *EdgeAuth) getEdgeRegistration(ctx context.Context, edgeID string) (*EdgeRegistrationResponse, error) {
	var publicKeyStr string
	var name string
	err := e.db.QueryRowContext(ctx,
		"SELECT wireguard_public_key, name FROM edges WHERE edge_id = ?",
		edgeID).Scan(&publicKeyStr, &name)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("edge not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query edge: %w", err)
	}

	publicKey, err := wgtypes.ParseKey(publicKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	// Get allowed IP from WireGuard peer info
	allowedIP := DeriveAllowedIP(edgeID, 0)
	config := e.generateClientConfig(edgeID, publicKey, allowedIP)

	return &EdgeRegistrationResponse{
		EdgeID:          edgeID,
		ServerPublicKey: e.wgServer.GetPublicKey().String(),
		ServerEndpoint:  fmt.Sprintf("%s:%d", e.getServerEndpoint(), e.wgServer.GetListenPort()),
		AllowedIPs:      []string{allowedIP.String()},
		Config:          config,
	}, nil
}

// GenerateBootstrapToken generates a new bootstrap token for Edge registration
func (e *EdgeAuth) GenerateBootstrapToken() (string, error) {
	token, err := GenerateBootstrapToken()
	if err != nil {
		return "", err
	}

	// Token expires in 1 hour
	expiration := time.Now().Add(1 * time.Hour)

	e.mu.Lock()
	defer e.mu.Unlock()
	e.bootstrapTokens[token] = expiration

	e.logger.Info("Generated bootstrap token", zap.Time("expires_at", expiration))
	return token, nil
}

// validateBootstrapToken validates a bootstrap token
func (e *EdgeAuth) validateBootstrapToken(token string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	expiration, exists := e.bootstrapTokens[token]
	if !exists {
		return false
	}

	if time.Now().After(expiration) {
		// Token expired, remove it
		e.mu.RUnlock()
		e.mu.Lock()
		delete(e.bootstrapTokens, token)
		e.mu.Unlock()
		e.mu.RLock()
		return false
	}

	return true
}

// removeBootstrapToken removes a bootstrap token (single use)
func (e *EdgeAuth) removeBootstrapToken(token string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.bootstrapTokens, token)
}

// generateEdgeID generates a unique edge ID
func (e *EdgeAuth) generateEdgeID() (string, error) {
	// Generate UUID-like ID
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", fmt.Errorf("failed to generate ID: %w", err)
	}

	// Format as edge-{base64}
	encoded := base64.URLEncoding.EncodeToString(id)
	return fmt.Sprintf("edge-%s", encoded[:12]), nil
}

// generateClientConfig generates a WireGuard client configuration
func (e *EdgeAuth) generateClientConfig(edgeID string, publicKey wgtypes.Key, allowedIP net.IPNet) string {
	config := fmt.Sprintf(`[Interface]
PrivateKey = <CLIENT_PRIVATE_KEY>
Address = %s

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = 10.0.0.0/24
PersistentKeepalive = 25

`, allowedIP.IP.String(), e.wgServer.GetPublicKey().String(), e.getServerEndpoint())
	return config
}

// getServerEndpoint returns the server endpoint address
func (e *EdgeAuth) getServerEndpoint() string {
	// For PoC, use localhost or Docker service name
	// In production, this would be the public IP or domain
	return "localhost" // TODO: Get from config or environment
}

// CleanupExpiredTokens removes expired bootstrap tokens
func (e *EdgeAuth) CleanupExpiredTokens() {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	for token, expiration := range e.bootstrapTokens {
		if now.After(expiration) {
			delete(e.bootstrapTokens, token)
		}
	}
}
