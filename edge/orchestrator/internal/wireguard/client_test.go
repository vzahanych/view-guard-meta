package wireguard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func setupTestClient(t *testing.T) (*Client, *config.WireGuardConfig) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.WireGuardConfig{
		Enabled:     true,
		ConfigPath:  filepath.Join(t.TempDir(), "wg0.conf"),
		KVMEndpoint: "192.168.1.100:51820",
	}

	client := NewClient(cfg, log)
	return client, cfg
}

func TestClient_NewClient(t *testing.T) {
	client, _ := setupTestClient(t)

	if client.Name() != "wireguard-client" {
		t.Errorf("Expected service name 'wireguard-client', got %s", client.Name())
	}

	if client.GetInterfaceName() != "wg0" {
		t.Errorf("Expected interface name 'wg0', got %s", client.GetInterfaceName())
	}
}

func TestClient_IsWireGuardInstalled(t *testing.T) {
	client, _ := setupTestClient(t)

	// This test will pass if WireGuard is installed, fail otherwise
	// For CI/CD, we might want to skip this test or mock it
	installed := client.isWireGuardInstalled()
	t.Logf("WireGuard installed: %v", installed)
}

func TestClient_EnsureConfigFile(t *testing.T) {
	client, cfg := setupTestClient(t)

	// Ensure config file is created
	err := client.ensureConfigFile()
	if err != nil {
		t.Fatalf("Failed to ensure config file: %v", err)
	}

	// Check if file exists
	if _, err := os.Stat(cfg.ConfigPath); os.IsNotExist(err) {
		t.Errorf("Config file was not created: %s", cfg.ConfigPath)
	}

	// Check file content
	content, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	if !strings.Contains(string(content), cfg.KVMEndpoint) {
		t.Errorf("Config file does not contain endpoint: %s", cfg.KVMEndpoint)
	}
}

func TestClient_IsTunnelUp(t *testing.T) {
	client, _ := setupTestClient(t)

	// This will fail if WireGuard is not installed or tunnel is not up
	// For unit tests, we can just check the method doesn't panic
	_ = client.isTunnelUp()
}

func TestClient_StartStop_Disabled(t *testing.T) {
	client, cfg := setupTestClient(t)
	cfg.Enabled = false

	ctx := context.Background()

	// Start should succeed but do nothing
	err := client.Start(ctx)
	if err != nil {
		t.Fatalf("Start should succeed when disabled: %v", err)
	}

	// Stop should succeed
	err = client.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop should succeed: %v", err)
	}
}

func TestClient_GetStats(t *testing.T) {
	client, _ := setupTestClient(t)

	stats, err := client.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.InterfaceName != "wg0" {
		t.Errorf("Expected interface name 'wg0', got %s", stats.InterfaceName)
	}

	if stats.Endpoint == "" {
		t.Error("Expected endpoint to be set")
	}
}

func TestClient_ConnectionState(t *testing.T) {
	client, _ := setupTestClient(t)

	// Initially not connected
	if client.IsConnected() {
		t.Error("Expected client to be disconnected initially")
	}

	// Get latency (should be 0 initially)
	latency := client.GetLatency()
	if latency != 0 {
		t.Errorf("Expected initial latency 0, got %v", latency)
	}
}

func TestClient_MeasureLatency(t *testing.T) {
	client, _ := setupTestClient(t)

	// Measure latency to localhost (should work even without WireGuard)
	client.config.KVMEndpoint = "127.0.0.1:51820"
	latency := client.measureLatency()

	// Latency should be reasonable (less than 1 second for localhost)
	if latency > 1*time.Second {
		t.Errorf("Expected latency < 1s for localhost, got %v", latency)
	}
}

func TestClient_GenerateConfigTemplate(t *testing.T) {
	client, cfg := setupTestClient(t)

	configContent := client.generateConfigTemplate()

	if !strings.Contains(configContent, cfg.KVMEndpoint) {
		t.Errorf("Config template should contain endpoint: %s", cfg.KVMEndpoint)
	}

	if !strings.Contains(configContent, "PersistentKeepalive") {
		t.Error("Config template should contain PersistentKeepalive")
	}
}

