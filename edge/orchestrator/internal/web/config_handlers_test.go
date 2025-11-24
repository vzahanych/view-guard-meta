package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func setupTestConfigServer(t *testing.T) (*Server, *config.Service, func()) {
	// Create temp directory for test config
	tmpDir, err := os.MkdirTemp("", "web-config-test-*")
	require.NoError(t, err)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a minimal test config file
	testConfig := `edge:
  orchestrator:
    log_level: info
    log_format: text
    data_dir: /tmp/test-data
  wireguard:
    enabled: false
  cameras:
    discovery:
      enabled: true
      interval: 30s
  storage:
    clips_dir: /tmp/clips
    snapshots_dir: /tmp/snapshots
    retention_days: 7
    max_disk_usage_percent: 80.0
  ai:
    service_url: http://localhost:8080
    confidence_threshold: 0.5
    inference_interval: 1s
  events:
    queue_size: 100
    batch_size: 10
    transmission_interval: 5s
  telemetry:
    enabled: true
    interval: 60s
  encryption:
    enabled: false
  web:
    enabled: true
    host: localhost
    port: 8080
`
	err = os.WriteFile(configPath, []byte(testConfig), 0644)
	require.NoError(t, err)

	// Create logger
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)

	// Create config service
	configSvc, err := config.NewService(configPath, log)
	require.NoError(t, err)

	// Create web server
	webCfg := &config.WebConfig{
		Enabled: true,
		Host:    "localhost",
		Port:    8080,
	}
	server := NewServer(webCfg, log)
	server.SetConfigDependency(configSvc)
	// Setup routes manually for testing
	server.setupRoutes()

	// Cleanup function
	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return server, configSvc, cleanup
}

func TestHandleGetConfig(t *testing.T) {
	server, _, cleanup := setupTestConfigServer(t)
	defer cleanup()

	// Test: Get full configuration
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/config", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "config")
	configData := response["config"].(map[string]interface{})
	// Config struct uses capitalized field names when marshaled
	assert.Contains(t, configData, "Edge")

	// Verify user secret is not exposed (should be empty string)
	edgeData := configData["Edge"].(map[string]interface{})
	encryptionData := edgeData["Encryption"].(map[string]interface{})
	userSecret, exists := encryptionData["UserSecret"]
	if exists {
		assert.Equal(t, "", userSecret, "User secret should not be exposed")
	} else {
		// If field doesn't exist, that's also fine
		assert.True(t, true)
	}
}

func TestHandleGetConfig_Section(t *testing.T) {
	server, _, cleanup := setupTestConfigServer(t)
	defer cleanup()

	// Test: Get specific section
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/config?section=ai", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ai", response["section"])
	assert.Contains(t, response, "config")
}

func TestHandleGetConfig_InvalidSection(t *testing.T) {
	server, _, cleanup := setupTestConfigServer(t)
	defer cleanup()

	// Test: Get invalid section
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/config?section=invalid", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleGetConfig_NoConfigService(t *testing.T) {
	// Create server without config service
	webCfg := &config.WebConfig{
		Enabled: true,
		Host:    "localhost",
		Port:    8080,
	}
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)
	server := NewServer(webCfg, log)
	server.setupRoutes()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/config", nil)
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleUpdateConfig_Section(t *testing.T) {
	server, configSvc, cleanup := setupTestConfigServer(t)
	defer cleanup()

	// Test: Update AI section
	updateData := map[string]interface{}{
		"service_url":          "http://localhost:9090",
		"confidence_threshold": 0.75,
		"inference_interval":   "2s",
	}

	jsonData, err := json.Marshal(updateData)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/config?section=ai", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify configuration was updated
	cfg := configSvc.Get()
	assert.Equal(t, "http://localhost:9090", cfg.Edge.AI.ServiceURL)
	assert.Equal(t, 0.75, cfg.Edge.AI.ConfidenceThreshold)
}

func TestHandleUpdateConfig_Full(t *testing.T) {
	server, configSvc, cleanup := setupTestConfigServer(t)
	defer cleanup()

	// Test: Update full configuration
	updateData := map[string]interface{}{
		"edge": map[string]interface{}{
			"ai": map[string]interface{}{
				"service_url":          "http://localhost:9090",
				"confidence_threshold": 0.8,
			},
			"telemetry": map[string]interface{}{
				"enabled":  false,
				"interval": "120s",
			},
		},
	}

	jsonData, err := json.Marshal(updateData)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/config", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify configuration was updated
	cfg := configSvc.Get()
	assert.Equal(t, "http://localhost:9090", cfg.Edge.AI.ServiceURL)
	assert.Equal(t, 0.8, cfg.Edge.AI.ConfidenceThreshold)
	assert.False(t, cfg.Edge.Telemetry.Enabled)
}

func TestHandleUpdateConfig_ValidationError(t *testing.T) {
	server, _, cleanup := setupTestConfigServer(t)
	defer cleanup()

	// Test: Update with invalid configuration (negative confidence threshold)
	updateData := map[string]interface{}{
		"confidence_threshold": -1.0, // Invalid: must be between 0 and 1
	}

	jsonData, err := json.Marshal(updateData)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/config?section=ai", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(w, req)

	// Should fail validation
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleUpdateConfig_EncryptionUserSecret(t *testing.T) {
	server, _, cleanup := setupTestConfigServer(t)
	defer cleanup()

	// Test: Try to update user secret (should be rejected)
	updateData := map[string]interface{}{
		"user_secret": "new-secret", // Should be rejected
	}

	jsonData, err := json.Marshal(updateData)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/config?section=encryption", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(w, req)

	// Should reject user secret update
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["error"].(string), "user_secret")
}

func TestHandleUpdateConfig_InvalidJSON(t *testing.T) {
	server, _, cleanup := setupTestConfigServer(t)
	defer cleanup()

	// Test: Invalid JSON
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/config", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleUpdateConfig_NoConfigService(t *testing.T) {
	// Create server without config service
	webCfg := &config.WebConfig{
		Enabled: true,
		Host:    "localhost",
		Port:    8080,
	}
	log, err := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})
	require.NoError(t, err)
	server := NewServer(webCfg, log)
	server.setupRoutes()

	updateData := map[string]interface{}{
		"service_url": "http://localhost:9090",
	}
	jsonData, _ := json.Marshal(updateData)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/config?section=ai", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestConfigSections(t *testing.T) {
	server, _, cleanup := setupTestConfigServer(t)
	defer cleanup()

	// Test all supported sections
	sections := []string{"cameras", "ai", "storage", "wireguard", "telemetry", "encryption", "web", "events"}

	for _, section := range sections {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/config?section="+section, nil)
		server.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "Section %s should be accessible", section)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, section, response["section"])
		assert.Contains(t, response, "config")
	}
}

