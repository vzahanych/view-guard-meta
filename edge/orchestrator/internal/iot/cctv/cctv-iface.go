package cctv

import (
	"context"
	"io"
	"time"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/impl"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/internal/discovery"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/iot/cctv/types"
	metastorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/meta-storage"
	objectstorage "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// CCTVService provides a unified interface for managing CCTV cameras.
// It is the single point of truth for all camera-related operations including
// discovery, screenshots, frames, and video clip recording.
//
// The service uses:
//   - meta-storage for camera and media metadata
//   - object-storage for storing clips and snapshots
//   - event-bus for publishing camera events

//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mocks/mock_cctv_service.go -package=mocks
type CCTVService interface {
	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Name() string

	// Camera Discovery
	// DiscoverCameras triggers immediate camera discovery
	DiscoverCameras(ctx context.Context) error

	// GetDiscoveredCameras returns all discovered cameras
	GetDiscoveredCameras(ctx context.Context) ([]*types.Camera, error)

	// GetCamera retrieves a camera by ID
	GetCamera(ctx context.Context, cameraID string) (*types.Camera, error)

	// ListCameras lists all registered cameras
	ListCameras(ctx context.Context, enabledOnly bool) ([]*types.Camera, error)

	// RegisterCamera registers a discovered camera
	RegisterCamera(ctx context.Context, camera *types.Camera) error

	// UpdateCamera updates camera configuration
	UpdateCamera(ctx context.Context, cameraID string, updates *types.CameraUpdate) error

	// EnableCamera enables a camera
	EnableCamera(ctx context.Context, cameraID string) error

	// DisableCamera disables a camera
	DisableCamera(ctx context.Context, cameraID string) error

	// DeleteCamera deletes a camera
	DeleteCamera(ctx context.Context, cameraID string) error

	// Frame Capture
	// StartFrameCapture starts capturing frames from a camera
	StartFrameCapture(ctx context.Context, cameraID string, onFrame func(frame *types.Frame)) error

	// StopFrameCapture stops capturing frames from a camera
	StopFrameCapture(ctx context.Context, cameraID string) error

	// CaptureFrame captures a single frame from a camera
	CaptureFrame(ctx context.Context, cameraID string) (*types.Frame, error)

	// Screenshot Management
	// SaveScreenshot saves a screenshot with image data and label
	SaveScreenshot(ctx context.Context, screenshot *types.Screenshot, imageData []byte) error

	// CaptureScreenshot captures a screenshot from a camera and saves it
	CaptureScreenshot(ctx context.Context, cameraID string, eventID string) (string, error)

	// CaptureScreenshotWithLabel captures a screenshot with a label for training
	CaptureScreenshotWithLabel(ctx context.Context, cameraID string, label string, customLabel string, description string) (string, error)

	// GetScreenshot retrieves a screenshot by ID
	GetScreenshot(ctx context.Context, screenshotID string) (*types.Screenshot, error)

	// ListScreenshots lists screenshots with optional filters
	ListScreenshots(ctx context.Context, filters *types.ScreenshotFilters) ([]*types.Screenshot, error)

	// UpdateScreenshot updates a screenshot's label and metadata
	UpdateScreenshot(ctx context.Context, screenshotID string, updates *types.ScreenshotUpdate) error

	// DeleteScreenshot deletes a screenshot
	DeleteScreenshot(ctx context.Context, screenshotID string) error

	// GetScreenshotImage retrieves the full image data for a screenshot
	GetScreenshotImage(ctx context.Context, screenshotID string) (*types.Frame, error)

	// GetScreenshotThumbnail retrieves the thumbnail image data for a screenshot
	GetScreenshotThumbnail(ctx context.Context, screenshotID string) (*types.Frame, error)

	// GetDatasetStatus returns dataset readiness status for a camera
	GetDatasetStatus(ctx context.Context, cameraID string, minSnapshots int) (*types.DatasetStatus, error)

	// GetStorageStats returns storage statistics for screenshots
	GetStorageStats(ctx context.Context) (*types.ScreenshotStorageStats, error)

	// CleanupStorage performs storage cleanup for screenshots
	CleanupStorage(ctx context.Context, opts types.StorageCleanupOptions) (*types.StorageCleanupResult, error)

	// ExportDataset exports labeled screenshots into a portable archive
	ExportDataset(ctx context.Context, filters *types.ScreenshotFilters, includeMetadata bool) (*types.DatasetExportResult, error)

	// Video Clip Recording
	// StartRecording starts recording a clip from a camera
	StartRecording(ctx context.Context, cameraID string, eventID string, duration time.Duration) (string, error)

	// StopRecording stops recording a clip
	StopRecording(ctx context.Context, cameraID string) error

	// GetClip retrieves clip information by ID
	GetClip(ctx context.Context, clipID string) (*types.Clip, error)

	// ListClips lists clips with optional filters
	ListClips(ctx context.Context, filters *types.ClipFilters) ([]*types.Clip, error)

	// DeleteClip deletes a clip
	DeleteClip(ctx context.Context, clipID string) error

	// GetClipStream returns a stream reader for a clip
	GetClipStream(ctx context.Context, clipID string) (io.ReadCloser, error)

	// MJPEG Streaming (moved from @streaming package)
	// StartMJPEGStream starts an MJPEG stream for a camera (for web UI)
	StartMJPEGStream(ctx context.Context, cameraID string) (*types.MJPEGStream, error)

	// StopMJPEGStream stops an MJPEG stream
	StopMJPEGStream(ctx context.Context, cameraID string) error

	// GetMJPEGStream gets an active MJPEG stream
	GetMJPEGStream(ctx context.Context, cameraID string) (*types.MJPEGStream, error)
}

// NewCCTVService creates a new CCTV service with a single implementation.
// The service requires meta-storage, object-storage, and event-bus dependencies
// which are created elsewhere in the orchestrator.
func NewCCTVService(
	ctx context.Context,
	config *types.CCTVServiceConfig,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (CCTVService, error) {
	// Construct FFmpeg wrapper
	ffmpegWrapper, err := impl.NewFFmpegWrapper(logger)
	if err != nil {
		return nil, err
	}

	// Construct discovery services if enabled
	var onvifDiscovery *discovery.ONVIFDiscoveryService
	var usbDiscovery *discovery.USBDiscoveryService

	if config.Discovery.Enabled {
		onvifDiscovery = discovery.NewONVIFDiscoveryService(config.Discovery.Interval, logger)
		
		// Use configurable USB device path, default to "/dev"
		usbDevicePath := config.Discovery.USBDevicePath
		if usbDevicePath == "" {
			usbDevicePath = "/dev"
		}
		usbDiscovery = discovery.NewUSBDiscoveryService(config.Discovery.Interval, usbDevicePath, logger)
	}

	// Create the CCTV service implementation
	return impl.NewCCTVService(
		metaStore,
		objectStore,
		eventBus,
		onvifDiscovery,
		usbDiscovery,
		ffmpegWrapper,
		logger,
	), nil
}

// CCTVServiceProvider creates the CCTV service with fx lifecycle management
func CCTVServiceProvider(
	lc fx.Lifecycle,
	cfg *types.CCTVServiceConfig,
	metaStore metastorage.MetaDataStore,
	objectStore objectstorage.ObjectStorageService,
	eventBus eventbus.EventBus,
	logger *zap.Logger,
) (CCTVService, error) {
	service, err := NewCCTVService(
		context.Background(),
		cfg,
		metaStore,
		objectStore,
		eventBus,
		logger,
	)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if service != nil {
				if err := service.Start(ctx); err != nil {
					return err
				}
			}
			logger.Info("CCTV service started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if service != nil {
				if err := service.Stop(ctx); err != nil {
					return err
				}
			}
			logger.Info("CCTV service stopped")
			return nil
		},
	})

	return service, nil
}
