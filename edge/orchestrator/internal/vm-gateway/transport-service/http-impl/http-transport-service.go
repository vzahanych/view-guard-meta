package httpimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"

	httpsclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-client-service"
	httpsserver "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-server-service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	"go.uber.org/zap"
)

// HTTPTransportService implements TransportService for HTTP/HTTPS transport.
// It wraps the HTTPS server and client services to provide a unified transport interface.
type HTTPTransportService struct {
	httpsServerService httpsserver.HTTPSServerService
	httpsClientService httpsclient.HTTPSClientService
	logger             *zap.Logger
	mu                 sync.RWMutex
	started            bool
}

// NewHTTPTransportService creates a new HTTP transport service.
// Returns typed TransportService interface.
func NewHTTPTransportService(
	httpsServerService httpsserver.HTTPSServerService,
	httpsClientService httpsclient.HTTPSClientService,
	logger *zap.Logger,
) *HTTPTransportService {
	return &HTTPTransportService{
		httpsServerService: httpsServerService,
		httpsClientService: httpsClientService,
		logger:             logger,
	}
}

// Start starts the HTTP transport service (both server and client).
func (s *HTTPTransportService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("HTTP transport service is already started")
	}

	if s.logger != nil {
		s.logger.Info("Starting HTTP transport service...")
	}

	// Start HTTPS server first
	if s.httpsServerService != nil {
		if s.logger != nil {
			s.logger.Info("Starting HTTPS server service...")
		}
		if err := s.httpsServerService.Start(ctx); err != nil {
			return fmt.Errorf("failed to start HTTPS server service: %w", err)
		}
	}

	// Start HTTPS client
	if s.httpsClientService != nil {
		if s.logger != nil {
			s.logger.Info("Starting HTTPS client service...")
		}
		if err := s.httpsClientService.Start(ctx); err != nil {
			// Try to stop server if client fails
			if s.httpsServerService != nil {
				_ = s.httpsServerService.Stop(ctx)
			}
			return fmt.Errorf("failed to start HTTPS client service: %w", err)
		}
	}

	s.started = true

	if s.logger != nil {
		s.logger.Info("HTTP transport service started successfully")
	}

	return nil
}

// Stop stops the HTTP transport service (both server and client).
// Stop stops the HTTP transport service.
// Locking strategy: Copy service references under lock, call them outside lock to avoid deadlocks.
// Uses errors.Join() to preserve individual error values (Go 1.20+).
func (s *HTTPTransportService) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil // Already stopped
	}

	// Copy service references under lock
	clientSvc := s.httpsClientService
	serverSvc := s.httpsServerService
	s.mu.Unlock()

	if s.logger != nil {
		s.logger.Info("Stopping HTTP transport service...")
	}

	var errs []error

	// Stop HTTPS client first
	if clientSvc != nil {
		if s.logger != nil {
			s.logger.Info("Stopping HTTPS client service...")
		}
		if err := clientSvc.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop HTTPS client service: %w", err))
		}
	}

	// Stop HTTPS server
	if serverSvc != nil {
		if s.logger != nil {
			s.logger.Info("Stopping HTTPS server service...")
		}
		if err := serverSvc.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop HTTPS server service: %w", err))
		}
	}

	// Mark as stopped under lock
	s.mu.Lock()
	s.started = false
	s.mu.Unlock()

	if len(errs) > 0 {
		if s.logger != nil {
			s.logger.Error("Some services failed to stop", zap.Errors("errors", errs))
		}
		return errors.Join(errs...)
	}

	if s.logger != nil {
		s.logger.Info("HTTP transport service stopped successfully")
	}

	return nil
}

// Name returns the service name.
func (s *HTTPTransportService) Name() string {
	return "http-transport-service"
}

// IsConnected returns whether the transport connection is ready.
func (s *HTTPTransportService) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.httpsClientService == nil {
		return false
	}
	return s.httpsClientService.IsConnected()
}

// Authenticate authenticates Edge with VM.
func (s *HTTPTransportService) Authenticate(ctx context.Context, edgeID string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.httpsClientService == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	return s.httpsClientService.Authenticate(ctx, edgeID)
}

// GetConfig retrieves VM configuration.
func (s *HTTPTransportService) GetConfig(ctx context.Context) (*types.GetConfigResponse, error) {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	// No translation needed - types are now unified in vm-gateway/types
	return client.GetConfig(ctx)
}

// SyncCapabilities syncs device capabilities to the VM.
func (s *HTTPTransportService) SyncCapabilities(ctx context.Context, req *types.SyncCapabilitiesRequest) (*types.SyncCapabilitiesResponse, error) {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	// No translation needed - types are now unified in vm-gateway/types
	return client.SyncCapabilities(ctx, req)
}

// SyncDevices syncs discovered devices to the VM.
func (s *HTTPTransportService) SyncDevices(ctx context.Context, req *types.SyncDevicesRequest) (*types.SyncDevicesResponse, error) {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	// No translation needed - types are now unified in vm-gateway/types
	return client.SyncDevices(ctx, req)
}

// SyncDataUnits syncs labeled data units to the VM for model training.
func (s *HTTPTransportService) SyncDataUnits(ctx context.Context, req *types.SyncDataUnitsRequest) (*types.SyncDataUnitsResponse, error) {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	// No translation needed - types are now unified in vm-gateway/types
	return client.SyncDataUnits(ctx, req)
}

// SyncAuditLogs syncs audit logs to the VM.
func (s *HTTPTransportService) SyncAuditLogs(ctx context.Context, req *types.SyncAuditLogsRequest) (*types.SyncAuditLogsResponse, error) {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	// No translation needed - types are now unified in vm-gateway/types
	return client.SyncAuditLogs(ctx, req)
}

// ReportDeploymentStatus reports deployment status to the VM.
// Locking strategy: Copy service reference under lock, call outside lock.
func (s *HTTPTransportService) ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	return client.ReportDeploymentStatus(ctx, deploymentID, status, errorMessage, modelPath)
}

// Heartbeat sends a heartbeat to the VM.
func (s *HTTPTransportService) Heartbeat(ctx context.Context, req *types.HeartbeatRequest) error {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	// No translation needed - types are now unified in vm-gateway/types
	return client.Heartbeat(ctx, req)
}

// SendTelemetry sends telemetry data to the VM.
func (s *HTTPTransportService) SendTelemetry(ctx context.Context, data *types.TelemetryData) error {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	// No translation needed - types are now unified in vm-gateway/types
	return client.SendTelemetry(ctx, data)
}

// SendEvents sends events to the VM.
func (s *HTTPTransportService) SendEvents(ctx context.Context, events []*types.Event) error {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	// No translation needed - types are now unified in vm-gateway/types
	return client.SendEvents(ctx, events)
}
