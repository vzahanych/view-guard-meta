package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func TestNewManager(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	mgr := NewManager(log)

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}

	if mgr.GetServiceCount() != 0 {
		t.Errorf("Expected 0 services, got %d", mgr.GetServiceCount())
	}

	if mgr.GetEventBus() == nil {
		t.Error("Event bus should be initialized")
	}
}

func TestManager_Register(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	mgr := NewManager(log)

	mockSvc := &mockService{name: "test-service"}
	mgr.Register(mockSvc)

	if mgr.GetServiceCount() != 1 {
		t.Errorf("Expected 1 service, got %d", mgr.GetServiceCount())
	}

	status := mgr.GetServiceStatus("test-service")
	if status == nil {
		t.Fatal("Service status should be created")
	}

	if status.GetStatus() != StatusStopped {
		t.Errorf("Expected status %s, got %s", StatusStopped, status.GetStatus())
	}
}

func TestManager_Register_WithEvents(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	mgr := NewManager(log)

	mockSvc := &mockServiceWithEvents{name: "event-service"}
	mgr.Register(mockSvc)

	if mockSvc.eventBus == nil {
		t.Error("Event bus should be set for service with events")
	}
}

func TestManager_Start(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	mgr := NewManager(log)

	mockSvc := &mockService{name: "test-service"}
	mgr.Register(mockSvc)

	cfg := &config.Config{}
	ctx := context.Background()

	err := mgr.Start(ctx, cfg)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	status := mgr.GetServiceStatus("test-service")
	if status.GetStatus() != StatusRunning {
		t.Errorf("Expected status %s, got %s", StatusRunning, status.GetStatus())
	}

	if !status.IsRunning() {
		t.Error("Service should be running")
	}
}

func TestManager_Start_ServiceError(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	mgr := NewManager(log)

	mockSvc := &mockService{
		name:        "failing-service",
		startError:  errors.New("start failed"),
	}
	mgr.Register(mockSvc)

	cfg := &config.Config{}
	ctx := context.Background()

	err := mgr.Start(ctx, cfg)
	if err != nil {
		t.Fatalf("Start should not fail even if service fails: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	status := mgr.GetServiceStatus("failing-service")
	if status.GetStatus() != StatusError {
		t.Errorf("Expected status %s, got %s", StatusError, status.GetStatus())
	}

	if status.GetError() == nil {
		t.Error("Service should have an error")
	}
}

func TestManager_Shutdown(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	mgr := NewManager(log)

	mockSvc1 := &mockService{name: "service-1"}
	mockSvc2 := &mockService{name: "service-2"}
	mgr.Register(mockSvc1)
	mgr.Register(mockSvc2)

	cfg := &config.Config{}
	ctx := context.Background()

	mgr.Start(ctx, cfg)
	time.Sleep(200 * time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := mgr.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	status1 := mgr.GetServiceStatus("service-1")
	status2 := mgr.GetServiceStatus("service-2")

	if status1.GetStatus() != StatusStopped {
		t.Errorf("Service 1 should be stopped, got %s", status1.GetStatus())
	}

	if status2.GetStatus() != StatusStopped {
		t.Errorf("Service 2 should be stopped, got %s", status2.GetStatus())
	}

	if !mockSvc1.stopped {
		t.Error("Service 1 should have been stopped")
	}

	if !mockSvc2.stopped {
		t.Error("Service 2 should have been stopped")
	}
}

func TestManager_Shutdown_ReverseOrder(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	mgr := NewManager(log)

	var stopOrder []string

	mockSvc1 := &mockService{
		name: "service-1",
		onStop: func() {
			stopOrder = append(stopOrder, "service-1")
		},
	}
	mockSvc2 := &mockService{
		name: "service-2",
		onStop: func() {
			stopOrder = append(stopOrder, "service-2")
		},
	}
	mockSvc3 := &mockService{
		name: "service-3",
		onStop: func() {
			stopOrder = append(stopOrder, "service-3")
		},
	}

	mgr.Register(mockSvc1)
	mgr.Register(mockSvc2)
	mgr.Register(mockSvc3)

	cfg := &config.Config{}
	ctx := context.Background()

	mgr.Start(ctx, cfg)
	time.Sleep(200 * time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr.Shutdown(shutdownCtx)

	if len(stopOrder) != 3 {
		t.Fatalf("Expected 3 services stopped, got %d", len(stopOrder))
	}

	if stopOrder[0] != "service-3" {
		t.Errorf("Expected service-3 to stop first, got %s", stopOrder[0])
	}

	if stopOrder[1] != "service-2" {
		t.Errorf("Expected service-2 to stop second, got %s", stopOrder[1])
	}

	if stopOrder[2] != "service-1" {
		t.Errorf("Expected service-1 to stop last, got %s", stopOrder[2])
	}
}

func TestManager_Shutdown_Timeout(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	mgr := NewManager(log)

	mockSvc := &mockService{
		name:      "slow-service",
		stopDelay: 2 * time.Second,
	}
	mgr.Register(mockSvc)

	cfg := &config.Config{}
	ctx := context.Background()

	mgr.Start(ctx, cfg)
	time.Sleep(200 * time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := mgr.Shutdown(shutdownCtx)
	if err == nil {
		t.Error("Shutdown should timeout and return error")
	}
}

func TestManager_GetAllStatuses(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})
	mgr := NewManager(log)

	mockSvc1 := &mockService{name: "service-1"}
	mockSvc2 := &mockService{name: "service-2"}
	mgr.Register(mockSvc1)
	mgr.Register(mockSvc2)

	statuses := mgr.GetAllStatuses()
	if len(statuses) != 2 {
		t.Errorf("Expected 2 statuses, got %d", len(statuses))
	}

	if statuses["service-1"] == nil {
		t.Error("Status for service-1 should exist")
	}

	if statuses["service-2"] == nil {
		t.Error("Status for service-2 should exist")
	}
}

type mockService struct {
	name      string
	started   bool
	stopped   bool
	startError error
	stopError  error
	stopDelay time.Duration
	onStop    func()
}

func (m *mockService) Name() string {
	return m.name
}

func (m *mockService) Start(ctx context.Context) error {
	if m.startError != nil {
		return m.startError
	}
	m.started = true
	return nil
}

func (m *mockService) Stop(ctx context.Context) error {
	if m.stopDelay > 0 {
		time.Sleep(m.stopDelay)
	}
	if m.onStop != nil {
		m.onStop()
	}
	if m.stopError != nil {
		return m.stopError
	}
	m.stopped = true
	return nil
}

type mockServiceWithEvents struct {
	name     string
	eventBus *EventBus
}

func (m *mockServiceWithEvents) Name() string {
	return m.name
}

func (m *mockServiceWithEvents) Start(ctx context.Context) error {
	return nil
}

func (m *mockServiceWithEvents) Stop(ctx context.Context) error {
	return nil
}

func (m *mockServiceWithEvents) SetEventBus(bus *EventBus) {
	m.eventBus = bus
}

