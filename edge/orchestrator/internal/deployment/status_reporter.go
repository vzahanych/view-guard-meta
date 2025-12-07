package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/wireguard"
)

// StatusReporter reports deployment status to VM
type StatusReporter struct {
	*service.ServiceBase
	config     *config.Config
	logger     *logger.Logger
	httpClient *http.Client
	vmEndpoint string
	wgClient   *wireguard.Client
	queue      []*StatusUpdate
	queueMu    sync.Mutex
	retryQueue chan *StatusUpdate
}

// StatusUpdate represents a deployment status update to send to VM
type StatusUpdate struct {
	DeploymentID string
	Status       string // "deployed", "failed", "active"
	ErrorMessage *string
	ModelPath    *string
	Timestamp    time.Time
	RetryCount   int
}

// NewStatusReporter creates a new deployment status reporter
func NewStatusReporter(cfg *config.Config, wgClient *wireguard.Client, log *logger.Logger) *StatusReporter {
	// Create HTTP client with reasonable timeout
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Determine VM endpoint from config
	vmEndpoint := cfg.Edge.WireGuard.KVMEndpoint
	if vmEndpoint == "" {
		// Default to localhost for PoC (when WireGuard is not configured)
		vmEndpoint = "http://localhost:8280"
	}

	// Ensure endpoint has http:// or https:// prefix
	if !hasProtocol(vmEndpoint) {
		vmEndpoint = "http://" + vmEndpoint
	}

	return &StatusReporter{
		ServiceBase: service.NewServiceBase("deployment-status-reporter", log),
		config:      cfg,
		logger:      log,
		httpClient:  httpClient,
		vmEndpoint:  vmEndpoint,
		wgClient:    wgClient,
		queue:       make([]*StatusUpdate, 0),
		retryQueue:  make(chan *StatusUpdate, 100),
	}
}

// hasProtocol checks if a URL has a protocol prefix
func hasProtocol(url string) bool {
	return len(url) > 7 && (url[:7] == "http://" || url[:8] == "https://")
}

// Start starts the status reporter service
func (sr *StatusReporter) Start(ctx context.Context) error {
	sr.GetStatus().SetStatus(service.StatusRunning)

	// Subscribe to WireGuard connection events
	if sr.GetEventBus() != nil {
		ch := sr.GetEventBus().Subscribe(service.EventTypeWireGuardConnected)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-ch:
					if !ok {
						return
					}
					sr.handleWireGuardConnected(event)
				}
			}
		}()
	}

	// Start retry worker
	go sr.retryWorker(ctx)

	// Process queued updates
	go sr.processQueue(ctx)

	sr.LogInfo("Deployment status reporter started", "vm_endpoint", sr.vmEndpoint)
	return nil
}

// Stop stops the status reporter service
func (sr *StatusReporter) Stop(ctx context.Context) error {
	sr.GetStatus().SetStatus(service.StatusStopped)
	sr.LogInfo("Deployment status reporter stopped")
	return nil
}

// ReportStatus reports deployment status to VM
func (sr *StatusReporter) ReportStatus(ctx context.Context, deploymentID string, status string, errorMessage *string, modelPath *string) error {
	update := &StatusUpdate{
		DeploymentID: deploymentID,
		Status:       status,
		ErrorMessage: errorMessage,
		ModelPath:    modelPath,
		Timestamp:    time.Now(),
		RetryCount:   0,
	}

	// Try to send immediately
	err := sr.sendStatusUpdate(ctx, update)
	if err != nil {
		sr.logger.Warn("Failed to send status update immediately, queuing for retry",
			"deployment_id", deploymentID,
			"status", status,
			"error", err,
		)

		// Queue for retry
		sr.queueMu.Lock()
		sr.queue = append(sr.queue, update)
		sr.queueMu.Unlock()
	} else {
		sr.logger.Info("Deployment status reported successfully",
			"deployment_id", deploymentID,
			"status", status,
		)
	}

	return nil
}

// sendStatusUpdate sends a status update to VM
func (sr *StatusReporter) sendStatusUpdate(ctx context.Context, update *StatusUpdate) error {
	// Check if WireGuard is connected
	if sr.wgClient != nil && !sr.wgClient.IsConnected() {
		return fmt.Errorf("WireGuard tunnel not connected")
	}

	// Build request body
	requestBody := map[string]interface{}{
		"status":      update.Status,
		"timestamp":   update.Timestamp.Format(time.RFC3339),
		"model_path":  update.ModelPath,
		"error":       update.ErrorMessage,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/deployments/%s/status", sr.vmEndpoint, update.DeploymentID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Edge-ID", sr.getEdgeID()) // Add Edge ID header

	// Send request
	resp, err := sr.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("VM returned status %d", resp.StatusCode)
	}

	return nil
}

// getEdgeID returns the Edge ID (from config or default)
func (sr *StatusReporter) getEdgeID() string {
	// For PoC, use a default Edge ID
	// In production, this would come from config or WireGuard peer info
	return "poc-edge-1"
}

// handleWireGuardConnected handles WireGuard connection events
func (sr *StatusReporter) handleWireGuardConnected(event service.Event) {
	sr.logger.Info("WireGuard connected, processing queued status updates")
	// Trigger queue processing
	// The processQueue goroutine will pick up queued updates
}

// processQueue processes queued status updates
func (sr *StatusReporter) processQueue(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second) // Check queue every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if WireGuard is connected
			if sr.wgClient != nil && !sr.wgClient.IsConnected() {
				continue // Skip if not connected
			}

			// Process queued updates
			sr.queueMu.Lock()
			if len(sr.queue) > 0 {
				// Take first update from queue
				update := sr.queue[0]
				sr.queue = sr.queue[1:]
				sr.queueMu.Unlock()

				// Try to send
				err := sr.sendStatusUpdate(ctx, update)
				if err != nil {
					// Re-queue if failed (with retry limit)
					update.RetryCount++
					if update.RetryCount < 5 { // Max 5 retries
						sr.queueMu.Lock()
						sr.queue = append(sr.queue, update)
						sr.queueMu.Unlock()
						sr.logger.Warn("Failed to send queued status update, will retry",
							"deployment_id", update.DeploymentID,
							"retry_count", update.RetryCount,
							"error", err,
						)
					} else {
						sr.logger.Error("Failed to send status update after max retries, dropping",
							"deployment_id", update.DeploymentID,
							"status", update.Status,
						)
					}
				} else {
					sr.logger.Info("Successfully sent queued status update",
						"deployment_id", update.DeploymentID,
						"status", update.Status,
					)
				}
			} else {
				sr.queueMu.Unlock()
			}
		}
	}
}

// retryWorker handles retry queue (for exponential backoff)
func (sr *StatusReporter) retryWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-sr.retryQueue:
			// Exponential backoff: 2s, 4s, 8s, 16s, 32s
			backoff := time.Duration(1<<update.RetryCount) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}

			time.Sleep(backoff)

			// Check if WireGuard is connected
			if sr.wgClient != nil && !sr.wgClient.IsConnected() {
				// Re-queue if not connected
				update.RetryCount++
				if update.RetryCount < 5 {
					select {
					case sr.retryQueue <- update:
					default:
						sr.logger.Warn("Retry queue full, dropping update",
							"deployment_id", update.DeploymentID,
						)
					}
				}
				continue
			}

			// Try to send
			err := sr.sendStatusUpdate(ctx, update)
			if err != nil {
				update.RetryCount++
				if update.RetryCount < 5 {
					select {
					case sr.retryQueue <- update:
					default:
						sr.logger.Warn("Retry queue full, dropping update",
							"deployment_id", update.DeploymentID,
						)
					}
				} else {
					sr.logger.Error("Failed to send status update after max retries, dropping",
						"deployment_id", update.DeploymentID,
						"status", update.Status,
					)
				}
			} else {
				sr.logger.Info("Successfully sent retried status update",
					"deployment_id", update.DeploymentID,
					"status", update.Status,
				)
			}
		}
	}
}

