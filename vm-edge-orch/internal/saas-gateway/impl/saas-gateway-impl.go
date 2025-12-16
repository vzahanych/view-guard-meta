package impl

import (
	"context"
	"fmt"
	"net/http"
	"time"

	saasgateway "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/saas-gateway"
	"go.uber.org/zap"
)

// Config provides configuration for the SaaS gateway.
type Config struct {
	Port int
}

// Logger provides logging interface for the SaaS gateway.
type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Debug(msg string, fields ...zap.Field)
}

type saasGateway struct {
	config Config
	logger Logger
	server *http.Server
}

// NewSaaSGateway creates a new SaaS gateway implementation.
// The gateway provides HTTP endpoints for SaaS components to interact with the VM application.
func NewSaaSGateway(cfg Config, log Logger) (saasgateway.SaaSGateway, error) {
	if log == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Set default port if not specified
	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	return &saasGateway{
		config: cfg,
		logger: log,
	}, nil
}

// Name returns the service name.
func (s *saasGateway) Name() string {
	return "saas-gateway"
}

// Start starts the HTTP server and begins serving requests.
func (s *saasGateway) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register HTTP endpoints
	s.registerRoutes(mux)

	// Use port from config
	port := s.config.Port

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.logger.Info("Starting SaaS gateway HTTP server", zap.Int("port", port))

	// Start server in a goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("SaaS gateway server error", zap.Error(err))
		}
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Stop gracefully shuts down the HTTP server.
func (s *saasGateway) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	s.logger.Info("Stopping SaaS gateway HTTP server")
	return s.server.Shutdown(ctx)
}

// registerRoutes registers all HTTP endpoints for the SaaS gateway.
func (s *saasGateway) registerRoutes(mux *http.ServeMux) {
	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// Admin API endpoints for SaaS components
	mux.HandleFunc("/api/admin/status", s.handleAdminStatus)
	mux.HandleFunc("/api/admin/info", s.handleAdminInfo)
}
