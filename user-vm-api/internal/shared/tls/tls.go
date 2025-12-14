package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc/credentials"
)

// LoadServerCredentials loads TLS server credentials from certificate files
func LoadServerCredentials(certFile, keyFile, caCertFile string) (credentials.TransportCredentials, error) {
	// Load server certificate and key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Load CA certificate for client verification (mTLS)
	caCert, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	// Configure TLS
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert, // mTLS: require client certificate
		MinVersion:   tls.VersionTLS12,
		// Allow connections from IP addresses (certificate has IP:10.0.0.1 in SAN)
		// ServerName verification is handled by certificate SAN matching
	}

	return credentials.NewTLS(tlsConfig), nil
}

// LoadClientCredentials loads TLS client credentials from certificate files
// serverName should match the certificate's CN or DNS SAN entry for proper TLS verification
// When connecting via IP address, ServerName must be set to match the certificate
func LoadClientCredentials(certFile, keyFile, caCertFile string, serverName string) (credentials.TransportCredentials, error) {
	// Load client certificate and key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	// Load CA certificate for server verification
	caCert, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	// Configure TLS
	// For IP-based connections (e.g., 10.0.0.1, 10.0.0.2), we need to set ServerName to match
	// the certificate's CN or DNS SAN entry for proper verification.
	// Even though we connect via IP, we must provide the server name for TLS verification.
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
		ServerName:   serverName, // Match certificate CN/DNS SAN (required for TLS verification)
	}

	return credentials.NewTLS(tlsConfig), nil
}
