package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"

	edge "github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
)

// EventSender handles sending events via gRPC
type EventSender struct {
	client *Client
	logger *logger.Logger
}

// NewEventSender creates a new event sender
func NewEventSender(client *Client, log *logger.Logger) *EventSender {
	return &EventSender{
		client: client,
		logger: log,
	}
}

// SendEvents sends a batch of events to the KVM VM
func (es *EventSender) SendEvents(ctx context.Context, eventList []*events.Event) error {
	if len(eventList) == 0 {
		return nil
	}

	client := es.client.GetEventClient()
	if client == nil {
		return fmt.Errorf("gRPC event client not available")
	}

	// Convert events to proto messages
	protoEvents := make([]*edge.Event, 0, len(eventList))
	for _, event := range eventList {
		protoEvent := es.convertEventToProto(event)
		protoEvents = append(protoEvents, protoEvent)
	}

	// Create request
	req := &edge.SendEventsRequest{
		Events: protoEvents,
	}

	// Send with timeout
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.SendEvents(sendCtx, req)
	if err != nil {
		// Check if error is retryable
		if es.isRetryableError(err) {
			return fmt.Errorf("retryable error sending events: %w", err)
		}
		return fmt.Errorf("failed to send events: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("KVM VM rejected events: %s", resp.ErrorMessage)
	}

	es.logger.Debug("Events sent successfully",
		"count", len(eventList),
		"received_count", resp.ReceivedCount,
	)

	return nil
}

// SendEvent sends a single event (convenience method)
func (es *EventSender) SendEvent(ctx context.Context, event *events.Event) error {
	client := es.client.GetEventClient()
	if client == nil {
		return fmt.Errorf("gRPC event client not available")
	}

	protoEvent := es.convertEventToProto(event)
	req := &edge.SendEventRequest{
		Event: protoEvent,
	}

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.SendEvent(sendCtx, req)
	if err != nil {
		if es.isRetryableError(err) {
			return fmt.Errorf("retryable error sending event: %w", err)
		}
		return fmt.Errorf("failed to send event: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("KVM VM rejected event: %s", resp.ErrorMessage)
	}

	es.logger.Debug("Event sent successfully", "event_id", event.ID)
	return nil
}

// convertEventToProto converts an internal Event to proto Event
func (es *EventSender) convertEventToProto(event *events.Event) *edge.Event {
	protoEvent := &edge.Event{
		Id:           event.ID,
		CameraId:     event.CameraID,
		EventType:    event.EventType,
		Timestamp:    event.Timestamp.UnixNano(),
		Confidence:   event.Confidence,
		ClipPath:     event.ClipPath,
		SnapshotPath: event.SnapshotPath,
	}

	// Convert bounding box if present
	if event.BoundingBox != nil {
		protoEvent.BoundingBox = &edge.BoundingBox{
			X1:        event.BoundingBox.X1,
			Y1:        event.BoundingBox.Y1,
			X2:        event.BoundingBox.X2,
			Y2:        event.BoundingBox.Y2,
			Confidence: event.BoundingBox.Confidence,
			ClassId:    int32(event.BoundingBox.ClassID),
			ClassName:  event.BoundingBox.ClassName,
		}
	}

	// Convert metadata (proto only supports string values)
	protoEvent.Metadata = make(map[string]string)
	for k, v := range event.Metadata {
		protoEvent.Metadata[k] = fmt.Sprintf("%v", v)
	}

	return protoEvent
}

// isRetryableError checks if an error is retryable
func (es *EventSender) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	st, ok := status.FromError(err)
	if !ok {
		// Unknown error, assume retryable
		return true
	}

	// Retryable gRPC status codes
	retryableCodes := []codes.Code{
		codes.Unavailable,
		codes.DeadlineExceeded,
		codes.ResourceExhausted,
		codes.Aborted,
		codes.Internal,
	}

	for _, code := range retryableCodes {
		if st.Code() == code {
			return true
		}
	}

	return false
}

