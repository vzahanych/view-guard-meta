package impl

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
)

// ConvertToExportEntry converts an audit log entry to an export-ready format
func ConvertToExportEntry(entry interface{}, format types.ExportFormat) (*types.ExportEntry, error) {
	// Marshal to JSON first
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal entry to JSON: %w", err)
	}

	exportEntry := &types.ExportEntry{
		Entry:  entry,
		Format: format,
		JSON:   string(jsonBytes),
	}

	// Generate CEF format if requested
	if format == types.ExportFormatCEF {
		cef, err := convertToCEF(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to convert entry to CEF: %w", err)
		}
		exportEntry.CEF = cef
	}

	return exportEntry, nil
}

// convertToCEF converts an audit log entry to CEF (Common Event Format) format
// CEF format: CEF:Version|Device Vendor|Device Product|Device Version|Signature ID|Name|Severity|Extension
func convertToCEF(entry interface{}) (string, error) {
	var baseEntry types.AuditEntry
	var entryType string
	var severity string
	var extension map[string]string

	// Extract base entry and type-specific data
	switch e := entry.(type) {
	case types.DataAccessEntry:
		baseEntry = e.AuditEntry
		entryType = "DataAccess"
		severity = getSeverityForResult(e.Result)
		extension = map[string]string{
			"act":     e.Action,
			"src":     e.ResourceType,
			"duser":   e.AuditEntry.UserID,
			"srcip":   e.AuditEntry.IPAddress,
			"outcome": e.Result,
		}
		if e.ResourceID != "" {
			extension["dproc"] = e.ResourceID
		}
	case types.AuthenticationEntry:
		baseEntry = e.AuditEntry
		entryType = "Authentication"
		severity = getSeverityForResult(e.Result)
		extension = map[string]string{
			"act":     "login",
			"duser":   e.Identity,
			"srcip":   e.AuditEntry.IPAddress,
			"outcome": e.Result,
			"method":  e.Method,
		}
		if e.SessionID != "" {
			extension["cs1"] = e.SessionID
		}
	case types.AuthorizationEntry:
		baseEntry = e.AuditEntry
		entryType = "Authorization"
		severity = getSeverityForResult(e.Result)
		extension = map[string]string{
			"act":     e.Action,
			"src":     e.Resource,
			"duser":   e.AuditEntry.UserID,
			"srcip":   e.AuditEntry.IPAddress,
			"outcome": e.Result,
			"cs2":     e.Permission,
		}
		if e.Granted {
			extension["cs3"] = "granted"
		} else {
			extension["cs3"] = "denied"
		}
	case types.ConfigurationChangeEntry:
		baseEntry = e.AuditEntry
		entryType = "ConfigurationChange"
		severity = "medium"
		extension = map[string]string{
			"act":     "modify",
			"src":     e.ConfigSection,
			"duser":   e.AuditEntry.UserID,
			"srcip":   e.AuditEntry.IPAddress,
			"outcome": e.Result,
			"cs1":     e.Field,
		}
	case types.ModelDeploymentEntry:
		baseEntry = e.AuditEntry
		entryType = "ModelDeployment"
		severity = "high"
		extension = map[string]string{
			"act":     e.Action,
			"duser":   e.AuditEntry.UserID,
			"srcip":   e.AuditEntry.IPAddress,
			"outcome": e.Result,
			"cs1":     e.ModelID,
			"cs2":     e.ModelVersion,
		}
		if e.CameraID != "" {
			extension["cs3"] = e.CameraID
		}
	case types.SecurityEventEntry:
		baseEntry = e.AuditEntry
		entryType = "SecurityEvent"
		severity = e.Severity
		extension = map[string]string{
			"act":     "security",
			"duser":   e.AuditEntry.UserID,
			"srcip":   e.AuditEntry.IPAddress,
			"outcome": e.Result,
			"cs1":     e.EventType,
			"msg":     e.Description,
		}
	default:
		return "", fmt.Errorf("unsupported entry type: %T", entry)
	}

	// Build CEF header
	cef := fmt.Sprintf("CEF:0|ViewGuard|Edge|1.0|%s|%s|%s|", baseEntry.ID, entryType, severity)

	// Add extension fields
	extParts := make([]string, 0, len(extension))
	for k, v := range extension {
		// Escape special characters in CEF
		escapedValue := escapeCEFValue(v)
		extParts = append(extParts, fmt.Sprintf("%s=%s", k, escapedValue))
	}

	// Add timestamp
	extParts = append(extParts, fmt.Sprintf("rt=%d", baseEntry.Timestamp.Unix()*1000))

	// Add edge ID
	if baseEntry.EdgeID != "" {
		extParts = append(extParts, fmt.Sprintf("dhost=%s", baseEntry.EdgeID))
	}

	// Add hash for tamper-proofing
	if baseEntry.Hash != "" {
		extParts = append(extParts, fmt.Sprintf("cs4=%s", baseEntry.Hash))
	}
	if baseEntry.PreviousHash != "" {
		extParts = append(extParts, fmt.Sprintf("cs5=%s", baseEntry.PreviousHash))
	}

	cef += strings.Join(extParts, " ")

	return cef, nil
}

// getSeverityForResult maps audit log result to CEF severity
func getSeverityForResult(result string) string {
	switch result {
	case "success":
		return "3" // Low
	case "failure":
		return "6" // Medium
	case "denied":
		return "8" // High
	default:
		return "5" // Medium
	}
}

// escapeCEFValue escapes special characters in CEF extension values
func escapeCEFValue(value string) string {
	// CEF requires escaping of =, \, and newlines
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "=", "\\=")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\r")
	return value
}

// BatchExportEntries groups audit log entries into batches for efficient export
func BatchExportEntries(entries []*types.ExportEntry, batchSize int) [][]*types.ExportEntry {
	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}

	batches := make([][]*types.ExportEntry, 0)
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batches = append(batches, entries[i:end])
	}
	return batches
}

