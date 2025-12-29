package types

import "errors"

// Sentinel errors for common Object Storage error conditions.
// These errors can be checked using errors.Is() for programmatic error handling.
var (
	// ErrNotInitialized indicates that the service or a required component is not initialized.
	// This typically occurs when trying to use the storage service before it has been properly started
	// or when a required component (e.g., storage provider) is nil.
	ErrNotInitialized = errors.New("object-storage: service not initialized")

	// ErrAlreadyStarted indicates that an operation was attempted on a service that is already started.
	// This prevents double-starting the storage service.
	ErrAlreadyStarted = errors.New("object-storage: service already started")

	// ErrQuotaExceeded indicates that a storage operation was rejected due to quota limits.
	// This occurs when storage usage exceeds the configured quota threshold (typically >95%).
	ErrQuotaExceeded = errors.New("object-storage: quota exceeded")

	// ErrObjectNotFound indicates that a requested object was not found in storage.
	// This is returned when attempting to get or delete a non-existent object.
	ErrObjectNotFound = errors.New("object-storage: object not found")

	// ErrCorruptionDetected indicates that storage corruption was detected during integrity checks.
	// This requires immediate attention and may require VM-assisted resync.
	ErrCorruptionDetected = errors.New("object-storage: corruption detected")
)

