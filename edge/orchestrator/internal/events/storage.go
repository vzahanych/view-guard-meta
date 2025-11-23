package events

import (
	"context"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
)

// Storage provides event storage operations
type Storage struct {
	stateManager *state.Manager
	logger       *logger.Logger
}

// NewStorage creates a new event storage
func NewStorage(stateManager *state.Manager, log *logger.Logger) *Storage {
	return &Storage{
		stateManager: stateManager,
		logger:       log,
	}
}

// SaveEvent saves an event to the database
func (s *Storage) SaveEvent(ctx context.Context, event *Event) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Convert to EventState
	eventState := event.ToEventState()

	// Save to state manager
	err := s.stateManager.SaveEvent(ctx, eventState)
	if err != nil {
		return fmt.Errorf("failed to save event: %w", err)
	}

	s.logger.Debug(
		"Event saved",
		"event_id", event.ID,
		"camera_id", event.CameraID,
		"event_type", event.EventType,
		"confidence", event.Confidence,
	)

	return nil
}

// GetEvent retrieves an event by ID
func (s *Storage) GetEvent(ctx context.Context, eventID string) (*Event, error) {
	// Get pending events and search for the ID
	// Note: This is a simplified implementation. In production, you might want
	// a dedicated GetEvent method in state.Manager
	pending, err := s.stateManager.GetPendingEvents(ctx, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	for _, es := range pending {
		if es.ID == eventID {
			return FromEventState(es), nil
		}
	}

	return nil, fmt.Errorf("event not found: %s", eventID)
}

// ListEvents retrieves events with optional filters
func (s *Storage) ListEvents(
	ctx context.Context,
	cameraID string,
	eventType string,
	startTime, endTime time.Time,
	limit int,
) ([]*Event, error) {
	// Get pending events
	pending, err := s.stateManager.GetPendingEvents(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	var events []*Event
	for _, es := range pending {
		// Apply filters
		if cameraID != "" && es.CameraID != cameraID {
			continue
		}
		if eventType != "" && es.EventType != eventType {
			continue
		}
		if !startTime.IsZero() && es.Timestamp.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && es.Timestamp.After(endTime) {
			continue
		}

		events = append(events, FromEventState(es))
	}

	return events, nil
}

// GetEventsByCamera retrieves all events for a specific camera
func (s *Storage) GetEventsByCamera(ctx context.Context, cameraID string, limit int) ([]*Event, error) {
	return s.ListEvents(ctx, cameraID, "", time.Time{}, time.Time{}, limit)
}

// GetEventsByType retrieves all events of a specific type
func (s *Storage) GetEventsByType(ctx context.Context, eventType string, limit int) ([]*Event, error) {
	return s.ListEvents(ctx, "", eventType, time.Time{}, time.Time{}, limit)
}

// GetEventsByTimeRange retrieves events within a time range
func (s *Storage) GetEventsByTimeRange(
	ctx context.Context,
	startTime, endTime time.Time,
	limit int,
) ([]*Event, error) {
	return s.ListEvents(ctx, "", "", startTime, endTime, limit)
}

// GetPendingEvents retrieves all pending (untransmitted) events
func (s *Storage) GetPendingEvents(ctx context.Context, limit int) ([]*Event, error) {
	pending, err := s.stateManager.GetPendingEvents(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending events: %w", err)
	}

	events := make([]*Event, len(pending))
	for i, es := range pending {
		events[i] = FromEventState(es)
	}

	return events, nil
}

// MarkEventTransmitted marks an event as transmitted
func (s *Storage) MarkEventTransmitted(ctx context.Context, eventID string) error {
	err := s.stateManager.MarkEventTransmitted(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark event as transmitted: %w", err)
	}

	s.logger.Debug("Event marked as transmitted", "event_id", eventID)
	return nil
}

// CleanupOldEvents removes old transmitted events
func (s *Storage) CleanupOldEvents(ctx context.Context, olderThan time.Duration) error {
	err := s.stateManager.CleanupOldEvents(ctx, olderThan)
	if err != nil {
		return fmt.Errorf("failed to cleanup old events: %w", err)
	}

	s.logger.Debug("Old events cleaned up", "older_than", olderThan)
	return nil
}

// CountEvents counts events matching the filters
func (s *Storage) CountEvents(
	ctx context.Context,
	cameraID string,
	eventType string,
	startTime, endTime time.Time,
) (int, error) {
	events, err := s.ListEvents(ctx, cameraID, eventType, startTime, endTime, 10000)
	if err != nil {
		return 0, err
	}
	return len(events), nil
}

