package metastorage

import (
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	metastoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	"go.uber.org/zap"
)

// NewAuditLogProviderFromMetaStorage creates an AuditLogProvider from a MetaDataStore.
// This is a convenience function for dependency injection when meta-storage is available.
func NewAuditLogProviderFromMetaStorage(
	metaStore metastorage.MetaDataStore,
	logger *zap.Logger,
) (types.AuditLogProvider, error) {
	if metaStore == nil {
		return nil, fmt.Errorf("meta storage store cannot be nil")
	}

	// MetaDataStore doesn't expose the underlying MetaStorageProvider directly
	// We need to extract it. For now, we'll create a new provider using the same approach
	// that meta-storage uses internally, but this requires access to the provider.
	// This is a limitation - we should expose GetProvider() or similar from MetaDataStore.
	
	// TODO: Add GetProvider() method to MetaDataStore interface or find another way
	// For now, return error indicating this needs to be implemented
	return nil, fmt.Errorf("creating AuditLogProvider from MetaDataStore requires access to underlying MetaStorageProvider - not yet implemented")
}

// NewAuditLogProviderFromMetaStorageProvider creates an AuditLogProvider from a MetaStorageProvider.
// This is the preferred way to create the provider when MetaStorageProvider is directly available.
func NewAuditLogProviderFromMetaStorageProvider(
	provider metastoragetypes.MetaStorageProvider,
	logger *zap.Logger,
) (types.AuditLogProvider, error) {
	return NewMetaStorageProvider(provider, logger)
}

