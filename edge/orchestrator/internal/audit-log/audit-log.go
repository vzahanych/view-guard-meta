package auditlog

import (
	"context"
	"fmt"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/impl"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/audit-log/types"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// NewAuditLogService creates a new audit log service instance
func NewAuditLogService(
	config *types.AuditLogConfig,
	objectStorage objectstorage.ObjectStorageService,
	vmGateway vmgateway.VMGateway,
	logger *zap.Logger,
	edgeID string,
) AuditLogService {
	return impl.NewAuditLogService(config, objectStorage, vmGateway, logger, edgeID)
}

// AuditLogProvider creates the audit log service with fx lifecycle management
func AuditLogProvider(
	lc fx.Lifecycle,
	config *types.AuditLogConfig,
	objectStorage objectstorage.ObjectStorageService,
	vmGateway vmgateway.VMGateway,
	logger *zap.Logger,
	edgeID string,
) (AuditLogService, error) {
	service := NewAuditLogService(config, objectStorage, vmGateway, logger, edgeID)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := service.Start(ctx); err != nil {
				return fmt.Errorf("failed to start audit log service: %w", err)
			}
			logger.Info("Audit log service started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if err := service.Stop(ctx); err != nil {
				return fmt.Errorf("failed to stop audit log service: %w", err)
			}
			logger.Info("Audit log service stopped")
			return nil
		},
	})

	return service, nil
}

