package service

import (
	"context"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// ExampleService demonstrates how to implement a service with event support
type ExampleService struct {
	name      string
	logger    *logger.Logger
	eventBus  *EventBus
	ctx       context.Context
	cancel    context.CancelFunc
	status    *ServiceStatus
}

// NewExampleService creates a new example service
func NewExampleService(name string, log *logger.Logger) *ExampleService {
	return &ExampleService{
		name:   name,
		logger: log,
		status: NewServiceStatus(name),
	}
}

// Name returns the service name
func (s *ExampleService) Name() string {
	return s.name
}

// SetEventBus sets the event bus for the service
func (s *ExampleService) SetEventBus(bus *EventBus) {
	s.eventBus = bus
}

// Start starts the service
func (s *ExampleService) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.status.SetStatus(StatusStarting)

	// Example: Subscribe to events
	if s.eventBus != nil {
		// Subscribe to specific event types
		detectionCh := s.eventBus.Subscribe(EventTypeDetection)
		go s.handleDetections(detectionCh)
	}

	// Start service work
	go s.run()

	s.status.SetStatus(StatusRunning)
	s.logger.Info("Example service started", "service", s.name)

	return nil
}

// Stop stops the service
func (s *ExampleService) Stop(ctx context.Context) error {
	s.status.SetStatus(StatusStopping)
	
	if s.cancel != nil {
		s.cancel()
	}

	// Wait for context cancellation or timeout
	select {
	case <-s.ctx.Done():
	case <-ctx.Done():
		return ctx.Err()
	}

	s.status.SetStatus(StatusStopped)
	s.logger.Info("Example service stopped", "service", s.name)

	return nil
}

// run performs the main service work
func (s *ExampleService) run() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Example: Publish an event
			if s.eventBus != nil {
				s.eventBus.Publish(Event{
					Type:   EventTypeInference,
					Source: s.name,
					Data: map[string]interface{}{
						"timestamp": time.Now(),
						"count":     1,
					},
				})
			}
			s.logger.Debug("Example service tick", "service", s.name)
		}
	}
}

// handleDetections handles detection events
func (s *ExampleService) handleDetections(ch <-chan Event) {
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			s.logger.Info("Received detection event",
				"service", s.name,
				"source", event.Source,
				"data", event.Data,
			)
		case <-s.ctx.Done():
			return
		}
	}
}

// ServiceBase provides a base implementation for services
type ServiceBase struct {
	name     string
	logger   *logger.Logger
	eventBus *EventBus
	status   *ServiceStatus
}

// NewServiceBase creates a new service base
func NewServiceBase(name string, log *logger.Logger) *ServiceBase {
	return &ServiceBase{
		name:   name,
		logger: log,
		status: NewServiceStatus(name),
	}
}

// Name returns the service name
func (sb *ServiceBase) Name() string {
	return sb.name
}

// SetEventBus sets the event bus
func (sb *ServiceBase) SetEventBus(bus *EventBus) {
	sb.eventBus = bus
}

// GetEventBus returns the event bus
func (sb *ServiceBase) GetEventBus() *EventBus {
	return sb.eventBus
}

// GetStatus returns the service status
func (sb *ServiceBase) GetStatus() *ServiceStatus {
	return sb.status
}

// PublishEvent publishes an event to the event bus
func (sb *ServiceBase) PublishEvent(eventType EventType, data map[string]interface{}) {
	if sb.eventBus != nil {
		sb.eventBus.Publish(Event{
			Type:   eventType,
			Source: sb.name,
			Data:   data,
		})
	}
}

// LogInfo logs an info message
func (sb *ServiceBase) LogInfo(msg string, fields ...interface{}) {
	sb.logger.Info(msg, append([]interface{}{"service", sb.name}, fields...)...)
}

// LogError logs an error message
func (sb *ServiceBase) LogError(msg string, err error, fields ...interface{}) {
	allFields := append([]interface{}{"service", sb.name, "error", err}, fields...)
	sb.logger.Error(msg, allFields...)
}

// LogDebug logs a debug message
func (sb *ServiceBase) LogDebug(msg string, fields ...interface{}) {
	sb.logger.Debug(msg, append([]interface{}{"service", sb.name}, fields...)...)
}

