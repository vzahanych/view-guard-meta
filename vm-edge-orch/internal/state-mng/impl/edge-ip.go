package impl

import (
	"fmt"
	"sync"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	edgegateway "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway"
	wgserver "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/wg-server-service"
)

// EdgeIPResolver provides a thread-safe way to resolve Edge device IP addresses
// from WireGuard peer information.
type EdgeIPResolver struct {
	gateway edgegateway.EdgeGateway
	mu      sync.RWMutex
	cache   map[string]string // Cache: WG public key -> IP address
}

// NewEdgeIPResolver creates a new EdgeIPResolver with the given EdgeGateway.
func NewEdgeIPResolver(gateway edgegateway.EdgeGateway) *EdgeIPResolver {
	return &EdgeIPResolver{
		gateway: gateway,
		cache:   make(map[string]string),
	}
}

// GetEdgeIP resolves the WireGuard IP address for an Edge device given its WireGuard public key.
// The function is thread-safe and caches results for performance.
//
// Parameters:
//   - wgPublicKey: The WireGuard public key of the Edge device (as a string)
//
// Returns:
//   - IP address as a string (e.g., "10.0.0.2")
//   - error if the Edge peer is not found or has no allowed IPs
func (r *EdgeIPResolver) GetEdgeIP(wgPublicKey string) (string, error) {
	if wgPublicKey == "" {
		return "", fmt.Errorf("wireguard public key is required")
	}

	// Check cache first (read lock)
	r.mu.RLock()
	if cachedIP, ok := r.cache[wgPublicKey]; ok {
		r.mu.RUnlock()
		return cachedIP, nil
	}
	r.mu.RUnlock()

	// Cache miss - resolve from WireGuard server
	if r.gateway == nil {
		return "", fmt.Errorf("edge gateway not available")
	}

	// Get WireGuard server service
	wgServerInterface := r.gateway.GetWGServerService()
	if wgServerInterface == nil {
		return "", fmt.Errorf("wireguard server service not available")
	}

	wgServer, ok := wgServerInterface.(wgserver.WGServerService)
	if !ok {
		return "", fmt.Errorf("wireguard server service does not implement WGServerService interface")
	}

	// Parse WireGuard public key
	wgKey, err := wgtypes.ParseKey(wgPublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse WireGuard public key: %w", err)
	}

	// Get peer info to find Edge's IP address
	peerInfo, exists := wgServer.GetPeerInfo(wgKey)
	if !exists || peerInfo == nil {
		return "", fmt.Errorf("edge peer not found in WireGuard interface")
	}

	// Extract IP address from peer info
	// PeerInfo.AllowedIPs contains the Edge's WireGuard IP addresses
	if len(peerInfo.AllowedIPs) == 0 {
		return "", fmt.Errorf("edge peer has no allowed IPs")
	}

	// Get first allowed IP (Edge's WireGuard IP, e.g., 10.0.0.2)
	// Extract the IP from the first AllowedIPs entry
	edgeIP := peerInfo.AllowedIPs[0].IP.String()
	if edgeIP == "" || edgeIP == "<nil>" {
		return "", fmt.Errorf("failed to extract edge IP from peer info")
	}

	// Cache the result (write lock)
	r.mu.Lock()
	r.cache[wgPublicKey] = edgeIP
	r.mu.Unlock()

	return edgeIP, nil
}

// ClearCache clears the IP address cache. Useful when peers are removed or reconfigured.
func (r *EdgeIPResolver) ClearCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]string)
}

// InvalidateCacheEntry removes a specific entry from the cache.
func (r *EdgeIPResolver) InvalidateCacheEntry(wgPublicKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, wgPublicKey)
}

