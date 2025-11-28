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

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/anomaly"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/capabilities"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	grpcclient "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/grpc"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/health"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/telemetry"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
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

		svcMgr.Register(webServer)
		log.Info("Web server registered", "host", cfg.Edge.Web.Host, "port", cfg.Edge.Web.Port)
	}

	// Register capability sync service (reports camera dataset readiness to VM)
	capabilitySync := capabilities.NewSyncService(cfg, cameraMgr, screenshotSvc, grpcClient, log)
	svcMgr.Register(capabilitySync)
	log.Info("Capability sync service registered")

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
