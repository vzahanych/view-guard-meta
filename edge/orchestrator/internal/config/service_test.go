package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"gopkg.in/yaml.v3"
)

func createTestConfig(t *testing.T, configPath string, cfg *Config) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
}

func TestNewService(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{}
	cfg.setDefaults()
	cfg.Edge.Orchestrator.DataDir = tmpDir
	cfg.Edge.AI.ServiceURL = "http://localhost:8080"

	createTestConfig(t, configPath, cfg)

	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	svc, err := NewService(configPath, log)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	if svc == nil {
		t.Fatal("NewService returned nil")
	}

	if svc.Get() == nil {
		t.Fatal("Get() returned nil")
	}
}

func TestService_Get(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{}
	cfg.setDefaults()
	cfg.Edge.Orchestrator.DataDir = tmpDir
	cfg.Edge.AI.ServiceURL = "http://localhost:8080"

	createTestConfig(t, configPath, cfg)

	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	svc, err := NewService(configPath, log)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	retrieved := svc.Get()
	if retrieved.Edge.Orchestrator.DataDir != tmpDir {
		t.Errorf("Expected DataDir %s, got %s", tmpDir, retrieved.Edge.Orchestrator.DataDir)
	}
}

func TestService_Reload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{}
	cfg.setDefaults()
	cfg.Edge.Orchestrator.DataDir = tmpDir
	cfg.Edge.AI.ServiceURL = "http://localhost:8080"

	createTestConfig(t, configPath, cfg)

	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	svc, err := NewService(configPath, log)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	cfg.Edge.Orchestrator.LogLevel = "debug"
	createTestConfig(t, configPath, cfg)

	ctx := context.Background()
	if err := svc.Reload(ctx); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	reloaded := svc.Get()
	if reloaded.Edge.Orchestrator.LogLevel != "debug" {
		t.Errorf("Expected LogLevel 'debug', got %s", reloaded.Edge.Orchestrator.LogLevel)
	}
}

func TestService_Watch(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{}
	cfg.setDefaults()
	cfg.Edge.Orchestrator.DataDir = tmpDir
	cfg.Edge.AI.ServiceURL = "http://localhost:8080"

	createTestConfig(t, configPath, cfg)

	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	svc, err := NewService(configPath, log)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	watcherCalled := false
	watcher := func(ctx context.Context, oldConfig, newConfig *Config) error {
		watcherCalled = true
		if oldConfig == nil || newConfig == nil {
			t.Error("Watcher should receive both old and new config")
		}
		return nil
	}

	svc.Watch(watcher)

	cfg.Edge.Orchestrator.LogLevel = "debug"
	createTestConfig(t, configPath, cfg)

	ctx := context.Background()
	if err := svc.Reload(ctx); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if !watcherCalled {
		t.Error("Watcher should have been called")
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{}
	cfg.setDefaults()
	cfg.Edge.Orchestrator.DataDir = tmpDir
	cfg.Edge.AI.ServiceURL = "http://localhost:8080"

	createTestConfig(t, configPath, cfg)

	os.Setenv("EDGE_LOG_LEVEL", "debug")
	os.Setenv("EDGE_DATA_DIR", "/custom/data")
	os.Setenv("EDGE_AI_SERVICE_URL", "http://custom:9090")
	defer func() {
		os.Unsetenv("EDGE_LOG_LEVEL")
		os.Unsetenv("EDGE_DATA_DIR")
		os.Unsetenv("EDGE_AI_SERVICE_URL")
	}()

	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	svc, err := NewService(configPath, log)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	retrieved := svc.Get()
	if retrieved.Edge.Orchestrator.LogLevel != "debug" {
		t.Errorf("Expected LogLevel 'debug' from env, got %s", retrieved.Edge.Orchestrator.LogLevel)
	}

	if retrieved.Edge.Orchestrator.DataDir != "/custom/data" {
		t.Errorf("Expected DataDir '/custom/data' from env, got %s", retrieved.Edge.Orchestrator.DataDir)
	}

	if retrieved.Edge.AI.ServiceURL != "http://custom:9090" {
		t.Errorf("Expected ServiceURL 'http://custom:9090' from env, got %s", retrieved.Edge.AI.ServiceURL)
	}
}

func TestGetEnvWithDefault(t *testing.T) {
	os.Unsetenv("TEST_ENV_VAR")
	result := GetEnvWithDefault("TEST_ENV_VAR", "default")
	if result != "default" {
		t.Errorf("Expected 'default', got %s", result)
	}

	os.Setenv("TEST_ENV_VAR", "custom")
	defer os.Unsetenv("TEST_ENV_VAR")
	result = GetEnvWithDefault("TEST_ENV_VAR", "default")
	if result != "custom" {
		t.Errorf("Expected 'custom', got %s", result)
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		envValue    string
		defaultVal  bool
		expected    bool
		description string
	}{
		{"", false, false, "empty env with false default"},
		{"", true, true, "empty env with true default"},
		{"true", false, true, "true string"},
		{"1", false, true, "1 string"},
		{"yes", false, true, "yes string"},
		{"on", false, true, "on string"},
		{"false", true, false, "false string"},
		{"0", true, false, "0 string"},
		{"no", true, false, "no string"},
		{"off", true, false, "off string"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			os.Setenv("TEST_BOOL", tt.envValue)
			defer os.Unsetenv("TEST_BOOL")
			result := GetEnvBool("TEST_BOOL", tt.defaultVal)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	os.Unsetenv("TEST_INT")
	result := GetEnvInt("TEST_INT", 42)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	os.Setenv("TEST_INT", "100")
	defer os.Unsetenv("TEST_INT")
	result = GetEnvInt("TEST_INT", 42)
	if result != 100 {
		t.Errorf("Expected 100, got %d", result)
	}

	os.Setenv("TEST_INT", "invalid")
	result = GetEnvInt("TEST_INT", 42)
	if result != 42 {
		t.Errorf("Expected 42 for invalid value, got %d", result)
	}
}

func TestGetEnvDuration(t *testing.T) {
	os.Unsetenv("TEST_DURATION")
	result := GetEnvDuration("TEST_DURATION", 5*time.Second)
	if result != 5*time.Second {
		t.Errorf("Expected 5s, got %v", result)
	}

	os.Setenv("TEST_DURATION", "10s")
	defer os.Unsetenv("TEST_DURATION")
	result = GetEnvDuration("TEST_DURATION", 5*time.Second)
	if result != 10*time.Second {
		t.Errorf("Expected 10s, got %v", result)
	}

	os.Setenv("TEST_DURATION", "invalid")
	result = GetEnvDuration("TEST_DURATION", 5*time.Second)
	if result != 5*time.Second {
		t.Errorf("Expected 5s for invalid value, got %v", result)
	}
}

func TestGetEnvFloat64(t *testing.T) {
	os.Unsetenv("TEST_FLOAT")
	result := GetEnvFloat64("TEST_FLOAT", 3.14)
	if result != 3.14 {
		t.Errorf("Expected 3.14, got %f", result)
	}

	os.Setenv("TEST_FLOAT", "2.71")
	defer os.Unsetenv("TEST_FLOAT")
	result = GetEnvFloat64("TEST_FLOAT", 3.14)
	if result != 2.71 {
		t.Errorf("Expected 2.71, got %f", result)
	}

	os.Setenv("TEST_FLOAT", "invalid")
	result = GetEnvFloat64("TEST_FLOAT", 3.14)
	if result != 3.14 {
		t.Errorf("Expected 3.14 for invalid value, got %f", result)
	}
}

