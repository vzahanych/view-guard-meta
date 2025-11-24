package ai

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"math"
	"os"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/camera"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/events"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/storage"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/video"
	webscreenshots "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/web/screenshots"
)

// LocalDetectorConfig contains configuration for the local anomaly detector.
type LocalDetectorConfig struct {
	Enabled          bool
	Interval         time.Duration
	Threshold        float64
	BaselineLabel    string
	ClipDuration     time.Duration
	PreEventDuration time.Duration
}

type baselineStats struct {
	CameraID       string
	MeanBrightness float64
	SampleCount    int
	UpdatedAt      time.Time
}

// LocalDetector performs lightweight anomaly detection on-device using
// customer-provided "normal" screenshots as the baseline.
type LocalDetector struct {
	*service.ServiceBase

	cfg           LocalDetectorConfig
	cameraMgr     *camera.Manager
	screenshotSvc *webscreenshots.Service
	storageSvc    *storage.StorageService
	eventQueue    *events.Queue
	eventStorage  *events.Storage
	ffmpeg        *video.FFmpegWrapper
	snapshotGen   *storage.SnapshotGenerator
	clipRecorder  *video.ClipRecorder

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.RWMutex
	baselines map[string]*baselineStats
}

// NewLocalDetector creates a new local anomaly detector service.
func NewLocalDetector(
	cfg LocalDetectorConfig,
	cameraMgr *camera.Manager,
	screenshotSvc *webscreenshots.Service,
	storageSvc *storage.StorageService,
	eventQueue *events.Queue,
	eventStorage *events.Storage,
	ffmpeg *video.FFmpegWrapper,
	log *logger.Logger,
) (*LocalDetector, error) {
	ctx, cancel := context.WithCancel(context.Background())

	var snapshotGen *storage.SnapshotGenerator
	if storageSvc != nil {
		snapshotGen = storage.NewSnapshotGenerator(storageSvc, storage.SnapshotConfig{}, log)
	}

	var clipRecorder *video.ClipRecorder
	if ffmpeg != nil && storageSvc != nil {
		recorder, err := video.NewClipRecorder(ffmpeg, video.ClipRecorderConfig{
			OutputDir: storageSvc.GetClipsDir(),
		}, log)
		if err != nil {
			log.Warn("Failed to initialize clip recorder for local detector", "error", err)
		} else {
			clipRecorder = recorder
		}
	}

	detector := &LocalDetector{
		ServiceBase:   service.NewServiceBase("local-anomaly-detector", log),
		cfg:           cfg,
		cameraMgr:     cameraMgr,
		screenshotSvc: screenshotSvc,
		storageSvc:    storageSvc,
		eventQueue:    eventQueue,
		eventStorage:  eventStorage,
		ffmpeg:        ffmpeg,
		snapshotGen:   snapshotGen,
		clipRecorder:  clipRecorder,
		ctx:           ctx,
		cancel:        cancel,
		baselines:     make(map[string]*baselineStats),
	}

	return detector, nil
}

// Start begins anomaly detection.
func (d *LocalDetector) Start(ctx context.Context) error {
	if !d.cfg.Enabled {
		d.LogInfo("Local anomaly detector disabled")
		d.GetStatus().SetStatus(service.StatusStopped)
		return nil
	}
	if d.cameraMgr == nil || d.screenshotSvc == nil || d.ffmpeg == nil {
		d.LogWarn("Local anomaly detector missing dependencies, disabling component")
		d.GetStatus().SetStatus(service.StatusStopped)
		return nil
	}

	d.GetStatus().SetStatus(service.StatusRunning)
	go d.run()
	return nil
}

// Stop stops the detector.
func (d *LocalDetector) Stop(ctx context.Context) error {
	d.cancel()
	d.GetStatus().SetStatus(service.StatusStopped)
	return nil
}

func (d *LocalDetector) run() {
	ticker := time.NewTicker(d.cfg.Interval)
	defer ticker.Stop()

	d.LogInfo("Local anomaly detector started",
		"interval", d.cfg.Interval,
		"threshold", d.cfg.Threshold,
		"baseline_label", d.cfg.BaselineLabel,
	)

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.evaluateCameras()
		}
	}
}

func (d *LocalDetector) evaluateCameras() {
	cameras := d.cameraMgr.ListCameras(true)
	for _, cam := range cameras {
		if cam == nil || !cam.Enabled {
			continue
		}

		input := d.getCameraInput(cam)
		if input == "" {
			continue
		}

		frame, err := d.ffmpeg.CaptureFrameJPEG(d.ctx, input, 85)
		if err != nil {
			d.LogDebug("Failed to capture frame for anomaly detection", "camera_id", cam.ID, "error", err)
			continue
		}

		score, baselineMean, err := d.computeAnomalyScore(cam.ID, frame)
		if err != nil {
			d.LogDebug("Failed to compute anomaly score", "camera_id", cam.ID, "error", err)
			continue
		}

		if score >= d.cfg.Threshold && d.cfg.Threshold > 0 {
			d.handleAnomaly(cam, frame, score, baselineMean)
		}
	}
}

func (d *LocalDetector) computeAnomalyScore(cameraID string, frame []byte) (float64, float64, error) {
	brightness, err := measureBrightness(frame)
	if err != nil {
		return 0, 0, err
	}

	baseline := d.getBaseline(cameraID)
	if baseline == nil || time.Since(baseline.UpdatedAt) > 5*time.Minute {
		if err := d.refreshBaseline(cameraID); err != nil {
			d.LogDebug("Failed to refresh baseline", "camera_id", cameraID, "error", err)
		}
		baseline = d.getBaseline(cameraID)
	}

	if baseline == nil || baseline.SampleCount == 0 {
		// Initialize baseline using current frame
		d.setBaseline(cameraID, brightness, 1)
		return 0, brightness, nil
	}

	diff := math.Abs(brightness - baseline.MeanBrightness)
	return diff, baseline.MeanBrightness, nil
}

func (d *LocalDetector) refreshBaseline(cameraID string) error {
	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()

	filters := &webscreenshots.ScreenshotFilters{
		CameraID: cameraID,
	}
	if d.cfg.BaselineLabel != "" {
		filters.Label = webscreenshots.Label(d.cfg.BaselineLabel)
	}
	filters.Limit = 500

	samples, err := d.screenshotSvc.ListScreenshots(ctx, filters)
	if err != nil {
		return err
	}
	if len(samples) == 0 {
		return fmt.Errorf("no baseline samples")
	}

	var total float64
	var count int
	for _, sample := range samples {
		data, err := osReadFile(sample.FilePath)
		if err != nil {
			d.LogDebug("Failed to read screenshot for baseline", "path", sample.FilePath, "error", err)
			continue
		}
		brightness, err := measureBrightness(data)
		if err != nil {
			continue
		}
		total += brightness
		count++
	}
	if count == 0 {
		return fmt.Errorf("no usable baseline samples")
	}

	d.setBaseline(cameraID, total/float64(count), count)
	return nil
}

func (d *LocalDetector) handleAnomaly(cam *camera.Camera, frame []byte, score, baseline float64) {
	event := events.NewEvent()
	event.CameraID = cam.ID
	event.EventType = events.EventTypeAnomalyDetected
	event.Confidence = math.Min(score/d.cfg.Threshold, 1.0)
	event.Metadata["anomaly_score"] = score
	event.Metadata["baseline_brightness"] = baseline
	event.Metadata["timestamp"] = time.Now().UTC()

	if d.snapshotGen != nil {
		if path, err := d.snapshotGen.GenerateSnapshot(d.ctx, frame, cam.ID, event.ID); err == nil {
			event.SnapshotPath = path
		} else {
			d.LogDebug("Failed to generate snapshot for anomaly", "camera_id", cam.ID, "error", err)
		}
	}

	if d.clipRecorder != nil && d.cfg.ClipDuration > 0 {
		input := d.getCameraInput(cam)
		if input != "" {
			duration := d.cfg.ClipDuration
			if d.cfg.PreEventDuration > 0 {
				duration += d.cfg.PreEventDuration
			}
			if clipPath, err := d.clipRecorder.StartRecording(cam.ID, input, duration); err == nil {
				event.ClipPath = clipPath
				if d.storageSvc != nil {
					go func(path, cameraID, eventID string, wait time.Duration) {
						select {
						case <-time.After(wait + time.Second):
						case <-d.ctx.Done():
							return
						}
						if info, err := os.Stat(path); err == nil {
							_ = d.storageSvc.SaveClip(context.Background(), path, cameraID, eventID, info.Size())
						}
					}(clipPath, cam.ID, event.ID, duration)
				}
			} else {
				d.LogDebug("Failed to start clip recording", "camera_id", cam.ID, "error", err)
			}
		}
	}

	if d.eventStorage != nil {
		if err := d.eventStorage.SaveEvent(d.ctx, event); err != nil {
			d.LogWarn("Failed to persist anomaly event", "camera_id", cam.ID, "error", err)
		}
	}
	if d.eventQueue != nil {
		if err := d.eventQueue.Enqueue(d.ctx, event, 1); err != nil {
			d.LogWarn("Failed to enqueue anomaly event", "camera_id", cam.ID, "error", err)
		}
	}

	d.LogInfo("Anomaly detected",
		"camera_id", cam.ID,
		"score", score,
		"baseline", baseline,
	)
}

func (d *LocalDetector) getBaseline(cameraID string) *baselineStats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.baselines[cameraID]
}

func (d *LocalDetector) setBaseline(cameraID string, mean float64, samples int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.baselines[cameraID] = &baselineStats{
		CameraID:       cameraID,
		MeanBrightness: mean,
		SampleCount:    samples,
		UpdatedAt:      time.Now(),
	}
}

func (d *LocalDetector) getCameraInput(cam *camera.Camera) string {
	if cam == nil {
		return ""
	}
	if cam.Type == camera.CameraTypeUSB && cam.DevicePath != "" {
		return cam.DevicePath
	}
	if len(cam.RTSPURLs) > 0 {
		return cam.RTSPURLs[0]
	}
	if cam.Config.RecordingEnabled && len(cam.Config.Resolution) > 0 && len(cam.RTSPURLs) > 0 {
		return cam.RTSPURLs[0]
	}
	return ""
}

// measureBrightness computes the average brightness of a JPEG frame.
func measureBrightness(frame []byte) (float64, error) {
	img, _, err := image.Decode(bytes.NewReader(frame))
	if err != nil {
		return 0, err
	}

	var total float64
	var count float64
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			brightness := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			total += brightness / 65535.0 * 255.0
			count++
		}
	}
	if count == 0 {
		return 0, fmt.Errorf("empty image")
	}
	return total / count, nil
}

// osReadFile is a helper for mocking in tests.
var osReadFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}
