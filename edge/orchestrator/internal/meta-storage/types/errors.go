package types

import "errors"

// Sentinel errors for common Meta Storage error conditions.
// These errors can be checked using errors.Is() for programmatic error handling.
var (
	// ErrNotInitialized indicates that the service or a required component is not initialized.
	// This typically occurs when trying to use the storage service before it has been properly started
	// or when a required component (e.g., storage provider) is nil.
	ErrNotInitialized = errors.New("meta-storage: service not initialized")

	// ErrAlreadyStarted indicates that an operation was attempted on a service that is already started.
	// This prevents double-starting the storage service.
	ErrAlreadyStarted = errors.New("meta-storage: service already started")

	// ErrQuotaExceeded indicates that a storage operation was rejected due to quota limits.
	// This occurs when storage usage exceeds the configured quota threshold (typically >95%).
	ErrQuotaExceeded = errors.New("meta-storage: quota exceeded")

	// ErrRecordNotFound indicates that a requested record was not found in storage.
	// This is returned when attempting to get, update, or delete a non-existent record.
	ErrRecordNotFound = errors.New("meta-storage: record not found")

	// ErrCorruptionDetected indicates that storage corruption was detected during integrity checks.
	// This requires immediate attention and may require VM-assisted resync.
	ErrCorruptionDetected = errors.New("meta-storage: corruption detected")

	// ErrInvalidSchemaVersion indicates that the storage schema version is invalid or incompatible.
	// This typically occurs when attempting to use a database with an incompatible schema version.
	ErrInvalidSchemaVersion = errors.New("meta-storage: invalid schema version")
)
