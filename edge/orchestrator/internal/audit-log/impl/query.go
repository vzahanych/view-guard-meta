package impl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"go.uber.org/zap"
)

// QueryAuditLogsFromStorage queries audit logs from object storage with filters
func QueryAuditLogsFromStorage(
	ctx context.Context,
	objectStorage objectstorage.ObjectStorageService,
	logger *zap.Logger,
	filters types.QueryFilters,
) ([]interface{}, error) {
	// Determine date range for query
	startDate := time.Now().AddDate(0, 0, -7) // Default: last 7 days
	endDate := time.Now()

	if filters.StartTime != nil {
		startDate = *filters.StartTime
	}
	if filters.EndTime != nil {
		endDate = *filters.EndTime
	}

	// Query entries day by day
	entries := make([]interface{}, 0)
	currentDate := startDate

	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		dateStr := currentDate.Format("2006-01-02")
		prefix := fmt.Sprintf("audit-logs/%s/", dateStr)

		// Note: Object storage doesn't have a List operation in the interface
		// This is a placeholder - in a real implementation, we'd need to:
		// 1. Add a List operation to object storage interface, or
		// 2. Maintain an index of audit log entries in meta-storage, or
		// 3. Use a different storage mechanism for querying

		// For now, return empty results with a log message
		logger.Debug("Querying audit logs",
			zap.String("date", dateStr),
			zap.String("prefix", prefix))

		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Apply filters
	filteredEntries := make([]interface{}, 0)
	for _, entry := range entries {
		if matchesFilters(entry, filters) {
			filteredEntries = append(filteredEntries, entry)
		}
	}

	// Apply pagination
	if filters.Limit > 0 {
		start := filters.Offset
		if start < 0 {
			start = 0
		}
		end := start + filters.Limit
		if end > len(filteredEntries) {
			end = len(filteredEntries)
		}
		if start < len(filteredEntries) {
			return filteredEntries[start:end], nil
		}
		return []interface{}{}, nil
	}

	return filteredEntries, nil
}

// GetAuditLogEntryFromStorage retrieves a specific audit log entry by ID
func GetAuditLogEntryFromStorage(
	ctx context.Context,
	objectStorage objectstorage.ObjectStorageService,
	entryID string,
	timestamp time.Time,
) (interface{}, error) {
	// Generate storage key based on entry ID and timestamp
	dateStr := timestamp.Format("2006-01-02")
	key := fmt.Sprintf("audit-logs/%s/%s.json", dateStr, entryID)

	// Load from object storage
	reader, err := objectStorage.LoadSnapshot(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load audit log entry: %w", err)
	}
	defer reader.Close()

	// Read and unmarshal
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read audit log entry: %w", err)
	}

	// Try to unmarshal as different entry types
	var baseEntry types.AuditEntry
	if err := json.Unmarshal(data, &baseEntry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal audit entry: %w", err)
	}

	// Unmarshal into the specific entry type
	switch baseEntry.Type {
	case types.EntryTypeDataAccess:
		var entry types.DataAccessEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal data access entry: %w", err)
		}
		return entry, nil
	case types.EntryTypeAuthentication:
		var entry types.AuthenticationEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal authentication entry: %w", err)
		}
		return entry, nil
	case types.EntryTypeAuthorization:
		var entry types.AuthorizationEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal authorization entry: %w", err)
		}
		return entry, nil
	case types.EntryTypeConfigurationChange:
		var entry types.ConfigurationChangeEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal configuration change entry: %w", err)
		}
		return entry, nil
	case types.EntryTypeModelDeployment:
		var entry types.ModelDeploymentEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal model deployment entry: %w", err)
		}
		return entry, nil
	case types.EntryTypeSecurityEvent:
		var entry types.SecurityEventEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal security event entry: %w", err)
		}
		return entry, nil
	default:
		return nil, fmt.Errorf("unknown entry type: %s", baseEntry.Type)
	}
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
