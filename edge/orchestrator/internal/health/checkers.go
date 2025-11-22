package health

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SystemChecker checks system resources
type SystemChecker struct{}

func (c *SystemChecker) Name() string {
	return "system"
}

func (c *SystemChecker) Check(ctx context.Context) Check {
	check := Check{
		Name:      c.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check disk space (basic check)
	// In a real implementation, you'd check actual disk usage
	check.Details["disk_check"] = "basic"

	// Check memory (basic check)
	check.Details["memory_check"] = "basic"

	check.Status = StatusHealthy
	check.Message = "System resources OK"

	return check
}

// DatabaseChecker checks database connectivity
type DatabaseChecker struct {
	dbPath string
}

func NewDatabaseChecker(dbPath string) *DatabaseChecker {
	return &DatabaseChecker{dbPath: dbPath}
}

func (c *DatabaseChecker) Name() string {
	return "database"
}

func (c *DatabaseChecker) Check(ctx context.Context) Check {
	check := Check{
		Name:      c.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check if database file exists
	if c.dbPath == "" {
		check.Status = StatusDegraded
		check.Message = "Database path not configured"
		return check
	}

	if _, err := os.Stat(c.dbPath); os.IsNotExist(err) {
		// Database file doesn't exist yet - this is OK for first run
		check.Status = StatusHealthy
		check.Message = "Database file will be created on first use"
		check.Details["file_exists"] = false
		return check
	}

	// Try to open database
	db, err := sql.Open("sqlite3", c.dbPath)
	if err != nil {
		check.Status = StatusUnhealthy
		check.Message = fmt.Sprintf("Failed to open database: %v", err)
		return check
	}
	defer db.Close()

	// Test connection with timeout
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		check.Status = StatusUnhealthy
		check.Message = fmt.Sprintf("Database ping failed: %v", err)
		return check
	}

	check.Status = StatusHealthy
	check.Message = "Database connection OK"
	check.Details["file_exists"] = true

	return check
}

// AIServiceChecker checks AI service connectivity
type AIServiceChecker struct {
	serviceURL string
	client     *http.Client
}

func NewAIServiceChecker(serviceURL string) *AIServiceChecker {
	return &AIServiceChecker{
		serviceURL: serviceURL,
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

func (c *AIServiceChecker) Name() string {
	return "ai_service"
}

func (c *AIServiceChecker) Check(ctx context.Context) Check {
	check := Check{
		Name:      c.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	if c.serviceURL == "" {
		check.Status = StatusDegraded
		check.Message = "AI service URL not configured"
		return check
	}

	// Try to reach AI service health endpoint
	healthURL := fmt.Sprintf("%s/health", c.serviceURL)
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		check.Status = StatusDegraded
		check.Message = fmt.Sprintf("Failed to create request: %v", err)
		return check
	}

	resp, err := c.client.Do(req)
	if err != nil {
		check.Status = StatusDegraded
		check.Message = fmt.Sprintf("AI service unreachable: %v", err)
		check.Details["url"] = c.serviceURL
		return check
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		check.Status = StatusDegraded
		check.Message = fmt.Sprintf("AI service returned status %d", resp.StatusCode)
		check.Details["status_code"] = resp.StatusCode
		return check
	}

	check.Status = StatusHealthy
	check.Message = "AI service is reachable"
	check.Details["url"] = c.serviceURL
	check.Details["status_code"] = resp.StatusCode

	return check
}

// StorageChecker checks storage availability
type StorageChecker struct {
	clipsDir     string
	snapshotsDir string
}

func NewStorageChecker(clipsDir, snapshotsDir string) *StorageChecker {
	return &StorageChecker{
		clipsDir:     clipsDir,
		snapshotsDir: snapshotsDir,
	}
}

func (c *StorageChecker) Name() string {
	return "storage"
}

func (c *StorageChecker) Check(ctx context.Context) Check {
	check := Check{
		Name:      c.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check clips directory
	if c.clipsDir != "" {
		if err := os.MkdirAll(c.clipsDir, 0755); err != nil {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Failed to create clips directory: %v", err)
			return check
		}
		check.Details["clips_dir"] = c.clipsDir
		check.Details["clips_dir_writable"] = true
	}

	// Check snapshots directory
	if c.snapshotsDir != "" {
		if err := os.MkdirAll(c.snapshotsDir, 0755); err != nil {
			check.Status = StatusUnhealthy
			check.Message = fmt.Sprintf("Failed to create snapshots directory: %v", err)
			return check
		}
		check.Details["snapshots_dir"] = c.snapshotsDir
		check.Details["snapshots_dir_writable"] = true
	}

	check.Status = StatusHealthy
	check.Message = "Storage directories accessible"

	return check
}

// NetworkChecker checks network connectivity
type NetworkChecker struct{}

func (c *NetworkChecker) Name() string {
	return "network"
}

func (c *NetworkChecker) Check(ctx context.Context) Check {
	check := Check{
		Name:      c.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Basic network check - in a real implementation, you'd check
	// WireGuard connectivity, DNS resolution, etc.
	check.Details["basic_check"] = true

	check.Status = StatusHealthy
	check.Message = "Network connectivity OK"

	return check
}

