package impl

import (
	"context"
	"sync"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
)

// TestProvider is a simple in-memory implementation of AuditLogProvider for testing.
type TestProvider struct {
	mu           sync.RWMutex
	entries      map[string]types.AuditEntry
	lastHash     string
	healthStatus string
}

// NewTestProvider creates a new test provider.
func NewTestProvider() *TestProvider {
	return &TestProvider{
		entries:      make(map[string]types.AuditEntry),
		healthStatus: "healthy",
	}
}

// SaveEntry saves an audit log entry to memory.
func (p *TestProvider) SaveEntry(ctx context.Context, entry types.AuditEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries[entry.ID] = entry
	return nil
}

// LoadEntry loads an audit log entry from memory by ID.
func (p *TestProvider) LoadEntry(ctx context.Context, entryID string) (*types.AuditEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	entry, ok := p.entries[entryID]
	if !ok {
		return nil, types.ErrRecordNotFound
	}
	return &entry, nil
}

// ListEntries lists audit log entries matching the provided filters.
func (p *TestProvider) ListEntries(ctx context.Context, filters types.QueryFilters) ([]types.AuditEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var entries []types.AuditEntry
	for _, entry := range p.entries {
		// Simple filter matching
		if filters.StartTime != nil && entry.Timestamp.Before(*filters.StartTime) {
			continue
		}
		if filters.EndTime != nil && entry.Timestamp.After(*filters.EndTime) {
			continue
		}
		if filters.EntryType != "" && string(entry.Type) != filters.EntryType {
			continue
		}
		if filters.UserID != "" && entry.UserID != filters.UserID {
			continue
		}
		if filters.IPAddress != "" && entry.IPAddress != filters.IPAddress {
			continue
		}
		if filters.Result != "" && entry.Result != filters.Result {
			continue
		}
		entries = append(entries, entry)
	}

	// Apply limit and offset
	start := filters.Offset
	if start < 0 {
		start = 0
	}
	end := start + filters.Limit
	if filters.Limit <= 0 {
		end = len(entries)
	}
	if end > len(entries) {
		end = len(entries)
	}
	if start >= len(entries) {
		return []types.AuditEntry{}, nil
	}

	return entries[start:end], nil
}

// DeleteEntry deletes an audit log entry from memory by ID.
func (p *TestProvider) DeleteEntry(ctx context.Context, entryID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, entryID)
	return nil
}

// GetLastHash retrieves the hash of the last audit log entry in the chain.
func (p *TestProvider) GetLastHash(ctx context.Context) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastHash, nil
}

// SaveLastHash saves the hash of the last audit log entry in the chain.
func (p *TestProvider) SaveLastHash(ctx context.Context, hash string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastHash = hash
	return nil
}

// HealthCheck performs a health check on the provider.
func (p *TestProvider) HealthCheck(ctx context.Context) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.healthStatus == "" {
		return "healthy", nil
	}
	return p.healthStatus, nil
}

// SetHealthStatus sets the health status for testing.
func (p *TestProvider) SetHealthStatus(status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthStatus = status
}

// Clear clears all entries from the test provider.
func (p *TestProvider) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries = make(map[string]types.AuditEntry)
	p.lastHash = ""
}

// GetEntryCount returns the number of entries in the provider.
func (p *TestProvider) GetEntryCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.entries)
}
