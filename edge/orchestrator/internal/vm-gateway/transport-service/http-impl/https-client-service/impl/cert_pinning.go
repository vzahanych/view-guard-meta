package impl

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"

	"go.uber.org/zap"

	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// VerifyCertificatePinning verifies that the server's CA certificate matches the pinned fingerprint.
// This is called during the TLS handshake to ensure we're connecting to the expected server.
func VerifyCertificatePinning(
	config *vmgatewaytypes.CertificatePinningConfig,
	logger *zap.Logger,
) func(*x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
	return func(rootCAs *x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
		return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			// If pinning is disabled, skip verification
			if !config.PinningEnabled {
				return nil
			}

			// If no pinned fingerprint is configured, skip verification
			if config.VMCAFingerprint == "" {
				logger.Debug("Certificate pinning enabled but no VM CA fingerprint configured, skipping pinning check")
				return nil
			}

			// Extract the CA certificate from the verified chain
			// The last certificate in the chain is the root CA
			if len(verifiedChains) == 0 || len(verifiedChains[0]) == 0 {
				return fmt.Errorf("no verified certificate chain available for pinning check")
			}

			// Find the root CA certificate in the chain
			var rootCA *x509.Certificate
			for _, chain := range verifiedChains {
				if len(chain) > 0 {
					// The last certificate in the chain is typically the root CA
					rootCA = chain[len(chain)-1]
					// If it's self-signed (Issuer == Subject), it's the root CA
					if rootCA.Issuer.String() == rootCA.Subject.String() {
						break
					}
				}
			}

			// If we couldn't find a root CA in the verified chains, try to find it in rawCerts
			if rootCA == nil {
				for _, rawCert := range rawCerts {
					cert, err := x509.ParseCertificate(rawCert)
					if err != nil {
						continue
					}
					// Check if it's a root CA (self-signed)
					if cert.Issuer.String() == cert.Subject.String() {
						rootCA = cert
						break
					}
				}
			}

			if rootCA == nil {
				return fmt.Errorf("could not find root CA certificate in certificate chain for pinning verification")
			}

			// Compute SHA-256 fingerprint of the root CA certificate
			fingerprint := computeFingerprint(rootCA.Raw)

			// Compare with pinned fingerprint (case-insensitive comparison)
			expectedFingerprint := config.VMCAFingerprint
			if len(expectedFingerprint) != 64 {
				return fmt.Errorf("invalid pinned fingerprint length: expected 64 characters (SHA-256 hex), got %d", len(expectedFingerprint))
			}

			// Normalize fingerprints to lowercase for comparison
			fingerprintLower := hex.EncodeToString(fingerprint)
			expectedFingerprintLower := ""
			for _, b := range []byte(expectedFingerprint) {
				if b >= 'A' && b <= 'F' {
					expectedFingerprintLower += string(b + 32) // Convert to lowercase
				} else {
					expectedFingerprintLower += string(b)
				}
			}

			if fingerprintLower != expectedFingerprintLower {
				logger.Error("Certificate pinning verification failed",
					zap.String("expected_fingerprint", expectedFingerprintLower),
					zap.String("actual_fingerprint", fingerprintLower),
					zap.String("certificate_subject", rootCA.Subject.String()),
					zap.String("certificate_issuer", rootCA.Issuer.String()))
				return fmt.Errorf("certificate pinning verification failed: expected fingerprint %s, got %s",
					expectedFingerprintLower, fingerprintLower)
			}

			logger.Debug("Certificate pinning verification succeeded",
				zap.String("fingerprint", fingerprintLower),
				zap.String("certificate_subject", rootCA.Subject.String()))

			return nil
		}
	}
}

// computeFingerprint computes the SHA-256 fingerprint of a certificate in hex format.
func computeFingerprint(certDER []byte) []byte {
	hash := sha256.Sum256(certDER)
	return hash[:]
}

// SetupCertificatePinning configures TLS to use certificate pinning verification.
// This should be called when creating the TLS config for the HTTPS client.
func SetupCertificatePinning(
	tlsConfig *tls.Config,
	pinningConfig *vmgatewaytypes.CertificatePinningConfig,
	logger *zap.Logger,
) {
	if !pinningConfig.PinningEnabled {
		logger.Debug("Certificate pinning is disabled")
		return
	}

	if pinningConfig.VMCAFingerprint == "" {
		logger.Debug("Certificate pinning enabled but no VM CA fingerprint configured")
		return
	}

	// Store the original VerifyPeerCertificate if it exists
	originalVerifyPeerCert := tlsConfig.VerifyPeerCertificate

	// Set up certificate pinning verification
	tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		// First, run the original verification if it exists
		if originalVerifyPeerCert != nil {
			if err := originalVerifyPeerCert(rawCerts, verifiedChains); err != nil {
				return err
			}
		}

		// Then, run certificate pinning verification
		verifyFunc := VerifyCertificatePinning(pinningConfig, logger)(tlsConfig.RootCAs)
		return verifyFunc(rawCerts, verifiedChains)
	}

	logger.Info("Certificate pinning configured for HTTPS client",
		zap.String("vm_ca_fingerprint", pinningConfig.VMCAFingerprint))
}

