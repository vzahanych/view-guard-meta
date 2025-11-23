package service

import (
	"context"
	"testing"
	"time"
)

func TestNewEventBus(t *testing.T) {
	bus := NewEventBus(100)
	if bus == nil {
		t.Fatal("NewEventBus returned nil")
	}

	bus2 := NewEventBus(0)
	if bus2 == nil {
		t.Fatal("NewEventBus with 0 buffer should use default")
	}
}

func TestEventBus_Subscribe(t *testing.T) {
	bus := NewEventBus(10)

	ch := bus.Subscribe(EventTypeCameraDiscovered)
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}

	event := Event{
		Type:   EventTypeCameraDiscovered,
		Source: "test",
		Data:   map[string]interface{}{"camera_id": "cam-1"},
	}

	bus.Publish(event)

	select {
	case received := <-ch:
		if received.Type != EventTypeCameraDiscovered {
			t.Errorf("Expected event type %s, got %s", EventTypeCameraDiscovered, received.Type)
		}
		if received.Source != "test" {
			t.Errorf("Expected source 'test', got %s", received.Source)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Event not received within timeout")
	}
}

func TestEventBus_SubscribeAll(t *testing.T) {
	bus := NewEventBus(10)

	// Subscribe to specific event types first to populate subscribers map
	bus.Subscribe(EventTypeCameraDiscovered)
	bus.Subscribe(EventTypeServiceStarted)

	ch := bus.SubscribeAll()
	if ch == nil {
		t.Fatal("SubscribeAll returned nil channel")
	}

	event1 := Event{
		Type:   EventTypeCameraDiscovered,
		Source: "test",
		Data:   map[string]interface{}{"camera_id": "cam-1"},
	}

	event2 := Event{
		Type:   EventTypeServiceStarted,
		Source: "manager",
		Data:   map[string]interface{}{"service": "test"},
	}

	bus.Publish(event1)
	bus.Publish(event2)

	receivedCount := 0
	timeout := time.After(1 * time.Second)

	for receivedCount < 2 {
		select {
		case <-ch:
			receivedCount++
		case <-timeout:
			t.Fatalf("Expected 2 events, received %d", receivedCount)
		}
	}
}

func TestEventBus_Publish(t *testing.T) {
	bus := NewEventBus(10)

	ch1 := bus.Subscribe(EventTypeCameraDiscovered)
	ch2 := bus.Subscribe(EventTypeCameraDiscovered)

	event := Event{
		Type:   EventTypeCameraDiscovered,
		Source: "test",
		Data:   map[string]interface{}{"camera_id": "cam-1"},
	}

	bus.Publish(event)

	select {
	case received := <-ch1:
		if received.Type != EventTypeCameraDiscovered {
			t.Errorf("Channel 1: Expected event type %s, got %s", EventTypeCameraDiscovered, received.Type)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Event not received on channel 1")
	}

	select {
	case received := <-ch2:
		if received.Type != EventTypeCameraDiscovered {
			t.Errorf("Channel 2: Expected event type %s, got %s", EventTypeCameraDiscovered, received.Type)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Event not received on channel 2")
	}
}

func TestEventBus_Publish_Timestamp(t *testing.T) {
	bus := NewEventBus(10)
	ch := bus.Subscribe(EventTypeCameraDiscovered)

	event := Event{
		Type:   EventTypeCameraDiscovered,
		Source: "test",
		Data:   map[string]interface{}{"camera_id": "cam-1"},
	}

	beforePublish := time.Now()
	bus.Publish(event)
	afterPublish := time.Now()

	select {
	case received := <-ch:
		if received.Timestamp.IsZero() {
			t.Error("Event timestamp should be set")
		}
		if received.Timestamp.Before(beforePublish) || received.Timestamp.After(afterPublish) {
			t.Error("Event timestamp should be between before and after publish time")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Event not received")
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus(10)

	ch := bus.Subscribe(EventTypeCameraDiscovered)

	event := Event{
		Type:   EventTypeCameraDiscovered,
		Source: "test",
		Data:   map[string]interface{}{"camera_id": "cam-1"},
	}

	bus.Publish(event)

	select {
	case <-ch:
	case <-time.After(1 * time.Second):
		t.Fatal("Event not received before unsubscribe")
	}

	bus.Unsubscribe(EventTypeCameraDiscovered, ch)

	// Give unsubscribe time to process
	time.Sleep(10 * time.Millisecond)

	bus.Publish(event)

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Should not receive event after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		// Channel closed, which is expected
	}
}

func TestEventBus_Close(t *testing.T) {
	bus := NewEventBus(10)

	ch1 := bus.Subscribe(EventTypeCameraDiscovered)
	ch2 := bus.Subscribe(EventTypeServiceStarted)

	bus.Close()

	select {
	case _, ok := <-ch1:
		if ok {
			t.Error("Channel 1 should be closed")
		}
	default:
		t.Error("Channel 1 should be closed")
	}

	select {
	case _, ok := <-ch2:
		if ok {
			t.Error("Channel 2 should be closed")
		}
	default:
		t.Error("Channel 2 should be closed")
	}
}

func TestEventBus_SubscribeWithHandler(t *testing.T) {
	bus := NewEventBus(10)

	receivedEvents := make([]Event, 0)
	handler := func(ctx context.Context, event Event) error {
		receivedEvents = append(receivedEvents, event)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus.SubscribeWithHandler(ctx, EventTypeCameraDiscovered, handler)

	event1 := Event{
		Type:   EventTypeCameraDiscovered,
		Source: "test",
		Data:   map[string]interface{}{"camera_id": "cam-1"},
	}

	event2 := Event{
		Type:   EventTypeCameraDiscovered,
		Source: "test",
		Data:   map[string]interface{}{"camera_id": "cam-2"},
	}

	bus.Publish(event1)
	bus.Publish(event2)

	time.Sleep(100 * time.Millisecond)

	if len(receivedEvents) != 2 {
		t.Errorf("Expected 2 events, got %d", len(receivedEvents))
	}

	if receivedEvents[0].Data["camera_id"] != "cam-1" {
		t.Errorf("Expected first event camera_id 'cam-1', got %v", receivedEvents[0].Data["camera_id"])
	}

	if receivedEvents[1].Data["camera_id"] != "cam-2" {
		t.Errorf("Expected second event camera_id 'cam-2', got %v", receivedEvents[1].Data["camera_id"])
	}
}

func TestEventBus_Publish_NonBlocking(t *testing.T) {
	bus := NewEventBus(1)

	ch := bus.Subscribe(EventTypeCameraDiscovered)

	event1 := Event{Type: EventTypeCameraDiscovered, Source: "test", Data: map[string]interface{}{"id": "1"}}
	event2 := Event{Type: EventTypeCameraDiscovered, Source: "test", Data: map[string]interface{}{"id": "2"}}
	event3 := Event{Type: EventTypeCameraDiscovered, Source: "test", Data: map[string]interface{}{"id": "3"}}

	bus.Publish(event1)
	bus.Publish(event2)
	bus.Publish(event3)

	time.Sleep(50 * time.Millisecond)

	receivedCount := 0
	timeout := time.After(100 * time.Millisecond)

	for {
		select {
		case <-ch:
			receivedCount++
		case <-timeout:
			goto done
		}
	}

done:
	if receivedCount == 0 {
		t.Error("Should receive at least one event")
	}
}

