package types

import (
	"net"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// PeerInfo contains information about a WireGuard peer
type PeerInfo struct {
	PublicKey     wgtypes.Key
	AllowedIPs    []net.IPNet
	Endpoint      *net.UDPAddr
	LastHandshake time.Time
	Connected     bool
	Latency       time.Duration // Measured latency
	LastPingTime  time.Time     // Last ping sent
	LastPongTime  time.Time     // Last pong received
	PingCount     int64         // Total ping count
	PongCount     int64         // Total pong count
	BytesReceived uint64        // Total bytes received
	BytesSent     uint64        // Total bytes sent
	mu            sync.RWMutex
}

