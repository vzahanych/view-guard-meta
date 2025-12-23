package impl

import (
	"time"
)

// EventState represents an event's persisted state (internal to state-mng)
// This replaces state.EventState from the deprecated @state package
type EventState struct {
	ID           string
	CameraID     string
	EventType    string
	Timestamp    time.Time
	Metadata     map[string]interface{}
	ClipPath     string
	SnapshotPath string
	Transmitted  bool
}

// QueueEntry represents an entry in the event queue
type queueEntry struct {
	EventID    string    `json:"event_id"`
	Priority   int       `json:"priority"`
	RetryCount int       `json:"retry_count"`
	CreatedAt  time.Time `json:"created_at"`
}

