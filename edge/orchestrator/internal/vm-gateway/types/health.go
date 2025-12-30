package types

import "time"

// HealthReporter provides a typed interface for health metrics reporting.
// Implementations should return typed structs instead of interface{} to avoid
// brittle map parsing and type assertions.
type HealthReporter interface {
	// GetCertificateRotationStatus returns the certificate rotation status.
	// Returns nil if certificate rotation is not supported or not initialized.
	GetCertificateRotationStatus() *CertificateRotationStatus

	// GetTimeSyncStatus returns the time synchronization status.
	// Returns nil if time sync checking is not supported or not initialized.
	GetTimeSyncStatus() *TimeSyncStatus

	// GetRateLimitStats returns rate limiting statistics.
	// Returns nil if rate limiting is not supported or not enabled.
	GetRateLimitStats() *RateLimitStats
}

// CertificateRotationStatus represents the status of certificate rotation.
// This type is shared between vm-gateway and transport service implementations.
type CertificateRotationStatus struct {
	// Status is the current rotation status: "idle", "scheduled", "in_progress", "completed", "failed"
	Status string `json:"status"`

	// ScheduledAt is when the rotation is scheduled (if scheduled)
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`

	// GracePeriodEnd is when the grace period ends (if in progress or completed)
	GracePeriodEnd *time.Time `json:"grace_period_end,omitempty"`

	// OldCAFingerprint is the fingerprint of the old CA certificate
	OldCAFingerprint string `json:"old_ca_fingerprint,omitempty"`

	// NewCAFingerprint is the fingerprint of the new CA certificate
	NewCAFingerprint string `json:"new_ca_fingerprint,omitempty"`
}

// TimeSyncStatus represents the status of time synchronization.
// This type is shared between vm-gateway and transport service implementations.
type TimeSyncStatus struct {
	// Status is the current sync status: "synced", "drift_warning", "drift_critical"
	Status string `json:"status"`

	// LastCheckTime is when the last time sync check was performed
	LastCheckTime *time.Time `json:"last_check_time,omitempty"`

	// DriftMinutes is the current clock drift in minutes (positive = Edge ahead, negative = Edge behind)
	DriftMinutes float64 `json:"drift_minutes,omitempty"`

	// ToleranceMinutes is the tolerance threshold in minutes
	ToleranceMinutes float64 `json:"tolerance_minutes,omitempty"`

	// CriticalDriftMinutes is the critical drift threshold in minutes
	CriticalDriftMinutes float64 `json:"critical_drift_minutes,omitempty"`
}

// RateLimitStats represents rate limiting statistics.
// This type is shared between vm-gateway and transport service implementations.
type RateLimitStats struct {
	// Enabled indicates whether rate limiting is enabled
	Enabled bool `json:"enabled"`

	// RequestsPerMinute is the configured requests per minute limit
	RequestsPerMinute int `json:"requests_per_minute,omitempty"`

	// BurstSize is the configured burst size
	BurstSize int `json:"burst_size,omitempty"`

	// TotalViolations is the total number of rate limit violations
	TotalViolations int64 `json:"total_violations,omitempty"`

	// ActiveBuckets is the number of active rate limit buckets (per client/endpoint)
	ActiveBuckets int `json:"active_buckets,omitempty"`
}

