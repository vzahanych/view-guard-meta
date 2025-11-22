package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

// Status represents the health status
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

// Check represents a health check
type Check struct {
	Name      string                 `json:"name"`
	Status    Status                `json:"status"`
	Message   string                `json:"message,omitempty"`
	Timestamp time.Time             `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// HealthReport represents the overall health report
type HealthReport struct {
	Status    Status           `json:"status"`
	Timestamp time.Time        `json:"timestamp"`
	Uptime    time.Duration    `json:"uptime"`
	Checks    map[string]Check `json:"checks"`
	Services  map[string]interface{} `json:"services,omitempty"`
}

// Checker is an interface for health checkers
type Checker interface {
	Name() string
	Check(ctx context.Context) Check
}

// Manager manages health checks
type Manager struct {
	logger      *logger.Logger
	checkers    []Checker
	svcManager  *service.Manager
	startTime   time.Time
	mu          sync.RWMutex
	httpServer  *http.Server
	httpMux     *http.ServeMux
}

// NewManager creates a new health check manager
func NewManager(log *logger.Logger, svcManager *service.Manager) *Manager {
	return &Manager{
		logger:     log,
		checkers:   make([]Checker, 0),
		svcManager: svcManager,
		startTime:  time.Now(),
		httpMux:    http.NewServeMux(),
	}
}

// RegisterChecker registers a health checker
func (m *Manager) RegisterChecker(checker Checker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkers = append(m.checkers, checker)
}

// Start starts the health check HTTP server
func (m *Manager) Start(ctx context.Context, cfg *config.Config) error {
	// Register HTTP endpoints
	m.httpMux.HandleFunc("/health", m.handleHealth)
	m.httpMux.HandleFunc("/health/live", m.handleLiveness)
	m.httpMux.HandleFunc("/health/ready", m.handleReadiness)
	m.httpMux.HandleFunc("/health/services", m.handleServices)

	// Determine port (default 8080)
	port := 8080
	addr := fmt.Sprintf(":%d", port)

	m.httpServer = &http.Server{
		Addr:         addr,
		Handler:      m.httpMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		m.logger.Info("Health check server starting", "addr", addr)
		if err := m.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("Health check server error", "error", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Stop stops the health check HTTP server
func (m *Manager) Stop(ctx context.Context) error {
	if m.httpServer != nil {
		m.logger.Info("Stopping health check server")
		return m.httpServer.Shutdown(ctx)
	}
	return nil
}

// Check performs all health checks
func (m *Manager) Check(ctx context.Context) HealthReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	checks := make(map[string]Check)
	overallStatus := StatusHealthy

	// Run all checkers
	for _, checker := range m.checkers {
		check := checker.Check(ctx)
		checks[check.Name] = check

		// Determine overall status
		if check.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		} else if check.Status == StatusDegraded && overallStatus == StatusHealthy {
			overallStatus = StatusDegraded
		}
	}

	// Add service statuses
	services := make(map[string]interface{})
	if m.svcManager != nil {
		allStatuses := m.svcManager.GetAllStatuses()
		for name, status := range allStatuses {
			services[name] = map[string]interface{}{
				"status":  status.GetStatus(),
				"uptime":  status.GetUptime().String(),
				"error":   status.GetError(),
			}
		}
	}

	return HealthReport{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Uptime:    time.Since(m.startTime),
		Checks:    checks,
		Services:  services,
	}
}

// handleHealth handles the /health endpoint
func (m *Manager) handleHealth(w http.ResponseWriter, r *http.Request) {
	report := m.Check(r.Context())
	
	w.Header().Set("Content-Type", "application/json")
	
	statusCode := http.StatusOK
	if report.Status == StatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	} else if report.Status == StatusDegraded {
		statusCode = http.StatusOK // Still OK, but degraded
	}
	
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(report)
}

// handleLiveness handles the /health/live endpoint (liveness probe)
func (m *Manager) handleLiveness(w http.ResponseWriter, r *http.Request) {
	// Liveness: is the process alive?
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "alive",
		"timestamp": time.Now(),
	})
}

// handleReadiness handles the /health/ready endpoint (readiness probe)
func (m *Manager) handleReadiness(w http.ResponseWriter, r *http.Request) {
	// Readiness: is the service ready to accept traffic?
	report := m.Check(r.Context())
	
	w.Header().Set("Content-Type", "application/json")
	
	// Only ready if healthy or degraded (not unhealthy)
	statusCode := http.StatusOK
	if report.Status == StatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}
	
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    report.Status,
		"timestamp": report.Timestamp,
		"ready":     report.Status != StatusUnhealthy,
	})
}

// handleServices handles the /health/services endpoint
func (m *Manager) handleServices(w http.ResponseWriter, r *http.Request) {
	services := make(map[string]interface{})
	if m.svcManager != nil {
		allStatuses := m.svcManager.GetAllStatuses()
		for name, status := range allStatuses {
			services[name] = map[string]interface{}{
				"status":  status.GetStatus(),
				"uptime":  status.GetUptime().String(),
				"error":   status.GetError(),
			}
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"services": services,
		"timestamp": time.Now(),
	})
}

