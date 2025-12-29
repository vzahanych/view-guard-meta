package types

import "errors"

// Sentinel errors for event-bus package.
// These errors follow the vm-gateway pattern for programmatic error handling.
//
// Use errors.Is() to check for these errors:
//   if errors.Is(err, types.ErrStoragePressure) {
//       // Handle storage pressure
//   }
//
// These errors are also re-exported from the eventbus package for convenience.

var (
	// ErrNotInitialized indicates that the event bus service has not been started.
	// This error is returned when operations are attempted before Start() is called.
	//
	// Operations that return this error:
	//   - EventBus.Publish() (if called before Start())
	//   - EventBus.Subscribe() (if called before Start())
	//
	// Resolution: Call EventBus.Start() before using the event bus.
	//
	// Example:
	//   if err := bus.Publish(event); err != nil {
	//       if errors.Is(err, types.ErrNotInitialized) {
	//           // Event bus not started yet
	//       }
	//   }
	ErrNotInitialized = errors.New("event bus not initialized")

	// ErrAlreadyStarted indicates that the event bus service has already been started.
	// This error is returned when Start() is called multiple times.
	//
	// Resolution: Start() is idempotent - don't call it multiple times.
	// Use lifecycle management (Fx) to ensure Start() is called only once.
	//
	// Example:
	//   if err := bus.Start(ctx); err != nil {
	//       if errors.Is(err, types.ErrAlreadyStarted) {
	//           // Already started, ignore
	//       }
	//   }
	ErrAlreadyStarted = errors.New("event bus already started")

	// ErrStoragePressure indicates that storage is >90% full and events are being dropped.
	// This error is returned when storage pressure is detected and droppable events are dropped.
	//
	// This error is returned for droppable events (EventCategoryWorkflowTrigger) when:
	//   - Storage usage > configured threshold (default: 90%)
	//   - Event is dropped immediately without persistence attempt
	//
	// This is expected behavior for workflow trigger events during storage pressure.
	// Non-droppable events will return ErrEventDropped instead.
	//
	// Resolution:
	//   - Increase storage capacity
	//   - Adjust retention policy (shorter retention)
	//   - Wait for cleanup to free space
	//
	// Example:
	//   if err := bus.Publish(event); err != nil {
	//       if errors.Is(err, types.ErrStoragePressure) {
	//           // Event was dropped (expected for workflow triggers)
	//           // Reconciliation will catch this event later
	//       }
	//   }
	ErrStoragePressure = errors.New("storage pressure: storage >90% full, dropping events")

	// ErrEventDropped indicates that a non-droppable event was dropped due to storage pressure.
	// This error is returned when a critical or operational event cannot be persisted.
	//
	// This is a critical error that should never happen in normal operation.
	// This error is returned when:
	//   - Storage usage > configured threshold (default: 90%)
	//   - Event is non-droppable (EventCategoryOperationalHealth or EventCategoryCritical)
	//   - Persistence attempt fails (storage completely full or other error)
	//
	// Resolution:
	//   - This is a critical error requiring immediate attention
	//   - Increase storage capacity immediately
	//   - Check storage provider health
	//   - Review event drop policy configuration
	//
	// Example:
	//   if err := bus.Publish(event); err != nil {
	//       if errors.Is(err, types.ErrEventDropped) {
	//           // CRITICAL: Non-droppable event was dropped
	//           // This should never happen - immediate action required
	//       }
	//   }
	ErrEventDropped = errors.New("event dropped: non-droppable event could not be persisted")
)

