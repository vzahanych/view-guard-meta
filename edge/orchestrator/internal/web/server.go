package web

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/state"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/streaming"
)

//go:embed static/*
var staticFiles embed.FS

var (
	staticContentFS fs.FS
	staticAssetsFS  fs.FS
)

func init() {
	var err error
	staticContentFS, err = fs.Sub(staticFiles, "static")
	if err != nil {
		staticContentFS = staticFiles
	}

	staticAssetsFS, err = fs.Sub(staticFiles, "static/assets")
	if err != nil {
		staticAssetsFS = staticFiles
	}
}

// Server represents the web server service
type Server struct {
	*service.ServiceBase
	config             *config.WebConfig
	logger             *logger.Logger
	httpServer         *http.Server
	router             *gin.Engine
	streamingSvc       *streaming.Service // Optional streaming service
	cameraMgr          *camera.Manager    // Optional camera manager
	stateMgr           *state.Manager     // Optional state manager for events
	storageSvc         StorageService     // Optional storage service for clips/snapshots
	configSvc          *config.Service    // Optional config service for configuration API
	telemetryCollector TelemetryCollector // Optional telemetry collector for metrics
	eventQueue         EventQueue         // Optional event queue for creating events
	eventStorage       EventStorage       // Optional event storage for saving events
	screenshotSvc      ScreenshotService  // Optional screenshot service for labeled screenshots
	version            string             // Application version
	startTime          time.Time          // Server start time for uptime calculation
}

// StorageService interface for serving clips and snapshots
type StorageService interface {
	GetClipsDir() string
	GetSnapshotsDir() string
}

// TelemetryCollector interface for accessing telemetry data
type TelemetryCollector interface {
	GetLastMetrics() interface{}                      // Returns *edge.TelemetryData
	Collect(ctx context.Context) (interface{}, error) // Returns (interface{}, error) - actual implementation returns (*edge.TelemetryData, error)
}

// EventQueue interface for enqueueing events
type EventQueue interface {
	Enqueue(ctx context.Context, event *events.Event, priority int) error
}

// EventStorage interface for saving events
type EventStorage interface {
	SaveEvent(ctx context.Context, event *events.Event) error
}

// ScreenshotService interface for managing labeled screenshots
type ScreenshotService interface {
	SaveScreenshot(ctx context.Context, screenshot *screenshots.Screenshot, imageData []byte) error
	GetScreenshot(ctx context.Context, id string) (*screenshots.Screenshot, error)
	ListScreenshots(ctx context.Context, filters *screenshots.ScreenshotFilters) ([]*screenshots.Screenshot, error)
	UpdateScreenshot(ctx context.Context, id string, updates *screenshots.ScreenshotUpdate) error
	DeleteScreenshot(ctx context.Context, id string) error
	GetScreenshotImage(ctx context.Context, id string) ([]byte, error)
}

// NewServer creates a new web server service
func NewServer(cfg *config.WebConfig, log *logger.Logger) *Server {
	// Set Gin mode to release mode for production
	// Debug mode can be enabled via GIN_MODE environment variable
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(ginLogger(log))
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	return &Server{
		ServiceBase: service.NewServiceBase("web-server", log),
		config:      cfg,
		logger:      log,
		router:      router,
		version:     "dev", // Default version, can be set via SetVersion
		startTime:   time.Now(),
	}
}

// SetVersion sets the application version
func (s *Server) SetVersion(version string) {
	s.version = version
}

// SetDependencies sets optional dependencies (camera manager, streaming service)
func (s *Server) SetDependencies(cameraMgr *camera.Manager, ffmpeg *video.FFmpegWrapper) {
	s.cameraMgr = cameraMgr
	if cameraMgr != nil && ffmpeg != nil {
		s.streamingSvc = streaming.NewService(cameraMgr, ffmpeg, s.logger)
	}
}

// SetEventDependencies sets dependencies for event API (state manager, storage service)
func (s *Server) SetEventDependencies(stateMgr *state.Manager, storageSvc StorageService) {
	s.stateMgr = stateMgr
	s.storageSvc = storageSvc
}

// SetEventQueueAndStorage sets dependencies for event creation
func (s *Server) SetEventQueueAndStorage(queue EventQueue, storage EventStorage) {
	s.eventQueue = queue
	s.eventStorage = storage
}

// SetConfigDependency sets dependency for configuration API
func (s *Server) SetConfigDependency(configSvc *config.Service) {
	s.configSvc = configSvc
}

// SetTelemetryDependency sets dependency for telemetry/metrics API
func (s *Server) SetTelemetryDependency(collector TelemetryCollector) {
	s.telemetryCollector = collector
}

// SetScreenshotService sets dependency for screenshot management API
func (s *Server) SetScreenshotService(svc ScreenshotService) {
	s.screenshotSvc = svc
}

// Start starts the web server
func (s *Server) Start(ctx context.Context) error {
	if !s.config.Enabled {
		s.LogInfo("Web server is disabled")
		return nil
	}

	// Setup routes
	s.setupRoutes()

	// Create HTTP server
	// Note: WriteTimeout and IdleTimeout are set to 0 (disabled) for streaming endpoints
	// Streaming endpoints handle their own timeouts via context cancellation
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // Disabled for streaming - streams handle their own timeouts
		IdleTimeout:  0, // Disabled for streaming - keep connections alive
	}

	// Start server in goroutine
	go func() {
		s.LogInfo("Starting web server", "address", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.LogError("Web server error", err, "address", addr)
		}
	}()

	// Wait for context cancellation or server startup
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		// Server started successfully
		s.LogInfo("Web server started", "address", addr)
		return nil
	}
}

// Stop stops the web server
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}

	s.LogInfo("Stopping web server")
	return s.httpServer.Shutdown(ctx)
}

// Name returns the service name
func (s *Server) Name() string {
	return "web-server"
}

// setupRoutes sets up all API routes
func (s *Server) setupRoutes() {
	// API routes
	api := s.router.Group("/api")
	{
		// Health check
		api.GET("/health", s.handleHealth)

		// System status
		api.GET("/status", s.handleStatus)

		// Camera endpoints (Step 1.9.5)
		cameras := api.Group("/cameras")
		{
			cameras.GET("", s.handleListCameras)
			cameras.GET("/discover", s.handleDiscoverCameras)
			cameras.GET("/:id", s.handleGetCamera)
			cameras.POST("", s.handleAddCamera)
			cameras.PUT("/:id", s.handleUpdateCamera)
			cameras.DELETE("/:id", s.handleDeleteCamera)
			cameras.POST("/:id/test", s.handleTestCamera)
			// Streaming endpoints (Step 1.9.2)
			cameras.GET("/:id/stream", s.handleMJPEGStream)
			cameras.GET("/:id/frame", s.handleSingleFrame)
			// Snapshot endpoint
			cameras.GET("/:id/snapshot", s.handleCameraSnapshot)
		}

		// Event endpoints (Step 1.9.3)
		events := api.Group("/events")
		{
			events.GET("", s.handleListEvents)
			events.GET("/:id", s.handleGetEvent)
			events.POST("/:camera_id/obstruction", s.handleTriggerObstructionEvent)
		}

		// Screenshot endpoints (for labeled training data)
		screenshots := api.Group("/screenshots")
		{
			screenshots.GET("", s.handleListScreenshots)
			screenshots.GET("/:id", s.handleGetScreenshot)
			screenshots.GET("/:id/image", s.handleGetScreenshotImage)
			screenshots.POST("/export", s.handleExportScreenshots)
			screenshots.POST("", s.handleSaveScreenshot)
			screenshots.PUT("/:id", s.handleUpdateScreenshot)
			screenshots.DELETE("/:id", s.handleDeleteScreenshot)
		}

		// Clip and snapshot endpoints (Step 1.9.3)
		api.GET("/clips/:id/play", s.handlePlayClip)
		api.GET("/clips/:id/download", s.handleDownloadClip)
		api.GET("/snapshots/:id", s.handleGetSnapshot)

		// Configuration endpoints (placeholder - will be implemented in Step 1.9.4)
		config := api.Group("/config")
		{
			config.GET("", s.handleGetConfig)
			config.PUT("", s.handleUpdateConfig)
		}

		// Metrics endpoints (Step 1.9.6)
		api.GET("/metrics", s.handleMetrics)
		api.GET("/metrics/app", s.handleAppMetrics)
		api.GET("/telemetry", s.handleTelemetry)
	}

	// Serve static files generated by the frontend build
	s.router.StaticFS("/static", http.FS(staticContentFS))
	s.router.StaticFS("/assets", http.FS(staticAssetsFS))
	s.router.GET("/vite.svg", s.handleStaticRootAsset("vite.svg", "image/svg+xml"))

	// Serve index.html for all non-API routes (SPA routing)
	s.router.NoRoute(func(c *gin.Context) {
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
				"message": fmt.Sprintf("Frontend not built. Please build the frontend first. Error: %v", err),
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
}

// handleStaticRootAsset serves individual files located in the root of the built static directory (e.g., vite.svg)
func (s *Server) handleStaticRootAsset(path string, contentType string) gin.HandlerFunc {
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
func ginLogger(log *logger.Logger) gin.HandlerFunc {
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
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency", latency,
			"client_ip", c.ClientIP(),
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
