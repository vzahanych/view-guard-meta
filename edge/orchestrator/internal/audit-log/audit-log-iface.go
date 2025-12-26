package auditlog

import (
	"context"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
)

// AuditLogService provides tamper-proof audit logging for security-sensitive operations.
// Audit logs are stored temporarily in edge object storage and synced to VM for long-term retention.

//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_audit_log.go -package=mocks
type AuditLogService interface {
	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Name() string

	// Log security-sensitive operations
	LogDataAccess(ctx context.Context, entry types.DataAccessEntry) error
	LogAuthentication(ctx context.Context, entry types.AuthenticationEntry) error
	LogAuthorization(ctx context.Context, entry types.AuthorizationEntry) error
	LogConfigurationChange(ctx context.Context, entry types.ConfigurationChangeEntry) error
	LogModelDeployment(ctx context.Context, entry types.ModelDeploymentEntry) error
	LogSecurityEvent(ctx context.Context, entry types.SecurityEventEntry) error

	// Sync audit logs to VM (should be called periodically)
	SyncToVM(ctx context.Context) error

	// Cleanup old audit logs (based on retention period)
	CleanupOldLogs(ctx context.Context) error

	// Query audit logs with filters
	QueryAuditLogs(ctx context.Context, filters types.QueryFilters) ([]interface{}, error)

	// GetAuditLogEntry retrieves a specific audit log entry by ID
	GetAuditLogEntry(ctx context.Context, entryID string) (interface{}, error)
}
