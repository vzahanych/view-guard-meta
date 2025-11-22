package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// Manager manages the lifecycle of all services
type Manager struct {
	logger     *logger.Logger
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
func NewManager(log *logger.Logger) *Manager {
	return &Manager{
		logger:     log,
		services:   make([]Service, 0),
		statuses:   make(map[string]*ServiceStatus),
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

	// Start event bus monitoring
	m.startEventMonitoring(ctx)

	for _, svc := range m.services {
		svc := svc // capture loop variable
		status := m.statuses[svc.Name()]
		
		status.SetStatus(StatusStarting)
		m.startOrder = append(m.startOrder, svc.Name())
		
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			
			// Publish service starting event
			m.eventBus.Publish(Event{
				Type:   EventTypeServiceStarted,
				Source: "manager",
				Data: map[string]interface{}{
					"service": svc.Name(),
				},
			})
			
			if err := svc.Start(ctx); err != nil {
				status.SetError(err)
				m.logger.Error("Service failed to start",
					"service", svc.Name(),
					"error", err,
				)
				m.eventBus.Publish(Event{
					Type:   EventTypeServiceError,
					Source: svc.Name(),
					Data: map[string]interface{}{
						"error": err.Error(),
					},
				})
			} else {
				status.SetStatus(StatusRunning)
				m.logger.Info("Service started", "service", svc.Name())
			}
		}()
	}

	// Give services a moment to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

// startEventMonitoring starts monitoring system events
func (m *Manager) startEventMonitoring(ctx context.Context) {
	ch := m.eventBus.SubscribeAll()
	go func() {
		defer m.eventBus.Unsubscribe(EventTypeServiceStarted, ch)
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return
				}
				m.logger.Debug("Event received",
					"type", event.Type,
					"source", event.Source,
					"timestamp", event.Timestamp,
				)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Shutdown gracefully shuts down all services
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("Shutting down services", "count", len(m.services))

	// Close event bus
	defer m.eventBus.Close()

	// Create a channel to signal when all services are stopped
	done := make(chan struct{})
	go func() {
		// Stop services in reverse order of start
		for i := len(m.startOrder) - 1; i >= 0; i-- {
			serviceName := m.startOrder[i]
			status := m.statuses[serviceName]
			
			// Find service by name
			var svc Service
			for _, s := range m.services {
				if s.Name() == serviceName {
					svc = s
					break
				}
			}
			
			if svc == nil {
				continue
			}

			status.SetStatus(StatusStopping)
			m.logger.Info("Stopping service", "service", svc.Name())

			stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := svc.Stop(stopCtx); err != nil {
				status.SetError(err)
				m.logger.Error("Error stopping service",
					"service", svc.Name(),
					"error", err,
				)
			} else {
				status.SetStatus(StatusStopped)
				m.logger.Info("Service stopped", "service", svc.Name())
			}
			cancel()
			
			// Publish service stopped event
			m.eventBus.Publish(Event{
				Type:   EventTypeServiceStopped,
				Source: "manager",
				Data: map[string]interface{}{
					"service": svc.Name(),
				},
			})
		}

		// Wait for all service goroutines to finish
		m.wg.Wait()
		close(done)
	}()

	// Wait for shutdown to complete or context timeout
	select {
	case <-done:
		m.logger.Info("All services stopped")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout: %w", ctx.Err())
	}
}

// GetServiceCount returns the number of registered services
func (m *Manager) GetServiceCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.services)
}

// GetServiceStatus returns the status of a service
func (m *Manager) GetServiceStatus(serviceName string) *ServiceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.statuses[serviceName]
}

// GetAllStatuses returns all service statuses
func (m *Manager) GetAllStatuses() map[string]*ServiceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	statuses := make(map[string]*ServiceStatus)
	for name, status := range m.statuses {
		statuses[name] = status
	}
	return statuses
}

