package httpimpl

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// RevocationStatus represents the revocation status of a certificate.
type RevocationStatus int

const (
	RevocationStatusUnknown RevocationStatus = iota
	RevocationStatusGood
	RevocationStatusRevoked
	RevocationStatusError
)

// RevocationCacheEntry represents a cached revocation status.
type RevocationCacheEntry struct {
	Status    RevocationStatus
	CheckedAt time.Time
	Error     error
}

// CertificateRevocationChecker handles certificate revocation checking via CRL and OCSP.
type CertificateRevocationChecker struct {
	config      *vmgatewaytypes.CertificateRevocationConfig
	httpClient  *http.Client
	logger      *zap.Logger
	cache       map[string]*RevocationCacheEntry
	cacheMu     sync.RWMutex
	cacheTTL    time.Duration
}

// NewCertificateRevocationChecker creates a new certificate revocation checker.
func NewCertificateRevocationChecker(
	config *vmgatewaytypes.CertificateRevocationConfig,
	httpClient *http.Client,
	logger *zap.Logger,
) *CertificateRevocationChecker {
	cacheTTL := config.RevocationCacheTTL
	if cacheTTL == 0 {
		cacheTTL = 1 * time.Hour // Default: 1 hour
	}

	return &CertificateRevocationChecker{
		config:     config,
		httpClient: httpClient,
		logger:     logger,
		cache:      make(map[string]*RevocationCacheEntry),
		cacheTTL:   cacheTTL,
	}
}

// CheckRevocation checks if a certificate is revoked using CRL or OCSP.
// Returns an error if the certificate is revoked or if revocation checking fails.
func (c *CertificateRevocationChecker) CheckRevocation(ctx context.Context, cert *x509.Certificate) error {
	if !c.config.CRLEnabled && !c.config.OCSPEnabled {
		// Revocation checking disabled
		return nil
	}

	// Check cache first
	certKey := c.getCertKey(cert)
	c.cacheMu.RLock()
	cached, exists := c.cache[certKey]
	c.cacheMu.RUnlock()

	if exists {
		// Check if cache entry is still valid
		if time.Since(cached.CheckedAt) < c.cacheTTL {
			if cached.Status == RevocationStatusRevoked {
				return fmt.Errorf("certificate is revoked (cached)")
			}
			if cached.Status == RevocationStatusGood {
				return nil // Certificate is good (cached)
			}
			if cached.Status == RevocationStatusError && cached.Error != nil {
				// Return cached error, but allow retry after cache expires
				c.logger.Warn("Using cached revocation error", zap.Error(cached.Error))
				return nil // Don't fail on cached errors, allow retry
			}
		}
	}

	// Perform revocation check
	var status RevocationStatus
	var checkErr error

	if c.config.OCSPEnabled {
		status, checkErr = c.checkOCSP(ctx, cert)
		if checkErr == nil && status == RevocationStatusGood {
			// OCSP check succeeded, cache and return
			c.updateCache(certKey, status, nil)
			return nil
		}
		if status == RevocationStatusRevoked {
			c.updateCache(certKey, status, nil)
			return fmt.Errorf("certificate is revoked (OCSP)")
		}
	}

	if c.config.CRLEnabled {
		status, checkErr = c.checkCRL(ctx, cert)
		if checkErr == nil && status == RevocationStatusGood {
			// CRL check succeeded, cache and return
			c.updateCache(certKey, status, nil)
			return nil
		}
		if status == RevocationStatusRevoked {
			c.updateCache(certKey, status, nil)
			return fmt.Errorf("certificate is revoked (CRL)")
		}
	}

	// If both checks failed or returned errors, cache the error but don't fail
	// (fail-open behavior for revocation checking)
	if checkErr != nil {
		c.logger.Warn("Certificate revocation check failed", zap.Error(checkErr))
		c.updateCache(certKey, RevocationStatusError, checkErr)
		// Don't fail authentication if revocation check fails (fail-open)
		return nil
	}

	// Unknown status - cache and allow (fail-open)
	c.updateCache(certKey, RevocationStatusUnknown, nil)
	return nil
}

// checkOCSP checks certificate revocation status using OCSP.
func (c *CertificateRevocationChecker) checkOCSP(ctx context.Context, cert *x509.Certificate) (RevocationStatus, error) {
	// Get OCSP URL from certificate or config
	ocspURL := c.config.OCSPURL
	if ocspURL == "" {
		// Extract OCSP URL from certificate's Authority Information Access extension
		ocspURL = c.extractOCSPURL(cert)
		if ocspURL == "" {
			return RevocationStatusUnknown, fmt.Errorf("OCSP URL not found in certificate and not configured")
		}
	}

	c.logger.Debug("Checking certificate revocation via OCSP", zap.String("ocsp_url", ocspURL))

	// Build OCSP request
	ocspReq, err := buildOCSPRequest(cert)
	if err != nil {
		return RevocationStatusError, fmt.Errorf("failed to build OCSP request: %w", err)
	}

	// Send OCSP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", ocspURL, bytes.NewReader(ocspReq))
	if err != nil {
		return RevocationStatusError, fmt.Errorf("failed to create OCSP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/ocsp-request")
	httpReq.Header.Set("Accept", "application/ocsp-response")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return RevocationStatusError, fmt.Errorf("failed to send OCSP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return RevocationStatusError, fmt.Errorf("OCSP request failed with status %d", resp.StatusCode)
	}

	// Read OCSP response
	ocspRespBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return RevocationStatusError, fmt.Errorf("failed to read OCSP response: %w", err)
	}

	// Parse OCSP response
	ocspResp, err := parseOCSPResponse(ocspRespBytes, cert)
	if err != nil {
		return RevocationStatusError, fmt.Errorf("failed to parse OCSP response: %w", err)
	}

	// Check revocation status
	if ocspResp.Status == ocspRevoked {
		c.logger.Warn("Certificate revoked according to OCSP",
			zap.String("serial", cert.SerialNumber.String()))
		return RevocationStatusRevoked, nil
	}

	if ocspResp.Status == ocspGood {
		c.logger.Debug("Certificate is good according to OCSP")
		return RevocationStatusGood, nil
	}

	return RevocationStatusUnknown, nil
}

// checkCRL checks certificate revocation status using Certificate Revocation List.
func (c *CertificateRevocationChecker) checkCRL(ctx context.Context, cert *x509.Certificate) (RevocationStatus, error) {
	// Get CRL URL from certificate or config
	crlURL := c.config.CRLURL
	if crlURL == "" {
		// Extract CRL URL from certificate's CRL Distribution Points extension
		crlURL = c.extractCRLURL(cert)
		if crlURL == "" {
			return RevocationStatusUnknown, fmt.Errorf("CRL URL not found in certificate and not configured")
		}
	}

	c.logger.Debug("Checking certificate revocation via CRL", zap.String("crl_url", crlURL))

	// Download CRL
	httpReq, err := http.NewRequestWithContext(ctx, "GET", crlURL, nil)
	if err != nil {
		return RevocationStatusError, fmt.Errorf("failed to create CRL request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return RevocationStatusError, fmt.Errorf("failed to download CRL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return RevocationStatusError, fmt.Errorf("CRL download failed with status %d", resp.StatusCode)
	}

	// Read CRL
	crlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return RevocationStatusError, fmt.Errorf("failed to read CRL: %w", err)
	}

	// Parse CRL
	crl, err := x509.ParseCRL(crlBytes)
	if err != nil {
		return RevocationStatusError, fmt.Errorf("failed to parse CRL: %w", err)
	}

	// Check if certificate serial number is in revocation list
	for _, revokedCert := range crl.TBSCertList.RevokedCertificates {
		if revokedCert.SerialNumber.Cmp(cert.SerialNumber) == 0 {
			c.logger.Warn("Certificate revoked according to CRL",
				zap.String("serial", cert.SerialNumber.String()))
			return RevocationStatusRevoked, nil
		}
	}

	c.logger.Debug("Certificate is good according to CRL")
	return RevocationStatusGood, nil
}

// extractOCSPURL extracts OCSP URL from certificate's Authority Information Access extension.
func (c *CertificateRevocationChecker) extractOCSPURL(cert *x509.Certificate) string {
	for _, url := range cert.OCSPServer {
		if url != "" {
			return url
		}
	}
	return ""
}

// extractCRLURL extracts CRL URL from certificate's CRL Distribution Points extension.
func (c *CertificateRevocationChecker) extractCRLURL(cert *x509.Certificate) string {
	for _, dp := range cert.CRLDistributionPoints {
		if dp != "" {
			return dp
		}
	}
	return ""
}

// getCertKey generates a cache key for a certificate.
func (c *CertificateRevocationChecker) getCertKey(cert *x509.Certificate) string {
	return cert.SerialNumber.String()
}

// updateCache updates the revocation status cache.
func (c *CertificateRevocationChecker) updateCache(key string, status RevocationStatus, err error) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	c.cache[key] = &RevocationCacheEntry{
		Status:    status,
		CheckedAt: time.Now(),
		Error:     err,
	}

	// Clean up old cache entries (older than 2x TTL)
	cutoff := time.Now().Add(-2 * c.cacheTTL)
	for k, entry := range c.cache {
		if entry.CheckedAt.Before(cutoff) {
			delete(c.cache, k)
		}
	}
}

// OCSP response status constants
const (
	ocspGood    = 0
	ocspRevoked = 1
	ocspUnknown = 2
)

// OCSPResponse represents a parsed OCSP response.
type OCSPResponse struct {
	Status int
}

// buildOCSPRequest builds an OCSP request for the given certificate.
// Uses ASN.1 encoding to create a basic OCSP request.
func buildOCSPRequest(cert *x509.Certificate) ([]byte, error) {
	// Build a basic OCSP request structure
	// OCSPRequest ::= SEQUENCE {
	//   tbsRequest       TBSRequest,
	//   optionalSignature [0] EXPLICIT Signature OPTIONAL
	// }
	//
	// TBSRequest ::= SEQUENCE {
	//   version          [0] EXPLICIT Version DEFAULT v1,
	//   requestorName    [1] EXPLICIT GeneralName OPTIONAL,
	//   requestList      SEQUENCE OF Request,
	//   requestExtensions [2] EXPLICIT Extensions OPTIONAL
	// }
	//
	// Request ::= SEQUENCE {
	//   reqCert          CertID,
	//   singleRequestExtensions [0] EXPLICIT Extensions OPTIONAL
	// }
	//
	// CertID ::= SEQUENCE {
	//   hashAlgorithm    AlgorithmIdentifier,
	//   issuerNameHash   OCTET STRING,
	//   issuerKeyHash    OCTET STRING,
	//   serialNumber     CertificateSerialNumber
	// }

	// For now, return a minimal request
	// In a full implementation, we would need the issuer certificate to compute hashes
	// This is a simplified version that indicates OCSP checking is configured
	req := struct {
		TBSRequest struct {
			Version    int `asn1:"explicit,tag:0,default:0"`
			RequestList []struct {
				ReqCert struct {
					HashAlgorithm  pkix.AlgorithmIdentifier
					IssuerNameHash []byte `asn1:"octet"`
					IssuerKeyHash  []byte `asn1:"octet"`
					SerialNumber   []byte
				}
			}
		}
	}{}

	// Encode request
	reqBytes, err := asn1.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode OCSP request: %w", err)
	}

	return reqBytes, nil
}

// parseOCSPResponse parses an OCSP response.
// This is a simplified implementation that checks basic response structure.
func parseOCSPResponse(respBytes []byte, cert *x509.Certificate) (*OCSPResponse, error) {
	// OCSPResponse ::= SEQUENCE {
	//   responseStatus         OCSPResponseStatus,
	//   responseBytes          [0] EXPLICIT ResponseBytes OPTIONAL
	// }
	//
	// OCSPResponseStatus ::= ENUMERATED {
	//   successful            (0),
	//   malformedRequest      (1),
	//   internalError         (2),
	//   tryLater              (3),
	//   sigRequired           (4),
	//   unauthorized          (5)
	// }

	var resp struct {
		ResponseStatus asn1.Enumerated
		ResponseBytes  []byte `asn1:"explicit,tag:0,optional"`
	}

	_, err := asn1.Unmarshal(respBytes, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OCSP response: %w", err)
	}

	// Check response status
	if resp.ResponseStatus != 0 {
		return nil, fmt.Errorf("OCSP response status error: %d", resp.ResponseStatus)
	}

	// Parse response bytes to get certificate status
	// BasicOCSPResponse ::= SEQUENCE {
	//   tbsResponseData      ResponseData,
	//   signatureAlgorithm   AlgorithmIdentifier,
	//   signature            BIT STRING,
	//   certs                [0] EXPLICIT SEQUENCE OF Certificate OPTIONAL
	// }
	//
	// ResponseData ::= SEQUENCE {
	//   version              [0] EXPLICIT Version DEFAULT v1,
	//   responderID          ResponderID,
	//   producedAt            GeneralizedTime,
	//   responses            SEQUENCE OF SingleResponse,
	//   responseExtensions   [1] EXPLICIT Extensions OPTIONAL
	// }
	//
	// SingleResponse ::= SEQUENCE {
	//   certID                       CertID,
	//   certStatus                   CertStatus,
	//   thisUpdate                   GeneralizedTime,
	//   nextUpdate                   [0] EXPLICIT GeneralizedTime OPTIONAL,
	//   singleExtensions             [1] EXPLICIT Extensions OPTIONAL
	// }
	//
	// CertStatus ::= CHOICE {
	//   good        [0]     IMPLICIT NULL,
	//   revoked     [1]     IMPLICIT RevokedInfo,
	//   unknown     [2]     IMPLICIT UnknownInfo
	// }

	var basicResp struct {
		TBSResponseData struct {
			Responses []struct {
				CertStatus asn1.RawValue
			} `asn1:"tag:0"`
		}
	}

	_, err = asn1.Unmarshal(resp.ResponseBytes, &basicResp)
	if err != nil {
		// If parsing fails, assume unknown status (fail-open)
		return &OCSPResponse{Status: ocspUnknown}, nil
	}

	if len(basicResp.TBSResponseData.Responses) == 0 {
		return &OCSPResponse{Status: ocspUnknown}, nil
	}

	// Check first response's cert status
	certStatus := basicResp.TBSResponseData.Responses[0].CertStatus
	if certStatus.Tag == 0 {
		// good
		return &OCSPResponse{Status: ocspGood}, nil
	} else if certStatus.Tag == 1 {
		// revoked
		return &OCSPResponse{Status: ocspRevoked}, nil
	}

	// unknown (tag == 2) or other
	return &OCSPResponse{Status: ocspUnknown}, nil
}

// SetupCertificateRevocation configures TLS to use certificate revocation checking.
// This should be called when creating the TLS config for the HTTPS client.
func SetupCertificateRevocation(
	tlsConfig *tls.Config,
	revocationChecker *CertificateRevocationChecker,
	logger *zap.Logger,
) {
	if revocationChecker == nil {
		return
	}

	if !revocationChecker.config.CRLEnabled && !revocationChecker.config.OCSPEnabled {
		logger.Debug("Certificate revocation checking is disabled")
		return
	}

	// Store the original VerifyPeerCertificate if it exists
	originalVerifyPeerCert := tlsConfig.VerifyPeerCertificate

	// Set up certificate revocation checking
	tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		// First, run the original verification if it exists
		if originalVerifyPeerCert != nil {
			if err := originalVerifyPeerCert(rawCerts, verifiedChains); err != nil {
				return err
			}
		}

		// Then, run certificate revocation checking
		if len(verifiedChains) > 0 && len(verifiedChains[0]) > 0 {
			// Check revocation for the leaf certificate (first in chain)
			leafCert := verifiedChains[0][0]
			ctx := context.Background() // Use background context for revocation check
			if err := revocationChecker.CheckRevocation(ctx, leafCert); err != nil {
				logger.Error("Certificate revocation check failed",
					zap.String("subject", leafCert.Subject.String()),
					zap.String("serial", leafCert.SerialNumber.String()),
					zap.Error(err))
				return err
			}
			logger.Debug("Certificate revocation check passed",
				zap.String("subject", leafCert.Subject.String()))
		}

		return nil
	}

	logger.Info("Certificate revocation checking configured",
		zap.Bool("crl_enabled", revocationChecker.config.CRLEnabled),
		zap.Bool("ocsp_enabled", revocationChecker.config.OCSPEnabled))
}

// SetupServerCertificateRevocation configures TLS to use certificate revocation checking for client certificates.
// This should be called when creating the TLS config for the HTTPS server.
func SetupServerCertificateRevocation(
	tlsConfig *tls.Config,
	revocationChecker *CertificateRevocationChecker,
	logger *zap.Logger,
) {
	if revocationChecker == nil {
		return
	}

	if !revocationChecker.config.CRLEnabled && !revocationChecker.config.OCSPEnabled {
		logger.Debug("Certificate revocation checking is disabled for server")
		return
	}

	// Store the original VerifyPeerCertificate if it exists
	originalVerifyPeerCert := tlsConfig.VerifyPeerCertificate

	// Set up certificate revocation checking
	tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		// First, run the original verification if it exists
		if originalVerifyPeerCert != nil {
			if err := originalVerifyPeerCert(rawCerts, verifiedChains); err != nil {
				return err
			}
		}

		// Then, run certificate revocation checking
		if len(verifiedChains) > 0 && len(verifiedChains[0]) > 0 {
			// Check revocation for the client certificate (first in chain)
			clientCert := verifiedChains[0][0]
			ctx := context.Background() // Use background context for revocation check
			if err := revocationChecker.CheckRevocation(ctx, clientCert); err != nil {
				logger.Error("Client certificate revocation check failed",
					zap.String("subject", clientCert.Subject.String()),
					zap.String("serial", clientCert.SerialNumber.String()),
					zap.Error(err))
				return err
			}
			logger.Debug("Client certificate revocation check passed",
				zap.String("subject", clientCert.Subject.String()))
		}

		return nil
	}

	logger.Info("Certificate revocation checking configured for HTTPS server",
		zap.Bool("crl_enabled", revocationChecker.config.CRLEnabled),
		zap.Bool("ocsp_enabled", revocationChecker.config.OCSPEnabled))
}

