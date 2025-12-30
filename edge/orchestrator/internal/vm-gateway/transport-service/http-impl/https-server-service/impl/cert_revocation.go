package impl

import (
	"crypto/tls"
	"net/http"
	"time"

	"go.uber.org/zap"

	httpsclientimpl "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-client-service/impl"
	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// SetupServerCertificateRevocation configures TLS to use certificate revocation checking for client certificates.
// This should be called when creating the TLS config for the HTTPS server.
func SetupServerCertificateRevocation(
	tlsConfig *tls.Config,
	revocationConfig *vmgatewaytypes.CertificateRevocationConfig,
	logger *zap.Logger,
) {
	if revocationConfig == nil {
		return
	}

	if !revocationConfig.CRLEnabled && !revocationConfig.OCSPEnabled {
		logger.Debug("Certificate revocation checking is disabled for server")
		return
	}

	// Create HTTP client for revocation checking
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create revocation checker using the client implementation
	revocationChecker := httpsclientimpl.NewCertificateRevocationChecker(
		revocationConfig,
		httpClient,
		logger,
	)

	// Use the SetupServerCertificateRevocation from the client package
	httpsclientimpl.SetupServerCertificateRevocation(tlsConfig, revocationChecker, logger)
}

