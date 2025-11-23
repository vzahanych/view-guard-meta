package integration

import (
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

// TestServiceManager_ServiceLifecycle tests the complete service lifecycle
func TestServiceManager_ServiceLifecycle(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create service manager
	manager := service.NewManager(env.Logger)

	// Create a test service
	testService := service.NewExampleService("test-service", env.Logger)

	// Register service
	manager.Register(testService)

	// Start services
	ctx, cancel := ContextWithTimeout(5 * time.Second)
	defer cancel()

	if err := manager.Start(ctx, env.Config); err != nil {
		t.Fatalf("Failed to start services: %v", err)
	}

	// Verify service is running
	// ExampleService doesn't expose GetStatus, so we check via manager
	status := manager.GetServiceStatus("test-service")
	if status.GetStatus() != service.StatusRunning {
		t.Errorf("Expected service status %v, got %v", service.StatusRunning, status.GetStatus())
	}

	// Stop services
	shutdownCtx, shutdownCancel := ContextWithTimeout(5 * time.Second)
	defer shutdownCancel()

	if err := manager.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Failed to shutdown services: %v", err)
	}

	// Verify service is stopped
	status = manager.GetServiceStatus("test-service")
	if status.GetStatus() != service.StatusStopped {
		t.Errorf("Expected service status %v, got %v", service.StatusStopped, status.GetStatus())
	}
}

// TestServiceManager_EventBusIntegration tests service communication via event bus
func TestServiceManager_EventBusIntegration(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create service manager
	manager := service.NewManager(env.Logger)

	// Create two test services
	service1 := service.NewExampleService("service-1", env.Logger)
	service2 := service.NewExampleService("service-2", env.Logger)

	// Register services
	manager.Register(service1)
	manager.Register(service2)

	// Start services
	ctx, cancel := ContextWithTimeout(5 * time.Second)
	defer cancel()

	if err := manager.Start(ctx, env.Config); err != nil {
		t.Fatalf("Failed to start services: %v", err)
	}
	defer manager.Shutdown(ctx)

	// Get event bus and subscribe to events
	eventBus := manager.GetEventBus()
	eventChan := eventBus.Subscribe(service.EventTypeInference)

	// Wait a bit for service1 to publish an event (it publishes every 5 seconds)
	// Or we can publish directly via event bus
	eventBus.Publish(service.Event{
		Type:   service.EventTypeInference,
		Source: "service-1",
		Data: map[string]interface{}{
			"message": "hello from service1",
		},
	})

	// Wait for event to be received
	select {
	case receivedEvent := <-eventChan:
		if receivedEvent.Type != service.EventTypeInference {
			t.Errorf("Expected event type '%s', got '%s'", service.EventTypeInference, receivedEvent.Type)
		}
		if receivedEvent.Data["message"] != "hello from service1" {
			t.Errorf("Expected message 'hello from service1', got '%v'", receivedEvent.Data["message"])
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for event")
	}
}

// TestServiceManager_MultipleServices tests multiple services working together
func TestServiceManager_MultipleServices(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create service manager
	manager := service.NewManager(env.Logger)

	// Create multiple services
	services := make([]*service.ExampleService, 5)
	for i := 0; i < 5; i++ {
		svc := service.NewExampleService("service-"+string(rune('A'+i)), env.Logger)
		services[i] = svc
		manager.Register(svc)
	}

	// Start services
	ctx, cancel := ContextWithTimeout(5 * time.Second)
	defer cancel()

	if err := manager.Start(ctx, env.Config); err != nil {
		t.Fatalf("Failed to start services: %v", err)
	}
	defer manager.Shutdown(ctx)

	// Verify all services are running
	allStatuses := manager.GetAllStatuses()
	if len(allStatuses) != 5 {
		t.Errorf("Expected 5 service statuses, got %d", len(allStatuses))
	}

	for i, svc := range services {
		status := manager.GetServiceStatus(svc.Name())
		if status == nil {
			t.Errorf("Service %d (%s) has no status", i, svc.Name())
			continue
		}
		if status.GetStatus() != service.StatusRunning {
			t.Errorf("Service %d (%s) expected status %v, got %v", i, svc.Name(), service.StatusRunning, status.GetStatus())
		}
	}

	// Verify service count
	serviceCount := manager.GetServiceCount()
	if serviceCount != 5 {
		t.Errorf("Expected 5 services, got %d", serviceCount)
	}
}

