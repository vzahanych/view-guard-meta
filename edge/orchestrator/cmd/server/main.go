package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/config"
	orchestrator "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/orchestrator"
	impl "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/orchestrator/impl"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
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

	// 1. Initialize logger first (needed for config loading)
	log, err := zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	log.Info("Starting Edge Orchestrator",
		zap.String("version", version),
		zap.String("build_time", buildTime),
		zap.String("git_commit", gitCommit),
	)

	// 2. Create fx.App with orchestrator module
	// Config will be loaded first via ConfigProviderWithPath, then other services
	app := fx.New(
		// Provide logger first (needed by config provider)
		fx.Provide(
			func() *zap.Logger { return log },
		),
		// Provide config using ConfigProvider (will be invoked first)
		config.ConfigProviderWithPath(configPath),
		// Include orchestrator module (provides all services in correct order)
		orchestrator.Module(),
		// Add lifecycle hook to start the server
		fx.Invoke(func(
			lc fx.Lifecycle,
			server *impl.Server,
			logger *zap.Logger,
		) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					logger.Info("Starting orchestrator server...")
					return server.Start(ctx)
				},
				OnStop: func(ctx context.Context) error {
					logger.Info("Stopping orchestrator server...")
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					return server.Shutdown(shutdownCtx)
				},
			})
		}),
		// Log lifecycle events
		fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: logger}
		}),
	)

	// 4. Start the application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		log.Error("Failed to start application", zap.Error(err))
		os.Exit(1)
	}

	log.Info("Orchestrator started")

	// 5. Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Info("Shutting down Edge Orchestrator...")

	// 6. Stop the application (fx will handle graceful shutdown of all services)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		log.Error("Error during shutdown", zap.Error(err))
		os.Exit(1)
	}

	log.Info("Edge Orchestrator stopped gracefully")
}
