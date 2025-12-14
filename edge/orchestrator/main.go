package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/anomaly"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/capabilities"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/dataset"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/deployment"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	grpcclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/grpc"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/health"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/snapshot_request"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/telemetry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/streaming"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"
)

// telemetryCollectorAdapter adapts telemetry.Collector to web.TelemetryCollector interface
type telemetryCollectorAdapter struct {
	collector *telemetry.Collector
}

func (a *telemetryCollectorAdapter) GetLastMetrics() interface{} {
	return a.collector.GetLastMetrics()
}

func (a *telemetryCollectorAdapter) Collect(ctx context.Context) (interface{}, error) {
	return a.collector.Collect(ctx)
}

// modelLoaderAdapter adapts ai.ModelLoader to web.ModelLoaderService interface
type modelLoaderAdapter struct {
	loader *ai.ModelLoader
}

func (a *modelLoaderAdapter) LoadModel(ctx context.Context, modelID string, cameraID *string) (*web.ActiveModelInfo, error) {
	aiModel, err := a.loader.LoadModel(ctx, modelID, cameraID)
	if err != nil {
		return nil, err
	}
	// Convert ai.ActiveModelInfo to web.ActiveModelInfo
	return &web.ActiveModelInfo{
		ModelID:      aiModel.ModelID,
		ModelPath:    aiModel.ModelPath,
		MetadataPath: aiModel.MetadataPath,
		Version:      aiModel.Version,
		ModelType:    aiModel.ModelType,
		Framework:    aiModel.Framework,
		CameraID:     aiModel.CameraID,
		LoadedAt:     aiModel.LoadedAt,
		Ready:        aiModel.Ready,
	}, nil
}

func (a *modelLoaderAdapter) GetActiveModel(cameraID string) (*web.ActiveModelInfo, bool) {
	aiModel, exists := a.loader.GetActiveModel(cameraID)
	if !exists {
		return nil, false
	}
	// Convert ai.ActiveModelInfo to web.ActiveModelInfo
	return &web.ActiveModelInfo{
		ModelID:      aiModel.ModelID,
		ModelPath:    aiModel.ModelPath,
		MetadataPath: aiModel.MetadataPath,
		Version:      aiModel.Version,
		ModelType:    aiModel.ModelType,
		Framework:    aiModel.Framework,
		CameraID:     aiModel.CameraID,
		LoadedAt:     aiModel.LoadedAt,
		Ready:        aiModel.Ready,
	}, true
}

func (a *modelLoaderAdapter) IsModelReady(cameraID string) bool {
	return a.loader.IsModelReady(cameraID)
}

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.StringVar(&configPath, "c", "", "Path to configuration file (short)")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.New(logger.LogConfig{
		Level:  cfg.Log.Level,
		Format: cfg.Log.Format,
		Output: cfg.Log.Output,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	log.Info("Starting Edge Orchestrator",
		"version", version,
		"build_time", buildTime,
		"git_commit", gitCommit,
	)

	// Create main context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create service manager
	svcMgr := service.NewManager(log)

	// Initialize state manager (required for cameras, events, storage)
	stateMgr, err := state.NewManager(cfg, log)
	if err != nil {
		log.Error("Failed to create state manager", "error", err)
		os.Exit(1)
	}
	defer stateMgr.Close()

	// Initialize camera discovery services
	var onvifDiscovery *camera.ONVIFDiscoveryService
	var usbDiscovery *camera.USBDiscoveryService
	if cfg.Edge.Cameras.Discovery.Enabled {
		discoveryInterval := cfg.Edge.Cameras.Discovery.Interval
		if discoveryInterval <= 0 {
			log.Warn("Invalid discovery interval, using default", "interval", discoveryInterval)
			discoveryInterval = 60 * time.Second
		}

		// Register USB camera discovery service
		usbDiscovery = camera.NewUSBDiscoveryService(discoveryInterval, "/dev", log)
		svcMgr.Register(usbDiscovery)
		log.Info("USB camera discovery service registered", "interval", discoveryInterval)

		// Register ONVIF camera discovery service (optional, for network cameras)
		onvifDiscovery = camera.NewONVIFDiscoveryService(discoveryInterval, log)
		svcMgr.Register(onvifDiscovery)
		log.Info("ONVIF camera discovery service registered", "interval", discoveryInterval)
	}

	// Initialize camera manager
	statusInterval := 30 * time.Second
	if cfg.Edge.Cameras.RTSP.ReconnectInterval > 0 {
		statusInterval = cfg.Edge.Cameras.RTSP.ReconnectInterval
	}
	cameraMgr := camera.NewManager(stateMgr, onvifDiscovery, usbDiscovery, statusInterval, log)
	svcMgr.Register(cameraMgr)
	log.Info("Camera manager registered")

	// Create storage state manager adapter
	storageStateMgr := storage.NewStorageStateManager(stateMgr.GetDB(), log)

	// Initialize storage service
	storageSvc, err := storage.NewStorageService(storage.StorageConfig{
		ClipsDir:            cfg.Edge.Storage.ClipsDir,
		SnapshotsDir:        cfg.Edge.Storage.SnapshotsDir,
		RetentionDays:       cfg.Edge.Storage.RetentionDays,
		MaxDiskUsagePercent: cfg.Edge.Storage.MaxDiskUsagePercent,
		StateManager:        storageStateMgr,
	}, log)
	if err != nil {
		log.Error("Failed to create storage service", "error", err)
		os.Exit(1)
	}
	log.Info("Storage service initialized")

	// Initialize event queue and storage
	eventQueue := events.NewQueue(events.QueueConfig{
		StateManager: stateMgr,
		MaxSize:      cfg.Edge.Events.QueueSize,
	}, log)
	eventStorage := events.NewStorage(stateMgr, log)
	log.Info("Event queue and storage initialized")

	// Initialize WireGuard client
	wgClient := wireguard.NewClient(&cfg.Edge.WireGuard, log)
	if cfg.Edge.WireGuard.Enabled {
		svcMgr.Register(wgClient)
		log.Info("WireGuard client registered")
	}

	// Initialize gRPC client (communicates with User VM over WireGuard)
	var grpcClient *grpcclient.Client
	log.Info("DEBUG: About to check WireGuard.Enabled", "enabled", cfg.Edge.WireGuard.Enabled)
	if cfg.Edge.WireGuard.Enabled {
		log.Info("DEBUG: WireGuard enabled, creating gRPC client")
		grpcClient = grpcclient.NewClient(&cfg.Edge.WireGuard, wgClient, log)
		log.Info("DEBUG: gRPC client created, registering")
		svcMgr.Register(grpcClient)
		log.Info("gRPC client registered")
	} else {
		log.Info("DEBUG: WireGuard disabled, skipping gRPC client")
	}

	// Initialize telemetry collector
	telemetryCollector := telemetry.NewCollector(
		&cfg.Edge.Telemetry,
		log,
		cameraMgr,
		eventQueue,
		eventStorage,
		storageSvc,
		wgClient,
	)
	if cfg.Edge.Telemetry.Enabled {
		svcMgr.Register(telemetryCollector)
		log.Info("Telemetry collector registered")
	}

	// Initialize telemetry sender (sends telemetry/heartbeat via gRPC)
	// This triggers edge auto-registration when it sends the first heartbeat
	var telemetrySender *telemetry.Sender
	if cfg.Edge.Telemetry.Enabled && grpcClient != nil {
		// Determine edge ID - use consistent ID for test environments
		// Priority: 1) EDGE_ID env var, 2) Default "poc-edge-1" for test environments
		// In production, EDGE_ID should be explicitly set
		edgeID := "poc-edge-1" // Default for PoC/test environment
		if envEdgeID := os.Getenv("EDGE_ID"); envEdgeID != "" {
			edgeID = envEdgeID
		}
		// Note: We don't use hostname by default to ensure consistency in test environments
		// where container hostnames may be Docker container IDs

		// Create gRPC telemetry sender wrapper
		grpcTelemetrySender := grpcclient.NewTelemetrySender(grpcClient, log)

		// Create telemetry sender service
		telemetrySender = telemetry.NewSender(
			telemetryCollector,
			grpcTelemetrySender,
			&cfg.Edge.Telemetry,
			edgeID,
			log,
		)
		svcMgr.Register(telemetrySender)
		log.Info("Telemetry sender registered", "edge_id", edgeID)
	}

	// Initialize FFmpeg wrapper (for streaming)
	var ffmpegWrapper *video.FFmpegWrapper
	ffmpegWrapper, err = video.NewFFmpegWrapper(log)
	if err != nil {
		log.Warn("FFmpeg not available, streaming will be limited", "error", err)
	} else {
		log.Info("FFmpeg wrapper initialized")
	}

	// Initialize config service
	configSvc, err := config.NewService(configPath, log)
	if err != nil {
		log.Warn("Failed to create config service, config API will be unavailable", "error", err)
		configSvc = nil
	} else {
		log.Info("Config service initialized")
	}

	// Initialize screenshot service (for labeled training data)
	screenshotSvc, err := screenshots.NewService(stateMgr, cfg, log)
	if err != nil {
		log.Warn("Failed to create screenshot service, screenshot API will be unavailable", "error", err)
		screenshotSvc = nil
	} else {
		log.Info("Screenshot service initialized")
	}

	var localDetector *anomaly.LocalDetector
	if cfg.Edge.AI.LocalInferenceEnabled {
		localCfg := anomaly.LocalDetectorConfig{
			Enabled:          cfg.Edge.AI.LocalInferenceEnabled,
			Interval:         cfg.Edge.AI.InferenceInterval,
			Threshold:        cfg.Edge.AI.AnomalyThreshold,
			BaselineLabel:    cfg.Edge.AI.BaselineLabel,
			ClipDuration:     cfg.Edge.AI.ClipDuration,
			PreEventDuration: cfg.Edge.AI.PreEventDuration,
		}
		detector, err := anomaly.NewLocalDetector(
			localCfg,
			cameraMgr,
			screenshotSvc,
			storageSvc,
			eventQueue,
			eventStorage,
			ffmpegWrapper,
			log,
		)
		if err != nil {
			log.Warn("Failed to initialize local anomaly detector", "error", err)
		} else {
			localDetector = detector
			svcMgr.Register(localDetector)
			log.Info("Local anomaly detector registered", "interval", localCfg.Interval, "threshold", localCfg.Threshold)
		}
	}

	// Register capability sync service first (reports camera dataset readiness to VM)
	// This must be created before web server so we can pass it to web server
	capabilitySync := capabilities.NewSyncService(cfg, cameraMgr, screenshotSvc, grpcClient, log)
	svcMgr.Register(capabilitySync)
	log.Info("Capability sync service registered")

	// Initialize snapshot request service (handles VM → Edge snapshot capture requests)
	// This requires streaming service which is created by web server, so we'll wire it later
	var snapshotRequestSvc *snapshot_request.Service
	if cfg.Edge.WireGuard.Enabled && screenshotSvc != nil {
		// Create snapshot request service (streaming service will be set later)
		snapshotRequestSvc = snapshot_request.NewService(
			log,
			cameraMgr,
			screenshotSvc,
			nil, // streaming service will be set after web server creates it
			cfg,
		)
		log.Info("Snapshot request service initialized")
	}

	// Initialize deployment status reporter (reports model deployment status to VM)
	var statusReporter *deployment.StatusReporter
	if cfg.Edge.WireGuard.Enabled && wgClient != nil {
		statusReporter = deployment.NewStatusReporter(cfg, wgClient, log)
		svcMgr.Register(statusReporter)
		log.Info("Deployment status reporter initialized and registered")
	}

	// Initialize model storage for Epic 2.8 (model deployment) - used by both web server and gRPC server
	var modelStorage *storage.ModelStorage
	if cfg.Edge.WireGuard.Enabled {
		modelStorage, err = storage.NewModelStorage(cfg, stateMgr, log)
		if err != nil {
			log.Warn("Failed to create model storage, model deployment will be unavailable", "error", err)
		} else {
			log.Info("Model storage service initialized")
		}
	}

	// Register web server if enabled
	if cfg.Edge.Web.Enabled {
		webServer := web.NewServer(&cfg.Edge.Web, log)
		webServer.SetVersion(version)

		// Inject dependencies
		webServer.SetDependencies(cameraMgr, ffmpegWrapper)
		webServer.SetEventDependencies(stateMgr, storageSvc)
		webServer.SetEventQueueAndStorage(eventQueue, eventStorage)
		if configSvc != nil {
			webServer.SetConfigDependency(configSvc)
		}
		if telemetryCollector != nil {
			// Create adapter for telemetry collector to match TelemetryCollector interface
			telemetryAdapter := &telemetryCollectorAdapter{collector: telemetryCollector}
			webServer.SetTelemetryDependency(telemetryAdapter)
		}
		if screenshotSvc != nil {
			webServer.SetScreenshotService(screenshotSvc)
		}
		if snapshotRequestSvc != nil {
			webServer.SetSnapshotRequestService(snapshotRequestSvc)
		}

		// Initialize dataset service for packaging and uploading datasets to VM
		// Use same edge ID logic as telemetry sender for consistency
		// Priority: 1) EDGE_ID env var, 2) Default "poc-edge-1" for test environments
		datasetEdgeID := "poc-edge-1" // Default for PoC/test environment
		if envEdgeID := os.Getenv("EDGE_ID"); envEdgeID != "" {
			datasetEdgeID = envEdgeID
		}
		// Note: We don't use hostname by default to ensure consistency in test environments
		// where container hostnames may be Docker container IDs
		datasetSvc := dataset.NewService(cfg, log, datasetEdgeID)
		webServer.SetDatasetService(datasetSvc)
		log.Info("Dataset service initialized", "edge_id", datasetEdgeID)

		// Set capability sync service on web server for manual sync triggers
		webServer.SetCapabilitySyncService(capabilitySync)
		log.Info("Capability sync service wired to web server")

		// Wire model storage to web server (if available)
		if modelStorage != nil {
			webServer.SetModelStorageService(modelStorage)
			log.Info("Model storage service wired to web server")

			// Initialize AI client for model loader
			aiClientConfig := ai.ClientConfig{
				ServiceURL:          cfg.Edge.AI.ServiceURL,
				Timeout:             30 * time.Second,
				ConfidenceThreshold: cfg.Edge.AI.AnomalyThreshold,
			}
			aiClient := ai.NewClient(aiClientConfig, log)
			modelLoader := ai.NewModelLoader(modelStorage, aiClient, log)
			// Use adapter to convert ai.ModelLoader to web.ModelLoaderService
			modelLoaderAdapter := &modelLoaderAdapter{loader: modelLoader}
			webServer.SetModelLoaderService(modelLoaderAdapter)
			log.Info("Model loader service initialized and wired to web server")
		}

		// Wire status reporter to web server
		if statusReporter != nil {
			webServer.SetStatusReporterService(statusReporter)
			log.Info("Deployment status reporter wired to web server")
		}

		svcMgr.Register(webServer)
		log.Info("Web server registered", "host", cfg.Edge.Web.Host, "port", cfg.Edge.Web.Port)

		// Wire snapshot request service with streaming service (created by web server)
		// Web server creates streaming service in SetDependencies, so we create our own for snapshot requests
		if snapshotRequestSvc != nil && cameraMgr != nil && ffmpegWrapper != nil {
			streamingSvc := streaming.NewService(cameraMgr, ffmpegWrapper, log)
			snapshotRequestSvc.SetStreamingService(streamingSvc)
			log.Info("Streaming service wired to snapshot request service")
		}
	}

	// Register Edge gRPC server (for VM → Edge calls like RequestSnapshotCapture and DeployModel)
	var grpcServer *grpcclient.Server
	if cfg.Edge.WireGuard.Enabled && snapshotRequestSvc != nil {
		grpcServer = grpcclient.NewServer(&cfg.Edge.WireGuard, log, snapshotRequestSvc)
		svcMgr.Register(grpcServer)
		log.Info("Edge gRPC server registered (for VM → Edge calls)")
	}

	// Wire model storage to gRPC server for model deployment (if both are available)
	if grpcServer != nil && modelStorage != nil {
		grpcServer.SetModelStorage(modelStorage)
		log.Info("Model storage service wired to gRPC server for model deployment")
	}

	// Create health check manager
	healthMgr := health.NewManager(log, svcMgr)

	// Register health checkers
	dbPath := filepath.Join(cfg.Edge.Orchestrator.DataDir, "db", "edge.db")
	healthMgr.RegisterChecker(&health.SystemChecker{})
	healthMgr.RegisterChecker(health.NewDatabaseChecker(dbPath))
	healthMgr.RegisterChecker(health.NewAIServiceChecker(cfg.Edge.AI.ServiceURL))
	healthMgr.RegisterChecker(health.NewStorageChecker(
		cfg.Edge.Storage.ClipsDir,
		cfg.Edge.Storage.SnapshotsDir,
	))
	healthMgr.RegisterChecker(&health.NetworkChecker{})

	// Start health check server
	if err := healthMgr.Start(ctx, cfg); err != nil {
		log.Error("Failed to start health check server", "error", err)
		os.Exit(1)
	}

	// Initialize and start services
	if err := svcMgr.Start(ctx, cfg); err != nil {
		log.Error("Failed to start services", "error", err)
		os.Exit(1)
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigChan
	log.Info("Received shutdown signal", "signal", sig)

	// Start graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop health check server first
	if err := healthMgr.Stop(shutdownCtx); err != nil {
		log.Error("Error stopping health check server", "error", err)
	}

	// Then stop all services
	if err := svcMgr.Shutdown(shutdownCtx); err != nil {
		log.Error("Error during shutdown", "error", err)
		os.Exit(1)
	}

	log.Info("Shutdown complete")
}
