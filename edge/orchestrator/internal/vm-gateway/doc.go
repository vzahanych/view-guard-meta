/*
Package vmgateway provides a unified, provider-agnostic interface for Edge↔VM bidirectional secure communication.

The VM Gateway is the core service that manages communication between Edge appliances and the VM (Virtual Machine).
It abstracts away the details of tunnel and transport implementations, allowing the system to work with any
combination of tunnel providers (WireGuard, OpenVPN, IPSec, etc.) and transport providers (HTTP, gRPC, WebSocket, etc.).

# Architecture

The gateway is composed of two core abstractions:

	┌─────────────────────────────────────────────────────────┐
	│                    VMGateway Interface                    │
	│  (Unified API for Edge↔VM Communication)                │
	└─────────────────────────────────────────────────────────┘
	                          │
	          ┌───────────────┴───────────────┐
	          │                                 │
	┌─────────▼──────────┐         ┌───────────▼──────────┐
	│ Tunnel Client      │         │ Transport Service     │
	│ Service            │         │                      │
	│                    │         │                      │
	│ - WireGuard        │         │ - HTTP/HTTPS         │
	│ - OpenVPN (future) │         │ - gRPC (future)      │
	│ - IPSec (future)   │         │ - WebSocket (future) │
	└────────────────────┘         └──────────────────────┘

The gateway coordinates the lifecycle of both services:
  - Tunnel service is started first (establishes secure network connection)
  - Transport service is started second (uses tunnel for communication)
  - Services are stopped in reverse order

# Provider Agnosticism

The gateway is designed to be provider-agnostic:

  - Tunnel providers: WireGuard (current), OpenVPN, IPSec (future)
  - Transport providers: HTTP (current), gRPC, WebSocket (future)
  - Device types: Cameras, sensors, audio devices, and other IoT devices

This allows the system to:
  - Switch tunnel/transport providers without changing application code
  - Support different deployment scenarios (production with tunnel, dev without tunnel)
  - Add new providers by implementing the appropriate interfaces

# Security Features

## Certificate Pinning

Certificate pinning provides defense-in-depth against man-in-the-middle (MITM) attacks by verifying
the CA certificate fingerprint during TLS handshake. This ensures that even if an attacker compromises
the certificate authority, they cannot intercept traffic.

Configuration:

	certificate_pinning:
	  pinning_enabled: true
	  vm_ca_fingerprint: "a1b2c3d4e5f6..."  # 64-char hex SHA-256 fingerprint
	  edge_ca_fingerprint: "f6e5d4c3b2a1..."  # 64-char hex SHA-256 fingerprint

How it works:
  - Client-side: Edge verifies VM CA fingerprint when connecting to VM
  - Server-side: Edge verifies VM client certificate fingerprint when VM connects to Edge
  - Fingerprints are SHA-256 hashes of the CA certificate's DER-encoded bytes
  - Validation occurs during TLS handshake before any data is exchanged

Security best practices:
  - Always enable certificate pinning in production
  - Store fingerprints securely (not in version control)
  - Rotate certificates and update fingerprints together
  - Use a grace period during certificate rotation

## Certificate Rotation

Certificate rotation allows updating CA certificates without service interruption. The gateway supports
scheduled rotation with grace periods to ensure zero-downtime updates.

Configuration:

	certificate_rotation:
	  enabled: true
	  grace_period_days: 7  # Grace period for old certificates

How it works:
  - VM schedules rotation via capabilities sync response
  - Edge receives rotation schedule and prepares new certificates
  - During grace period, both old and new certificates are accepted
  - After grace period, only new certificates are accepted
  - Rotation events are emitted for monitoring

Rotation states:
  - "idle": No rotation scheduled
  - "scheduled": Rotation scheduled for future time
  - "in_progress": Rotation is currently happening
  - "completed": Rotation completed successfully
  - "failed": Rotation failed (requires manual intervention)

## Certificate Revocation

Certificate revocation checking validates that certificates have not been revoked using CRL (Certificate
Revocation List) or OCSP (Online Certificate Status Protocol).

Configuration:

	certificate_revocation:
	  enabled: true
	  check_crl: true
	  check_ocsp: true
	  cache_ttl: "1h"  # Cache revocation status for 1 hour

How it works:
  - Client-side: Edge checks VM certificate revocation status
  - Server-side: Edge checks VM client certificate revocation status
  - CRL: Downloads and checks Certificate Revocation List
  - OCSP: Queries Online Certificate Status Protocol responder
  - Caching: Reduces network overhead by caching revocation status
  - Revoked certificates are rejected immediately

## Time Synchronization

Time synchronization ensures Edge and VM clocks are within acceptable drift. This is critical for:
  - Certificate validity checks
  - Audit log timestamps
  - Event ordering
  - Security protocol correctness

Configuration:

	time_sync:
	  enabled: true
	  tolerance_minutes: 5      # Warning threshold (±5 minutes)
	  critical_drift_minutes: 30  # Critical threshold (±30 minutes)

How it works:
  - Time is extracted from VM certificate validity period during mTLS handshake
  - Edge time is compared with VM time
  - Drift within tolerance: Warning event, authentication continues
  - Drift exceeds critical threshold: Authentication fails, error event
  - Events: "time_sync.drift_warning", "time_sync.critical_drift"

Best practices:
  - Ensure NTP is configured on both Edge and VM
  - Set tolerance to match your NTP accuracy requirements
  - Monitor time sync events for clock drift issues
  - Critical drift should be large enough to catch real problems but small enough to prevent security issues

# Reliability Features

## Idempotency Keys

Idempotency keys ensure that repeated requests have the same effect as a single request, preventing
duplicate processing of operations.

Format: {EdgeID}-{operation}-{UUID}

Example: "edge-123-sync-devices-a1b2c3d4-e5f6-7890-abcd-ef1234567890"

Configuration:

	# Idempotency keys are automatically generated if not provided
	# Format is enforced: {EdgeID}-{operation}-{UUID}

Supported operations:
  - sync-capabilities: "edge-123-sync-capabilities-{UUID}"
  - sync-devices: "edge-123-sync-devices-{UUID}"
  - sync-data-units: "edge-123-sync-data-units-{UUID}"
  - sync-audit-logs: "edge-123-sync-audit-logs-{UUID}"

How it works:
  - Keys are auto-generated if not provided in request
  - VM uses keys to detect duplicate requests
  - Duplicate requests return the same response without re-processing
  - Keys are included in request headers: "X-Idempotency-Key"

Best practices:
  - Always include idempotency keys for state-changing operations
  - Use the same key when retrying failed requests
  - Keys should be unique per operation attempt
  - Store keys for retry scenarios

## Rate Limiting

Rate limiting protects the Edge from being overwhelmed by VM commands using a token bucket algorithm.

Configuration:

	rate_limit:
	  enabled: true
	  requests_per_minute: 60
	  burst_size: 10
	  per_endpoint_limits:
	    "/api/v1/models/deploy": 5    # 5 requests per minute for model deployment
	    "/api/v1/snapshots/capture": 30  # 30 requests per minute for snapshots

How it works:
  - Token bucket algorithm: Tokens refill at constant rate, allow bursts up to bucket size
  - Per-client tracking: Rate limits are tracked per client certificate fingerprint
  - Per-endpoint limits: Different endpoints can have different rate limits
  - HTTP 429 response: Rate-limited requests return "Too Many Requests" with Retry-After header
  - Events: "vm_gateway.rate_limit_exceeded" events are emitted

Default limits:
  - Global: 60 requests per minute, burst of 10
  - Model deployment: 5 requests per minute (resource-intensive)
  - Data unit capture: 30 requests per minute (more frequent)

Best practices:
  - Set limits based on expected VM command frequency
  - Use per-endpoint limits for resource-intensive operations
  - Monitor rate limit events to adjust limits
  - Ensure Retry-After header is respected by VM

## Timeout Configuration

Timeout configuration ensures operations complete within acceptable timeframes, preventing resource
exhaustion and improving system responsiveness.

Configuration:

	timeouts:
	  tunnel_establishment_timeout: "30s"
	  transport_establishment_timeout: "30s"
	  authentication_timeout: "30s"
	  vm_api_request_timeout: "30s"
	  vm_command_processing_timeout: "10s"

Timeout types:
  - TunnelEstablishmentTimeout: Maximum time to wait for tunnel connection
  - TransportEstablishmentTimeout: Maximum time to wait for transport (HTTPS server/client) readiness
  - AuthenticationTimeout: Maximum time allowed for authentication handshake
  - VMAPIRequestTimeout: Default timeout for Edge → VM API requests
  - VMCommandProcessingTimeout: Default timeout for processing VM → Edge commands

How it works:
  - All timeouts use context.WithTimeout() for cancellation
  - Timeouts are enforced at the operation level
  - Context cancellation propagates to all dependent operations
  - Timeout errors are returned with context.DeadlineExceeded

Best practices:
  - Set timeouts based on network latency and operation complexity
  - Use shorter timeouts for frequent operations (heartbeats, commands)
  - Use longer timeouts for infrequent operations (authentication, model deployment)
  - Monitor timeout events to adjust values
  - Consider network conditions when setting timeouts

## Retry and Backoff Strategies

Retry strategies handle transient failures with exponential backoff and jitter to prevent overwhelming
the system during outages.

Configuration:

	retry:
	  max_retries: 3
	  initial_backoff: "1s"
	  max_backoff: "60s"
	  backoff_multiplier: 2.0
	  jitter_enabled: true

Retry strategies:
  - Authentication: Custom backoff (10s, 20s, 40s, max 5min) - longer delays for auth
  - VM API calls: Standard backoff (1s, 2s, 4s, 8s, max 60s) - shorter delays for API calls

How it works:
  - Exponential backoff: Delay doubles with each retry attempt
  - Jitter: Random variation (±25%) prevents thundering herd
  - Transient errors: Network errors, 5xx status codes, 429 (rate limit), 408 (timeout)
  - Permanent errors: 4xx client errors (except 429, 408), authentication failures
  - Max retries: After max retries, operation fails with last error

Error classification:
  - Transient: Network errors, timeouts, 5xx server errors, 429 (rate limit), 408 (timeout)
  - Permanent: 4xx client errors (except 429, 408), authentication failures, invalid requests

Best practices:
  - Use jitter to prevent synchronized retries
  - Set max_backoff to prevent excessive delays
  - Distinguish transient vs permanent errors
  - Monitor retry statistics for system health
  - Adjust retry config based on network conditions

# Input Validation and Security

## Request Validation

All VM commands are validated before processing to prevent injection attacks and ensure data integrity.

Validation includes:
  - Request body size limits (configurable per endpoint)
  - String field validation (length, UTF-8, format)
  - ID validation (device_id, model_id, deployment_id) with regex patterns
  - Label validation (normal, threat, abnormal, custom)
  - Count validation (min/max bounds)
  - JSON structure validation
  - Input sanitization (removes null bytes, control characters)

Resource limits:
  - Model deployments: 100MB (configurable)
  - Data unit capture: 1MB
  - Capabilities sync: 1MB
  - Config update: 1MB
  - Service restart: 1KB

Validation errors:
  - HTTP 400 Bad Request for invalid inputs
  - Structured error messages with field names
  - Logged with field names and validation details

## Security Best Practices

1. **Certificate Management**
  - Use strong CA certificates (2048-bit RSA or 256-bit ECDSA minimum)
  - Rotate certificates regularly (annually recommended)
  - Store private keys securely (encrypted at rest, restricted permissions)
  - Never commit certificates or keys to version control
  - Use separate certificates for Edge and VM

2. **Certificate Pinning**
  - Always enable in production environments
  - Store fingerprints securely (environment variables, secrets manager)
  - Update fingerprints when rotating certificates
  - Use grace periods during rotation

3. **Time Synchronization**
  - Configure NTP on all Edge devices and VM
  - Monitor time sync events for drift warnings
  - Set tolerance based on NTP accuracy
  - Investigate critical drift immediately

4. **Rate Limiting**
  - Set limits based on expected load
  - Use per-endpoint limits for resource-intensive operations
  - Monitor rate limit events
  - Adjust limits based on actual usage

5. **Input Validation**
  - Validate all inputs at service boundaries
  - Sanitize user-provided data
  - Use strict type validation
  - Enforce size limits to prevent DoS

6. **Timeout Configuration**
  - Set timeouts based on operation complexity
  - Use shorter timeouts for frequent operations
  - Monitor timeout events
  - Adjust based on network conditions

7. **Retry Strategies**
  - Use exponential backoff with jitter
  - Distinguish transient vs permanent errors
  - Set reasonable max retries
  - Monitor retry statistics

8. **Logging Security**
  - Never log sensitive data (certificates, keys, model artifacts, datasets)
  - Log request metadata (method, path, client fingerprint) but not body
  - Use structured logging with appropriate log levels
  - Sanitize error messages before logging

# Configuration Examples

## Production Configuration (with Tunnel)

	edge_id: "edge-123"
	transport_provider: "http"

	tunnel:
	  provider: "wireguard"
	  enabled: true
	  kvm_endpoint: "10.0.0.1:51820"
	  interface_name: "wg0"
	  raw_config:
	    config_path: "/etc/wireguard/wg0.conf"

	https_server_config:
	  listen_address: "10.0.0.2:8443"
	  server_cert_path: "/etc/ssl/certs/edge-server.crt"
	  server_key_path: "/etc/ssl/private/edge-server.key"
	  ca_cert_path: "/etc/ssl/certs/ca.crt"
	  certificate_pinning:
	    pinning_enabled: true
	    edge_ca_fingerprint: "a1b2c3d4e5f6..."
	  certificate_revocation:
	    enabled: true
	    check_crl: true
	    check_ocsp: true
	    cache_ttl: "1h"
	  rate_limit:
	    enabled: true
	    requests_per_minute: 60
	    burst_size: 10

	https_client_config:
	  vm_endpoint: "10.0.0.1:8443"
	  client_cert_path: "/etc/ssl/certs/edge-client.crt"
	  client_key_path: "/etc/ssl/private/edge-client.key"
	  ca_cert_path: "/etc/ssl/certs/ca.crt"
	  certificate_pinning:
	    pinning_enabled: true
	    vm_ca_fingerprint: "f6e5d4c3b2a1..."
	  certificate_revocation:
	    enabled: true
	    check_crl: true
	    check_ocsp: true
	    cache_ttl: "1h"
	  time_sync:
	    enabled: true
	    tolerance_minutes: 5
	    critical_drift_minutes: 30

	timeouts:
	  tunnel_establishment_timeout: "30s"
	  transport_establishment_timeout: "30s"
	  authentication_timeout: "30s"
	  vm_api_request_timeout: "30s"
	  vm_command_processing_timeout: "10s"

	retry:
	  max_retries: 3
	  initial_backoff: "1s"
	  max_backoff: "60s"
	  backoff_multiplier: 2.0
	  jitter_enabled: true

## Development Configuration (localhost, no tunnel)

	edge_id: "edge-dev"
	transport_provider: "http"

	tunnel:
	  enabled: false

	https_server_config:
	  listen_address: "localhost:8443"
	  server_cert_path: "/tmp/edge-server.crt"
	  server_key_path: "/tmp/edge-server.key"
	  ca_cert_path: "/tmp/ca.crt"
	  certificate_pinning:
	    pinning_enabled: false  # Disabled for dev
	  rate_limit:
	    enabled: false  # Disabled for dev

	https_client_config:
	  vm_endpoint: "localhost:8443"
	  client_cert_path: "/tmp/edge-client.crt"
	  client_key_path: "/tmp/edge-client.key"
	  ca_cert_path: "/tmp/ca.crt"
	  certificate_pinning:
	    pinning_enabled: false  # Disabled for dev
	  time_sync:
	    enabled: false  # Disabled for dev

	timeouts:
	  tunnel_establishment_timeout: "5s"
	  transport_establishment_timeout: "5s"
	  authentication_timeout: "10s"
	  vm_api_request_timeout: "10s"
	  vm_command_processing_timeout: "5s"

	retry:
	  max_retries: 1  # Fewer retries for dev
	  initial_backoff: "500ms"
	  max_backoff: "5s"
	  backoff_multiplier: 2.0
	  jitter_enabled: true

# Connection State Machine

The gateway tracks connection state through a state machine:

	disconnected → tunnel_connecting → tunnel_connected → transport_connecting →
	transport_connected → authenticated → capabilities_received

Error states can recover:
  - tunnel_connection_error → tunnel_connected (recovery)
  - transport_connection_error → transport_connected (recovery)
  - error → disconnected (recovery)

# Health Monitoring

The gateway provides comprehensive health monitoring through the HealthSnapshot() method:

	status := gateway.HealthSnapshot()

Health metrics include:
  - Connection state information
  - Tunnel status (connected, interface name, endpoint)
  - Transport status (connected, service name)
  - Certificate rotation status
  - Time synchronization status
  - Rate limiting statistics
  - Retry statistics
  - Event emission statistics

# Usage

The gateway is typically created using dependency injection (Fx):

	gateway, err := VMGatewayProvider(
	    lc,           // fx.Lifecycle
	    cfg,          // *types.VMGatewayConfig
	    metaStore,    // metastorage.MetaDataStore
	    objectStore,  // objectstorage.ObjectStorageService
	    eventBus,     // eventbus.EventBus
	    logger,       // *zap.Logger
	)

The gateway manages its own lifecycle and starts/stops sub-services automatically.

# Recent Refactoring

This package has been refactored to be fully provider-agnostic:

  - Types consolidated: All VM API types are in vm-gateway/types
  - Tunnel config promoted: TunnelConfig is now top-level in VMGatewayConfig
  - State machine generalized: States use generic tunnel/transport terminology
  - Interfaces moved: TransportService and TunnelClientService are in types package
  - Method names generalized: IsTransportConnected(), GetTunnelInterfaceName(), etc.
  - Error handling standardized: Sentinel errors and consistent error wrapping
  - Context handling fixed: Contexts flow from callers, not created in constructors
  - Observability enhanced: HealthSnapshot() method for debugging
  - Comments updated: All comments use provider-agnostic terminology
  - Security enhanced: Certificate pinning, rotation, revocation, time sync
  - Reliability enhanced: Idempotency keys, rate limiting, timeouts, retries
  - Input validation: Comprehensive validation and sanitization

# Examples

See the Example* functions in the test files for usage examples.
*/
package vmgateway
