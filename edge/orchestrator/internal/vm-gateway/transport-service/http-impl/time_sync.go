package httpimpl

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	eventbus "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus"
	evtbusstypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/event-bus/types"
	"go.uber.org/zap"

	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// TimeSyncChecker performs time synchronization checks between Edge and VM.
type TimeSyncChecker struct {
	config   *vmgatewaytypes.TimeSyncConfig
	logger    *zap.Logger
	eventBus  eventbus.EventBus
}

// NewTimeSyncChecker creates a new time synchronization checker.
func NewTimeSyncChecker(
	config *vmgatewaytypes.TimeSyncConfig,
	logger *zap.Logger,
	eventBus eventbus.EventBus,
) *TimeSyncChecker {
	return &TimeSyncChecker{
		config:  config,
		logger:  logger,
		eventBus: eventBus,
	}
}

// CheckTimeSync checks the time synchronization between Edge and VM.
// It extracts the VM clock time from the mTLS handshake by examining the certificate validity period.
// Returns an error if the time drift exceeds the critical threshold.
func (c *TimeSyncChecker) CheckTimeSync(state tls.ConnectionState) error {
	if !c.config.Enabled {
		c.logger.Debug("Time synchronization checking is disabled")
		return nil
	}

	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("no peer certificates found for time synchronization check")
	}

	// Extract VM time from certificate validity period
	// The certificate's NotBefore and NotAfter fields indicate when the certificate was issued
	// and when it expires. We can use the midpoint or NotBefore + some offset to estimate VM time.
	vmCert := state.PeerCertificates[0]
	vmTime := c.extractVMTimeFromCertificate(vmCert)

	// Get Edge time
	edgeTime := time.Now()

	// Calculate time drift
	drift := edgeTime.Sub(vmTime)
	driftAbs := drift
	if driftAbs < 0 {
		driftAbs = -driftAbs
	}

	tolerance := c.config.GetToleranceDuration()
	criticalDrift := c.config.GetCriticalDriftDuration()

	c.logger.Info("Time synchronization check",
		zap.Time("edge_time", edgeTime),
		zap.Time("vm_time", vmTime),
		zap.Duration("drift", drift),
		zap.Duration("tolerance", tolerance),
		zap.Duration("critical_drift", criticalDrift))

	// Check critical drift threshold (>30 minutes by default)
	if driftAbs > criticalDrift {
		errorMsg := fmt.Sprintf("Time synchronization critical: Edge clock drift %.1f minutes exceeds critical threshold of %.1f minutes. Authentication failed. Operator must fix NTP configuration.", driftAbs.Minutes(), criticalDrift.Minutes())
		
		c.logger.Error("Time synchronization critical drift detected",
			zap.Duration("drift", driftAbs),
			zap.Duration("critical_threshold", criticalDrift),
			zap.String("edge_time", edgeTime.Format(time.RFC3339)),
			zap.String("vm_time", vmTime.Format(time.RFC3339)))

		// Emit critical alert event
		if c.eventBus != nil {
			event := evtbusstypes.Event[evtbusstypes.TimeSyncCriticalDriftEventData]{
				Type:      evtbusstypes.EventType("time_sync.critical_drift"),
				Source:    "vm-gateway",
				Timestamp: time.Now(),
				Data: evtbusstypes.TimeSyncCriticalDriftEventData{
					DriftMinutes:      driftAbs.Minutes(),
					CriticalThreshold: criticalDrift.Minutes(),
					EdgeTime:          edgeTime.Format(time.RFC3339),
					VMTime:            vmTime.Format(time.RFC3339),
					ToleranceMinutes:  tolerance.Minutes(),
				},
			}
			if err := eventbus.PublishTyped(c.eventBus, event); err != nil {
				c.logger.Warn("Failed to publish time sync critical drift event", zap.Error(err))
			}
		}

		return fmt.Errorf("time synchronization critical: %s", errorMsg)
	}

	// Check tolerance threshold (±5 minutes by default)
	if driftAbs > tolerance {
		c.logger.Warn("Time synchronization drift exceeds tolerance",
			zap.Duration("drift", driftAbs),
			zap.Duration("tolerance", tolerance),
			zap.String("edge_time", edgeTime.Format(time.RFC3339)),
			zap.String("vm_time", vmTime.Format(time.RFC3339)))

		// Emit warning event (non-critical, authentication continues)
		if c.eventBus != nil {
			event := evtbusstypes.Event[evtbusstypes.TimeSyncDriftWarningEventData]{
				Type:      evtbusstypes.EventType("time_sync.drift_warning"),
				Source:    "vm-gateway",
				Timestamp: time.Now(),
				Data: evtbusstypes.TimeSyncDriftWarningEventData{
					DriftMinutes:     driftAbs.Minutes(),
					ToleranceMinutes: tolerance.Minutes(),
					EdgeTime:         edgeTime.Format(time.RFC3339),
					VMTime:           vmTime.Format(time.RFC3339),
				},
			}
			if err := eventbus.PublishTyped(c.eventBus, event); err != nil {
				c.logger.Warn("Failed to publish time sync drift warning event", zap.Error(err))
			}
		}

		// Continue authentication (warning only)
		return nil
	}

	// Time sync is within tolerance
	c.logger.Debug("Time synchronization check passed",
		zap.Duration("drift", driftAbs),
		zap.Duration("tolerance", tolerance))
	return nil
}

// extractVMTimeFromCertificate extracts an estimate of VM time from the certificate.
// Uses the certificate's NotBefore time as a reference point, assuming the certificate
// was issued recently relative to the VM's current time.
// For more accuracy, we could use the midpoint of the validity period or request
// a timestamp from the VM, but for now we use NotBefore as a conservative estimate.
func (c *TimeSyncChecker) extractVMTimeFromCertificate(cert *x509.Certificate) time.Time {
	// Use NotBefore as the reference time
	// This assumes the certificate was issued at or near the VM's current time
	// In practice, certificates are typically issued with NotBefore set to the current time
	// or slightly in the past (to account for clock skew during issuance)
	
	// For a more accurate estimate, we could:
	// 1. Use the midpoint of the validity period: (NotBefore + NotAfter) / 2
	// 2. Use NotBefore + some offset (e.g., half the certificate lifetime)
	// 3. Request a timestamp from the VM via a separate API call
	
	// For now, we'll use NotBefore as a conservative estimate
	// This will work well if certificates are issued close to the current time
	vmTime := cert.NotBefore
	
	// If the certificate is very old (NotBefore is more than 1 day in the past),
	// we can't reliably estimate VM time from it. In this case, we'll use NotBefore
	// but log a warning.
	age := time.Since(vmTime)
	if age > 24*time.Hour {
		c.logger.Warn("Certificate is old, time estimation may be inaccurate",
			zap.Duration("certificate_age", age),
			zap.Time("not_before", cert.NotBefore),
			zap.Time("not_after", cert.NotAfter))
	}
	
	return vmTime
}

// SetupTimeSyncCheck configures TLS to perform time synchronization checking.
// This should be called when creating the TLS config for the HTTPS client.
func SetupTimeSyncCheck(
	tlsConfig *tls.Config,
	timeSyncChecker *TimeSyncChecker,
	logger *zap.Logger,
) {
	if timeSyncChecker == nil || !timeSyncChecker.config.Enabled {
		logger.Debug("Time synchronization checking is disabled")
		return
	}

	// Store the original VerifyPeerCertificate if it exists
	originalVerifyPeerCert := tlsConfig.VerifyPeerCertificate

	// Set up time synchronization checking
	tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		// First, run the original verification if it exists
		if originalVerifyPeerCert != nil {
			if err := originalVerifyPeerCert(rawCerts, verifiedChains); err != nil {
				return err
			}
		}

		// Then, perform time synchronization check
		// We need to construct a ConnectionState from the verified chains
		// For time sync, we only need the peer certificates
		if len(verifiedChains) > 0 && len(verifiedChains[0]) > 0 {
			// Create a minimal ConnectionState for time sync check
			state := tls.ConnectionState{
				PeerCertificates: verifiedChains[0],
			}
			
			if err := timeSyncChecker.CheckTimeSync(state); err != nil {
				logger.Error("Time synchronization check failed",
					zap.Error(err))
				return err
			}
			logger.Debug("Time synchronization check passed")
		}

		return nil
	}

	logger.Info("Time synchronization checking configured",
		zap.Bool("enabled", timeSyncChecker.config.Enabled),
		zap.Duration("tolerance", timeSyncChecker.config.GetToleranceDuration()),
		zap.Duration("critical_drift", timeSyncChecker.config.GetCriticalDriftDuration()))
}

