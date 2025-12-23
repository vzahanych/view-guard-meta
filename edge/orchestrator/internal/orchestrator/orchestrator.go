package orchestrator

import (
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/config"
	aigateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway"
	aigwtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/types"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	evtbustypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	cctv "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	metastoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage/types"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	objectstoragetypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
	impl "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/orchestrator/impl"
	statemng "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state-mng"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
	webgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway"
	webgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway/types"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Note: the legacy Orchestrator interface has been removed.
// Fx now manages the full lifecycle via Module() and impl.Server.

// Module returns the fx module with all service providers
// Startup order is enforced via dependencies:
// 1. Config (provided first, no dependencies)
// 2. EventBus (depends on Config)
// 3. MetaStorage (depends on Config and EventBus for startup ordering)
// 4. ObjectStorage (depends on Config and MetaStorage for startup ordering)
// 5. VMGateway (depends on Config and ObjectStorage for startup ordering)
// 6. CCTV (depends on Config and VMGateway for startup ordering)
// 7. AIGateway (depends on Config, CCTV, and ObjectStorage for startup ordering)
func Module() fx.Option {
	return fx.Options(
		fx.Provide(
			EdgeIDProvider,
			// Provide EventBusConfig from Config (EventBus depends on Config)
			func(cfg *config.Config) *evtbustypes.EventBusConfig {
				return &cfg.EventBus
			},
			// EventBusProvider - depends on Config, will start after Config
			eventbus.EventBusProvider,
			// Provide MetaStorageConfig from Config
			func(cfg *config.Config) *metastoragetypes.MetaStorageConfig {
				return &cfg.MetaStorage
			},
			// MetaStorageProvider - depends on Config and EventBus (for startup ordering)
			// EventBus dependency ensures EventBus starts before MetaStorage
			func(cfg *config.Config, eventBus eventbus.EventBus, lc fx.Lifecycle, metaCfg *metastoragetypes.MetaStorageConfig, logger *zap.Logger) (metastorage.MetaDataStore, error) {
				return metastorage.MetaStorageProvider(lc, metaCfg, logger)
			},
			// Provide ObjectStorageConfig from Config
			func(cfg *config.Config) *objectstoragetypes.ObjectStorageConfig {
				return &cfg.ObjectStorage
			},
			// ObjectStorageProvider - depends on Config and MetaStorage (for startup ordering)
			// MetaStorage dependency ensures MetaStorage starts before ObjectStorage
			func(cfg *config.Config, metaStore metastorage.MetaDataStore, lc fx.Lifecycle, objCfg *objectstoragetypes.ObjectStorageConfig, logger *zap.Logger) (objectstorage.ObjectStorageService, error) {
				return objectstorage.ObjectStorageProvider(lc, objCfg, logger)
			},
			// StateManagerProvider from internal/state-mng
			statemng.StateManagerProvider,
			// Provide AIGatewayConfig from Config
			func(cfg *config.Config) *aigwtypes.AIGatewayConfig {
				return &cfg.AI
			},
			// AIGatewayProvider - depends on Config, CCTV, and ObjectStorage (for startup ordering)
			// Dependencies ensure: ObjectStorage starts before AIGateway, and CCTV starts before AIGateway
			// Note: VMGateway dependency is handled implicitly via CCTV dependency (CCTV depends on VMGateway)
			func(cfg *config.Config, cctvService cctv.CCTVService, objectStore objectstorage.ObjectStorageService, lc fx.Lifecycle, aiCfg *aigwtypes.AIGatewayConfig, metaStore metastorage.MetaDataStore, eventBus eventbus.EventBus, logger *zap.Logger) (aigateway.AIGateway, error) {
				return aigateway.AIGatewayProvider(lc, aiCfg, cctvService, objectStore, metaStore, eventBus, logger)
			},
			// Provide VMGatewayConfig from Config
			// Use config values from VMGatewayConfig, with defaults only if not set
			func(cfg *config.Config, edgeID string) *vmgatewaytypes.VMGatewayConfig {
				// Use HTTPS server config from config file, with defaults if not set
				httpsServerCfg := cfg.VMGateway.HTTPServerConfig
				if httpsServerCfg.ListenAddress == "" {
					// Default to WireGuard IP for production, localhost for dev
					if cfg.VMGateway.WireGuard.Enabled {
						httpsServerCfg.ListenAddress = "10.0.0.2:8443"
					} else {
						httpsServerCfg.ListenAddress = "localhost:8443" // Development mode
					}
				}
				if httpsServerCfg.ReadTimeout == 0 {
					httpsServerCfg.ReadTimeout = 30 * time.Second
				}
				if httpsServerCfg.WriteTimeout == 0 {
					httpsServerCfg.WriteTimeout = 30 * time.Second
				}
				if httpsServerCfg.IdleTimeout == 0 {
					httpsServerCfg.IdleTimeout = 120 * time.Second
				}
				if httpsServerCfg.WireGuardInterfaceWaitTimeout == 0 {
					httpsServerCfg.WireGuardInterfaceWaitTimeout = 30 * time.Second
				}
				if httpsServerCfg.WireGuardInterfaceCheckInterval == 0 {
					httpsServerCfg.WireGuardInterfaceCheckInterval = 500 * time.Millisecond
				}
				if httpsServerCfg.MultipartFormMaxMemory == 0 {
					httpsServerCfg.MultipartFormMaxMemory = 10 << 20 // 10MB
				}

				// Use HTTPS client config from config file, with defaults if not set
				httpsClientCfg := cfg.VMGateway.HTTPSClientConfig
				if httpsClientCfg.VMEndpoint == "" {
					// Default to WireGuard endpoint or localhost for dev
					if cfg.VMGateway.WireGuard.Enabled {
						httpsClientCfg.VMEndpoint = cfg.VMGateway.WireGuard.KVMEndpoint
						if httpsClientCfg.VMEndpoint == "" {
							httpsClientCfg.VMEndpoint = "10.0.0.1:8443"
						}
					} else {
						httpsClientCfg.VMEndpoint = "localhost:8443" // Development mode
					}
				}
				if httpsClientCfg.Timeout == 0 {
					httpsClientCfg.Timeout = 30 * time.Second
				}

				return &vmgatewaytypes.VMGatewayConfig{
					Provider:          cfg.VMGateway.Provider,
					WireGuard:         cfg.VMGateway.WireGuard,
					HTTPServerConfig:  httpsServerCfg,
					HTTPSClientConfig: httpsClientCfg,
					EdgeID:            edgeID,
				}
			},
			// VMGatewayProvider - depends on Config and ObjectStorage (for startup ordering)
			// ObjectStorage dependency ensures ObjectStorage starts before VMGateway
			func(cfg *config.Config, objectStore objectstorage.ObjectStorageService, lc fx.Lifecycle, vmCfg *vmgatewaytypes.VMGatewayConfig, metaStore metastorage.MetaDataStore, eventBus eventbus.EventBus, logger *zap.Logger) (vmgateway.VMGateway, error) {
				return vmgateway.VMGatewayProvider(lc, vmCfg, metaStore, objectStore, eventBus, logger)
			},
			// Provide CCTVServiceConfig from Config
			func(cfg *config.Config) *cctvtypes.CCTVServiceConfig {
				// Use CCTV field from Config
				return &cfg.CCTV
			},
			// CCTVServiceProvider - depends on Config and VMGateway (for startup ordering)
			// VMGateway dependency ensures VMGateway starts before CCTV
			func(cfg *config.Config, vmGateway vmgateway.VMGateway, lc fx.Lifecycle, cctvCfg *cctvtypes.CCTVServiceConfig, metaStore metastorage.MetaDataStore, objectStore objectstorage.ObjectStorageService, eventBus eventbus.EventBus, logger *zap.Logger) (cctv.CCTVService, error) {
				return cctv.CCTVServiceProvider(lc, cctvCfg, metaStore, objectStore, eventBus, logger)
			},
			// Provide WebGatewayConfig from Config
			func(cfg *config.Config) *webgatewaytypes.WebGatewayConfig {
				return &cfg.WebGateway
			},
			// Web gateway provider with fx lifecycle management
			// Depends on MetaStorage, ObjectStorage, CCTVService, VMGateway, and EventBus (for startup ordering)
			func(cfg *config.Config, metaStore metastorage.MetaDataStore, objectStore objectstorage.ObjectStorageService, cctvService cctv.CCTVService, vmGateway vmgateway.VMGateway, eventBus eventbus.EventBus, lc fx.Lifecycle, webCfg *webgatewaytypes.WebGatewayConfig, logger *zap.Logger) (webgateway.WebGateway, error) {
				return webgateway.WebGatewayProvider(lc, webCfg, metaStore, objectStore, cctvService, vmGateway, eventBus, logger)
			},
			// Server constructor - wires all services together
			func(
				edgeID string,
				cfg *config.Config,
				logger *zap.Logger,
				eventBus eventbus.EventBus,
				metaStorage metastorage.MetaDataStore,
				objectStorage objectstorage.ObjectStorageService,
				edgeStateMgr statemng.StateManager,
				cctvService cctv.CCTVService,
				aiGateway aigateway.AIGateway,
				vmGateway vmgateway.VMGateway,
				webGateway webgateway.WebGateway,
			) *impl.Server {
				return impl.NewServer(
					edgeID,
					cfg,
					logger,
					eventBus,
					metaStorage,
					objectStorage,
					edgeStateMgr,
					cctvService,
					aiGateway,
					vmGateway,
					webGateway,
					nil, // telemetryProvider - can be added later
				)
			},
		),
	)
}
