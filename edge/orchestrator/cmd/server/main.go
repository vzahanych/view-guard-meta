package main

import (
	"context"
	"flag"
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

	// 1. Create fx.App with orchestrator module
	// Config will be loaded first, then logger created from config, then other services
	app := fx.New(
		// Provide config first (no dependencies)
		config.ConfigProviderWithPath(configPath),
		// Provide logger based on config (depends on Config)
		fx.Provide(config.LoggerProvider),
		// Include orchestrator module (provides all services in correct order)
		orchestrator.Module(),
		// Add lifecycle hook to start the server and log startup
		fx.Invoke(func(
			lc fx.Lifecycle,
			server *impl.Server,
			logger *zap.Logger,
			cfg *config.Config,
		) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					logger.Info("Starting Edge Orchestrator",
						zap.String("version", version),
						zap.String("build_time", buildTime),
						zap.String("git_commit", gitCommit),
						zap.String("config_file", cfg.ConfigFile),
						zap.String("environment", cfg.Environment),
					)
					logger.Info("Starting orchestrator server...")
					return server.Start(ctx)
				},
				OnStop: func(ctx context.Context) error {
					logger.Info("Stopping orchestrator server...")
					shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					err := server.Shutdown(shutdownCtx)
					logger.Info("Edge Orchestrator stopped gracefully")
					return err
				},
			})
		}),
		// Log lifecycle events
		fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: logger}
		}),
	)

	// 2. Check for build errors before starting
	if err := app.Err(); err != nil {
		// Use a fallback logger for build errors (before config is loaded)
		fallbackLogger, _ := zap.NewDevelopment()
		defer func() { _ = fallbackLogger.Sync() }()
		fallbackLogger.Error("Failed to build application", zap.Error(err))
		os.Exit(1)
	}

	// 3. Start the application (logger will be available through fx lifecycle)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.Start(ctx); err != nil {
		// Fallback logger for startup errors
		fallbackLogger, _ := zap.NewDevelopment()
		defer func() { _ = fallbackLogger.Sync() }()
		fallbackLogger.Error("Failed to start application", zap.Error(err))
		os.Exit(1)
	}

	// 4. Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan

	// 5. Stop the application (fx will handle graceful shutdown of all services)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()

	if err := app.Stop(stopCtx); err != nil {
		// Fallback logger for shutdown errors
		fallbackLogger, _ := zap.NewDevelopment()
		defer func() { _ = fallbackLogger.Sync() }()
		fallbackLogger.Error("Error during shutdown", zap.Error(err))
		os.Exit(1)
	}
}
