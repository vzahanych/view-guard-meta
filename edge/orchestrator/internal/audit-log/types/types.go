package types

import (
	"time"
)

// DeviceID is a type alias for device identifiers.
// This replaces CameraID to support device-agnostic architecture.
type DeviceID string

// DeviceType represents the type of device.
type DeviceType string

const (
	// DeviceTypeCamera represents a camera device
	DeviceTypeCamera DeviceType = "camera"
	// DeviceTypeSensor represents a sensor device
	DeviceTypeSensor DeviceType = "sensor"
	// DeviceTypeAudioDevice represents an audio device
	DeviceTypeAudioDevice DeviceType = "audio_device"
	// DeviceTypeOther represents other types of IoT devices
	DeviceTypeOther DeviceType = "other"
)

// String returns the string representation of DeviceType
func (dt DeviceType) String() string {
	return string(dt)
}

// ResourceType represents the type of resource being accessed.
// This is device-agnostic and supports various resource types.
type ResourceType string

const (
	// ResourceTypeDataUnit represents a data unit (image, sensor reading, audio sample, etc.)
	ResourceTypeDataUnit ResourceType = "data_unit"
	// ResourceTypeVideoClip represents a video clip
	ResourceTypeVideoClip ResourceType = "video_clip"
	// ResourceTypeDevice represents a device
	ResourceTypeDevice ResourceType = "device"
	// ResourceTypeSecurityEvent represents a security event
	ResourceTypeSecurityEvent ResourceType = "security_event"
	// ResourceTypeModel represents a model
	ResourceTypeModel ResourceType = "model"
	// ResourceTypeDataset represents a dataset
	ResourceTypeDataset ResourceType = "dataset"
)

// String returns the string representation of ResourceType
func (rt ResourceType) String() string {
	return string(rt)
}

// AuditEntryType represents the type of audit log entry
type AuditEntryType string

const (
	EntryTypeDataAccess          AuditEntryType = "data_access"
	EntryTypeAuthentication      AuditEntryType = "authentication"
	EntryTypeAuthorization        AuditEntryType = "authorization"
	EntryTypeConfigurationChange AuditEntryType = "configuration_change"
	EntryTypeModelDeployment    AuditEntryType = "model_deployment"
	EntryTypeSecurityEvent        AuditEntryType = "security_event"
	EntryTypeDatasetLifecycle   AuditEntryType = "dataset_lifecycle"
	EntryTypeRecoveryAction     AuditEntryType = "recovery_action"
)

// AuditEntry is the base structure for all audit log entries.
// All entry types (DataAccessEntry, AuthenticationEntry, etc.) embed this structure.
//
// Hash Chain Integrity:
//   - Hash: SHA256 hash of the entry (calculated as SHA256(previousHash:entryJSON))
//   - PreviousHash: Hash of the previous entry in the chain
//   - These fields are automatically populated by the service when logging entries
//   - Hash chain creates tamper-evident audit trail: any modification breaks the chain
//
// Fields:
//   - ID: Unique entry identifier (UUID, automatically generated)
//   - Type: Entry type (data_access, authentication, authorization, etc.)
//   - Timestamp: When the action occurred (automatically set to current time)
//   - EdgeID: Edge device identifier
//   - UserID: User who performed the action (optional, may be empty for system actions)
//   - IPAddress: Source IP address (optional, for API requests)
//   - UserAgent: User agent string (optional, for web requests)
//   - Result: Operation result ("success", "failure", "denied")
//   - Error: Error message (optional, set when result is "failure")
//   - PreviousHash: Hash of previous entry (automatically set, empty for first entry)
//   - Hash: Hash of this entry (automatically calculated and set)
//
// Usage:
//   When creating entries, only set the fields relevant to the entry type.
//   The service will automatically populate ID, Timestamp, EdgeID, Hash, and PreviousHash.
type AuditEntry struct {
	ID          string         `json:"id"`                      // Unique entry identifier (UUID, automatically generated)
	Type        AuditEntryType `json:"type"`                    // Entry type (data_access, authentication, etc.)
	Timestamp   time.Time      `json:"timestamp"`               // When the action occurred (automatically set to current time)
	EdgeID      string         `json:"edge_id"`                 // Edge device identifier
	UserID      string         `json:"user_id,omitempty"`       // User who performed the action (optional, may be empty for system actions)
	IPAddress   string         `json:"ip_address,omitempty"`    // Source IP address (optional, for API requests)
	UserAgent   string         `json:"user_agent,omitempty"`    // User agent string (optional, for web requests)
	Result      string         `json:"result"`                  // Operation result: "success", "failure", "denied"
	Error       string         `json:"error,omitempty"`         // Error message (optional, set when result is "failure")
	PreviousHash string        `json:"previous_hash,omitempty"` // Hash of previous entry (for tamper-proofing, automatically set)
	Hash         string        `json:"hash"`                    // Hash of this entry (for tamper-proofing, automatically calculated)
}

// DataAccessEntry logs data access operations (reads, writes, deletions).
// This entry type is device-agnostic and supports access to various resource types.
//
// Resource Types:
//   - ResourceTypeDataUnit: Image, sensor reading, audio sample, etc.
//   - ResourceTypeVideoClip: Video clip
//   - ResourceTypeDevice: Device metadata
//   - ResourceTypeSecurityEvent: Security event
//   - ResourceTypeModel: ML model
//   - ResourceTypeDataset: Dataset
//
// Actions:
//   - "read": Reading/accessing a resource
//   - "write": Creating or updating a resource
//   - "delete": Deleting a resource
//   - "list": Listing resources
//
// Usage Example:
//   entry := types.DataAccessEntry{
//       AuditEntry: types.AuditEntry{
//           UserID:   "user-123",
//           IPAddress: "192.168.1.100",
//           Result:   "success",
//       },
//       ResourceType: types.ResourceTypeDataUnit,
//       ResourceID:   "screenshot-001",
//       Action:       "read",
//       DeviceID:     types.DeviceID("camera-001"),
//       DeviceType:   types.DeviceTypeCamera,
//   }
//   err := auditLogService.LogDataAccess(ctx, entry)
type DataAccessEntry struct {
	AuditEntry
	ResourceType string                 `json:"resource_type"` // Device-agnostic: "data_unit", "video_clip", "device", "security_event", "model", "dataset"
	ResourceID   string                 `json:"resource_id"`   // Resource identifier (e.g., DeviceID, DataUnitID, EventID)
	Action       string                 `json:"action"`        // Action performed: "read", "write", "delete", "list"
	DeviceID     DeviceID               `json:"device_id,omitempty"`   // Device associated with the resource (optional, device-agnostic)
	DeviceType   DeviceType             `json:"device_type,omitempty"` // Type of device (optional, device-agnostic)
	Metadata     map[string]interface{} `json:"metadata,omitempty"`    // Additional metadata about the access operation
}

// AuthenticationEntry logs authentication attempts.
//
// Methods:
//   - "api_key": API key authentication
//   - "token": Token-based authentication (JWT, OAuth, etc.)
//   - "certificate": Certificate-based authentication
//
// Usage Example:
//   entry := types.AuthenticationEntry{
//       AuditEntry: types.AuditEntry{
//           UserID:   "user-123",
//           IPAddress: "192.168.1.100",
//           Result:   "success",
//       },
//       Method:   "api_key",
//       Identity: "user-123",
//   }
//   err := auditLogService.LogAuthentication(ctx, entry)
type AuthenticationEntry struct {
	AuditEntry
	Method       string `json:"method"`             // Authentication method: "api_key", "token", "certificate"
	Identity     string `json:"identity"`           // User/device identifier (username, email, device ID, etc.)
	SessionID    string `json:"session_id,omitempty"` // Session identifier (optional, for session-based auth)
}

// AuthorizationEntry logs authorization decisions (access granted or denied).
//
// Usage Example:
//   entry := types.AuthorizationEntry{
//       AuditEntry: types.AuditEntry{
//           UserID:   "user-123",
//           IPAddress: "192.168.1.100",
//           Result:   "success",
//       },
//       Resource:   "/api/v1/cameras",
//       Action:     "list",
//       Permission: "cameras:read",
//       Granted:    true,
//   }
//   err := auditLogService.LogAuthorization(ctx, entry)
type AuthorizationEntry struct {
	AuditEntry
	Resource     string `json:"resource"`    // Resource being accessed (e.g., "/api/v1/cameras", "camera-001")
	Action       string `json:"action"`      // Action attempted (e.g., "read", "write", "delete")
	Permission   string `json:"permission"`  // Required permission (e.g., "cameras:read", "cameras:write")
	Granted     bool   `json:"granted"`      // Whether access was granted (true) or denied (false)
}

// ConfigurationChangeEntry logs configuration changes.
//
// Usage Example:
//   entry := types.ConfigurationChangeEntry{
//       AuditEntry: types.AuditEntry{
//           UserID:   "admin-001",
//           Result:   "success",
//       },
//       ConfigSection: "audit_log",
//       Field:         "retention_days",
//       OldValue:      7,
//       NewValue:      90,
//   }
//   err := auditLogService.LogConfigurationChange(ctx, entry)
type ConfigurationChangeEntry struct {
	AuditEntry
	ConfigSection string                 `json:"config_section"` // Section of config changed (e.g., "audit_log", "storage", "vm_gateway")
	Field         string                 `json:"field"`          // Field changed (e.g., "retention_days", "sync_interval")
	OldValue      interface{}            `json:"old_value,omitempty"` // Previous value (optional)
	NewValue      interface{}            `json:"new_value,omitempty"` // New value (optional)
	Metadata      map[string]interface{} `json:"metadata,omitempty"`  // Additional metadata about the change
}

// ModelDeploymentEntry logs model deployment operations.
// This entry type is device-agnostic and supports model deployment to any device type.
//
// Actions:
//   - "deploy": Deploying a model to a device
//   - "verify": Verifying model integrity (signature, hash, compatibility)
//   - "activate": Activating a deployed model
//   - "deactivate": Deactivating an active model
//   - "remove": Removing a model from a device
//
// Deployment Status:
//   - "deployed": Model successfully deployed
//   - "verification_failed": Model verification failed (signature, hash, compatibility)
//   - "activation_failed": Model activation failed
//
// Verification Results:
//   - May include: signature_valid, hash_match, compatibility_check, etc.
//
// Usage Example:
//   entry := types.ModelDeploymentEntry{
//       AuditEntry: types.AuditEntry{
//           UserID:   "operator-001",
//           Result:   "success",
//       },
//       ModelID:      "model-001",
//       ModelVersion: "v1.0.0",
//       DeviceID:     types.DeviceID("camera-001"),
//       DeviceType:   types.DeviceTypeCamera,
//       Action:       "deploy",
//       Checksum:     "sha256:abc123...",
//       VerificationResults: map[string]interface{}{
//           "signature_valid": true,
//           "hash_match":      true,
//       },
//       DeploymentStatus: "deployed",
//   }
//   err := auditLogService.LogModelDeployment(ctx, entry)
type ModelDeploymentEntry struct {
	AuditEntry
	ModelID             string                 `json:"model_id"`                       // Unique model identifier
	ModelVersion        string                 `json:"model_version"`                  // Model version (e.g., "v1.0.0")
	DeviceID            DeviceID               `json:"device_id,omitempty"`            // Device to which the model is deployed (device-agnostic)
	DeviceType          DeviceType             `json:"device_type,omitempty"`          // Type of device (camera, sensor, audio_device, etc.)
	Action              string                 `json:"action"`                         // Action: "deploy", "verify", "activate", "deactivate", "remove"
	Checksum            string                 `json:"checksum,omitempty"`             // Model file checksum (SHA256, optional)
	VerificationResults map[string]interface{} `json:"verification_results,omitempty"` // Results of signature, hash, compatibility checks (optional)
	DeploymentStatus    string                 `json:"deployment_status,omitempty"`    // Status: "deployed", "verification_failed", "activation_failed" (optional)
}

// SecurityEventEntry logs security-related events (intrusions, anomalies, threats, etc.).
//
// Severity Levels:
//   - "low": Low severity security event
//   - "medium": Medium severity security event
//   - "high": High severity security event
//   - "critical": Critical security event requiring immediate attention
//
// Usage Example:
//   entry := types.SecurityEventEntry{
//       AuditEntry: types.AuditEntry{
//           Result: "success",
//       },
//       EventType:   "intrusion_detected",
//       Severity:    "high",
//       Description: "Unauthorized access attempt detected",
//       Metadata: map[string]interface{}{
//           "source_ip": "192.168.1.200",
//           "attempts":  5,
//       },
//   }
//   err := auditLogService.LogSecurityEvent(ctx, entry)
type SecurityEventEntry struct {
	AuditEntry
	EventType    string                 `json:"event_type"`    // Type of security event (e.g., "intrusion_detected", "anomaly", "threat")
	Severity     string                 `json:"severity"`      // Severity level: "low", "medium", "high", "critical"
	Description  string                 `json:"description"`   // Human-readable description of the event
	Metadata     map[string]interface{} `json:"metadata,omitempty"` // Additional metadata about the security event
}

// DatasetLifecycleEntry logs dataset lifecycle operations (creation, labeling, upload, deletion).
// This entry type is device-agnostic and supports dataset operations for any device.
//
// Actions:
//   - "created": Dataset was created
//   - "labeled": Dataset was labeled (labels added/updated)
//   - "uploaded": Dataset was uploaded to VM
//   - "deleted": Dataset was deleted
//
// Usage Example:
//   entry := types.DatasetLifecycleEntry{
//       AuditEntry: types.AuditEntry{
//           UserID:   "user-123",
//           Result:   "success",
//       },
//       DatasetID:     "dataset-001",
//       Action:        "created",
//       DeviceID:      types.DeviceID("camera-001"),
//       DeviceType:    types.DeviceTypeCamera,
//       DataUnitCount: 1000,
//   }
//   err := auditLogService.LogDatasetLifecycle(ctx, entry)
type DatasetLifecycleEntry struct {
	AuditEntry
	DeviceID      DeviceID               `json:"device_id,omitempty"`       // Device associated with the dataset (optional, device-agnostic)
	DeviceType    DeviceType             `json:"device_type,omitempty"`     // Type of device (optional)
	DatasetID     string                 `json:"dataset_id"`                // Unique identifier for the dataset
	Action        string                 `json:"action"`                    // Action: "created", "labeled", "uploaded", "deleted"
	DataUnitCount int                    `json:"data_unit_count,omitempty"` // Number of data units in the dataset (optional)
	Metadata      map[string]interface{} `json:"metadata,omitempty"`        // Additional metadata about the dataset
}

// RecoveryActionEntry logs recovery actions taken by the system or operator.
// This entry type is device-agnostic and supports recovery actions on any device.
//
// Recovery Reasons:
//   - "storage_corruption": Storage corruption detected
//   - "integrity_failure": Hash chain integrity failure
//   - "operator_initiated": Operator-initiated recovery
//   - "system_initiated": System-initiated recovery
//
// Corrupted Resources:
//   - May include: "model", "dataset", "hash_chain", "storage", etc.
//
// Usage Example:
//   entry := types.RecoveryActionEntry{
//       AuditEntry: types.AuditEntry{
//           UserID:   "operator-001",
//           Result:   "failure",
//       },
//       RecoveryReason:     "integrity_failure",
//       CorruptedResources: []string{"model", "dataset"},
//       DeviceID:           types.DeviceID("camera-001"),
//       DeviceType:         types.DeviceTypeCamera,
//       LastKnownGoodState: map[string]interface{}{
//           "last_verified_hash": "sha256:xyz789...",
//       },
//   }
//   err := auditLogService.LogRecoveryAction(ctx, entry)
type RecoveryActionEntry struct {
	AuditEntry
	DeviceID           DeviceID               `json:"device_id,omitempty"`            // Device on which recovery action was taken (optional, device-agnostic)
	DeviceType         DeviceType             `json:"device_type,omitempty"`          // Type of device (optional)
	RecoveryReason     string                 `json:"recovery_reason"`                // Reason: "storage_corruption", "integrity_failure", "operator_initiated", "system_initiated"
	CorruptedResources []string               `json:"corrupted_resources,omitempty"`  // List of affected resources: "model", "dataset", "hash_chain", etc. (optional)
	LastKnownGoodState map[string]interface{} `json:"last_known_good_state,omitempty"` // Snapshot of last known good state (optional)
	VMResponseStatus   string                 `json:"vm_response_status,omitempty"`   // Status of VM response to recovery action (optional)
	Metadata           map[string]interface{} `json:"metadata,omitempty"`             // Additional metadata about the recovery action
}

// QueryFilters defines filters for querying audit logs.
//
// All filter fields are optional. If a field is zero/empty, it is not applied as a filter.
//
// Time Range:
//   - StartTime: Filter entries after this time (inclusive). If nil, no start time filter.
//   - EndTime: Filter entries before this time (inclusive). If nil, no end time filter.
//
// Entry Filtering:
//   - EntryType: Filter by entry type (must match exactly: "data_access", "authentication", etc.)
//   - UserID: Filter by user ID (exact match)
//   - IPAddress: Filter by IP address (exact match)
//   - Result: Filter by result (exact match: "success", "failure", "denied")
//   - ResourceType: Filter by resource type (for DataAccessEntry)
//   - ResourceID: Filter by resource ID (exact match)
//
// Pagination:
//   - Limit: Maximum number of entries to return (default: 100 if 0). Use 0 for no limit.
//   - Offset: Number of entries to skip (for pagination). Default: 0.
//
// Usage Example:
//   filters := types.QueryFilters{
//       StartTime:  timePtr(time.Now().Add(-24 * time.Hour)),
//       EndTime:    timePtr(time.Now()),
//       EntryType:  string(types.EntryTypeDataAccess),
//       UserID:     "user-123",
//       Result:     "success",
//       Limit:      100,
//       Offset:     0,
//   }
//   entries, err := auditLogService.QueryAuditLogs(ctx, filters)
type QueryFilters struct {
	StartTime    *time.Time // Filter entries after this time (inclusive, optional)
	EndTime      *time.Time // Filter entries before this time (inclusive, optional)
	EntryType    string     // Filter by entry type (optional, exact match)
	UserID       string     // Filter by user ID (optional, exact match)
	IPAddress    string     // Filter by IP address (optional, exact match)
	Result       string     // Filter by result (optional, exact match: "success", "failure", "denied")
	ResourceType string     // Filter by resource type (optional, for DataAccessEntry)
	ResourceID   string     // Filter by resource ID (optional, exact match)
	Limit        int        // Maximum number of entries to return (default: 100 if 0, 0 = no limit)
	Offset       int        // Number of entries to skip for pagination (default: 0)
}

// ExportFormat represents the format for audit log export
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCEF  ExportFormat = "cef"
)

// ExportEntry represents an audit log entry ready for export
type ExportEntry struct {
	Entry  interface{} // The original entry (DataAccessEntry, AuthenticationEntry, etc.)
	Format ExportFormat
	JSON   string // JSON representation
	CEF    string // CEF representation (if format is CEF)
}

