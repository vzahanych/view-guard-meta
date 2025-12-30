package impl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	"go.uber.org/zap"
)

// AuditLogImpl implements the AuditLogService interface with tamper-proof storage
type AuditLogImpl struct {
	config         *types.AuditLogConfig
	objectStorage  objectstorage.ObjectStorageService
	provider       types.AuditLogProvider // Provider-agnostic storage provider (preferred: meta-storage)
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
	started        bool   // Flag to track if service is started
	mu             sync.Mutex // Mutex for thread-safe start/stop operations
	stopCh         chan struct{} // Channel for graceful shutdown
	syncQueue         *SyncQueueManager    // Sync queue manager for failed/pending syncs
	syncTrigger       *SyncTriggerManager  // Sync trigger manager for time/count-based triggers
	cleanupManager    *CleanupManager      // Cleanup manager for expired entries
	hashChainManager  *HashChainManager    // Hash chain manager for integrity verification
	isPaused          bool                 // Flag to track if operations are paused due to queue full
	pauseMu           sync.RWMutex         // Mutex for pause state
	metricsTracker    *MetricsTracker     // Comprehensive metrics tracking
	integrityTicker   *time.Ticker         // Ticker for periodic integrity checks
	integrityStop     chan struct{}        // Channel for stopping integrity checks
	eventBus          eventbus.EventBus    // Optional event bus for operational event emission
	lastHealthStatus  types.HealthStatus   // Track last health status for state change detection
	healthStatusMu    sync.RWMutex         // Mutex for last health status
}

// NewAuditLogService creates a new audit log service instance
func NewAuditLogService(
	config *types.AuditLogConfig,
	objectStorage objectstorage.ObjectStorageService,
	vmGateway vmgateway.VMGateway,
	logger *zap.Logger,
	edgeID string,
	provider types.AuditLogProvider, // Optional provider (preferred: meta-storage)
	eventBus eventbus.EventBus, // Optional event bus for operational event emission
) *AuditLogImpl {
	// Validate and set defaults
	config.Validate()

	// Initialize sync queue manager
	var syncQueue *SyncQueueManager
	if config.SyncQueueConfig != nil {
		syncQueue = NewSyncQueueManager(config.SyncQueueConfig, logger)
	} else {
		// Create default config if not provided
		defaultConfig := &types.SyncQueueConfig{}
		defaultConfig.Validate()
		syncQueue = NewSyncQueueManager(defaultConfig, logger)
	}

	// Initialize sync trigger manager
	syncTrigger := NewSyncTriggerManager(config, logger)

	// Initialize cleanup manager
	cleanupManager := NewCleanupManager(config, logger)

	// Initialize hash chain manager
	hashChainManager := NewHashChainManager(logger)

	return &AuditLogImpl{
		config:        config,
		objectStorage: objectStorage,
		provider:      provider,
		vmGateway:     vmGateway,
		logger:        logger,
		edgeID:        edgeID,
		syncStop:      make(chan struct{}),
		cleanupStop:   make(chan struct{}),
		integrityStop: make(chan struct{}),
		stopCh:        make(chan struct{}),
		syncQueue:     syncQueue,
		syncTrigger:   syncTrigger,
		cleanupManager: cleanupManager,
		hashChainManager: hashChainManager,
		isPaused:      false,
		metricsTracker: NewMetricsTracker(logger),
		eventBus:      eventBus,
	}
}

func (a *AuditLogImpl) Name() string {
	return "audit-log-service"
}

// IsOperationPaused returns true if operations are paused due to sync queue being full.
// Callers should check this before performing sensitive operations (dataset creation, model deployment, etc.)
// and pause those operations when the queue is full.
func (a *AuditLogImpl) IsOperationPaused() bool {
	a.pauseMu.RLock()
	defer a.pauseMu.RUnlock()
	return a.isPaused
}

// updatePauseState checks queue state and updates pause flag accordingly.
// This is called from sync worker to monitor queue state.
func (a *AuditLogImpl) updatePauseState() {
	if a.syncQueue == nil {
		return
	}

	isFull := a.syncQueue.IsQueueFull()

	a.pauseMu.Lock()
	wasPaused := a.isPaused
	a.isPaused = isFull
	currentlyPaused := a.isPaused
	a.pauseMu.Unlock()

	// Emit events when state changes
	if !wasPaused && currentlyPaused {
		// Queue just became full
		queueDepth := a.syncQueue.GetQueueDepth()
		queueMaxSize := a.syncQueue.config.MaxQueueSize
		queueUsagePercent := a.syncQueue.GetQueueUsagePercent()
		if a.logger != nil {
			a.logger.Error("Sync queue is full - operations paused (CRITICAL ALERT)",
				zap.Int("queue_depth", queueDepth),
				zap.Int("max_queue_size", queueMaxSize))
		}
		// Emit event: audit_log.queue_full
		a.emitQueueFullEvent(queueDepth, queueMaxSize, queueUsagePercent)
	} else if wasPaused && !currentlyPaused {
		// Queue has space again
		queueDepth := a.syncQueue.GetQueueDepth()
		queueMaxSize := a.syncQueue.config.MaxQueueSize
		queueUsagePercent := a.syncQueue.GetQueueUsagePercent()
		if a.logger != nil {
			a.logger.Info("Sync queue has space - operations can resume",
				zap.Int("queue_depth", queueDepth),
				zap.Int("max_queue_size", queueMaxSize))
		}
		// Emit event: audit_log.queue_resumed
		a.emitQueueResumedEvent(queueDepth, queueMaxSize, queueUsagePercent)
	}
}

// HealthSnapshot returns the current health status of the audit log service.
// This follows the vm-gateway/meta-storage/object-storage pattern for health snapshots.
// It queries queue status, sync status, hash chain integrity, and provider health,
// then aggregates the information into an AuditLogHealth struct.
func (a *AuditLogImpl) HealthSnapshot() types.AuditLogHealth {
	health := types.AuditLogHealth{
		Status:            types.HealthStatusHealthy,
		QueueDepth:        0,
		QueueMaxSize:      100000, // Default
		QueueUsagePercent: 0.0,
		IsPaused:          false,
		LastSyncTime:      time.Time{},
		LastSyncSuccess:  true,
		SyncFailures:      0,
		EntriesLogged:     0,
		EntriesSynced:     0,
		EntriesPending:    0,
		HashChainIntegrity: true,
		ProviderHealth:    "healthy",
		ProviderStatus:     make(map[string]interface{}),
	}

	// Query queue status
	if a.syncQueue != nil {
		health.QueueDepth = a.syncQueue.GetQueueDepth()
		health.QueueMaxSize = a.syncQueue.config.MaxQueueSize
		health.QueueUsagePercent = a.syncQueue.GetQueueUsagePercent()
		health.IsPaused = a.IsOperationPaused()
		health.EntriesPending = int64(health.QueueDepth)
	}

	// Query sync status
	if a.syncTrigger != nil {
		health.LastSyncTime = a.syncTrigger.GetLastSyncTime()
	}

	// Get metrics snapshot for comprehensive metrics
	if a.metricsTracker != nil {
		snapshot := a.metricsTracker.GetSnapshot()
		health.EntriesLogged = snapshot.EntriesLoggedTotal
		health.EntriesSynced = snapshot.EntriesSyncedTotal
		health.SyncFailures = int(snapshot.SyncFailures)
		health.LastSyncSuccess = snapshot.SyncSuccessRate > 0.5 // If success rate > 50%, consider last sync successful
	}

	// Query hash chain integrity
	if a.hashChainManager != nil {
		report := a.hashChainManager.GetLastReport()
		if report != nil {
			health.HashChainIntegrity = report.IsIntegrityIntact
		}
	}
	
	// Update metrics with hash chain verification result if available
	if a.hashChainManager != nil && a.metricsTracker != nil {
		report := a.hashChainManager.GetLastReport()
		if report != nil {
			a.metricsTracker.RecordHashChainVerification(report.IsIntegrityIntact)
		}
	}

	// Query provider health
	if a.provider != nil {
		ctx := context.Background()
		providerHealth, err := a.provider.HealthCheck(ctx)
		if err != nil {
			health.ProviderHealth = "unhealthy"
			health.ProviderStatus["error"] = err.Error()
		} else {
			health.ProviderHealth = providerHealth
		}
	} else {
		// No provider - assume healthy for backward compatibility
		health.ProviderHealth = "healthy"
		health.ProviderStatus["provider"] = "object-storage (legacy)"
	}

	// Calculate overall health status based on conditions
	previousStatus := a.getLastHealthStatus()
	health.Status = a.calculateHealthStatus(health)

	// Emit health_degraded event if status transitions to degraded/warning from healthy
	if previousStatus == types.HealthStatusHealthy && 
		(health.Status == types.HealthStatusDegraded || 
		 health.Status == types.HealthStatusWarning || 
		 health.Status == types.HealthStatusSyncFailed || 
		 health.Status == types.HealthStatusQueueFull) {
		healthStatusStr := health.Status.String()
		healthReason := a.getHealthReason(health)
		a.emitHealthDegradedEvent(healthStatusStr, healthReason)
	}

	// Update last health status
	a.setLastHealthStatus(health.Status)

	return health
}

// calculateHealthStatus determines the overall health status based on various conditions.
// Priority order (most critical first):
// 1. QueueFull - queue is 100% full and operations are paused
// 2. SyncFailed - sync failures detected
// 3. Degraded - hash chain issues or provider errors
// 4. Warning - queue >80% full or other warning conditions
// 5. Healthy - all systems operating normally
func (a *AuditLogImpl) calculateHealthStatus(health types.AuditLogHealth) types.HealthStatus {
	// Priority 1: Queue is full and operations are paused (critical)
	if health.IsPaused || health.QueueUsagePercent >= 100.0 {
		return types.HealthStatusQueueFull
	}

	// Priority 2: Sync failures detected (critical)
	if health.SyncFailures > 0 || !health.LastSyncSuccess {
		// Check if failures are recent (within last hour)
		if !health.LastSyncTime.IsZero() && time.Since(health.LastSyncTime) < time.Hour {
			return types.HealthStatusSyncFailed
		}
		// If last sync was more than an hour ago and failed, still mark as sync failed
		if !health.LastSyncSuccess {
			return types.HealthStatusSyncFailed
		}
	}

	// Priority 3: Hash chain integrity issues or provider errors (degraded)
	if !health.HashChainIntegrity {
		return types.HealthStatusDegraded
	}
	if health.ProviderHealth == "unhealthy" || health.ProviderHealth == "degraded" {
		return types.HealthStatusDegraded
	}

	// Priority 4: Warning conditions (queue >80% full, but not paused)
	if health.QueueUsagePercent >= 80.0 {
		return types.HealthStatusWarning
	}

	// Priority 5: All systems healthy
	return types.HealthStatusHealthy
}

// getLastHealthStatus returns the last health status.
func (a *AuditLogImpl) getLastHealthStatus() types.HealthStatus {
	a.healthStatusMu.RLock()
	defer a.healthStatusMu.RUnlock()
	return a.lastHealthStatus
}

// setLastHealthStatus updates the last health status.
func (a *AuditLogImpl) setLastHealthStatus(status types.HealthStatus) {
	a.healthStatusMu.Lock()
	defer a.healthStatusMu.Unlock()
	a.lastHealthStatus = status
}

// getHealthReason returns a human-readable reason for the current health status.
func (a *AuditLogImpl) getHealthReason(health types.AuditLogHealth) string {
	switch health.Status {
	case types.HealthStatusQueueFull:
		return "Sync queue is full and operations are paused"
	case types.HealthStatusSyncFailed:
		return "Recent sync failures detected"
	case types.HealthStatusDegraded:
		if !health.HashChainIntegrity {
			return "Hash chain integrity compromised"
		}
		if health.ProviderHealth == "unhealthy" || health.ProviderHealth == "degraded" {
			return "Provider health issues detected"
		}
		return "Service degraded"
	case types.HealthStatusWarning:
		if health.QueueUsagePercent >= 80.0 {
			return "Sync queue usage exceeds 80%"
		}
		return "Service in warning state"
	default:
		return "Service healthy"
	}
}

// Start starts the audit log service and initializes all components.
// This follows the vm-gateway/meta-storage/object-storage lifecycle pattern.
func (a *AuditLogImpl) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return types.ErrAlreadyStarted
	}

	if !a.config.Enabled {
		a.logger.Info("Audit logging is disabled")
		return nil
	}

	a.logger.Info("Starting audit log service",
		zap.String("provider", a.config.Provider),
		zap.Int("retention_days", a.config.RetentionDays),
		zap.Duration("sync_interval", a.config.SyncInterval))

	// Step 1: Initialize and verify provider
	if a.provider != nil {
		// Use provider if available (preferred: meta-storage)
		healthStatus, err := a.provider.HealthCheck(ctx)
		if err != nil {
			a.logger.Warn("Provider health check failed, will retry",
				zap.String("provider", a.config.Provider),
				zap.Error(err))
		} else {
			a.logger.Info("Provider initialized and verified",
				zap.String("provider", a.config.Provider),
				zap.String("health_status", healthStatus))
		}
	} else {
		// Fall back to object-storage for backward compatibility
		a.logger.Info("Provider not available, using object-storage (backward compatibility)")
	}

	// Step 3: Initialize hash chain (load last hash from storage and verify continuity)
	if a.hashChainManager != nil {
		// Set provider on hash chain manager if available
		if a.provider != nil {
			a.hashChainManager.SetProvider(a.provider)
		}
		
		// Initialize hash chain (loads last hash, verifies continuity, attempts recovery if broken)
		if err := a.hashChainManager.InitializeHashChain(ctx); err != nil {
			// Recovery failed or initialization error - log but don't fail startup
			// Service can still operate, but chain integrity may be compromised
			a.logger.Error("Hash chain initialization failed - chain integrity may be compromised",
				zap.Error(err))
			// Continue with empty hash (first entry scenario)
			a.hashMu.Lock()
			a.lastHash = ""
			a.hashMu.Unlock()
			a.hashChainManager.SetLastHash("")
		} else {
			// Initialization succeeded - get last hash from manager
			lastHash := a.hashChainManager.GetLastHash()
			a.hashMu.Lock()
			a.lastHash = lastHash
			a.hashMu.Unlock()
			
			if a.logger != nil {
				a.logger.Info("Hash chain initialized successfully",
					zap.String("last_hash", lastHash))
			}
		}
	} else {
		// Hash chain manager not available - load last hash directly from provider
		if a.provider != nil {
			lastHash, err := a.provider.GetLastHash(ctx)
			if err != nil {
				a.logger.Warn("Failed to load last hash from provider", zap.Error(err))
				lastHash = "" // Start with empty hash (first entry scenario)
			}
			a.hashMu.Lock()
			a.lastHash = lastHash
			a.hashMu.Unlock()
		} else {
			// No provider - start with empty hash chain (first entry scenario)
			a.hashMu.Lock()
			a.lastHash = ""
			a.hashMu.Unlock()
		}
		
		if a.logger != nil {
			a.logger.Info("Hash chain initialized (legacy approach)")
		}
	}

	// Step 4: Initialize sync queue
	if a.syncQueue != nil {
		// Set provider on sync queue if available
		if a.provider != nil {
			a.syncQueue.SetProvider(a.provider)
		}
		
		// Load queue state from provider (for crash recovery)
		if err := a.syncQueue.LoadQueueFromProvider(ctx); err != nil {
			a.logger.Warn("Failed to load sync queue from provider", zap.Error(err))
		}
		a.logger.Info("Sync queue initialized",
			zap.Int("max_queue_size", a.syncQueue.config.MaxQueueSize))
	}

	// Step 5: Initialize cleanup manager
	if a.cleanupManager != nil {
		// Set provider on cleanup manager if available
		if a.provider != nil {
			a.cleanupManager.SetProvider(a.provider)
		}
		a.logger.Info("Cleanup manager initialized",
			zap.Int("retention_days", a.config.RetentionDays),
			zap.Duration("cleanup_interval", a.config.CleanupInterval),
			zap.Int("cleanup_batch_size", a.config.CleanupBatchSize))
	}

	// Step 6: Start background tasks
	// Start sync worker (runs every 5 minutes or when 1000 records queued - will be enhanced in Epic 4)
	a.syncTicker = time.NewTicker(a.config.SyncInterval)
	go a.syncLoop(ctx)
	if a.logger != nil {
		a.logger.Info("Sync worker started", zap.Duration("interval", a.config.SyncInterval))
	}

	// Start cleanup worker (runs daily)
	a.cleanupTicker = time.NewTicker(a.config.CleanupInterval)
	go a.cleanupLoop(ctx)
	if a.logger != nil {
		a.logger.Info("Cleanup worker started", zap.Duration("interval", a.config.CleanupInterval))
	}

	// Start integrity check worker (runs daily for hash chain verification)
	integrityInterval := 24 * time.Hour // Daily integrity checks
	a.integrityTicker = time.NewTicker(integrityInterval)
	go a.integrityCheckLoop(ctx)
	if a.logger != nil {
		a.logger.Info("Integrity check worker started", zap.Duration("interval", integrityInterval))
	}

	// Start health check worker (runs every 1 minute - stub for now, will be enhanced in Epic 9)
	// TODO: Start health check worker in Epic 9 (Health Monitoring and Observability)
	if a.logger != nil {
		a.logger.Info("Health check worker: pending Epic 9 (Health Monitoring)")
	}

	a.started = true

	if a.logger != nil {
		a.logger.Info("Audit log service started successfully")
	}

	return nil
}

// Stop gracefully shuts down the audit log service.
// This stops background tasks, performs final sync, and closes provider connections.
// This follows the vm-gateway/meta-storage/object-storage lifecycle pattern.
func (a *AuditLogImpl) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.started {
		return nil // Already stopped
	}

	if !a.config.Enabled {
		return nil
	}

	a.logger.Info("Stopping audit log service")

	var errs []error

	// Step 1: Stop background tasks gracefully
	close(a.stopCh)
	a.stopCh = make(chan struct{}) // Reset for potential restart

	// Stop sync loop
	if a.syncTicker != nil {
		a.syncTicker.Stop()
		select {
		case a.syncStop <- struct{}{}:
		default:
			// Channel already closed or full
		}
		close(a.syncStop)
		a.syncStop = make(chan struct{}) // Reset for potential restart
		if a.logger != nil {
			a.logger.Info("Sync worker stopped")
		}
	}

	// Stop cleanup loop
	if a.cleanupTicker != nil {
		a.cleanupTicker.Stop()
		select {
		case a.cleanupStop <- struct{}{}:
		default:
			// Channel already closed or full
		}
		close(a.cleanupStop)
		a.cleanupStop = make(chan struct{}) // Reset for potential restart
		if a.logger != nil {
			a.logger.Info("Cleanup worker stopped")
		}
	}

	// Stop integrity check loop
	if a.integrityTicker != nil {
		a.integrityTicker.Stop()
		select {
		case a.integrityStop <- struct{}{}:
		default:
			// Channel already closed or full
		}
		close(a.integrityStop)
		a.integrityStop = make(chan struct{}) // Reset for potential restart
		if a.logger != nil {
			a.logger.Info("Integrity check worker stopped")
		}
	}

	// Step 2: Final sync before shutdown
	if err := a.SyncToVM(ctx); err != nil {
		errs = append(errs, fmt.Errorf("final sync failed: %w", err))
		a.logger.Warn("Failed to sync audit logs during shutdown", zap.Error(err))
	} else {
		if a.logger != nil {
			a.logger.Info("Final sync completed")
		}
	}

	// Step 3: Close provider connections
	// Note: Provider lifecycle is managed by the provider itself (meta-storage manages its own lifecycle)
	// We don't need to close it here, but we can flush any pending operations if needed
	if a.provider != nil {
		// Provider lifecycle is managed externally (by meta-storage service)
		if a.logger != nil {
			a.logger.Info("Provider connections: managed by provider lifecycle")
		}
	} else {
		// Object-storage is managed by its own lifecycle, so we don't close it here
		if a.logger != nil {
			a.logger.Info("Object-storage connections: managed by object-storage lifecycle")
		}
	}

	// Step 4: Flush pending operations
	// Note: Provider operations are synchronous, so no explicit flush is needed
	// Sync queue entries are persisted on each operation (when provider is available)
	if a.logger != nil {
		a.logger.Info("Pending operations flushed")
	}

	a.started = false

	if len(errs) > 0 {
		if a.logger != nil {
			a.logger.Error("Some operations failed during stop", zap.Errors("errors", errs))
		}
		return fmt.Errorf("stop failed with errors: %w", errors.Join(errs...))
	}

	if a.logger != nil {
		a.logger.Info("Audit log service stopped successfully")
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

func (a *AuditLogImpl) LogDatasetLifecycle(ctx context.Context, entry types.DatasetLifecycleEntry) error {
	entry.AuditEntry.Type = types.EntryTypeDatasetLifecycle
	return a.logEntry(ctx, entry.AuditEntry, entry)
}

func (a *AuditLogImpl) LogRecoveryAction(ctx context.Context, entry types.RecoveryActionEntry) error {
	entry.AuditEntry.Type = types.EntryTypeRecoveryAction
	return a.logEntry(ctx, entry.AuditEntry, entry)
}

// logEntry is the internal method that handles all audit log entries
func (a *AuditLogImpl) logEntry(ctx context.Context, baseEntry types.AuditEntry, fullEntry interface{}) error {
	if !a.config.Enabled {
		return nil
	}

	// CRITICAL: Check if operations are paused due to queue full
	// NEVER drop audit records - if queue is full, return error to pause operations
	if a.IsOperationPaused() {
		if a.logger != nil {
			a.logger.Warn("Cannot log audit entry - operations paused due to sync queue full",
				zap.String("entry_type", string(baseEntry.Type)),
				zap.Int("queue_depth", a.syncQueue.GetQueueDepth()))
		}
		return types.ErrQueueFull
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
	
	// Update hash chain manager
	if a.hashChainManager != nil {
		a.hashChainManager.SetLastHash(baseEntry.Hash)
	}

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
	case types.DatasetLifecycleEntry:
		e.AuditEntry = baseEntry
		fullEntry = e
	case types.RecoveryActionEntry:
		e.AuditEntry = baseEntry
		fullEntry = e
	}

		// Store entry using provider (meta-storage is required)
		if a.provider == nil {
			return fmt.Errorf("audit log provider is required - meta-storage provider must be configured")
		}

		// Use provider (meta-storage)
		if err := a.provider.SaveEntry(ctx, baseEntry); err != nil {
			return fmt.Errorf("failed to save audit log entry to provider: %w", err)
		}
		
		// Save last hash to provider
		if err := a.provider.SaveLastHash(ctx, baseEntry.Hash); err != nil {
			a.logger.Warn("Failed to save last hash to provider", zap.Error(err))
			// Continue - hash is still tracked in memory
		}
		
		a.logger.Debug("Audit log entry stored via provider",
			zap.String("entry_id", baseEntry.ID),
			zap.String("type", string(baseEntry.Type)),
			zap.String("provider", a.config.Provider))

	// CRITICAL: Add entry to sync queue for VM synchronization
	// This ensures at-least-once delivery - entries remain in queue until VM acknowledgment
	if a.syncQueue != nil {
		if err := a.syncQueue.EnqueueEntry(ctx, baseEntry); err != nil {
			if errors.Is(err, types.ErrQueueFull) {
				// Queue is full - update pause state
				a.updatePauseState()
				// Return error to pause operations
				return err
			}
			// Other errors - log but don't fail the operation (entry is already stored)
			a.logger.Warn("Failed to enqueue entry to sync queue",
				zap.String("entry_id", baseEntry.ID),
				zap.Error(err))
		} else {
			// Record pending entry in trigger manager
			if a.syncTrigger != nil {
				a.syncTrigger.RecordPendingEntry()
			}
		}
	}

	// Track entry logged in metrics
	if a.metricsTracker != nil {
		a.metricsTracker.RecordEntryLogged(baseEntry.Type)
	}

	return nil
}

// syncLoop periodically syncs audit logs to VM
func (a *AuditLogImpl) syncLoop(ctx context.Context) {
	for {
		select {
		case <-a.syncTicker.C:
			// Check if sync should be triggered (time-based, count-based, or hybrid)
			if a.syncTrigger != nil && !a.syncTrigger.ShouldSync() {
				// Not time to sync yet - skip this tick
				a.logger.Debug("Sync skipped - trigger conditions not met",
					zap.Int("pending_count", a.syncTrigger.GetPendingCount()))
				continue
			}

			// Update pause state before syncing (monitor queue state)
			a.updatePauseState()
			
			// Record queue depth in metrics
			if a.syncQueue != nil && a.metricsTracker != nil {
				a.metricsTracker.RecordQueueDepth(a.syncQueue.GetQueueDepth())
			}

			if err := a.SyncToVM(ctx); err != nil {
				a.logger.Warn("Failed to sync audit logs to VM", zap.Error(err))
				// Record sync failure in metrics
				if a.metricsTracker != nil {
					a.metricsTracker.RecordSyncFailure()
				}
			}

			// Update pause state after syncing (queue may have space now)
			a.updatePauseState()
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

// integrityCheckLoop periodically verifies hash chain integrity.
// Runs daily to detect tampering in the audit log chain.
func (a *AuditLogImpl) integrityCheckLoop(ctx context.Context) {
	for {
		select {
		case <-a.integrityTicker.C:
			if a.hashChainManager == nil {
				continue
			}

			report, err := a.hashChainManager.VerifyHashChain(ctx)
			if err != nil {
				a.logger.Error("Hash chain integrity verification failed", zap.Error(err))
				// Record verification failure in metrics
				if a.metricsTracker != nil {
					a.metricsTracker.RecordHashChainVerification(false)
				}
				// TODO: Emit event: audit_log.integrity_check_failed (will be implemented when event-bus integration is added)
				continue
			}

			// Record verification result in metrics
			if a.metricsTracker != nil {
				a.metricsTracker.RecordHashChainVerification(report.IsIntegrityIntact)
			}

			if !report.IsIntegrityIntact {
				// Tampering detected - this is critical
				brokenLinks := len(report.BrokenLinks)
				tamperIndicators := len(report.TamperIndicators)
				verifiedEntries := report.VerifiedEntries
				totalEntries := report.TotalEntries
				a.logger.Error("CRITICAL: Hash chain tampering detected",
					zap.Int("broken_links", brokenLinks),
					zap.Int("tamper_indicators", tamperIndicators),
					zap.Int("verified_entries", verifiedEntries),
					zap.Int("total_entries", totalEntries))

				// Emit event: audit_log.tamper_detected (critical event)
				a.emitTamperDetectedEvent(brokenLinks, tamperIndicators, verifiedEntries, totalEntries)
			} else {
				a.logger.Info("Hash chain integrity verification completed: chain is intact",
					zap.Int("total_entries", report.TotalEntries),
					zap.Int("verified_entries", report.VerifiedEntries),
					zap.Duration("verification_duration", report.VerificationDuration))
			}
		case <-a.integrityStop:
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

	// Track sync start time for metrics
	syncStartTime := time.Now()

	a.syncMu.Lock()
	defer a.syncMu.Unlock()

	// Process sync queue entries first (failed/pending syncs)
	var totalSynced int64
	var syncErr error

	if a.syncQueue != nil {
		// Dequeue entries ready for sync (up to batch size)
		batchSize := a.config.SyncBatchSize
		if batchSize <= 0 {
			batchSize = 1000 // Default batch size
		}

		queueEntries, err := a.syncQueue.DequeueEntries(ctx, batchSize)
		if err != nil {
			a.logger.Warn("Failed to dequeue entries from sync queue", zap.Error(err))
		} else if len(queueEntries) > 0 {
			// Sync queue entries to VM
			syncedEntryIDs, failedEntryIDs, err := a.syncQueueEntriesToVM(ctx, queueEntries)
			totalSynced += int64(len(syncedEntryIDs))

			// Mark synced entries as synced, failed entries for retry
			for _, entry := range queueEntries {
				if err != nil {
					// Sync failed - mark for retry
					if markErr := a.syncQueue.MarkFailed(ctx, entry.EntryID, err); markErr != nil {
						a.logger.Warn("Failed to mark entry as failed", zap.String("entry_id", entry.EntryID), zap.Error(markErr))
					}
				} else {
					// Check if this entry was successfully synced
					found := false
					for _, syncedID := range syncedEntryIDs {
						if syncedID == entry.EntryID {
							found = true
							break
						}
					}
					if found {
						// Mark as synced
						if markErr := a.syncQueue.MarkSynced(ctx, entry.EntryID); markErr != nil {
							a.logger.Warn("Failed to mark entry as synced", zap.String("entry_id", entry.EntryID), zap.Error(markErr))
						}
					} else {
						// Check if in failed list
						isFailed := false
						for _, failedID := range failedEntryIDs {
							if failedID == entry.EntryID {
								isFailed = true
								break
							}
						}
						if !isFailed {
							// Not in synced or failed list - mark for retry (may have been skipped)
							if markErr := a.syncQueue.MarkFailed(ctx, entry.EntryID, fmt.Errorf("entry not acknowledged by VM")); markErr != nil {
								a.logger.Warn("Failed to mark entry as failed", zap.String("entry_id", entry.EntryID), zap.Error(markErr))
							}
						}
					}
				}
			}

			if err != nil {
				syncErr = err
			}
		}
	}

	// If we still have capacity, also sync new entries from storage (legacy path for backward compatibility)
	// This can be removed once all entries go through sync queue
	if totalSynced < int64(a.config.SyncBatchSize) {
		remainingCapacity := a.config.SyncBatchSize - int(totalSynced)
		
		// Determine time range for sync (since last sync, or last hour if first sync)
		syncStartTimeRange := a.lastSyncedTime
		if syncStartTimeRange.IsZero() {
			syncStartTimeRange = time.Now().Add(-1 * time.Hour)
		}
		syncEndTimeRange := time.Now()

		// Query audit logs in the time range
		filters := types.QueryFilters{
			StartTime: &syncStartTimeRange,
			EndTime:   &syncEndTimeRange,
			Limit:     remainingCapacity,
		}

		// Query from provider (meta-storage is required)
		if a.provider == nil {
			if syncErr == nil {
				syncErr = fmt.Errorf("audit log provider is required - meta-storage provider must be configured")
			}
		} else {
			// Query from provider
			auditEntries, queryErr := a.provider.ListEntries(ctx, filters)
			if queryErr != nil {
				if syncErr == nil {
					syncErr = fmt.Errorf("failed to query audit logs from provider: %w", queryErr)
				}
			} else {
				// Convert AuditEntry to interface{} for compatibility
				entries := make([]interface{}, len(auditEntries))
				for i := range auditEntries {
					entries[i] = auditEntries[i]
				}
				
				if len(entries) > 0 {
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
							if syncErr == nil {
								syncErr = err
							}
							// Continue with next batch
							continue
						}
						totalSynced += int64(len(batch))
					}

					// Update last synced time
					a.lastSyncedTime = syncEndTimeRange
				}
			}
		}
	}

	// Record sync completion
	syncDuration := time.Since(syncStartTime)
	if syncErr == nil && totalSynced > 0 {
		// Successful sync
		// Record sync in trigger manager
		if a.syncTrigger != nil {
			a.syncTrigger.RecordSync()
		}
		// Record in metrics (this also handles legacy sync metrics)
		a.recordSyncSuccess(totalSynced, syncDuration)
		// Emit event: audit_log.sync_succeeded
		a.emitSyncSucceededEvent(totalSynced, syncDuration)
	} else if syncErr != nil {
		// Sync failed
		// Record in metrics (this also handles legacy sync metrics)
		a.recordSyncFailure()
		// Emit event: audit_log.sync_failed
		errorMsg := syncErr.Error()
		a.emitSyncFailedEvent(errorMsg)
	}

	if totalSynced > 0 {
		a.logger.Info("Audit logs synced to VM",
			zap.Int64("entry_count", totalSynced),
			zap.Duration("sync_duration", syncDuration))
	}

	return syncErr
}

// syncQueueEntriesToVM syncs entries from the sync queue to VM using the VM sync protocol.
// Returns: (syncedEntryIDs, failedEntryIDs, error)
func (a *AuditLogImpl) syncQueueEntriesToVM(ctx context.Context, queueEntries []types.SyncQueueEntry) ([]string, []string, error) {
	if len(queueEntries) == 0 {
		return nil, nil, nil
	}

	// Deserialize queue entries to audit entries
	entries := make([]types.AuditEntry, 0, len(queueEntries))

	for _, queueEntry := range queueEntries {
		var entry types.AuditEntry
		if err := json.Unmarshal(queueEntry.EntryData, &entry); err != nil {
			a.logger.Warn("Failed to unmarshal queue entry", zap.String("entry_id", queueEntry.EntryID), zap.Error(err))
			continue
		}
		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return nil, nil, fmt.Errorf("no valid entries to sync")
	}

	// Use VM sync protocol to sync entries
	batchSize := a.config.SyncBatchSize
	if batchSize <= 0 {
		batchSize = 1000 // Default batch size
	}

	results, err := SyncAuditLogsToVM(ctx, entries, a.edgeID, a.vmGateway, a.logger, batchSize)
	if err != nil {
		// Protocol-level error - mark all as failed
		failedIDs := make([]string, len(entries))
		for i, entry := range entries {
			failedIDs[i] = entry.ID
		}
		return nil, failedIDs, err
	}

	// Process results
	var syncedEntryIDs []string
	var failedEntryIDs []string

	for _, result := range results {
		if result.Synced || result.Duplicate {
			// Entry was synced (or was duplicate) - mark as synced
			syncedEntryIDs = append(syncedEntryIDs, result.EntryID)
			if result.Duplicate && a.logger != nil {
				a.logger.Debug("Entry was duplicate (VM already had it)",
					zap.String("entry_id", result.EntryID))
			}
		} else if result.Failed {
			// Entry sync failed - mark for retry
			failedEntryIDs = append(failedEntryIDs, result.EntryID)
			if a.logger != nil && result.Error != nil {
				a.logger.Warn("Entry sync failed",
					zap.String("entry_id", result.EntryID),
					zap.Error(result.Error))
			}
		}
	}

	return syncedEntryIDs, failedEntryIDs, nil
}

// recordSyncSuccess records a successful sync operation.
// This method is kept for backward compatibility and delegates to MetricsTracker.
func (a *AuditLogImpl) recordSyncSuccess(recordsSynced int64, duration time.Duration) {
	if a.metricsTracker != nil {
		a.metricsTracker.RecordSyncSuccess(recordsSynced, duration)
	}
}

// recordSyncFailure records a failed sync operation.
// This method is kept for backward compatibility and delegates to MetricsTracker.
func (a *AuditLogImpl) recordSyncFailure() {
	if a.metricsTracker != nil {
		a.metricsTracker.RecordSyncFailure()
	}
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
	auditLogEntries := make([]*vmgatewaytypes.AuditLogEntry, 0, len(batch))
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

		entry := &vmgatewaytypes.AuditLogEntry{
			ID:           baseEntry.ID,
			Type:         string(baseEntry.Type),
			Timestamp:    baseEntry.Timestamp.Unix(),
			EdgeID:       baseEntry.EdgeID,
			// UserID omitted - VM only knows about EdgeID, not individual users
			// IPAddress omitted - Edge devices are behind NAT, internal IP not meaningful to VM
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
	req := &vmgatewaytypes.SyncAuditLogsRequest{
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

	if a.cleanupManager == nil {
		a.logger.Warn("Cleanup manager not initialized")
		return nil
	}

	// Emit event: audit_log.cleanup_started
	a.emitCleanupStartedEvent()

	// Use cleanup manager to clean up expired entries
	stats, err := a.cleanupManager.CleanupExpiredEntries(ctx)
	if err != nil {
		a.logger.Warn("Cleanup of expired audit logs encountered errors",
			zap.Error(err),
			zap.Int64("entries_deleted", stats.EntriesDeleted),
			zap.Int64("entries_skipped", stats.EntriesSkipped),
			zap.Int("errors_encountered", stats.ErrorsEncountered))
		return err
	}

	a.logger.Info("Cleanup of expired audit logs completed",
		zap.Int64("entries_deleted", stats.EntriesDeleted),
		zap.Int64("entries_skipped", stats.EntriesSkipped),
		zap.Int("errors_encountered", stats.ErrorsEncountered),
		zap.Duration("cleanup_duration", stats.CleanupDuration))

	// Record cleanup metrics
	if a.metricsTracker != nil {
		a.metricsTracker.RecordCleanup(stats.EntriesDeleted, stats.CleanupDuration)
	}

	// Emit event: audit_log.cleanup_completed
	a.emitCleanupCompletedEvent(stats.EntriesDeleted, stats.EntriesSkipped, stats.CleanupDuration)

	return nil
}

func (a *AuditLogImpl) QueryAuditLogs(ctx context.Context, filters types.QueryFilters) ([]interface{}, error) {
	if !a.config.Enabled {
		return []interface{}{}, nil
	}

	// Provider (meta-storage) is required
	if a.provider == nil {
		return nil, fmt.Errorf("audit log provider is required - meta-storage provider must be configured")
	}

	// Query from provider
	auditEntries, err := a.provider.ListEntries(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs from provider: %w", err)
	}
	
	// Convert AuditEntry to interface{} for compatibility
	entries := make([]interface{}, len(auditEntries))
	for i := range auditEntries {
		entries[i] = auditEntries[i]
	}
	return entries, nil
}

func (a *AuditLogImpl) GetAuditLogEntry(ctx context.Context, entryID string) (interface{}, error) {
	if !a.config.Enabled {
		return nil, fmt.Errorf("audit logging is disabled")
	}

	// Provider (meta-storage) is required
	if a.provider == nil {
		return nil, fmt.Errorf("audit log provider is required - meta-storage provider must be configured")
	}

	// Load entry directly from provider
	entry, err := a.provider.LoadEntry(ctx, entryID)
	if err != nil {
		if errors.Is(err, types.ErrRecordNotFound) {
			return nil, fmt.Errorf("audit log entry not found: %s", entryID)
		}
		return nil, fmt.Errorf("failed to load audit log entry: %w", err)
	}
	return *entry, nil
}
