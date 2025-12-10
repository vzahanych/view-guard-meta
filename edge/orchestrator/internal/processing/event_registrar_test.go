package processing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	svc "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

// MockEventStorage is a mock implementation of EventStorageService
type MockEventStorage struct {
	events map[string]*events.Event
}

func NewMockEventStorage() *MockEventStorage {
	return &MockEventStorage{
		events: make(map[string]*events.Event),
	}
}

func (m *MockEventStorage) SaveEvent(ctx context.Context, event *events.Event) error {
	m.events[event.ID] = event
	return nil
}

func (m *MockEventStorage) GetEvent(ctx context.Context, eventID string) (*events.Event, error) {
	if event, ok := m.events[eventID]; ok {
		return event, nil
	}
	return nil, errors.New("event not found")
}

func (m *MockEventStorage) MarkEventTransmitted(ctx context.Context, eventID string) error {
	if _, ok := m.events[eventID]; ok {
		// Event marked as transmitted (field may not exist in Event struct)
		return nil
	}
	return errors.New("event not found")
}

// MockEventQueue is a mock implementation of EventQueueService
type MockEventQueue struct {
	events []*events.Event
}

func NewMockEventQueue() *MockEventQueue {
	return &MockEventQueue{
		events: make([]*events.Event, 0),
	}
}

func (m *MockEventQueue) Enqueue(ctx context.Context, event *events.Event, priority int) error {
	m.events = append(m.events, event)
	return nil
}

func (m *MockEventQueue) Dequeue(ctx context.Context) (*events.Event, error) {
	if len(m.events) == 0 {
		return nil, nil
	}
	event := m.events[0]
	m.events = m.events[1:]
	return event, nil
}

func (m *MockEventQueue) Peek(ctx context.Context) (*events.Event, error) {
	if len(m.events) == 0 {
		return nil, nil
	}
	return m.events[0], nil
}

func (m *MockEventQueue) BatchDequeue(ctx context.Context, batchSize int) ([]*events.Event, error) {
	if len(m.events) == 0 {
		return nil, nil
	}
	if batchSize > len(m.events) {
		batchSize = len(m.events)
	}
	events := m.events[:batchSize]
	m.events = m.events[batchSize:]
	return events, nil
}

func (m *MockEventQueue) Size(ctx context.Context) (int, error) {
	return len(m.events), nil
}

func (m *MockEventQueue) IsEmpty(ctx context.Context) (bool, error) {
	return len(m.events) == 0, nil
}

func (m *MockEventQueue) IncrementRetryCount(ctx context.Context, eventID string) error {
	return nil
}

func (m *MockEventQueue) GetRetryCount(ctx context.Context, eventID string) (int, error) {
	return 0, nil
}

func (m *MockEventQueue) SetPriority(ctx context.Context, eventID string, priority int) error {
	return nil
}

func (m *MockEventQueue) GetQueueStats(ctx context.Context) (*QueueStats, error) {
	return &QueueStats{
		Size: len(m.events),
	}, nil
}

func TestNewEventRegistrarService(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	eventStorage := NewMockEventStorage()
	eventQueue := NewMockEventQueue()

	config := EventRegistrarServiceConfig{
		EventStorage: eventStorage,
		EventQueue:   eventQueue,
	}

	service := NewEventRegistrarService(config, log)
	if service == nil {
		t.Fatal("NewEventRegistrarService returned nil")
	}
}

func TestEventRegistrarService_StartStop(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	eventStorage := NewMockEventStorage()
	eventQueue := NewMockEventQueue()

	config := EventRegistrarServiceConfig{
		EventStorage: eventStorage,
		EventQueue:   eventQueue,
	}

	service := NewEventRegistrarService(config, log)

	ctx := context.Background()
	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	status := service.GetStatus().GetStatus()
	if status != svc.StatusRunning {
		t.Errorf("Expected status %s, got %s", svc.StatusRunning, status)
	}

	err = service.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	status = service.GetStatus().GetStatus()
	if status != svc.StatusStopped {
		t.Errorf("Expected status %s, got %s", svc.StatusStopped, status)
	}
}

func TestEventRegistrarService_RegisterEvent(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	eventStorage := NewMockEventStorage()
	eventQueue := NewMockEventQueue()

	config := EventRegistrarServiceConfig{
		EventStorage: eventStorage,
		EventQueue:   eventQueue,
	}

	service := NewEventRegistrarService(config, log)

	ctx := context.Background()
	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop()

	event := &events.Event{
		ID:        "event-1",
		CameraID:  "camera-1",
		EventType: "anomaly",
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	err = service.RegisterEvent(ctx, event)
	if err != nil {
		t.Fatalf("RegisterEvent failed: %v", err)
	}

	// Verify event was saved
	savedEvent, err := eventStorage.GetEvent(ctx, "event-1")
	if err != nil {
		t.Fatalf("Failed to get saved event: %v", err)
	}

	if savedEvent.ID != "event-1" {
		t.Errorf("Expected event ID 'event-1', got '%s'", savedEvent.ID)
	}

	// Verify event was queued
	if len(eventQueue.events) != 1 {
		t.Errorf("Expected 1 event in queue, got %d", len(eventQueue.events))
	}
}

func TestEventRegistrarService_RegisterEvent_InvalidEvent(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{Level: "info", Format: "text"})

	eventStorage := NewMockEventStorage()
	eventQueue := NewMockEventQueue()

	config := EventRegistrarServiceConfig{
		EventStorage: eventStorage,
		EventQueue:   eventQueue,
	}

	service := NewEventRegistrarService(config, log)

	ctx := context.Background()
	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop()

	// Test nil event
	err = service.RegisterEvent(ctx, nil)
	if err == nil {
		t.Error("RegisterEvent should fail with nil event")
	}

	// Test event without ID
	event := &events.Event{
		CameraID:  "camera-1",
		EventType: "anomaly",
	}
	err = service.RegisterEvent(ctx, event)
	if err == nil {
		t.Error("RegisterEvent should fail with event without ID")
	}
}

