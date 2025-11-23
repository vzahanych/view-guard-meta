package camera

import (
	"fmt"
	"net"
	"strings"
)

// findLocalNetworkInterface finds a suitable network interface for discovery
// Prefers WiFi/Ethernet interfaces on local networks (not loopback, not VPN)
func (s *ONVIFDiscoveryService) findLocalNetworkInterface() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to list interfaces: %w", err)
	}

	// Prefer interfaces that are:
	// 1. Up and not loopback
	// 2. Have an IPv4 address
	// 3. Are on a private network (192.168.x.x, 10.x.x.x, 172.16-31.x.x)
	// 4. Not VPN interfaces (tun, tap, vpn, etc.)

	var bestInterface *net.Interface
	var bestAddr *net.IPNet

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Skip VPN interfaces
		ifaceName := strings.ToLower(iface.Name)
		if strings.Contains(ifaceName, "tun") || strings.Contains(ifaceName, "tap") ||
			strings.Contains(ifaceName, "vpn") || strings.Contains(ifaceName, "wg") {
			continue
		}

		// Get addresses for this interface
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		// Find IPv4 address on private network
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			if ip == nil || ip.To4() == nil {
				continue // Skip IPv6
			}

			// Check if it's a private network address
			if isPrivateIP(ip) {
				bestInterface = &iface
				bestAddr = ipNet
				break
			}
		}
	}

	if bestInterface == nil || bestAddr == nil {
		return "", fmt.Errorf("no suitable network interface found")
	}

	// Return the IP address to bind to
	return bestAddr.IP.String() + ":0", nil
}

// isPrivateIP checks if an IP address is on a private network
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	ipv4 := ip.To4()
	if ipv4 == nil {
		return false
	}

	// Private network ranges:
	// 10.0.0.0/8
	// 172.16.0.0/12
	// 192.168.0.0/16
	return (ipv4[0] == 10) ||
		(ipv4[0] == 172 && ipv4[1] >= 16 && ipv4[1] <= 31) ||
		(ipv4[0] == 192 && ipv4[1] == 168)
}

