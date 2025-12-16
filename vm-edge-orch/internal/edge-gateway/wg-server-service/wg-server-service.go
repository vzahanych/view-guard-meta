package wgserver

import (
	"context"
	"net"

	"github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/edge-gateway/wg-server-service/types"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// WGServerService provides WireGuard server functionality for configuring
// VM OS WireGuard server to connect to one or more edge WireGuard clients.
//
// The service provides:
//   - WireGuard interface management
//   - Peer configuration and management
//   - Connection monitoring
type WGServerService interface {
	// Start starts the WireGuard server service.
	// This method should be called after all dependencies are configured.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the WireGuard server service.
	// This method should be called during service shutdown.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string

	// AddPeer adds a new peer to the WireGuard interface.
	AddPeer(publicKey wgtypes.Key, allowedIPs []net.IPNet) error

	// RemovePeer removes a peer from the WireGuard interface.
	RemovePeer(publicKey wgtypes.Key) error

	// GetPublicKey returns the server's public key.
	GetPublicKey() wgtypes.Key

	// GetListenPort returns the server's listen port.
	GetListenPort() int

	// GetInterface returns the WireGuard interface name.
	GetInterface() string

	// GetPeerInfo returns connection information for a peer.
	GetPeerInfo(publicKey wgtypes.Key) (*types.PeerInfo, bool)

	// GetConnectedPeers returns list of connected peer public keys.
	GetConnectedPeers() []string
}
