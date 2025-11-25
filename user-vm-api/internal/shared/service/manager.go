package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
)

// Manager manages the lifecycle of all services
type Manager struct {
	logger     *logging.Logger
	services   []Service
	statuses   map[string]*ServiceStatus
	eventBus   *EventBus
	mu         sync.RWMutex
	wg         sync.WaitGroup
	startOrder []string // Track service start order for proper shutdown
}

// Service represents a service that can be started and stopped
type Service interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Name() string
}

// ServiceWithEvents is a service that can publish events
type ServiceWithEvents interface {
	Service
	SetEventBus(bus *EventBus)
}

// NewManager creates a new service manager
func NewManager(log *logging.Logger) *Manager {
	return &Manager{
		logger:     log,
		services:   make([]Service, 0),
		statuses:   make(map[string]*Status),
		eventBus:   NewEventBus(100),
		startOrder: make([]string, 0),
	}
}

// GetEventBus returns the event bus for inter-service communication
func (m *Manager) GetEventBus() *EventBus {
	return m.eventBus
}

// Register registers a service with the manager
func (m *Manager) Register(svc Service) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.services = append(m.services, svc)

	// Initialize status tracking
	status := NewServiceStatus(svc.Name())
	m.statuses[svc.Name()] = status

	// Set event bus if service supports it
	if svcWithEvents, ok := svc.(ServiceWithEvents); ok {
		svcWithEvents.SetEventBus(m.eventBus)
	}
}

// Start starts all registered services
func (m *Manager) Start(ctx context.Context, cfg *config.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("Starting services", "count", len(m.services))

	for _, svc := range m.services {
		status := m.statuses[svc.Name()]
		status.SetStatus(StatusStarting)

		m.logger.Info("Starting service", "service", svc.Name())

		m.wg.Add(1)
		go func(s Service, st *Status) {
			defer m.wg.Done()
			if err := s.Start(ctx); err != nil {
				m.logger.Error("Service failed to start", "service", s.Name(), "error", err)
				st.SetStatus(StatusError)
				st.SetError(err)
			} else {
				st.SetStatus(StatusRunning)
				m.logger.Info("Service started", "service", s.Name())
			}
		}(svc, status)

		m.startOrder = append(m.startOrder, svc.Name())
	}

	// Wait a bit for services to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Stop stops all registered services in reverse order
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("Stopping services", "count", len(m.services))

	// Stop in reverse order
	for i := len(m.services) - 1; i >= 0; i-- {
		svc := m.services[i]
		status := m.statuses[svc.Name()]

		if status.GetStatus() == StatusRunning {
			status.SetStatus(StatusStopping)
			m.logger.Info("Stopping service", "service", svc.Name())

			if err := svc.Stop(ctx); err != nil {
				m.logger.Error("Service failed to stop", "service", svc.Name(), "error", err)
				status.SetStatus(StatusError)
				status.SetError(err)
			} else {
				status.SetStatus(StatusStopped)
				m.logger.Info("Service stopped", "service", svc.Name())
			}
		}
	}

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for services to stop")
	}
}

// GetStatus returns the overall status of the manager
func (m *Manager) GetStatus() *ServiceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	overallStatus := NewServiceStatus("orchestrator")
	overallStatus.SetStatus(StatusRunning)

	// Check if any service is in error state
	for _, status := range m.statuses {
		if status.GetStatus() == StatusError {
			overallStatus.SetStatus(StatusError)
			overallStatus.SetError(status.GetError())
			break
		}
	}

	return overallStatus
}

// GetServiceStatus returns the status of a specific service
func (m *Manager) GetServiceStatus(name string) *ServiceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if status, ok := m.statuses[name]; ok {
		return status
	}

	return nil
}

