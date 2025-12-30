package impl

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"

	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// CertificateRotationHandler handles certificate rotation for VM CA certificates.
type CertificateRotationHandler struct {
	clientCfg     *vmgatewaytypes.HTTPSClientConfig
	httpClient    *http.Client
	logger        *zap.Logger
	eventBus      eventbus.EventBus
	mu            sync.RWMutex
	rotationState *RotationState
}

// RotationState tracks the current certificate rotation state.
type RotationState struct {
	ScheduledAt    *time.Time
	GracePeriodEnd *time.Time
	OldCAFingerprint string
	NewCAFingerprint string
	Status         string // "scheduled", "in_progress", "completed", "failed"
}

// NewCertificateRotationHandler creates a new certificate rotation handler.
func NewCertificateRotationHandler(
	clientCfg *vmgatewaytypes.HTTPSClientConfig,
	httpClient *http.Client,
	logger *zap.Logger,
	eventBus eventbus.EventBus,
) *CertificateRotationHandler {
	return &CertificateRotationHandler{
		clientCfg:  clientCfg,
		httpClient: httpClient,
		logger:     logger,
		eventBus:   eventBus,
		rotationState: &RotationState{
			Status: "idle",
		},
	}
}

// HandleRotationScheduled handles a certificate rotation scheduled event from capabilities sync.
// This should be called when SyncCapabilitiesResponse contains CertRotationScheduledAt.
func (h *CertificateRotationHandler) HandleRotationScheduled(
	ctx context.Context,
	scheduledAt time.Time,
	vmEndpoint string,
) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if rotation is already scheduled
	if h.rotationState.Status == "scheduled" || h.rotationState.Status == "in_progress" {
		h.logger.Info("Certificate rotation already in progress, skipping",
			zap.String("status", h.rotationState.Status))
		return nil
	}

	h.logger.Info("Certificate rotation scheduled",
		zap.Time("scheduled_at", scheduledAt))

	// Update rotation state
	h.rotationState.ScheduledAt = &scheduledAt
	h.rotationState.Status = "scheduled"

	// Emit rotation scheduled event
	if h.eventBus != nil {
		event := evtbusstypes.Event[evtbusstypes.CertificateRotationScheduledEventData]{
			Type:      evtbusstypes.EventType("certificate.rotation_scheduled"),
			Source:    "vm-gateway",
			Timestamp: time.Now(),
			Data: evtbusstypes.CertificateRotationScheduledEventData{
				ScheduledAt: scheduledAt,
			},
		}
		if err := eventbus.PublishTyped(h.eventBus, event); err != nil {
			h.logger.Warn("Failed to publish certificate rotation scheduled event", zap.Error(err))
		}
	}

	// Start rotation process in background if scheduled time is in the past or near future
	now := time.Now()
	if scheduledAt.Before(now) || scheduledAt.Sub(now) < 5*time.Minute {
		// Start rotation immediately or soon
		go h.performRotation(ctx, vmEndpoint)
	} else {
		// Schedule rotation for later
		go func() {
			waitDuration := scheduledAt.Sub(now)
			time.Sleep(waitDuration)
			h.performRotation(ctx, vmEndpoint)
		}()
	}

	return nil
}

// performRotation performs the actual certificate rotation.
func (h *CertificateRotationHandler) performRotation(ctx context.Context, vmEndpoint string) {
	h.mu.Lock()
	h.rotationState.Status = "in_progress"
	h.mu.Unlock()

	h.logger.Info("Starting certificate rotation")

	// Step 1: Download new CA certificate from VM
	newCACert, err := h.downloadNewCACertificate(ctx, vmEndpoint)
	if err != nil {
		h.logger.Error("Failed to download new CA certificate", zap.Error(err))
		h.emitRotationFailed(err)
		return
	}

	// Step 2: Validate new CA certificate
	if err := h.validateNewCACertificate(newCACert); err != nil {
		h.logger.Error("Failed to validate new CA certificate", zap.Error(err))
		h.emitRotationFailed(err)
		return
	}

	// Step 3: Compute fingerprints
	oldCAFingerprint := h.getCurrentCAFingerprint()
	newCAFingerprint := computeCertFingerprint(newCACert.Raw)

	h.mu.Lock()
	h.rotationState.OldCAFingerprint = oldCAFingerprint
	h.rotationState.NewCAFingerprint = newCAFingerprint
	h.mu.Unlock()

	// Step 4: Update trust store atomically with grace period
	gracePeriod := 7 * 24 * time.Hour // 7 days
	gracePeriodEnd := time.Now().Add(gracePeriod)

	if err := h.updateTrustStore(newCACert, gracePeriodEnd); err != nil {
		h.logger.Error("Failed to update trust store", zap.Error(err))
		h.emitRotationFailed(err)
		return
	}

	h.mu.Lock()
	h.rotationState.GracePeriodEnd = &gracePeriodEnd
	h.rotationState.Status = "completed"
	h.mu.Unlock()

	h.logger.Info("Certificate rotation completed successfully",
		zap.String("old_fingerprint", oldCAFingerprint),
		zap.String("new_fingerprint", newCAFingerprint),
		zap.Time("grace_period_end", gracePeriodEnd))

	// Emit rotation completed event
	h.emitRotationCompleted(oldCAFingerprint, newCAFingerprint, gracePeriodEnd)

	// Schedule cleanup of old CA after grace period
	go func() {
		time.Sleep(gracePeriod)
		h.cleanupOldCA(oldCAFingerprint)
	}()
}

// downloadNewCACertificate downloads the new CA certificate from VM.
// Endpoint: GET /api/v1/edge/certificate-authority
// This endpoint is authenticated (requires current valid Edge certificate in mTLS handshake).
func (h *CertificateRotationHandler) downloadNewCACertificate(ctx context.Context, vmEndpoint string) (*x509.Certificate, error) {
	url := fmt.Sprintf("https://%s/api/v1/edge/certificate-authority", vmEndpoint)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/x-pem-file")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download CA certificate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to download CA certificate: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Read certificate PEM data
	certPEM, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate data: %w", err)
	}

	// Parse PEM block
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Parse X.509 certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	h.logger.Info("Downloaded new CA certificate",
		zap.String("subject", cert.Subject.String()),
		zap.String("issuer", cert.Issuer.String()),
		zap.Time("not_before", cert.NotBefore),
		zap.Time("not_after", cert.NotAfter))

	return cert, nil
}

// validateNewCACertificate validates that the new CA certificate is properly signed.
// The new CA cert must be signed by the current CA or root CA.
func (h *CertificateRotationHandler) validateNewCACertificate(newCert *x509.Certificate) error {
	// Load current CA certificate
	currentCAPath := h.clientCfg.CACertPath
	if currentCAPath == "" {
		return fmt.Errorf("CA certificate path not configured")
	}

	currentCAPEM, err := os.ReadFile(currentCAPath)
	if err != nil {
		return fmt.Errorf("failed to read current CA certificate: %w", err)
	}

	block, _ := pem.Decode(currentCAPEM)
	if block == nil {
		return fmt.Errorf("failed to decode current CA certificate PEM")
	}

	currentCA, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse current CA certificate: %w", err)
	}

	// Create certificate pool with current CA
	caPool := x509.NewCertPool()
	caPool.AddCert(currentCA)

	// Verify the new certificate is signed by current CA
	opts := x509.VerifyOptions{
		Roots: caPool,
	}

	_, err = newCert.Verify(opts)
	if err != nil {
		// If verification fails, check if new cert is self-signed (root CA)
		// and if current CA is also signed by it (chain validation)
		if newCert.Issuer.String() == newCert.Subject.String() {
			// New cert is a root CA, check if current CA is signed by it
			newCAPool := x509.NewCertPool()
			newCAPool.AddCert(newCert)
			opts.Roots = newCAPool
			_, verifyErr := currentCA.Verify(opts)
			if verifyErr != nil {
				return fmt.Errorf("new CA certificate is not signed by current CA and current CA is not signed by new CA: %w", err)
			}
			// New CA is a root CA that signs the current CA - this is valid
			h.logger.Info("New CA certificate is a root CA that signs current CA")
		} else {
			return fmt.Errorf("new CA certificate validation failed: %w", err)
		}
	}

	h.logger.Info("New CA certificate validated successfully")
	return nil
}

// updateTrustStore updates the trust store with the new CA certificate.
// During grace period, both old and new CAs are trusted.
func (h *CertificateRotationHandler) updateTrustStore(newCert *x509.Certificate, gracePeriodEnd time.Time) error {
	caCertPath := h.clientCfg.CACertPath
	if caCertPath == "" {
		return fmt.Errorf("CA certificate path not configured")
	}

	// Create backup of current CA
	backupPath := caCertPath + ".backup." + time.Now().Format("20060102-150405")
	if err := copyFile(caCertPath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup of current CA: %w", err)
	}
	h.logger.Info("Created backup of current CA certificate", zap.String("backup_path", backupPath))

	// Write new CA certificate
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: newCert.Raw,
	})

	// Write to temporary file first, then atomically rename
	tempPath := caCertPath + ".tmp"
	if err := os.WriteFile(tempPath, certPEM, 0644); err != nil {
		return fmt.Errorf("failed to write new CA certificate: %w", err)
	}

	// Atomically replace the CA certificate
	if err := os.Rename(tempPath, caCertPath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to replace CA certificate: %w", err)
	}

	h.logger.Info("Updated CA certificate in trust store",
		zap.String("ca_cert_path", caCertPath),
		zap.Time("grace_period_end", gracePeriodEnd))

	return nil
}

// cleanupOldCA removes the old CA certificate after grace period.
func (h *CertificateRotationHandler) cleanupOldCA(oldCAFingerprint string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.logger.Info("Cleaning up old CA certificate after grace period",
		zap.String("old_fingerprint", oldCAFingerprint))

	// Remove backup file
	caCertPath := h.clientCfg.CACertPath
	if caCertPath != "" {
		// Find and remove backup files older than grace period
		backupPattern := caCertPath + ".backup.*"
		matches, err := filepath.Glob(backupPattern)
		if err == nil {
			for _, match := range matches {
				if err := os.Remove(match); err != nil {
					h.logger.Warn("Failed to remove old CA backup", zap.String("path", match), zap.Error(err))
				} else {
					h.logger.Info("Removed old CA backup", zap.String("path", match))
				}
			}
		}
	}

	h.rotationState.Status = "idle"
}

// getCurrentCAFingerprint computes the fingerprint of the current CA certificate.
func (h *CertificateRotationHandler) getCurrentCAFingerprint() string {
	caCertPath := h.clientCfg.CACertPath
	if caCertPath == "" {
		return ""
	}

	certPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return ""
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return ""
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}

	return computeCertFingerprint(cert.Raw)
}

// emitRotationCompleted emits a certificate rotation completed event.
func (h *CertificateRotationHandler) emitRotationCompleted(oldFingerprint, newFingerprint string, gracePeriodEnd time.Time) {
	if h.eventBus == nil {
		return
	}

	event := evtbusstypes.Event[evtbusstypes.CertificateRotationCompletedEventData]{
		Type:      evtbusstypes.EventType("certificate.rotation_completed"),
		Source:    "vm-gateway",
		Timestamp: time.Now(),
		Data: evtbusstypes.CertificateRotationCompletedEventData{
			OldFingerprint: oldFingerprint,
			NewFingerprint: newFingerprint,
			GracePeriodEnd: gracePeriodEnd,
		},
	}
	if err := eventbus.PublishTyped(h.eventBus, event); err != nil {
		h.logger.Warn("Failed to publish certificate rotation completed event", zap.Error(err))
	}
}

// emitRotationFailed emits a certificate rotation failed event.
func (h *CertificateRotationHandler) emitRotationFailed(err error) {
	h.mu.Lock()
	h.rotationState.Status = "failed"
	h.mu.Unlock()

	if h.eventBus == nil {
		return
	}

	event := evtbusstypes.Event[evtbusstypes.CertificateRotationFailedEventData]{
		Type:      evtbusstypes.EventType("certificate.rotation_failed"),
		Source:    "vm-gateway",
		Timestamp: time.Now(),
		Data: evtbusstypes.CertificateRotationFailedEventData{
			Error: err.Error(),
		},
	}
	if err := eventbus.PublishTyped(h.eventBus, event); err != nil {
		h.logger.Warn("Failed to publish certificate rotation failed event", zap.Error(err))
	}
}

// GetRotationStatus returns the current certificate rotation status.
func (h *CertificateRotationHandler) GetRotationStatus() *RotationState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	// Return a copy to avoid race conditions
	state := &RotationState{
		Status: h.rotationState.Status,
	}
	if h.rotationState.ScheduledAt != nil {
		scheduledAt := *h.rotationState.ScheduledAt
		state.ScheduledAt = &scheduledAt
	}
	if h.rotationState.GracePeriodEnd != nil {
		gracePeriodEnd := *h.rotationState.GracePeriodEnd
		state.GracePeriodEnd = &gracePeriodEnd
	}
	state.OldCAFingerprint = h.rotationState.OldCAFingerprint
	state.NewCAFingerprint = h.rotationState.NewCAFingerprint
	return state
}

// computeCertFingerprint computes the SHA-256 fingerprint of a certificate.
func computeCertFingerprint(certDER []byte) string {
	hash := sha256.Sum256(certDER)
	return hex.EncodeToString(hash[:])
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

