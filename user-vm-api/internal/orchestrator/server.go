package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	datasetstorage "github.com/vzahanych/view-guard-meta/user-vm-api/internal/dataset-storage"
	modelcatalog "github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
	modeldeployment "github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-deployment"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/config"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database/migrations"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/logging"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/service"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
	storagesync "github.com/vzahanych/view-guard-meta/user-vm-api/internal/storage-sync"
	tunnelgateway "github.com/vzahanych/view-guard-meta/user-vm-api/internal/tunnel-gateway"
	"go.uber.org/zap"
)

// Server is the main orchestrator server
type Server struct {
	config  *config.Config
	logger  *logging.Logger
	manager *service.Manager
	db      *database.DB
}

// NewServer creates a new orchestrator server
func NewServer(cfg *config.Config, log *logging.Logger) *Server {
	return &Server{
		config:  cfg,
		logger:  log,
		manager: service.NewManager(log),
	}
}

// Start starts the orchestrator server and all services
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting User VM API orchestrator")

	// Initialize database (single SQLite DB for all services)
	dbCfg := database.DefaultConfig(s.config.UserVMAPI.EventCache.DatabasePath)
	db, err := database.New(dbCfg)
	if err != nil {
		s.logger.Error("Failed to initialize database", zap.Error(err))
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	s.db = db

	// Run database migrations (create all tables)
	migrator := migrations.NewMigrator(db)
	if err := migrator.Up(ctx); err != nil {
		s.logger.Error("Failed to run database migrations", zap.Error(err))
		return fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Get event bus from manager (already created)
	eventBus := s.manager.GetEventBus()

	var edgeAPIServer *tunnelgateway.EdgeAPIServer
	var capStore *tunnelgateway.CapabilityStore
	var wgServer *tunnelgateway.WireGuardServer
	var edgeClient *tunnelgateway.EdgeClient

	// Register Tunnel Gateway / WireGuard services when enabled
	if s.config.UserVMAPI.WireGuardServer.Enabled {
		// WireGuard server (acts as KVM-side tunnel endpoint)
		wgServer, err = tunnelgateway.NewWireGuardServer(s.config, s.logger, db)
		if err != nil {
			s.logger.Error("Failed to create WireGuard server", zap.Error(err))
			return fmt.Errorf("failed to create WireGuard server: %w", err)
		}
		wgServer.SetEventBus(eventBus)
		s.manager.Register(wgServer)

		// Edge authentication/registration manager
		edgeAuth := tunnelgateway.NewEdgeAuth(s.config, s.logger, db, wgServer)

		// Edge-facing gRPC API (over WireGuard tunnel)
		edgeAPIServer, err = tunnelgateway.NewEdgeAPIServer(s.config, s.logger, db, wgServer, edgeAuth)
		if err != nil {
			s.logger.Error("Failed to create Edge API server", zap.Error(err))
			return fmt.Errorf("failed to create Edge API server: %w", err)
		}
		edgeAPIServer.SetEventBus(eventBus)
		capStore = edgeAPIServer.GetCapabilityStore()
		// Set up default telemetry handler
		telemetryHandler := tunnelgateway.NewDefaultTelemetryHandler(s.logger.Logger)
		edgeAPIServer.SetTelemetryHandler(telemetryHandler)
		s.manager.Register(edgeAPIServer)

		// Create Edge client for VM → Edge calls (like RequestSnapshotCapture)
		edgeClient = tunnelgateway.NewEdgeClient(wgServer, s.logger)
		// Set EdgeAPIServer for better edge lookup
		if edgeAPIServer != nil {
			edgeClient.SetEdgeAPIServer(edgeAPIServer)
			// Create connection monitor with EdgeClient for active gRPC health checks
			// VM monitors Edge status through gRPC every 30s (security requirement)
			connectionMonitor := tunnelgateway.NewConnectionMonitor(s.config, s.logger, db, edgeAPIServer, wgServer, edgeClient)
			connectionMonitor.SetEventBus(eventBus)
			edgeAPIServer.SetConnectionMonitor(connectionMonitor)
		}
		s.logger.Info("Edge client initialized (for VM → Edge calls)")
	}

	// Register Dataset Storage service
	var datasetReceiver *datasetstorage.Receiver
	if s.config.UserVMAPI.APIGateway.Enabled {
		datasetReceiver = datasetstorage.NewReceiver(s.config, s.logger, db, capStore, edgeAPIServer)
		datasetReceiver.SetEventBus(eventBus)
		s.logger.Info("Dataset receiver initialized")
	}

	// Register API Gateway when enabled
	if s.config.UserVMAPI.APIGateway.Enabled {
		apiServer := NewAPIServer(s.config, s.logger, capStore, edgeAPIServer, edgeClient)
		apiServer.SetDatabase(db.GetDB()) // Set database for health checks (GetDB returns *sql.DB)
		if datasetReceiver != nil {
			apiServer.SetDatasetReceiver(datasetReceiver)
		}

		// Initialize Model Storage and Catalog
		modelsDir := filepath.Join(s.config.UserVMAPI.Orchestrator.DataDir, "models")
		modelStorage, err := storage.NewModelStorage(modelsDir)
		if err != nil {
			s.logger.Error("Failed to create model storage", zap.Error(err))
			return fmt.Errorf("failed to create model storage: %w", err)
		}
		apiServer.SetModelStorage(modelStorage)

		modelCatalog, err := modelcatalog.NewModelCatalog(modelStorage, modelsDir, db)
		if err != nil {
			s.logger.Error("Failed to create model catalog", zap.Error(err))
			return fmt.Errorf("failed to create model catalog: %w", err)
		}
		// Note: Models are registered via admin API instead of filesystem scanning
		// This allows SaaS components to manage models through the API
		// ScanModels is kept for backward compatibility but not called on startup
		apiServer.SetModelCatalog(modelCatalog)

		// Initialize Model Deployment Service (Epic 2.8)
		// Model deployment service is always initialized, MinIO is required for trained models
		var minioStorage *modeldeployment.MinIOModelStorage
		if s.config.UserVMAPI.StorageSync.Enabled && s.config.UserVMAPI.StorageSync.Provider == "s3" {
			// Initialize MinIO S3 client for model archiving
			providerConfig := s.config.UserVMAPI.StorageSync.ProviderConfig
			endpoint, _ := providerConfig["endpoint"].(string)
			accessKey, _ := providerConfig["access_key_id"].(string)
			secretKey, _ := providerConfig["secret_access_key"].(string)
			useSSL, _ := providerConfig["use_ssl"].(bool)

			// Validate required config values
			if endpoint == "" {
				s.logger.Error("MinIO endpoint not configured in storage_sync.provider_config.endpoint")
				return fmt.Errorf("MinIO endpoint is required for model storage (configured in storage_sync.provider_config.endpoint)")
			}
			if accessKey == "" {
				s.logger.Error("MinIO access key not configured in storage_sync.provider_config.access_key_id")
				return fmt.Errorf("MinIO access key is required for model storage")
			}
			if secretKey == "" {
				s.logger.Error("MinIO secret key not configured in storage_sync.provider_config.secret_access_key")
				return fmt.Errorf("MinIO secret key is required for model storage")
			}

			// Parse endpoint URL to extract host:port (MinIO client doesn't accept http:// prefix)
			// Remove http:// or https:// prefix if present
			endpointHostPort := endpoint
			if strings.HasPrefix(endpoint, "http://") {
				endpointHostPort = strings.TrimPrefix(endpoint, "http://")
				useSSL = false
			} else if strings.HasPrefix(endpoint, "https://") {
				endpointHostPort = strings.TrimPrefix(endpoint, "https://")
				useSSL = true
			}

			s.logger.Info("Initializing MinIO client for model storage",
				zap.String("endpoint", endpointHostPort),
				zap.String("bucket", "models"),
				zap.Bool("use_ssl", useSSL),
			)

			// Use "models" bucket for model storage (separate from camera buckets)
			// Get CA certificate path from config (for TLS verification)
			caCertPath, _ := providerConfig["ca_cert_path"].(string)
			// Default CA cert path if not specified
			if caCertPath == "" && useSSL {
				caCertPath = "/etc/ssl/certs/ca.crt"
			}

			s3Config := storagesync.S3Config{
				Endpoint:   endpointHostPort, // Use parsed host:port (no http:// prefix)
				AccessKey:  accessKey,
				SecretKey:  secretKey,
				BucketName: "models",
				UseSSL:     useSSL,
				CACertPath: caCertPath, // CA certificate for TLS verification
			}

			s3Client, err := storagesync.NewS3Client(s3Config, s.logger.Logger)
			if err != nil {
				s.logger.Error("Failed to initialize MinIO client for model storage - trained models require MinIO",
					zap.Error(err),
					zap.String("endpoint", endpointHostPort),
					zap.String("original_endpoint", endpoint),
				)
				// For trained models, MinIO is required - fail initialization
				return fmt.Errorf("failed to initialize MinIO client (required for trained model storage): %w", err)
			}

			// Initialize MinIO model storage
			minioStorage, err = modeldeployment.NewMinIOModelStorage(s3Client, modelStorage, s.logger)
			if err != nil {
				s.logger.Error("Failed to initialize MinIO model storage - trained models require MinIO",
					zap.Error(err),
				)
				// For trained models, MinIO is required - fail initialization
				return fmt.Errorf("failed to initialize MinIO model storage (required for trained model storage): %w", err)
			}

			s.logger.Info("MinIO model storage initialized",
				zap.String("endpoint", endpointHostPort),
				zap.String("bucket", "models"),
			)
		}

		// Initialize Model Deployment components (always, even without MinIO)
		deploymentStore, err := modeldeployment.NewDeploymentStore(db)
		if err != nil {
			s.logger.Error("Failed to create deployment store", zap.Error(err))
			return fmt.Errorf("failed to create deployment store: %w", err)
		}

		modelConverter, err := modeldeployment.NewModelConverter(modelStorage, modelCatalog, s.logger)
		if err != nil {
			s.logger.Error("Failed to create model converter", zap.Error(err))
			return fmt.Errorf("failed to create model converter: %w", err)
		}
		transferService, err := modeldeployment.NewModelTransferService(modelStorage, minioStorage, modelCatalog, edgeAPIServer, edgeClient, s.logger)
		if err != nil {
			s.logger.Error("Failed to create model transfer service", zap.Error(err))
			return fmt.Errorf("failed to create model transfer service: %w", err)
		}

		deploymentOrchestrator, err := modeldeployment.NewModelDeploymentOrchestrator(
			deploymentStore,
			modelCatalog,
			modelStorage,
			modelConverter,
			transferService,
			minioStorage, // Can be nil if MinIO is not available
			edgeAPIServer,
			s.logger,
		)
		if err != nil {
			s.logger.Error("Failed to create model deployment orchestrator", zap.Error(err))
			return fmt.Errorf("failed to create model deployment orchestrator: %w", err)
		}

		deploymentService, err := modeldeployment.NewModelDeploymentService(
			deploymentOrchestrator,
			modelCatalog,
			eventBus,
			s.logger,
		)
		if err != nil {
			s.logger.Error("Failed to create model deployment service", zap.Error(err))
			return fmt.Errorf("failed to create model deployment service: %w", err)
		}

		// Register deployment service (will be started by manager)
		s.manager.Register(deploymentService)

		// Wire deployment service and orchestrator to API server
		apiServer.SetModelDeploymentService(deploymentService)
		apiServer.SetModelDeploymentOrchestrator(deploymentOrchestrator)
		apiServer.SetMinIOModelStorage(minioStorage) // For archiving trained models on upload

		s.logger.Info("Model deployment service initialized and started")

		s.manager.Register(apiServer)
		s.logger.Info("API Gateway registered")
	}

	// Register services here as they are implemented
	// Example:
	// wireguardSvc := wireguardserver.New(s.config, s.logger)
	// s.manager.Register(wireguardSvc)

	// Start all services
	if err := s.manager.Start(ctx, s.config); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	s.logger.Info("User VM API orchestrator started successfully")
	return nil
}

// Stop stops the orchestrator server and all services
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping User VM API orchestrator")

	if err := s.manager.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	if s.db != nil {
		if err := s.db.Close(); err != nil {
			s.logger.Error("Failed to close database", zap.Error(err))
		}
	}

	s.logger.Info("User VM API orchestrator stopped")
	return nil
}

// GetStatus returns the status of the orchestrator
func (s *Server) GetStatus() *service.ServiceStatus {
	return s.manager.GetStatus()
}
