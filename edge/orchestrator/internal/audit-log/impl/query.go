package impl

import (
	"context"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"go.uber.org/zap"
)

// QueryAuditLogsFromStorage queries audit logs from object storage with filters.
// DEPRECATED: This function is no longer supported. Use meta-storage provider instead.
// This function always returns an error since object-storage fallback is not supported.
func QueryAuditLogsFromStorage(
	ctx context.Context,
	objectStorage objectstorage.ObjectStorageService,
	logger *zap.Logger,
	filters types.QueryFilters,
) ([]interface{}, error) {
	// Object-storage fallback is no longer supported - meta-storage provider is required
	return nil, fmt.Errorf("object-storage fallback is not supported - meta-storage provider is required")
}

// GetAuditLogEntryFromStorage retrieves a specific audit log entry by ID
func GetAuditLogEntryFromStorage(
	ctx context.Context,
	objectStorage objectstorage.ObjectStorageService,
	entryID string,
	timestamp time.Time,
) (interface{}, error) {
	// Object-storage fallback is no longer supported - meta-storage provider is required
	return nil, fmt.Errorf("object-storage fallback is not supported - meta-storage provider is required")
}

// matchesFilters checks if an entry matches the query filters
func matchesFilters(entry interface{}, filters types.QueryFilters) bool {
	var baseEntry types.AuditEntry

	// Extract base entry
	switch e := entry.(type) {
	case types.DataAccessEntry:
		baseEntry = e.AuditEntry
		if filters.ResourceType != "" && e.ResourceType != filters.ResourceType {
			return false
		}
		if filters.ResourceID != "" && e.ResourceID != filters.ResourceID {
			return false
		}
	case types.AuthenticationEntry:
		baseEntry = e.AuditEntry
	case types.AuthorizationEntry:
		baseEntry = e.AuditEntry
	case types.ConfigurationChangeEntry:
		baseEntry = e.AuditEntry
	case types.ModelDeploymentEntry:
		baseEntry = e.AuditEntry
	case types.SecurityEventEntry:
		baseEntry = e.AuditEntry
	case types.DatasetLifecycleEntry:
		baseEntry = e.AuditEntry
	case types.RecoveryActionEntry:
		baseEntry = e.AuditEntry
	default:
		return false
	}

	// Check filters
	if filters.StartTime != nil && baseEntry.Timestamp.Before(*filters.StartTime) {
		return false
	}
	if filters.EndTime != nil && baseEntry.Timestamp.After(*filters.EndTime) {
		return false
	}
	if filters.EntryType != "" && string(baseEntry.Type) != filters.EntryType {
		return false
	}
	if filters.UserID != "" && baseEntry.UserID != filters.UserID {
		return false
	}
	if filters.IPAddress != "" && baseEntry.IPAddress != filters.IPAddress {
		return false
	}
	if filters.Result != "" && baseEntry.Result != filters.Result {
		return false
	}

	return true
}
