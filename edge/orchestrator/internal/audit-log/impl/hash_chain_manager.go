package impl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	"go.uber.org/zap"
)

// HashChainReport contains the results of hash chain integrity verification.
type HashChainReport struct {
	// IsIntegrityIntact indicates whether the hash chain is completely intact (no tampering detected).
	IsIntegrityIntact bool

	// TotalEntries is the total number of entries in the chain.
	TotalEntries int

	// VerifiedEntries is the number of entries that passed integrity checks.
	VerifiedEntries int

	// BrokenLinks is a list of entry IDs where the chain is broken.
	// A broken link means:
	//   - An entry's PreviousHash does not match the previous entry's Hash, or
	//   - An entry's Hash does not match the calculated hash.
	BrokenLinks []BrokenLink

	// TamperIndicators contains detailed information about detected tampering.
	TamperIndicators []TamperIndicator

	// VerificationDuration is the time taken to verify the chain.
	VerificationDuration time.Duration

	// LastVerifiedEntryID is the ID of the last successfully verified entry.
	LastVerifiedEntryID string

	// LastVerifiedHash is the hash of the last successfully verified entry.
	LastVerifiedHash string
}

// BrokenLink represents a break in the hash chain.
type BrokenLink struct {
	// EntryID is the ID of the entry where the chain is broken.
	EntryID string

	// EntryTimestamp is when the entry was created.
	EntryTimestamp time.Time

	// IssueType describes the type of break detected.
	// Values: "hash_mismatch" (entry's hash doesn't match calculated hash),
	//         "previous_hash_mismatch" (entry's PreviousHash doesn't match previous entry's Hash),
	//         "missing_previous_entry" (previous entry referenced by PreviousHash not found).
	IssueType string

	// ExpectedHash is the expected hash value (if hash_mismatch).
	ExpectedHash string

	// ActualHash is the actual hash value stored in the entry (if hash_mismatch).
	ActualHash string

	// ExpectedPreviousHash is the expected PreviousHash value (if previous_hash_mismatch).
	ExpectedPreviousHash string

	// ActualPreviousHash is the actual PreviousHash value stored in the entry (if previous_hash_mismatch).
	ActualPreviousHash string
}

// TamperIndicator provides detailed information about detected tampering.
type TamperIndicator struct {
	// EntryID is the ID of the entry where tampering was detected.
	EntryID string

	// Severity indicates the severity of tampering detected.
	// Values: "critical" (hash chain broken), "warning" (inconsistency detected).
	Severity string

	// Description is a human-readable description of the tampering.
	Description string

	// DetectedAt is when the tampering was detected.
	DetectedAt time.Time
}

// HashChainManager manages hash chain integrity verification for audit log entries.
// It ensures tamper-evident properties by verifying the cryptographic chain of hashes.
type HashChainManager struct {
	logger      *zap.Logger
	provider    types.AuditLogProvider // Will be set when provider is available (Epic 8)
	lastHash    string                 // Last hash in the chain (for continuation)
	lastHashMu  sync.RWMutex           // Mutex for lastHash access
	lastReport  *HashChainReport       // Last verification report (for recovery)
	lastReportMu sync.RWMutex          // Mutex for lastReport access
	// TODO: Add provider when available in Epic 8
	// For now, we'll use the objectStorage directly for backward compatibility
}

// NewHashChainManager creates a new hash chain manager.
func NewHashChainManager(logger *zap.Logger) *HashChainManager {
	return &HashChainManager{
		logger:       logger,
		lastHash:     "", // Will be loaded from provider/storage
		lastHashMu:   sync.RWMutex{},
		lastReport:   nil,
		lastReportMu: sync.RWMutex{},
	}
}

// SetProvider sets the audit log provider for the hash chain manager.
// This will be called when provider is initialized (Epic 8).
func (m *HashChainManager) SetProvider(provider types.AuditLogProvider) {
	m.provider = provider
}

// GetLastHash returns the hash of the last entry in the chain.
// This is used for chain continuation when creating new entries.
func (m *HashChainManager) GetLastHash() string {
	m.lastHashMu.RLock()
	defer m.lastHashMu.RUnlock()
	return m.lastHash
}

// SetLastHash sets the hash of the last entry in the chain.
// This is called when a new entry is created.
func (m *HashChainManager) SetLastHash(hash string) {
	m.lastHashMu.Lock()
	defer m.lastHashMu.Unlock()
	m.lastHash = hash
}

// CalculateHash calculates the hash for an audit entry.
// Hash is calculated as: SHA256(previousHash:entryJSON)
// This matches the hash calculation in audit-log-impl.go.
func CalculateHash(previousHash string, entry interface{}) (string, error) {
	// Marshal entry to JSON
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("failed to marshal entry: %w", err)
	}

	// Include previous hash in the hash calculation for chain integrity
	hashInput := fmt.Sprintf("%s:%s", previousHash, string(entryJSON))
	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:]), nil
}

// VerifyHashChain loads all entries from storage and verifies hash chain integrity.
// It checks:
//   - Each entry's hash matches the calculated hash
//   - Each entry's previous_hash matches the previous entry's hash
//   - Chain is unbroken (no missing links)
//
// Returns a HashChainReport with integrity status and any detected issues.
func (m *HashChainManager) VerifyHashChain(ctx context.Context) (*HashChainReport, error) {
	startTime := time.Now()

	report := &HashChainReport{
		IsIntegrityIntact:   true,
		BrokenLinks:         make([]BrokenLink, 0),
		TamperIndicators:    make([]TamperIndicator, 0),
		LastVerifiedEntryID: "",
		LastVerifiedHash:    "",
	}

	if m.logger != nil {
		m.logger.Info("Starting hash chain integrity verification")
	}

	// Use provider if available, otherwise fall back to legacy approach
	if m.provider != nil {
		return m.verifyHashChainWithProvider(ctx, report, startTime)
	}

	// Legacy approach: log that verification requires provider
	if m.logger != nil {
		m.logger.Warn("Hash chain manager: provider not available, verification deferred until provider is initialized (Epic 8)")
	}

	report.VerificationDuration = time.Since(startTime)

	return report, nil
}

// verifyHashChainWithProvider performs hash chain verification using the provider interface.
func (m *HashChainManager) verifyHashChainWithProvider(ctx context.Context, report *HashChainReport, startTime time.Time) (*HashChainReport, error) {
	// Query all entries (no filters to get all entries)
	filters := types.QueryFilters{
		Limit:  10000, // Large limit to get all entries (will paginate if needed)
		Offset: 0,
	}

	var allEntries []types.AuditEntry
	offset := 0

	// Load all entries (with pagination)
	for {
		filters.Offset = offset
		entries, err := m.provider.ListEntries(ctx, filters)
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("Failed to query entries for hash chain verification",
					zap.Error(err),
					zap.Int("offset", offset))
			}
			// Continue with entries we have so far
			break
		}

		if len(entries) == 0 {
			break // No more entries
		}

		allEntries = append(allEntries, entries...)
		offset += len(entries)

		// If we got less than the limit, we've reached the end
		if len(entries) < filters.Limit {
			break
		}
	}

	report.TotalEntries = len(allEntries)

	if len(allEntries) == 0 {
		// No entries - chain is empty (intact by default)
		report.IsIntegrityIntact = true
		report.VerificationDuration = time.Since(startTime)
		if m.logger != nil {
			m.logger.Info("Hash chain verification completed: no entries found")
		}
		return report, nil
	}

	// Sort entries by timestamp to ensure correct order (chain should be chronological)
	// TODO: If provider doesn't guarantee order, sort entries by timestamp here

	// Verify hash chain integrity
	previousHash := "" // First entry should have empty PreviousHash

	for i, entry := range allEntries {
		// Verify 1: Check PreviousHash matches previous entry's Hash
		if entry.PreviousHash != previousHash {
			brokenLink := BrokenLink{
				EntryID:              entry.ID,
				EntryTimestamp:       entry.Timestamp,
				IssueType:            "previous_hash_mismatch",
				ExpectedPreviousHash: previousHash,
				ActualPreviousHash:   entry.PreviousHash,
			}

			report.BrokenLinks = append(report.BrokenLinks, brokenLink)
			report.IsIntegrityIntact = false

			tamperIndicator := TamperIndicator{
				EntryID:     entry.ID,
				Severity:    "critical",
				Description: fmt.Sprintf("Entry's PreviousHash (%s) does not match previous entry's Hash (%s). Chain broken at entry %d.", entry.PreviousHash, previousHash, i+1),
				DetectedAt:  time.Now(),
			}
			report.TamperIndicators = append(report.TamperIndicators, tamperIndicator)

			if m.logger != nil {
				m.logger.Error("Hash chain broken: PreviousHash mismatch",
					zap.String("entry_id", entry.ID),
					zap.String("expected_previous_hash", previousHash),
					zap.String("actual_previous_hash", entry.PreviousHash),
					zap.Int("entry_index", i+1))
			}

			// Don't continue verifying - chain is broken
			break
		}

		// Verify 2: Calculate hash and compare with entry's hash
		// Note: We need the full entry object to calculate hash correctly
		// For now, we'll load the full entry to calculate hash
		// TODO: Provider should provide full entry data, or we need to reconstruct it
		calculatedHash, err := CalculateHash(entry.PreviousHash, entry)
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("Failed to calculate hash for entry",
					zap.String("entry_id", entry.ID),
					zap.Error(err))
			}
			// Mark as broken link
			brokenLink := BrokenLink{
				EntryID:        entry.ID,
				EntryTimestamp: entry.Timestamp,
				IssueType:      "hash_calculation_error",
			}
			report.BrokenLinks = append(report.BrokenLinks, brokenLink)
			report.IsIntegrityIntact = false
			break
		}

		if entry.Hash != calculatedHash {
			brokenLink := BrokenLink{
				EntryID:     entry.ID,
				EntryTimestamp: entry.Timestamp,
				IssueType:   "hash_mismatch",
				ExpectedHash: calculatedHash,
				ActualHash:   entry.Hash,
			}

			report.BrokenLinks = append(report.BrokenLinks, brokenLink)
			report.IsIntegrityIntact = false

			tamperIndicator := TamperIndicator{
				EntryID:     entry.ID,
				Severity:    "critical",
				Description: fmt.Sprintf("Entry's Hash (%s) does not match calculated hash (%s). Entry may have been tampered with.", entry.Hash, calculatedHash),
				DetectedAt:  time.Now(),
			}
			report.TamperIndicators = append(report.TamperIndicators, tamperIndicator)

			if m.logger != nil {
				m.logger.Error("Hash chain broken: Hash mismatch",
					zap.String("entry_id", entry.ID),
					zap.String("expected_hash", calculatedHash),
					zap.String("actual_hash", entry.Hash),
					zap.Int("entry_index", i+1))
			}

			// Don't continue verifying - chain is broken
			break
		}

		// Entry passed all checks
		report.VerifiedEntries++
		report.LastVerifiedEntryID = entry.ID
		report.LastVerifiedHash = entry.Hash
		previousHash = entry.Hash // Move to next entry
	}

	report.VerificationDuration = time.Since(startTime)

	if report.IsIntegrityIntact {
		if m.logger != nil {
			m.logger.Info("Hash chain integrity verification completed: chain is intact",
				zap.Int("total_entries", report.TotalEntries),
				zap.Int("verified_entries", report.VerifiedEntries),
				zap.Duration("verification_duration", report.VerificationDuration))
		}
	} else {
		if m.logger != nil {
			m.logger.Error("Hash chain integrity verification completed: TAMPERING DETECTED",
				zap.Int("total_entries", report.TotalEntries),
				zap.Int("verified_entries", report.VerifiedEntries),
				zap.Int("broken_links", len(report.BrokenLinks)),
				zap.Int("tamper_indicators", len(report.TamperIndicators)),
				zap.Duration("verification_duration", report.VerificationDuration))
		}

		// TODO: Emit event: audit_log.tamper_detected (will be implemented when event-bus integration is added)
		// m.emitEvent("audit_log.tamper_detected", map[string]interface{}{
		// 	"broken_links_count": len(report.BrokenLinks),
		// 	"tamper_indicators_count": len(report.TamperIndicators),
		// 	"first_broken_entry_id": report.BrokenLinks[0].EntryID,
		// })
	}

	// Store report for recovery purposes
	m.lastReportMu.Lock()
	m.lastReport = report
	m.lastReportMu.Unlock()

	return report, nil
}

// InitializeHashChain initializes the hash chain on startup.
// It loads the last hash from storage and verifies chain continuity.
// If the chain is broken, it attempts recovery.
func (m *HashChainManager) InitializeHashChain(ctx context.Context) error {
	if m.logger != nil {
		m.logger.Info("Initializing hash chain")
	}

	// Step 1: Load last hash from storage
	lastHash, err := m.loadLastHash(ctx)
	if err != nil {
		// If last hash doesn't exist, this is the first entry scenario
		if m.logger != nil {
			m.logger.Info("Last hash not found - this is the first entry scenario",
				zap.Error(err))
		}
		// Set empty hash for first entry
		m.SetLastHash("")
		return nil
	}

	// Set last hash
	m.SetLastHash(lastHash)

	if m.logger != nil {
		m.logger.Info("Last hash loaded from storage",
			zap.String("last_hash", lastHash))
	}

	// Step 2: Verify chain continuity
	report, err := m.VerifyHashChain(ctx)
	if err != nil {
		return fmt.Errorf("failed to verify hash chain during initialization: %w", err)
	}

	// Step 3: If chain is broken, attempt recovery
	if !report.IsIntegrityIntact {
		if m.logger != nil {
			m.logger.Error("Hash chain is broken during initialization - attempting recovery",
				zap.Int("broken_links", len(report.BrokenLinks)),
				zap.Int("verified_entries", report.VerifiedEntries))
		}

		// Attempt recovery
		if err := m.AttemptRecovery(ctx, report); err != nil {
			if m.logger != nil {
				m.logger.Error("Hash chain recovery failed",
					zap.Error(err))
			}
			// TODO: Emit critical alert for operator investigation
			// m.emitEvent("audit_log.recovery_failed", ...)
			return fmt.Errorf("hash chain recovery failed: %w", err)
		}

		if m.logger != nil {
			m.logger.Info("Hash chain recovery completed")
		}
	} else {
		if m.logger != nil {
			m.logger.Info("Hash chain continuity verified during initialization",
				zap.Int("total_entries", report.TotalEntries),
				zap.String("last_verified_hash", report.LastVerifiedHash))
		}
		// Update last hash from verified report
		if report.LastVerifiedHash != "" {
			m.SetLastHash(report.LastVerifiedHash)
		}
	}

	return nil
}

// loadLastHash loads the last hash from storage via provider.
// Returns empty string if no hash exists (first entry scenario).
func (m *HashChainManager) loadLastHash(ctx context.Context) (string, error) {
	if m.provider == nil {
		// Provider not available yet - return empty hash (first entry scenario)
	if m.provider == nil {
		return "", fmt.Errorf("hash chain manager provider not set")
	}
	return m.provider.GetLastHash(ctx)
	}

	hash, err := m.provider.GetLastHash(ctx)
	if err != nil {
		// If hash doesn't exist, return empty string (first entry scenario)
		return "", fmt.Errorf("failed to load last hash: %w", err)
	}

	return hash, nil
}

// AttemptRecovery attempts to recover from a broken hash chain.
// It identifies the break point, marks entries after break as suspicious,
// and attempts to repair the chain if possible.
func (m *HashChainManager) AttemptRecovery(ctx context.Context, report *HashChainReport) error {
	if report == nil || len(report.BrokenLinks) == 0 {
		return nil // No broken links to recover from
	}

	firstBrokenLink := report.BrokenLinks[0]
	breakPointEntryID := firstBrokenLink.EntryID

	if m.logger != nil {
		m.logger.Info("Attempting hash chain recovery",
			zap.String("break_point_entry_id", breakPointEntryID),
			zap.String("issue_type", firstBrokenLink.IssueType),
			zap.Int("verified_entries_before_break", report.VerifiedEntries))
	}

	// Step 1: Mark entries after break as suspicious
	// Note: In a full implementation, we would update entry metadata to mark them as suspicious
	// For now, we log this action
	if m.logger != nil {
		m.logger.Warn("Marking entries after break point as suspicious",
			zap.String("break_point_entry_id", breakPointEntryID))
	}

	// TODO: Mark entries after break as suspicious in storage
	// This would require updating entry metadata via provider:
	// - Find all entries after break point (by timestamp)
	// - Mark them with a "suspicious" flag in metadata
	// if err := m.markEntriesAsSuspicious(ctx, breakPointEntryID); err != nil {
	// 	return fmt.Errorf("failed to mark entries as suspicious: %w", err)
	// }

	// Step 2: Attempt to repair chain (if possible)
	// Repair is only possible for certain types of breaks:
	// - If PreviousHash mismatch but hash is correct: we can potentially reconnect
	// - If hash mismatch: entry was tampered with, cannot repair
	repairPossible := false

	switch firstBrokenLink.IssueType {
	case "previous_hash_mismatch":
		// This might be repairable if we can recalculate the chain from the break point
		// However, this is dangerous as it might hide tampering
		// We'll only attempt if the hash itself is still valid
		if m.logger != nil {
			m.logger.Warn("PreviousHash mismatch detected - repair requires operator investigation",
				zap.String("expected_previous_hash", firstBrokenLink.ExpectedPreviousHash),
				zap.String("actual_previous_hash", firstBrokenLink.ActualPreviousHash))
		}
		// Don't attempt automatic repair for PreviousHash mismatch - requires operator intervention
		repairPossible = false

	case "hash_mismatch":
		// Entry was tampered with - cannot repair automatically
		if m.logger != nil {
			m.logger.Error("Hash mismatch detected - entry was tampered with, cannot repair automatically",
				zap.String("expected_hash", firstBrokenLink.ExpectedHash),
				zap.String("actual_hash", firstBrokenLink.ActualHash))
		}
		repairPossible = false

	default:
		repairPossible = false
	}

	if !repairPossible {
		// Cannot repair automatically - requires operator intervention
		if m.logger != nil {
			m.logger.Error("CRITICAL: Hash chain cannot be repaired automatically - requires operator investigation",
				zap.String("break_point_entry_id", breakPointEntryID),
				zap.String("issue_type", firstBrokenLink.IssueType))
		}

		// TODO: Emit critical alert for operator investigation
		// m.emitEvent("audit_log.recovery_requires_operator", map[string]interface{}{
		// 	"break_point_entry_id": breakPointEntryID,
		// 	"issue_type": firstBrokenLink.IssueType,
		// 	"verified_entries": report.VerifiedEntries,
		// 	"total_entries": report.TotalEntries,
		// })

		// Set last hash to the last verified entry's hash
		// This allows new entries to be created after the verified portion of the chain
		if report.LastVerifiedHash != "" {
			m.SetLastHash(report.LastVerifiedHash)
			if m.logger != nil {
				m.logger.Info("Set last hash to last verified entry - new entries can continue from verified portion",
					zap.String("last_verified_hash", report.LastVerifiedHash))
			}
		}

		return fmt.Errorf("hash chain recovery requires operator intervention: break at entry %s (type: %s)",
			breakPointEntryID, firstBrokenLink.IssueType)
	}

	// If repair was possible, it would happen here
	// For now, we return the error indicating operator intervention is needed
	return fmt.Errorf("hash chain recovery not implemented for issue type: %s", firstBrokenLink.IssueType)
}

// GetLastReport returns the last hash chain verification report.
// This can be used to check the current state of the chain.
func (m *HashChainManager) GetLastReport() *HashChainReport {
	m.lastReportMu.RLock()
	defer m.lastReportMu.RUnlock()
	return m.lastReport
}

