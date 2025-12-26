package impl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-client-service/types"
	"go.uber.org/zap"
)

// AuditLogImpl implements the AuditLogService interface with tamper-proof storage
type AuditLogImpl struct {
	config         *types.AuditLogConfig
	objectStorage  objectstorage.ObjectStorageService
	vmGateway      vmgateway.VMGateway // VM gateway for syncing audit logs
	logger         *zap.Logger
	lastHash       string // Hash of the last entry (for chain of hashes)
	hashMu         sync.RWMutex
	edgeID         string
	syncTicker     *time.Ticker
	syncStop       chan struct{}
	cleanupTicker  *time.Ticker
	cleanupStop    chan struct{}
	lastSyncedTime time.Time // Track last sync time to avoid duplicates
	syncMu         sync.Mutex
}

// NewAuditLogService creates a new audit log service instance
func NewAuditLogService(
	config *types.AuditLogConfig,
	objectStorage objectstorage.ObjectStorageService,
	vmGateway vmgateway.VMGateway,
	logger *zap.Logger,
	edgeID string,
) *AuditLogImpl {
	// Set defaults
	if config.RetentionDays == 0 {
		config.RetentionDays = 7 // Default: 1 week
	}
	if config.SyncInterval == 0 {
		config.SyncInterval = 1 * time.Hour // Default: 1 hour
	}

	return &AuditLogImpl{
		config:        config,
		objectStorage: objectStorage,
		vmGateway:     vmGateway,
		logger:        logger,
		edgeID:        edgeID,
		syncStop:      make(chan struct{}),
		cleanupStop:   make(chan struct{}),
	}
}

func (a *AuditLogImpl) Name() string {
	return "audit-log-service"
}

func (a *AuditLogImpl) Start(ctx context.Context) error {
	if !a.config.Enabled {
		a.logger.Info("Audit logging is disabled")
		return nil
	}

	a.logger.Info("Starting audit log service",
		zap.Int("retention_days", a.config.RetentionDays),
		zap.Duration("sync_interval", a.config.SyncInterval))

	// Start periodic sync to VM
	a.syncTicker = time.NewTicker(a.config.SyncInterval)
	go a.syncLoop(ctx)

	// Start periodic cleanup of old logs
	a.cleanupTicker = time.NewTicker(24 * time.Hour) // Run cleanup daily
	go a.cleanupLoop(ctx)

	return nil
}

func (a *AuditLogImpl) Stop(ctx context.Context) error {
	if !a.config.Enabled {
		return nil
	}

	a.logger.Info("Stopping audit log service")

	// Stop sync loop
	if a.syncTicker != nil {
		a.syncTicker.Stop()
		close(a.syncStop)
	}

	// Stop cleanup loop
	if a.cleanupTicker != nil {
		a.cleanupTicker.Stop()
		close(a.cleanupStop)
	}

	// Final sync before shutdown
	if err := a.SyncToVM(ctx); err != nil {
		a.logger.Warn("Failed to sync audit logs during shutdown", zap.Error(err))
	}

	return nil
}

func (a *AuditLogImpl) LogDataAccess(ctx context.Context, entry types.DataAccessEntry) error {
	entry.AuditEntry.Type = types.EntryTypeDataAccess
	return a.logEntry(ctx, entry.AuditEntry, entry)
}

func (a *AuditLogImpl) LogAuthentication(ctx context.Context, entry types.AuthenticationEntry) error {
	entry.AuditEntry.Type = types.EntryTypeAuthentication
	return a.logEntry(ctx, entry.AuditEntry, entry)
}

func (a *AuditLogImpl) LogAuthorization(ctx context.Context, entry types.AuthorizationEntry) error {
	entry.AuditEntry.Type = types.EntryTypeAuthorization
	return a.logEntry(ctx, entry.AuditEntry, entry)
}

func (a *AuditLogImpl) LogConfigurationChange(ctx context.Context, entry types.ConfigurationChangeEntry) error {
	entry.AuditEntry.Type = types.EntryTypeConfigurationChange
	return a.logEntry(ctx, entry.AuditEntry, entry)
}

func (a *AuditLogImpl) LogModelDeployment(ctx context.Context, entry types.ModelDeploymentEntry) error {
	entry.AuditEntry.Type = types.EntryTypeModelDeployment
	return a.logEntry(ctx, entry.AuditEntry, entry)
}

func (a *AuditLogImpl) LogSecurityEvent(ctx context.Context, entry types.SecurityEventEntry) error {
	entry.AuditEntry.Type = types.EntryTypeSecurityEvent
	return a.logEntry(ctx, entry.AuditEntry, entry)
}

// logEntry is the internal method that handles all audit log entries
func (a *AuditLogImpl) logEntry(ctx context.Context, baseEntry types.AuditEntry, fullEntry interface{}) error {
	if !a.config.Enabled {
		return nil
	}

	// Set common fields
	baseEntry.ID = uuid.New().String()
	baseEntry.Timestamp = time.Now()
	baseEntry.EdgeID = a.edgeID

	// Get previous hash for tamper-proofing
	a.hashMu.RLock()
	baseEntry.PreviousHash = a.lastHash
	a.hashMu.RUnlock()

	// Calculate hash of this entry (for tamper-proofing)
	entryJSON, err := json.Marshal(fullEntry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	// Include previous hash in the hash calculation for chain integrity
	hashInput := fmt.Sprintf("%s:%s", baseEntry.PreviousHash, string(entryJSON))
	hash := sha256.Sum256([]byte(hashInput))
	baseEntry.Hash = hex.EncodeToString(hash[:])

	// Update last hash
	a.hashMu.Lock()
	a.lastHash = baseEntry.Hash
	a.hashMu.Unlock()

	// Update the base entry in the full entry
	// We need to set it back to the full entry struct
	switch e := fullEntry.(type) {
	case types.DataAccessEntry:
		e.AuditEntry = baseEntry
		fullEntry = e
	case types.AuthenticationEntry:
		e.AuditEntry = baseEntry
		fullEntry = e
	case types.AuthorizationEntry:
		e.AuditEntry = baseEntry
		fullEntry = e
	case types.ConfigurationChangeEntry:
		e.AuditEntry = baseEntry
		fullEntry = e
	case types.ModelDeploymentEntry:
		e.AuditEntry = baseEntry
		fullEntry = e
	case types.SecurityEventEntry:
		e.AuditEntry = baseEntry
		fullEntry = e
	}

	// Marshal the complete entry
	finalJSON, err := json.Marshal(fullEntry)
	if err != nil {
		return fmt.Errorf("failed to marshal final audit entry: %w", err)
	}

	// Generate storage key: audit-logs/YYYY-MM-DD/entry-id.json
	dateStr := baseEntry.Timestamp.Format("2006-01-02")
	key := fmt.Sprintf("audit-logs/%s/%s.json", dateStr, baseEntry.ID)

	// Store in object storage
	reader := io.NopCloser(bytes.NewReader(finalJSON))
	defer reader.Close()

	if err := a.objectStorage.StoreSnapshot(ctx, key, reader, int64(len(finalJSON)), "application/json"); err != nil {
		return fmt.Errorf("failed to store audit log entry: %w", err)
	}

	a.logger.Debug("Audit log entry stored",
		zap.String("entry_id", baseEntry.ID),
		zap.String("type", string(baseEntry.Type)),
		zap.String("key", key))

	return nil
}

// syncLoop periodically syncs audit logs to VM
func (a *AuditLogImpl) syncLoop(ctx context.Context) {
	for {
		select {
		case <-a.syncTicker.C:
			if err := a.SyncToVM(ctx); err != nil {
				a.logger.Warn("Failed to sync audit logs to VM", zap.Error(err))
			}
		case <-a.syncStop:
			return
		case <-ctx.Done():
			return
		}
	}
}

// cleanupLoop periodically cleans up old audit logs
func (a *AuditLogImpl) cleanupLoop(ctx context.Context) {
	for {
		select {
		case <-a.cleanupTicker.C:
			if err := a.CleanupOldLogs(ctx); err != nil {
				a.logger.Warn("Failed to cleanup old audit logs", zap.Error(err))
			}
		case <-a.cleanupStop:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (a *AuditLogImpl) SyncToVM(ctx context.Context) error {
	if !a.config.Enabled {
		return nil
	}

	if a.vmGateway == nil {
		a.logger.Debug("VM gateway not available, skipping audit log sync")
		return nil
	}

	a.syncMu.Lock()
	defer a.syncMu.Unlock()

	// Determine time range for sync (since last sync, or last hour if first sync)
	syncStartTime := a.lastSyncedTime
	if syncStartTime.IsZero() {
		syncStartTime = time.Now().Add(-1 * time.Hour)
	}
	syncEndTime := time.Now()

	// Query audit logs in the time range
	filters := types.QueryFilters{
		StartTime: &syncStartTime,
		EndTime:   &syncEndTime,
		Limit:     1000, // Batch size
	}

	entries, err := QueryAuditLogsFromStorage(ctx, a.objectStorage, a.logger, filters)
	if err != nil {
		return fmt.Errorf("failed to query audit logs: %w", err)
	}

	if len(entries) == 0 {
		a.logger.Debug("No new audit logs to sync")
		return nil
	}

	// Convert entries to export format
	exportEntries := make([]*types.ExportEntry, 0, len(entries))
	for _, entry := range entries {
		exportEntry, err := ConvertToExportEntry(entry, types.ExportFormatJSON)
		if err != nil {
			a.logger.Warn("Failed to convert entry to export format", zap.Error(err))
			continue
		}
		exportEntries = append(exportEntries, exportEntry)
	}

	// Batch entries for efficient transfer
	batches := BatchExportEntries(exportEntries, 100)

	for _, batch := range batches {
		if err := a.syncBatchToVM(ctx, batch); err != nil {
			a.logger.Warn("Failed to sync batch to VM", zap.Error(err))
			// Continue with next batch
			continue
		}
	}

	// Update last synced time
	a.lastSyncedTime = syncEndTime
	a.logger.Info("Audit logs synced to VM",
		zap.Int("entry_count", len(entries)),
		zap.Time("sync_end_time", syncEndTime))

	return nil
}

// syncBatchToVM syncs a batch of audit log entries to VM
func (a *AuditLogImpl) syncBatchToVM(ctx context.Context, batch []*types.ExportEntry) error {
	if len(batch) == 0 {
		return nil
	}

	// Convert export entries to VM format
	vmEntries := make([]interface{}, 0, len(batch))
	var startTime, endTime time.Time

	for _, exportEntry := range batch {
		// Extract timestamp from entry
		var baseEntry types.AuditEntry
		if err := json.Unmarshal([]byte(exportEntry.JSON), &baseEntry); err != nil {
			continue
		}

		if startTime.IsZero() || baseEntry.Timestamp.Before(startTime) {
			startTime = baseEntry.Timestamp
		}
		if endTime.IsZero() || baseEntry.Timestamp.After(endTime) {
			endTime = baseEntry.Timestamp
		}

		// Create VM entry format
		vmEntry := map[string]interface{}{
			"json": exportEntry.JSON,
		}
		if exportEntry.Format == types.ExportFormatCEF && exportEntry.CEF != "" {
			vmEntry["cef"] = exportEntry.CEF
			vmEntry["format"] = "cef"
		} else {
			vmEntry["format"] = "json"
		}

		vmEntries = append(vmEntries, vmEntry)
	}

	// Convert export entries to VM format
	vmAuditEntries := make([]interface{}, 0, len(batch))
	for _, exportEntry := range batch {
		// Extract base entry for VM format
		var baseEntry types.AuditEntry
		if err := json.Unmarshal([]byte(exportEntry.JSON), &baseEntry); err != nil {
			continue
		}

		// Create VM entry format matching SyncAuditLogsRequest
		vmEntry := map[string]interface{}{
			"id":            baseEntry.ID,
			"type":          string(baseEntry.Type),
			"timestamp":     baseEntry.Timestamp.Unix(),
			"edge_id":       baseEntry.EdgeID,
			"user_id":       baseEntry.UserID,
			"ip_address":    baseEntry.IPAddress,
			"user_agent":    baseEntry.UserAgent,
			"result":        baseEntry.Result,
			"error":         baseEntry.Error,
			"previous_hash": baseEntry.PreviousHash,
			"hash":          baseEntry.Hash,
			"data":          exportEntry.Entry, // Full entry data
			"format":        "json",
		}

		// Add CEF if available
		if exportEntry.Format == types.ExportFormatCEF && exportEntry.CEF != "" {
			vmEntry["cef"] = exportEntry.CEF
			vmEntry["format"] = "cef"
		}

		vmAuditEntries = append(vmAuditEntries, vmEntry)
	}

	// Convert export entries to VM AuditLogEntry format
	auditLogEntries := make([]*httpsclienttypes.AuditLogEntry, 0, len(batch))
	for _, exportEntry := range batch {
		var baseEntry types.AuditEntry
		if err := json.Unmarshal([]byte(exportEntry.JSON), &baseEntry); err != nil {
			continue
		}

		// Extract entry-specific data
		var entryData map[string]interface{}
		if err := json.Unmarshal([]byte(exportEntry.JSON), &entryData); err != nil {
			continue
		}

		entry := &httpsclienttypes.AuditLogEntry{
			ID:           baseEntry.ID,
			Type:         string(baseEntry.Type),
			Timestamp:    baseEntry.Timestamp.Unix(),
			EdgeID:       baseEntry.EdgeID,
			UserID:       baseEntry.UserID,
			IPAddress:    baseEntry.IPAddress,
			UserAgent:    baseEntry.UserAgent,
			Result:       baseEntry.Result,
			Error:        baseEntry.Error,
			PreviousHash: baseEntry.PreviousHash,
			Hash:         baseEntry.Hash,
			Data:         entryData,
			Format:       "json",
		}

		if exportEntry.Format == types.ExportFormatCEF && exportEntry.CEF != "" {
			entry.CEF = exportEntry.CEF
			entry.Format = "cef"
		}

		auditLogEntries = append(auditLogEntries, entry)
	}

	// Create sync request
	req := &httpsclienttypes.SyncAuditLogsRequest{
		EdgeID:     a.edgeID,
		StartTime:  startTime.Unix(),
		EndTime:    endTime.Unix(),
		EntryCount: len(auditLogEntries),
		Entries:    auditLogEntries,
		Format:     "json",
	}

	// Call the sync method
	resp, err := a.vmGateway.SyncAuditLogs(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to sync audit logs to VM: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("VM rejected audit log sync: %s", resp.ErrorMessage)
	}

	a.logger.Info("Audit logs synced to VM",
		zap.Int("entry_count", len(auditLogEntries)),
		zap.Int("synced_count", resp.SyncedCount),
		zap.Int64("start_time", startTime.Unix()),
		zap.Int64("end_time", endTime.Unix()))
	return nil
}

func (a *AuditLogImpl) CleanupOldLogs(ctx context.Context) error {
	if !a.config.Enabled {
		return nil
	}

	// Calculate cutoff date
	cutoffDate := time.Now().AddDate(0, 0, -a.config.RetentionDays)
	a.logger.Info("Cleaning up old audit logs",
		zap.Time("cutoff_date", cutoffDate),
		zap.Int("retention_days", a.config.RetentionDays))

	// Note: Actual cleanup requires listing objects from object storage
	// This would need to be implemented in the object storage service
	// For now, this is a placeholder that logs the intent
	a.logger.Debug("Cleanup of old audit logs (requires object storage list operation)")
	return nil
}

func (a *AuditLogImpl) QueryAuditLogs(ctx context.Context, filters types.QueryFilters) ([]interface{}, error) {
	if !a.config.Enabled {
		return []interface{}{}, nil
	}

	return QueryAuditLogsFromStorage(ctx, a.objectStorage, a.logger, filters)
}

func (a *AuditLogImpl) GetAuditLogEntry(ctx context.Context, entryID string) (interface{}, error) {
	if !a.config.Enabled {
		return nil, fmt.Errorf("audit logging is disabled")
	}

	// Try to find the entry by searching recent dates
	// In a real implementation, we'd maintain an index or use meta-storage
	now := time.Now()
	for i := 0; i < a.config.RetentionDays; i++ {
		date := now.AddDate(0, 0, -i)
		entry, err := GetAuditLogEntryFromStorage(ctx, a.objectStorage, entryID, date)
		if err == nil {
			return entry, nil
		}
	}

	return nil, fmt.Errorf("audit log entry not found: %s", entryID)
}
