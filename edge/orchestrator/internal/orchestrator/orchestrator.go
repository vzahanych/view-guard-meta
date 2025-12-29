package orchestrator

import (
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/config"
	aigateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway"
	aigwtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/types"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	evtbustypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	iot "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot"
	cctv "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	iottypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/types"
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
// 2. MetaStorage (depends on Config) - must be created before EventBus if using "metastorage" provider
// 3. EventBus (depends on Config and optionally MetaStorage)
// 4. ObjectStorage (depends on Config and MetaStorage for startup ordering)
// 5. VMGateway (depends on Config and ObjectStorage for startup ordering)
// 6. CCTV (depends on Config and VMGateway for startup ordering)
// 7. AIGateway (depends on Config, CCTV, and ObjectStorage for startup ordering)
func Module() fx.Option {
	return fx.Options(
		fx.Provide(
			EdgeIDProvider,
			// Provide MetaStorageConfig from Config (MetaStorage must be created before EventBus)
			func(cfg *config.Config) *metastoragetypes.MetaStorageConfig {
				return &cfg.MetaStorage
			},
			// MetaStorageProvider - must be created before EventBus
			// Meta-storage is required for event bus (event bus uses metastorage provider)
			// Dependencies: Config (via MetaStorageConfig)
			metastorage.MetaStorageProvider,
			// Provide EventBusConfig from Config (EventBus depends on Config and MetaStorage)
			func(cfg *config.Config) *evtbustypes.EventBusConfig {
				return &cfg.EventBus
			},
			// EventBusProvider - depends on Config and MetaStorage
			// MetaStorage dependency ensures MetaStorage is created before EventBus
			func(cfg *config.Config, lc fx.Lifecycle, evtCfg *evtbustypes.EventBusConfig, logger *zap.Logger, metaStore metastorage.MetaDataStore) (eventbus.EventBus, error) {
				// Event bus always uses meta-storage provider
				metaStorePtr := &metaStore
				return eventbus.EventBusProvider(lc, evtCfg, logger, metaStorePtr)
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
			// Provide IoTServiceConfig from Config (optional - can be nil for defaults)
			// If IoT config is not in Config, this will return nil (service will use defaults)
			func(cfg *config.Config) *iottypes.IoTServiceConfig {
				// TODO: Add IoT config to Config struct when needed
				// For now, return nil to use default config
				return nil
			},
			// IoTServiceProvider - creates all subcomponents internally
			// Dependencies: Config (optional), Logger
			// Note: IoT service is self-contained and creates all subcomponents internally
			// (plugin registry, device registry, state registry, processing service, hook registry)
			iot.IoTServiceProvider,
			// CCTVDevicePluginProvider - registers CCTV as a device plugin with IoTService
			// Dependencies: fx.Lifecycle, CCTVService, IoTService, Logger
			// Note: This provider must be called after both CCTVService and IoTService are provided
			// The plugin is registered with IoTService on startup via a lifecycle hook
			cctv.CCTVDevicePluginProvider,
			// Provide AIGatewayConfig from Config
			func(cfg *config.Config) *aigwtypes.AIGatewayConfig {
				return &cfg.AI
			},
			// AIGatewayProvider - depends on CCTV and ObjectStorage (for startup ordering)
			// Dependencies ensure: ObjectStorage starts before AIGateway, and CCTV starts before AIGateway
			func(cctvService cctv.CCTVService, objectStore objectstorage.ObjectStorageService, lc fx.Lifecycle, aiCfg *aigwtypes.AIGatewayConfig, metaStore metastorage.MetaDataStore, eventBus eventbus.EventBus, logger *zap.Logger) (aigateway.AIGateway, error) {
				return aigateway.AIGatewayProvider(lc, aiCfg, cctvService, objectStore, metaStore, eventBus, logger)
			},
			// Provide VMGatewayConfig from Config
			// Use config values from VMGatewayConfig, with defaults only if not set
			func(cfg *config.Config, edgeID string) *vmgatewaytypes.VMGatewayConfig {
				// Start with a copy of the config from the file
				vmCfg := cfg.VMGateway
				vmCfg.EdgeID = edgeID

				// Set defaults for HTTPS server config if not provided
				if vmCfg.HTTPServerConfig.ListenAddress == "" {
					// Default to tunnel IP for production, localhost for dev
					if vmCfg.Tunnel.Enabled {
						vmCfg.HTTPServerConfig.ListenAddress = "10.0.0.2:8443"
					} else {
						vmCfg.HTTPServerConfig.ListenAddress = "localhost:8443" // Development mode
					}
				}
				if vmCfg.HTTPServerConfig.ReadTimeout == 0 {
					vmCfg.HTTPServerConfig.ReadTimeout = 30 * time.Second
				}
				if vmCfg.HTTPServerConfig.WriteTimeout == 0 {
					vmCfg.HTTPServerConfig.WriteTimeout = 30 * time.Second
				}
				if vmCfg.HTTPServerConfig.IdleTimeout == 0 {
					vmCfg.HTTPServerConfig.IdleTimeout = 120 * time.Second
				}
				if vmCfg.HTTPServerConfig.TunnelInterfaceWaitTimeout == 0 {
					vmCfg.HTTPServerConfig.TunnelInterfaceWaitTimeout = 30 * time.Second
				}
				if vmCfg.HTTPServerConfig.TunnelInterfaceCheckInterval == 0 {
					vmCfg.HTTPServerConfig.TunnelInterfaceCheckInterval = 500 * time.Millisecond
				}
				if vmCfg.HTTPServerConfig.MultipartFormMaxMemory == 0 {
					vmCfg.HTTPServerConfig.MultipartFormMaxMemory = 10 << 20 // 10MB
				}

				// Set defaults for HTTPS client config if not provided
				if vmCfg.HTTPSClientConfig.VMEndpoint == "" {
					// Default to tunnel endpoint or localhost for dev
					if vmCfg.Tunnel.Enabled {
						vmCfg.HTTPSClientConfig.VMEndpoint = "10.0.0.1:8443"
					} else {
						vmCfg.HTTPSClientConfig.VMEndpoint = "localhost:8443" // Development mode
					}
				}
				if vmCfg.HTTPSClientConfig.Timeout == 0 {
					vmCfg.HTTPSClientConfig.Timeout = 30 * time.Second
				}

				return &vmCfg
			},
			// VMGatewayProvider - depends on ObjectStorage (for startup ordering)
			// ObjectStorage dependency ensures ObjectStorage starts before VMGateway
			func(objectStore objectstorage.ObjectStorageService, lc fx.Lifecycle, vmCfg *vmgatewaytypes.VMGatewayConfig, metaStore metastorage.MetaDataStore, eventBus eventbus.EventBus, logger *zap.Logger) (vmgateway.VMGateway, error) {
				return vmgateway.VMGatewayProvider(lc, vmCfg, metaStore, objectStore, eventBus, logger)
			},
			// Provide CCTVServiceConfig from Config
			func(cfg *config.Config) *cctvtypes.CCTVServiceConfig {
				// Use CCTV field from Config
				return &cfg.CCTV
			},
			// CCTVServiceProvider - depends on MetaStorage and ObjectStorage (for startup ordering)
			func(lc fx.Lifecycle, cctvCfg *cctvtypes.CCTVServiceConfig, metaStore metastorage.MetaDataStore, objectStore objectstorage.ObjectStorageService, eventBus eventbus.EventBus, logger *zap.Logger) (cctv.CCTVService, error) {
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
