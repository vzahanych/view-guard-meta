package processing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

// EventRegistrarService handles event registration in local database
type EventRegistrarService struct {
	*service.ServiceBase
	logger       *logger.Logger
	eventStorage EventStorageService
	eventQueue   EventQueueService
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// EventStorageService interface for event storage operations
type EventStorageService interface {
	SaveEvent(ctx context.Context, event *events.Event) error
	GetEvent(ctx context.Context, eventID string) (*events.Event, error)
	MarkEventTransmitted(ctx context.Context, eventID string) error
}

// EventRegistrarServiceConfig contains event registrar configuration
type EventRegistrarServiceConfig struct {
	EventStorage EventStorageService
	EventQueue   EventQueueService // Optional: if nil, events are only stored, not queued
}

// NewEventRegistrarService creates a new event registrar service
func NewEventRegistrarService(config EventRegistrarServiceConfig, log *logger.Logger) *EventRegistrarService {
	ctx, cancel := context.WithCancel(context.Background())

	return &EventRegistrarService{
		ServiceBase:  service.NewServiceBase("event-registrar-service", log),
		logger:       log,
		eventStorage: config.EventStorage,
		eventQueue:   config.EventQueue,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the event registrar service
func (ers *EventRegistrarService) Start(ctx context.Context) error {
	ers.LogInfo("Event registrar service started")
	return nil
}

// Stop stops the event registrar service
func (ers *EventRegistrarService) Stop() error {
	ers.cancel()
	ers.LogInfo("Event registrar service stopped")
	return nil
}

// RegisterEvent registers an event in the local database immediately upon detection
func (ers *EventRegistrarService) RegisterEvent(ctx context.Context, event *events.Event) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Ensure event has an ID (should already be set by events.NewEvent())
	if event.ID == "" {
		return fmt.Errorf("event ID is required")
	}

	// Ensure event has required fields
	if event.CameraID == "" {
		return fmt.Errorf("event camera ID is required")
	}
	if event.EventType == "" {
		return fmt.Errorf("event type is required")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Ensure metadata is initialized
	if event.Metadata == nil {
		event.Metadata = make(map[string]interface{})
	}

	// Save event to database (this automatically adds it to queue if transmitted = false)
	err := ers.eventStorage.SaveEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to save event to database: %w", err)
	}

	ers.logger.Info("Event registered",
		"event_id", event.ID,
		"camera_id", event.CameraID,
		"event_type", event.EventType,
		"timestamp", event.Timestamp,
		"confidence", event.Confidence,
		"clip_path", event.ClipPath,
		"snapshot_path", event.SnapshotPath,
	)

	// Optionally enqueue event for transmission (if queue service is configured)
	// Note: state.Manager.SaveEvent() already adds events to queue automatically,
	// but we can use the queue service for priority management
	if ers.eventQueue != nil {
		// Use default priority (0) - newer events will be processed first due to created_at ordering
		err = ers.eventQueue.Enqueue(ctx, event, 0)
		if err != nil {
			// Log warning but don't fail registration (event is already saved)
			ers.logger.Warn("Failed to enqueue event",
				"event_id", event.ID,
				"error", err,
			)
		}
	}

	return nil
}

// UpdateEventClipPath updates the clip path for an event
func (ers *EventRegistrarService) UpdateEventClipPath(ctx context.Context, eventID string, clipPath string) error {
	// Get event
	event, err := ers.eventStorage.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to get event: %w", err)
	}

	if event == nil {
		return fmt.Errorf("event not found: %s", eventID)
	}

	// Update clip path
	event.ClipPath = clipPath

	// Save updated event
	err = ers.eventStorage.SaveEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to update event clip path: %w", err)
	}

	ers.logger.Debug("Event clip path updated",
		"event_id", eventID,
		"clip_path", clipPath,
	)

	return nil
}

// UpdateEventSnapshotPath updates the snapshot path for an event
func (ers *EventRegistrarService) UpdateEventSnapshotPath(ctx context.Context, eventID string, snapshotPath string) error {
	// Get event
	event, err := ers.eventStorage.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to get event: %w", err)
	}

	if event == nil {
		return fmt.Errorf("event not found: %s", eventID)
	}

	// Update snapshot path
	event.SnapshotPath = snapshotPath

	// Save updated event
	err = ers.eventStorage.SaveEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to update event snapshot path: %w", err)
	}

	ers.logger.Debug("Event snapshot path updated",
		"event_id", eventID,
		"snapshot_path", snapshotPath,
	)

	return nil
}

// UpdateEventPaths updates both clip and snapshot paths for an event
func (ers *EventRegistrarService) UpdateEventPaths(ctx context.Context, eventID string, clipPath string, snapshotPath string) error {
	// Get event
	event, err := ers.eventStorage.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to get event: %w", err)
	}

	if event == nil {
		return fmt.Errorf("event not found: %s", eventID)
	}

	// Update paths
	event.ClipPath = clipPath
	event.SnapshotPath = snapshotPath

	// Save updated event
	err = ers.eventStorage.SaveEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to update event paths: %w", err)
	}

	ers.logger.Debug("Event paths updated",
		"event_id", eventID,
		"clip_path", clipPath,
		"snapshot_path", snapshotPath,
	)

	return nil
}

// GetEvent retrieves an event by ID
func (ers *EventRegistrarService) GetEvent(ctx context.Context, eventID string) (*events.Event, error) {
	return ers.eventStorage.GetEvent(ctx, eventID)
}

// MarkEventTransmitted marks an event as transmitted
func (ers *EventRegistrarService) MarkEventTransmitted(ctx context.Context, eventID string) error {
	err := ers.eventStorage.MarkEventTransmitted(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark event as transmitted: %w", err)
	}

	ers.logger.Debug("Event marked as transmitted", "event_id", eventID)
	return nil
}

