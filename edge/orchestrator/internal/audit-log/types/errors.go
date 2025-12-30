package types

import "errors"

// Sentinel errors for common Audit Log error conditions.
// These errors can be checked using errors.Is() for programmatic error handling.
var (
	// ErrNotInitialized indicates that the service or a required component is not initialized.
	// This typically occurs when trying to use the audit log service before it has been properly started
	// or when a required component (e.g., storage provider) is nil.
	ErrNotInitialized = errors.New("audit-log: service not initialized")

	// ErrAlreadyStarted indicates that an operation was attempted on a service that is already started.
	// This prevents double-starting the audit log service.
	ErrAlreadyStarted = errors.New("audit-log: service already started")

	// ErrQueueFull indicates that the sync queue is full (reached max capacity of 100,000 records).
	// When the queue is full, sensitive operations should be paused until sync resumes.
	// Audit records must NEVER be dropped, even if queue is full.
	ErrQueueFull = errors.New("audit-log: sync queue is full")

	// ErrSyncFailed indicates that syncing audit logs to VM failed.
	// This error is returned when a sync operation fails and entries remain in the queue for retry.
	ErrSyncFailed = errors.New("audit-log: sync to VM failed")

	// ErrTamperDetected indicates that hash chain integrity verification detected tampering.
	// This is a critical security error that requires immediate attention.
	ErrTamperDetected = errors.New("audit-log: tamper detected in hash chain")

	// ErrRecordNotFound indicates that a requested record was not found in storage.
	// This is used when loading entries or queue items that don't exist.
	ErrRecordNotFound = errors.New("audit-log: record not found")
)

