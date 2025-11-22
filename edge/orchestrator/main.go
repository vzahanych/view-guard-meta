package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/health"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"path/filepath"
)

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

