package httpimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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
	timeoutConfig      *types.TimeoutConfig
	logger             *zap.Logger
	mu                 sync.RWMutex
	started            bool
}

// NewHTTPTransportService creates a new HTTP transport service.
// Returns typed TransportService interface.
func NewHTTPTransportService(
	httpsServerService httpsserver.HTTPSServerService,
	httpsClientService httpsclient.HTTPSClientService,
	timeoutConfig *types.TimeoutConfig,
	logger *zap.Logger,
) *HTTPTransportService {
	return &HTTPTransportService{
		httpsServerService: httpsServerService,
		httpsClientService: httpsClientService,
		timeoutConfig:      timeoutConfig,
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

		// Wait for server readiness using TransportEstablishmentTimeout
		if s.logger != nil {
			s.logger.Info("Waiting for HTTPS server readiness...")
		}
		readinessTimeout := s.getTransportEstablishmentTimeout()
		readinessCtx, cancel := context.WithTimeout(ctx, readinessTimeout)
		defer cancel()

		// Wait for server readiness by polling the readiness endpoint
		// The server starts in a goroutine, so we need to wait for it to be ready
		readinessDeadline := time.Now().Add(readinessTimeout)
		checkInterval := 200 * time.Millisecond
		
		ready := false
		for time.Now().Before(readinessDeadline) {
			select {
			case <-readinessCtx.Done():
				_ = s.httpsServerService.Stop(ctx)
				return fmt.Errorf("context cancelled while waiting for server readiness: %w", readinessCtx.Err())
			default:
			}

			// Check if server is ready
			if s.httpsServerService.IsServerReady() {
				ready = true
				break
			}
			
			time.Sleep(checkInterval)
		}
		
		if !ready {
			_ = s.httpsServerService.Stop(ctx)
			return fmt.Errorf("HTTPS server did not become ready within %v", readinessTimeout)
		}

		if s.logger != nil {
			s.logger.Info("HTTPS server is ready")
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

// getTransportEstablishmentTimeout returns the transport establishment timeout with default.
func (s *HTTPTransportService) getTransportEstablishmentTimeout() time.Duration {
	if s.timeoutConfig != nil {
		return s.timeoutConfig.GetTransportEstablishmentTimeout()
	}
	return 30 * time.Second // Default: 30 seconds
}

// getAuthenticationTimeout returns the authentication timeout with default.
func (s *HTTPTransportService) getAuthenticationTimeout() time.Duration {
	if s.timeoutConfig != nil {
		return s.timeoutConfig.GetAuthenticationTimeout()
	}
	return 30 * time.Second // Default: 30 seconds
}

// getVMAPIRequestTimeout returns the VM API request timeout with default.
func (s *HTTPTransportService) getVMAPIRequestTimeout() time.Duration {
	if s.timeoutConfig != nil {
		return s.timeoutConfig.GetVMAPIRequestTimeout()
	}
	return 30 * time.Second // Default: 30 seconds
}

// Authenticate authenticates Edge with VM.
// This method blocks until the HTTPS server is ready before attempting authentication.
func (s *HTTPTransportService) Authenticate(ctx context.Context, edgeID string) error {
	s.mu.RLock()
	serverSvc := s.httpsServerService
	clientSvc := s.httpsClientService
	s.mu.RUnlock()

	if clientSvc == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}

	// Ensure HTTPS server is ready before authenticating
	if serverSvc != nil {
		if !serverSvc.IsServerReady() {
			// Wait for server readiness with timeout
			readinessTimeout := s.getTransportEstablishmentTimeout()
			readinessCtx, cancel := context.WithTimeout(ctx, readinessTimeout)
			defer cancel()

			if serverImpl, ok := serverSvc.(interface {
				WaitForServerReady(context.Context, time.Duration) error
			}); ok {
				if err := serverImpl.WaitForServerReady(readinessCtx, readinessTimeout); err != nil {
					return fmt.Errorf("HTTPS server is not ready: %w", err)
				}
			} else {
				// Fallback: poll IsServerReady
				deadline := time.Now().Add(readinessTimeout)
				for time.Now().Before(deadline) {
					select {
					case <-readinessCtx.Done():
						return fmt.Errorf("HTTPS server is not ready: %w", readinessCtx.Err())
					default:
					}
					if serverSvc.IsServerReady() {
						break
					}
					time.Sleep(200 * time.Millisecond)
				}
				if !serverSvc.IsServerReady() {
					return fmt.Errorf("HTTPS server is not ready after %v", readinessTimeout)
				}
			}
		}
	}

	// Use authentication timeout
	authTimeout := s.getAuthenticationTimeout()
	authCtx, cancel := context.WithTimeout(ctx, authTimeout)
	defer cancel()

	if s.logger != nil {
		s.logger.Debug("Authenticating with timeout", zap.Duration("timeout", authTimeout))
	}

	err := clientSvc.Authenticate(authCtx, edgeID)
	if err != nil {
		if authCtx.Err() == context.DeadlineExceeded {
			if s.logger != nil {
				s.logger.Warn("Authentication timed out", zap.Duration("timeout", authTimeout))
			}
			return fmt.Errorf("authentication timed out after %v: %w", authTimeout, err)
		}
		return err
	}
	return nil
}

// GetConfig retrieves VM configuration.
func (s *HTTPTransportService) GetConfig(ctx context.Context) (*types.GetConfigResponse, error) {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	// Use VM API request timeout
	timeout := s.getVMAPIRequestTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// No translation needed - types are now unified in vm-gateway/types
	return client.GetConfig(timeoutCtx)
}

// SyncCapabilities syncs device capabilities to the VM.
func (s *HTTPTransportService) SyncCapabilities(ctx context.Context, req *types.SyncCapabilitiesRequest) (*types.SyncCapabilitiesResponse, error) {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	// Use VM API request timeout
	timeout := s.getVMAPIRequestTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// No translation needed - types are now unified in vm-gateway/types
	return client.SyncCapabilities(timeoutCtx, req)
}

// SyncDevices syncs discovered devices to the VM.
func (s *HTTPTransportService) SyncDevices(ctx context.Context, req *types.SyncDevicesRequest) (*types.SyncDevicesResponse, error) {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	// Use VM API request timeout
	timeout := s.getVMAPIRequestTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// No translation needed - types are now unified in vm-gateway/types
	return client.SyncDevices(timeoutCtx, req)
}

// SyncDataUnits syncs labeled data units to the VM for model training.
func (s *HTTPTransportService) SyncDataUnits(ctx context.Context, req *types.SyncDataUnitsRequest) (*types.SyncDataUnitsResponse, error) {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	// Use VM API request timeout
	timeout := s.getVMAPIRequestTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// No translation needed - types are now unified in vm-gateway/types
	return client.SyncDataUnits(timeoutCtx, req)
}

// SyncAuditLogs syncs audit logs to the VM.
func (s *HTTPTransportService) SyncAuditLogs(ctx context.Context, req *types.SyncAuditLogsRequest) (*types.SyncAuditLogsResponse, error) {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("HTTPS client not initialized")
	}
	// Use VM API request timeout
	timeout := s.getVMAPIRequestTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// No translation needed - types are now unified in vm-gateway/types
	return client.SyncAuditLogs(timeoutCtx, req)
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
	// Use VM API request timeout
	timeout := s.getVMAPIRequestTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.ReportDeploymentStatus(timeoutCtx, deploymentID, status, errorMessage, modelPath)
}

// Heartbeat sends a heartbeat to the VM.
func (s *HTTPTransportService) Heartbeat(ctx context.Context, req *types.HeartbeatRequest) error {
	s.mu.RLock()
	client := s.httpsClientService
	s.mu.RUnlock()
	if client == nil {
		return fmt.Errorf("HTTPS client not initialized")
	}
	// Use VM API request timeout
	timeout := s.getVMAPIRequestTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// No translation needed - types are now unified in vm-gateway/types
	return client.Heartbeat(timeoutCtx, req)
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
	// Use VM API request timeout
	timeout := s.getVMAPIRequestTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// No translation needed - types are now unified in vm-gateway/types
	return client.SendEvents(timeoutCtx, events)
}

// GetHealthMetrics returns health metrics from the HTTP transport service.
// This includes certificate rotation status, time sync status, and rate limit stats.
// Returns nil for metrics that are not available.
func (s *HTTPTransportService) GetHealthMetrics() (
	certRotation interface{},
	timeSync interface{},
	rateLimit interface{},
) {
	// Get certificate rotation status from HTTPS client if available
	if clientSvc, ok := s.httpsClientService.(interface {
		GetCertificateRotationStatus() interface{}
	}); ok {
		certRotation = clientSvc.GetCertificateRotationStatus()
	}

	// Get time sync status from HTTPS client if available
	if clientSvc, ok := s.httpsClientService.(interface {
		GetTimeSyncStatus() interface{}
	}); ok {
		timeSync = clientSvc.GetTimeSyncStatus()
	}

	// Get rate limit stats from HTTPS server if available
	if serverSvc, ok := s.httpsServerService.(interface {
		GetRateLimitStats() interface{}
	}); ok {
		rateLimit = serverSvc.GetRateLimitStats()
	}

	return certRotation, timeSync, rateLimit
}
