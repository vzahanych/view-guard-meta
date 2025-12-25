package impl

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	cctv "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv"
	cctvtypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	vmgateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web-gateway/types"
	"go.uber.org/zap"
)

var (
	staticContentFS fs.FS
	staticAssetsFS  fs.FS
)

func init() {
	// Static files are optional - only needed if serving a frontend UI
	// If static files directory exists, load them from disk
	// Otherwise, the web gateway will only serve API endpoints
	staticDir := "../../web/static"
	if info, err := os.Stat(staticDir); err == nil && info.IsDir() {
		staticContentFS = os.DirFS(staticDir)
		if assetsDir, err := fs.Sub(staticContentFS, "assets"); err == nil {
			staticAssetsFS = assetsDir
		} else {
			staticAssetsFS = staticContentFS
		}
	}
}

// WebGatewayImpl implements the WebGateway interface
// Exported so main.go can access Set methods
type WebGatewayImpl struct {
	config        *types.WebGatewayConfig
	logger        *zap.Logger
	httpServer    *http.Server
	router        *gin.Engine
	eventBus      eventbus.EventBus                  // Event bus for publishing actions and events
	metaStorage   metastorage.MetaDataStore          // Meta-storage for cameras, security events, snapshot requests
	objectStorage objectstorage.ObjectStorageService // Optional object-storage for model files and screenshots
	cctvService   cctv.CCTVService                   // CCTV service for capturing camera screenshots
	vmGateway     vmgateway.VMGateway                 // VM gateway for WireGuard and HTTP client status
	version       string                             // Application version
	startTime     time.Time                          // Server start time for uptime calculation
}

// StorageService interface removed - use config values directly

// SecurityEventManager interface is now part of statemng.StateManager
// Use statemng.StateManager directly instead

// Screenshot types (matching CCTV service types)
type Screenshot = cctvtypes.Screenshot
type ScreenshotFilters = cctvtypes.ScreenshotFilters
type ScreenshotUpdate = cctvtypes.ScreenshotUpdate
type DatasetStatus = cctvtypes.DatasetStatus
type ScreenshotStorageStats = cctvtypes.ScreenshotStorageStats
type StorageCleanupOptions = cctvtypes.StorageCleanupOptions
type StorageCleanupResult = cctvtypes.StorageCleanupResult
type DatasetExportResult = cctvtypes.DatasetExportResult

// NewWebGateway creates a new web gateway implementation
func NewWebGateway(
	cfg *types.WebGatewayConfig,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	cctvService cctv.CCTVService,
	vmGateway vmgateway.VMGateway,
	eventBus eventbus.EventBus,
	log *zap.Logger,
) (*WebGatewayImpl, error) {
	// Set Gin mode to release mode for production
	// Debug mode can be enabled via GIN_MODE environment variable
	gin.SetMode(gin.ReleaseMode)

	if log == nil {
		log = zap.NewNop()
	}

	router := gin.New()
	router.Use(ginLogger(log))
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	return &WebGatewayImpl{
		config:        cfg,
		logger:        log,
		router:        router,
		metaStorage:   metaStore,
		objectStorage: objectStore,
		cctvService:   cctvService,
		vmGateway:     vmGateway,
		eventBus:      eventBus,
		version:       "dev", // Default version, can be set via SetVersion
		startTime:     time.Now(),
	}, nil
}

// Name returns the service name
func (g *WebGatewayImpl) Name() string {
	return "web-gateway"
}

// SetVersion sets the application version
func (g *WebGatewayImpl) SetVersion(version string) {
	g.version = version
}

// Model deployment is now handled by the VM service, not the web-gateway.

// Start starts the web gateway
func (g *WebGatewayImpl) Start(ctx context.Context) error {
	if !g.config.Enabled {
		g.logger.Info("Web gateway is disabled")
		return nil
	}

	// Setup routes
	g.setupRoutes()

	// Create HTTP server
	// Note: WriteTimeout and IdleTimeout are set to 0 (disabled) for streaming endpoints
	// Streaming endpoints handle their own timeouts via context cancellation
	addr := fmt.Sprintf("%s:%d", g.config.Host, g.config.Port)
	g.httpServer = &http.Server{
		Addr:         addr,
		Handler:      g.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // Disabled for streaming - streams handle their own timeouts
		IdleTimeout:  0, // Disabled for streaming - keep connections alive
	}

	// Start server in goroutine
	go func() {
		g.logger.Info("Starting web gateway", zap.String("address", addr))
		if err := g.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			g.logger.Error("Web gateway error", zap.Error(err), zap.String("address", addr))
		}
	}()

	// Wait for context cancellation or server startup
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		// Server started successfully
		g.logger.Info("Web gateway started", zap.String("address", addr))
		return nil
	}
}

// Stop stops the web gateway
func (g *WebGatewayImpl) Stop(ctx context.Context) error {
	if g.httpServer == nil {
		return nil
	}

	g.logger.Info("Stopping web gateway")
	return g.httpServer.Shutdown(ctx)
}

// Name returns the service name
// func (g *WebGatewayImpl) Name() string {
// 	return "web-gateway"
// }

// setupRoutes sets up all API routes
func (g *WebGatewayImpl) setupRoutes() {
	// API routes
	api := g.router.Group("/api")
	{
		// Health check
		api.GET("/health", g.handleHealth)

		// System status
		api.GET("/status", g.handleStatus)

		// Edge application state
		api.GET("/state", g.handleGetState)

		// Edge capabilities (received from VM)
		api.GET("/capabilities", g.handleGetCapabilities)

		// Camera endpoints (Step 1.9.5)
		camerag := api.Group("/cameras")
		{
			camerag.GET("", g.handleListCameras)
			camerag.GET("/discover", g.handleDiscoverCameras)
			camerag.GET("/:id", g.handleGetCamera)
			// Dataset status (read-only and refresh)
			camerag.GET("/:id/dataset", g.handleGetDatasetStatus)
			camerag.GET("/:id/dataset-status", g.handleGetDatasetStatus)
			camerag.POST("/:id/dataset/sync", g.handleSyncDatasetStatus)
			camerag.POST("", g.handleAddCamera)
			camerag.PUT("/:id", g.handleUpdateCamera)
			camerag.DELETE("/:id", g.handleDeleteCamera)
			camerag.POST("/:id/test", g.handleTestCamera)
			camerag.POST("/:id/capture", g.handleCaptureScreenshot)
			// Streaming endpoints (Step 1.9.2)
			// camerag.GET("/:id/stream", g.handleMJPEGStream)
			// camerag.GET("/:id/frame", g.handleSingleFrame)
			// // Snapshot endpoint
			// camerag.GET("/:id/snapshot", g.handleCameraSnapshot)
			// // Dataset status refresh endpoint (Substep 2.2.2.4.2)
			// camerag.POST("/:id/dataset/refresh", g.handleRefreshDatasetStatus)
		}

		// Event endpoints (Step 1.9.3)
		events := api.Group("/events")
		{
			events.GET("", g.handleListEvents)
			events.GET("/:id", g.handleGetEvent)
			events.POST("/:camera_id/obstruction", g.handleTriggerObstructionEvent)
		}

		// Screenshot endpoints (for labeled training data)
		screenshots := api.Group("/screenshots")
		{
			screenshots.GET("", g.handleListScreenshots)
			screenshots.GET("/:id", g.handleGetScreenshot)
			screenshots.GET("/:id/image", g.handleGetScreenshotImage)
			screenshots.GET("/:id/thumbnail", g.handleGetScreenshotThumbnail)
			// screenshots.GET("/storage", g.handleGetScreenshotStorageStats)
			// screenshots.POST("/storage/cleanup", g.handleCleanupScreenshotStorage)
			// screenshots.POST("/export", g.handleExportScreenshots)
			screenshots.POST("", g.handleSaveScreenshot)
			screenshots.PUT("/:id", g.handleUpdateScreenshot)
			screenshots.DELETE("/:id", g.handleDeleteScreenshot)
		}

		// Snapshot request endpoints (for VM → Edge snapshot requests)
		snapshotRequests := api.Group("/snapshot-requests")
		{
			snapshotRequests.GET("", g.handleListSnapshotRequests)
			snapshotRequests.GET("/:camera_id", g.handleGetSnapshotRequest)
			snapshotRequests.POST("/:camera_id/ready", g.handleMarkScreenshotSetReady)
		}

		// Clip and snapshot endpoints (Step 1.9.3)
		api.GET("/clips/:id/play", g.handlePlayClip)
		api.GET("/clips/:id/download", g.handleDownloadClip)
		api.GET("/snapshots/:id", g.handleGetSnapshot)

		// Configuration endpoints (placeholder - will be implemented in Step 1.9.4)
		config := api.Group("/config")
		{
			config.GET("", g.handleGetConfig)
			config.PUT("", g.handleUpdateConfig)
		}

		// Metrics endpoints (Step 1.9.6)
		api.GET("/metrics", g.handleMetrics)
		api.GET("/metrics/app", g.handleAppMetrics)
		api.GET("/telemetry", g.handleTelemetry)

		// Telemetry reminder endpoint (Epic 2.2.1.3)
		api.POST("/telemetry/reminder", g.handleReminderTelemetry)
	}

	// Serve static files generated by the frontend build (if available)
	if staticContentFS != nil {
		g.router.StaticFS("/static", http.FS(staticContentFS))
		if staticAssetsFS != nil {
			g.router.StaticFS("/assets", http.FS(staticAssetsFS))
		}
		g.router.GET("/vite.svg", g.handleStaticRootAsset("vite.svg", "image/svg+xml"))

		// Serve index.html for all non-API routes (SPA routing)
		g.router.NoRoute(func(c *gin.Context) {
			// Don't serve index.html for API routes
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
				return
			}

			// Try to serve index.html from static files
			indexFile, err := staticContentFS.Open("index.html")
			if err != nil {
				// If index.html doesn't exist, return 404
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Not found",
					"message": "Frontend not available. Static files not found.",
				})
				return
			}
			defer indexFile.Close()

			// Read and serve index.html
			content, err := io.ReadAll(indexFile)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read index.html"})
				return
			}

			c.Data(http.StatusOK, "text/html; charset=utf-8", content)
		})
	} else {
		// If static files are not available, return 404 for non-API routes
		g.router.NoRoute(func(c *gin.Context) {
			if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{
					"error":   "Not found",
					"message": "Frontend not available. Static files not found.",
				})
				return
			}
			c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		})
	}
}

// handleStaticRootAsset serves individual files located in the root of the built static directory (e.g., vite.svg)
func (g *WebGatewayImpl) handleStaticRootAsset(path string, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := staticContentFS.Open(path)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}

		if contentType != "" {
			c.Data(http.StatusOK, contentType, content)
		} else {
			c.Data(http.StatusOK, http.DetectContentType(content), content)
		}
	}
}

// ginLogger creates a Gin middleware for logging
func ginLogger(log *zap.Logger) gin.HandlerFunc {
	if log == nil {
		log = zap.NewNop()
	}
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Log request
		latency := time.Since(start)
		status := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		log.Debug("HTTP request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}

// corsMiddleware creates a CORS middleware for local network access
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
