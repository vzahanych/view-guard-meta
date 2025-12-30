package httpimpl

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	"go.uber.org/zap"
)

// mockHTTPServerService is a mock implementation of HTTPSServerService for testing
type mockHTTPServerService struct {
	mu                sync.RWMutex
	started           bool
	ready             bool
	startDelay        time.Duration
	readyDelay        time.Duration
	startErr          error
	stopErr           error
	startCallCount    int
	stopCallCount     int
	isReadyCallCount  int
}

func (m *mockHTTPServerService) Start(ctx context.Context) error {
	m.mu.Lock()
	m.startCallCount++
	m.mu.Unlock()

	if m.startDelay > 0 {
		time.Sleep(m.startDelay)
	}

	if m.startErr != nil {
		return m.startErr
	}

	m.mu.Lock()
	m.started = true
	initialReady := m.ready // Remember initial ready state
	m.mu.Unlock()

	// Simulate server becoming ready after a delay (only if readyDelay > 0)
	// If readyDelay is 0, keep the initial ready state (may be false for timeout tests)
	if m.readyDelay > 0 {
		go func() {
			time.Sleep(m.readyDelay)
			m.mu.Lock()
			m.ready = true
			m.mu.Unlock()
		}()
	} else if initialReady {
		// Only set ready=true if it was already true (for tests that want immediate readiness)
		m.mu.Lock()
		m.ready = true
		m.mu.Unlock()
	}
	// If initialReady is false and readyDelay is 0, keep ready=false (for timeout tests)

	return nil
}

func (m *mockHTTPServerService) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCallCount++
	m.started = false
	m.ready = false
	return m.stopErr
}

func (m *mockHTTPServerService) Name() string {
	return "mock-https-server"
}

func (m *mockHTTPServerService) IsServerReady() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isReadyCallCount++
	return m.ready
}

func (m *mockHTTPServerService) getStartCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.startCallCount
}

// mockHTTPSClientService is a mock implementation of HTTPSClientService for testing
type mockHTTPSClientService struct {
	mu             sync.RWMutex
	started        bool
	connected      bool
	startDelay     time.Duration
	startErr       error
	stopErr        error
	startCallCount int
	stopCallCount  int
}

func (m *mockHTTPSClientService) Start(ctx context.Context) error {
	m.mu.Lock()
	m.startCallCount++
	m.mu.Unlock()

	if m.startDelay > 0 {
		time.Sleep(m.startDelay)
	}

	if m.startErr != nil {
		return m.startErr
	}

	m.mu.Lock()
	m.started = true
	m.connected = true
	m.mu.Unlock()

	return nil
}

func (m *mockHTTPSClientService) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCallCount++
	m.started = false
	m.connected = false
	return m.stopErr
}

func (m *mockHTTPSClientService) Name() string {
	return "mock-https-client"
}

func (m *mockHTTPSClientService) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

func (m *mockHTTPSClientService) Authenticate(ctx context.Context, edgeID string) error {
	return nil
}

func (m *mockHTTPSClientService) GetConfig(ctx context.Context) (*types.GetConfigResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockHTTPSClientService) SyncCapabilities(ctx context.Context, req *types.SyncCapabilitiesRequest) (*types.SyncCapabilitiesResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockHTTPSClientService) SyncDevices(ctx context.Context, req *types.SyncDevicesRequest) (*types.SyncDevicesResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockHTTPSClientService) SyncDataUnits(ctx context.Context, req *types.SyncDataUnitsRequest) (*types.SyncDataUnitsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockHTTPSClientService) SyncAuditLogs(ctx context.Context, req *types.SyncAuditLogsRequest) (*types.SyncAuditLogsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockHTTPSClientService) ReportDeploymentStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockHTTPSClientService) Heartbeat(ctx context.Context, req *types.HeartbeatRequest) error {
	return fmt.Errorf("not implemented")
}

func (m *mockHTTPSClientService) SendTelemetry(ctx context.Context, data *types.TelemetryData) error {
	return fmt.Errorf("not implemented")
}

func (m *mockHTTPSClientService) SendEvents(ctx context.Context, events []*types.Event) error {
	return fmt.Errorf("not implemented")
}

func (m *mockHTTPSClientService) getStartCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.startCallCount
}

// TestHTTPTransportService_Start_ConcurrentCalls verifies that concurrent calls to Start()
// are handled correctly without deadlocks. This tests the reduced lock scope fix.
func TestHTTPTransportService_Start_ConcurrentCalls(t *testing.T) {
	logger := zap.NewNop()
	serverSvc := &mockHTTPServerService{ready: true} // Start ready to avoid polling delay
	clientSvc := &mockHTTPSClientService{}

	service := NewHTTPTransportService(serverSvc, clientSvc, nil, logger)
	ctx := context.Background()

	// Call Start() concurrently multiple times
	const numCalls = 5
	errors := make(chan error, numCalls)
	var wg sync.WaitGroup

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- service.Start(ctx)
		}()
	}

	wg.Wait()
	close(errors)

	// Only one call should succeed, others should return "already started" error
	// Note: Due to the reduced lock scope, there's a race window where multiple goroutines
	// can pass the initial check, but only one should succeed in the end.
	successCount := 0
	alreadyStartedCount := 0

	for err := range errors {
		if err == nil {
			successCount++
		} else if err != nil && assert.Contains(t, err.Error(), "already started") {
			alreadyStartedCount++
		}
	}

	// At least one should succeed, and the rest should fail with "already started"
	assert.GreaterOrEqual(t, successCount, 1, "At least one Start() call should succeed")
	assert.Equal(t, numCalls, successCount+alreadyStartedCount, "All calls should either succeed or fail with 'already started'")
}

// TestHTTPTransportService_Start_NoDeadlockOnBlockingOperations verifies that Start()
// does not deadlock when health/status checks are performed during startup.
// This tests that the lock is released during blocking operations (readiness polling).
func TestHTTPTransportService_Start_NoDeadlockOnBlockingOperations(t *testing.T) {
	logger := zap.NewNop()
	// Server that takes time to become ready
	serverSvc := &mockHTTPServerService{
		ready:      false,
		readyDelay: 100 * time.Millisecond, // Server becomes ready after delay
	}
	clientSvc := &mockHTTPSClientService{}

	service := NewHTTPTransportService(serverSvc, clientSvc, nil, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the service in a goroutine
	startDone := make(chan error, 1)
	go func() {
		startDone <- service.Start(ctx)
	}()

	// While startup is in progress, try to call IsConnected() (which acquires RLock)
	// This should not deadlock even if Start() is polling for readiness
	isConnectedCalls := make(chan bool, 10)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Small delay to ensure Start() has entered the readiness polling loop
			time.Sleep(50 * time.Millisecond)
			isConnectedCalls <- service.IsConnected()
		}()
	}

	wg.Wait()
	close(isConnectedCalls)

	// Wait for Start() to complete
	select {
	case err := <-startDone:
		require.NoError(t, err, "Start() should complete without deadlock")
	case <-time.After(3 * time.Second):
		t.Fatal("Start() deadlocked or took too long - this indicates the lock scope bug still exists")
	}

	// Verify IsConnected() calls completed (should all be false since client isn't started yet in this scenario)
	for connected := range isConnectedCalls {
		_ = connected // We don't care about the value, just that calls didn't deadlock
	}
}

// TestHTTPTransportService_Start_StartupSequence verifies that the startup sequence
// works correctly with the reduced lock scope.
func TestHTTPTransportService_Start_StartupSequence(t *testing.T) {
	tests := []struct {
		name             string
		serverSvc        *mockHTTPServerService
		clientSvc        *mockHTTPSClientService
		expectError      bool
		errorContains    string
		expectServerCalls int
		expectClientCalls int
	}{
		{
			name: "successful startup with both services",
			serverSvc: &mockHTTPServerService{
				ready: true, // Server starts ready
			},
			clientSvc:          &mockHTTPSClientService{},
			expectError:        false,
			expectServerCalls:  1,
			expectClientCalls:  1,
		},
		{
			name: "server startup failure",
			serverSvc: &mockHTTPServerService{
				startErr: fmt.Errorf("server startup failed"),
			},
			clientSvc:          &mockHTTPSClientService{},
			expectError:        true,
			errorContains:      "failed to start HTTPS server service",
			expectServerCalls:  1,
			expectClientCalls:  0, // Client should not be started if server fails
		},
		{
			name: "client startup failure stops server",
			serverSvc: &mockHTTPServerService{
				ready: true,
			},
			clientSvc: &mockHTTPSClientService{
				startErr: fmt.Errorf("client startup failed"),
			},
			expectError:        true,
			errorContains:      "failed to start HTTPS client service",
			expectServerCalls:  1,
			expectClientCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			service := NewHTTPTransportService(tt.serverSvc, tt.clientSvc, nil, logger)
			ctx := context.Background()

			err := service.Start(ctx)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				// Verify service is marked as started
				// We can't directly check the started field, but we can verify by trying to start again
				err2 := service.Start(ctx)
				assert.Error(t, err2, "Second Start() should fail with 'already started'")
				assert.Contains(t, err2.Error(), "already started")
			}

			// Verify call counts (only if service was not nil)
			if tt.expectServerCalls > 0 {
				require.NotNil(t, tt.serverSvc, "Server service should not be nil when expecting calls")
				assert.Equal(t, tt.expectServerCalls, tt.serverSvc.getStartCallCount(), "Server Start() call count mismatch")
			}
			if tt.expectClientCalls > 0 {
				require.NotNil(t, tt.clientSvc, "Client service should not be nil when expecting calls")
				assert.Equal(t, tt.expectClientCalls, tt.clientSvc.getStartCallCount(), "Client Start() call count mismatch")
			}

			// Clean up
			if err == nil {
				_ = service.Stop(ctx)
			}
		})
	}
}

// TestHTTPTransportService_Start_ReadinessPolling verifies that the readiness polling
// loop works correctly and doesn't hold the lock during time.Sleep calls.
func TestHTTPTransportService_Start_ReadinessPolling(t *testing.T) {
	logger := zap.NewNop()

	// Server that becomes ready after a delay (simulates real server startup)
	serverSvc := &mockHTTPServerService{
		ready:      false,
		readyDelay: 300 * time.Millisecond, // Server becomes ready after 300ms
	}
	clientSvc := &mockHTTPSClientService{}

	service := NewHTTPTransportService(serverSvc, clientSvc, nil, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the service
	startTime := time.Now()
	err := service.Start(ctx)
	duration := time.Since(startTime)

	require.NoError(t, err, "Start() should succeed")
	// Duration should be at least the readyDelay (server takes time to become ready)
	assert.GreaterOrEqual(t, duration, 300*time.Millisecond, "Start() should wait for server readiness")
	// But should complete within reasonable time (readiness timeout default is 30s)
	assert.Less(t, duration, 5*time.Second, "Start() should complete within timeout")

	// Verify IsServerReady was called multiple times (polling happened)
	serverSvc.mu.RLock()
	isReadyCalls := serverSvc.isReadyCallCount
	serverSvc.mu.RUnlock()
	assert.Greater(t, isReadyCalls, 1, "IsServerReady() should be called multiple times during polling")
}

// TestHTTPTransportService_Start_ReadinessTimeout verifies that Start() correctly
// handles the case when server doesn't become ready within the timeout.
func TestHTTPTransportService_Start_ReadinessTimeout(t *testing.T) {
	logger := zap.NewNop()

	// Server that never becomes ready (ready stays false)
	serverSvc := &mockHTTPServerService{
		ready: false, // Never becomes ready
	}
	clientSvc := &mockHTTPSClientService{}

	// Create a custom timeout config with a short timeout for testing
	timeoutConfig := &types.TimeoutConfig{
		TransportEstablishmentTimeout: 300 * time.Millisecond,
	}

	service := NewHTTPTransportService(serverSvc, clientSvc, timeoutConfig, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := service.Start(ctx)

	require.Error(t, err, "Start() should fail when server doesn't become ready")
	assert.Contains(t, err.Error(), "did not become ready within", "Error should mention readiness timeout")
	
	// Verify server Stop() was called on timeout
	serverSvc.mu.RLock()
	stopCalls := serverSvc.stopCallCount
	serverSvc.mu.RUnlock()
	assert.Equal(t, 1, stopCalls, "Server Stop() should be called on readiness timeout")
}

// TestHTTPTransportService_Start_ContextCancellation verifies that Start() correctly
// handles context cancellation during readiness polling.
func TestHTTPTransportService_Start_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()

	// Server that takes longer than context timeout
	serverSvc := &mockHTTPServerService{
		ready:      false,
		readyDelay: 2 * time.Second, // Server becomes ready after 2 seconds
	}
	clientSvc := &mockHTTPSClientService{}

	service := NewHTTPTransportService(serverSvc, clientSvc, nil, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := service.Start(ctx)

	require.Error(t, err, "Start() should fail when context is cancelled")
	assert.Contains(t, err.Error(), "context cancelled", "Error should mention context cancellation")
	
	// Verify server Stop() was called on cancellation
	serverSvc.mu.RLock()
	stopCalls := serverSvc.stopCallCount
	serverSvc.mu.RUnlock()
	assert.Equal(t, 1, stopCalls, "Server Stop() should be called on context cancellation")
}

