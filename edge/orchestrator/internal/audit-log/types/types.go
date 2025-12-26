package types

import (
	"time"
)

// AuditEntryType represents the type of audit log entry
type AuditEntryType string

const (
	EntryTypeDataAccess          AuditEntryType = "data_access"
	EntryTypeAuthentication      AuditEntryType = "authentication"
	EntryTypeAuthorization        AuditEntryType = "authorization"
	EntryTypeConfigurationChange AuditEntryType = "configuration_change"
	EntryTypeModelDeployment    AuditEntryType = "model_deployment"
	EntryTypeSecurityEvent        AuditEntryType = "security_event"
)

// AuditEntry is the base structure for all audit log entries
type AuditEntry struct {
	ID          string         `json:"id"`
	Type        AuditEntryType `json:"type"`
	Timestamp   time.Time      `json:"timestamp"`
	EdgeID      string         `json:"edge_id"`
	UserID      string         `json:"user_id,omitempty"`      // User who performed the action (if applicable)
	IPAddress   string         `json:"ip_address,omitempty"`   // Source IP address
	UserAgent   string         `json:"user_agent,omitempty"`    // User agent (for web requests)
	Result      string         `json:"result"`                 // "success", "failure", "denied"
	Error       string         `json:"error,omitempty"`        // Error message if result is "failure"
	PreviousHash string        `json:"previous_hash,omitempty"` // Hash of previous entry (for tamper-proofing)
	Hash         string        `json:"hash"`                    // Hash of this entry (for tamper-proofing)
}

// DataAccessEntry logs data access operations (reads, writes, deletions)
type DataAccessEntry struct {
	AuditEntry
	ResourceType string                 `json:"resource_type"` // "screenshot", "clip", "camera", "event", etc.
	ResourceID   string                 `json:"resource_id"`
	Action       string                 `json:"action"` // "read", "write", "delete", "list"
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// AuthenticationEntry logs authentication attempts
type AuthenticationEntry struct {
	AuditEntry
	Method       string `json:"method"`        // "api_key", "token", "certificate"
	Identity     string `json:"identity"`     // User/device identifier
	SessionID    string `json:"session_id,omitempty"`
}

// AuthorizationEntry logs authorization decisions
type AuthorizationEntry struct {
	AuditEntry
	Resource     string `json:"resource"`      // Resource being accessed
	Action       string `json:"action"`       // Action attempted
	Permission   string `json:"permission"`   // Required permission
	Granted     bool   `json:"granted"`      // Whether access was granted
}

// ConfigurationChangeEntry logs configuration changes
type ConfigurationChangeEntry struct {
	AuditEntry
	ConfigSection string                 `json:"config_section"` // Section of config changed
	Field         string                 `json:"field"`          // Field changed
	OldValue      interface{}            `json:"old_value,omitempty"`
	NewValue      interface{}            `json:"new_value,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// ModelDeploymentEntry logs model deployment operations
type ModelDeploymentEntry struct {
	AuditEntry
	ModelID      string `json:"model_id"`
	ModelVersion string `json:"model_version"`
	CameraID     string `json:"camera_id,omitempty"` // If model is camera-specific
	Action       string `json:"action"`             // "deploy", "update", "remove"
	Checksum     string `json:"checksum,omitempty"` // Model file checksum
}

// SecurityEventEntry logs security-related events
type SecurityEventEntry struct {
	AuditEntry
	EventType    string                 `json:"event_type"`    // Type of security event
	Severity     string                 `json:"severity"`     // "low", "medium", "high", "critical"
	Description  string                 `json:"description"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// AuditLogConfig contains configuration for audit logging
type AuditLogConfig struct {
	// Retention period for audit logs in edge storage (before syncing to VM)
	RetentionDays int `yaml:"retention_days"` // Default: 7 days (1 week)
	
	// Sync interval for syncing audit logs to VM
	SyncInterval time.Duration `yaml:"sync_interval"` // Default: 1 hour
	
	// Enable audit logging
	Enabled bool `yaml:"enabled"` // Default: true
}

// QueryFilters defines filters for querying audit logs
type QueryFilters struct {
	StartTime    *time.Time
	EndTime      *time.Time
	EntryType    string
	UserID       string
	IPAddress    string
	Result       string
	ResourceType string
	ResourceID   string
	Limit        int
	Offset       int
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

