package statemng

import (
	"context"
)

// StateManager coordinates edge lifecycle using the application event bus and meta-storage.
// It:
//   - Listens for edge-related events from other components
//   - Updates edge state in meta-storage
//   - Computes the next edge task based on the new state
//   - Executes that task asynchronously in a goroutine
type StateManager interface {
	// Start begins processing events from the event bus.
	Start(ctx context.Context) error

	// Stop stops processing events and waits for in-flight tasks to finish.
	Stop(ctx context.Context) error

	// Name returns the service name for identification and logging.
	Name() string
}


