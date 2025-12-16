package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vzahanych/view-guard-meta/vm-edge-orch/config"
	orchestrator "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/orchestrator"
	"github.com/vzahanych/view-guard-meta/vm-edge-orch/pkg/telemetry"
	"go.uber.org/zap"
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

	// 1. Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize logger (simple Zap logger for now)
	log, err := zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	log.Info("Starting User VM API",
		zap.String("version", version),
		zap.String("build_time", buildTime),
		zap.String("git_commit", gitCommit),
	)

	// 3. Create main context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. Initialize telemetry (optional, based on environment)
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	var tp *telemetry.Provider
	if otlpEndpoint != "" {
		tp, err = telemetry.Init(ctx, telemetry.Config{
			ServiceName:        "vm-edge-orchestrator",
			Environment:        os.Getenv("ENVIRONMENT"),
			Mode:               telemetry.ModeVM,
			OTLPEndpoint:       otlpEndpoint,
			Insecure:           true,
			Timeout:            10 * time.Second,
			ResourceAttributes: map[string]string{},
		})
		if err != nil {
			log.Warn("Telemetry initialisation failed, continuing without OTEL", zap.Error(err))
		} else {
			defer func() {
				shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancelShutdown()
				if err := tp.Shutdown(shutdownCtx); err != nil {
					log.Warn("Failed to shutdown telemetry provider cleanly", zap.Error(err))
				}
			}()
			log.Info("Telemetry initialised", zap.String("otlp_endpoint", otlpEndpoint))
		}
	} else {
		log.Info("OTEL_EXPORTER_OTLP_ENDPOINT not set; telemetry will not be initialised")
	}

	// 5. Initialise orchestrator (constructs dependencies)
	orch := orchestrator.New()
	if err := orch.Init(cfg); err != nil {
		log.Error("Failed to initialise orchestrator", zap.Error(err))
		os.Exit(1)
	}
	log.Info("Orchestrator initialised successfully")

	// 6. Start orchestrator (starts all services)
	if err := orch.Start(ctx); err != nil {
		log.Error("Failed to start orchestrator", zap.Error(err))
		os.Exit(1)
	}

	log.Info("Orchestrator started")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Info("Shutting down User VM API...")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop the orchestrator
	if err := orch.Shutdown(shutdownCtx); err != nil {
		log.Error("Error during shutdown", zap.Error(err))
		os.Exit(1)
	}

	log.Info("User VM API stopped gracefully")
}
