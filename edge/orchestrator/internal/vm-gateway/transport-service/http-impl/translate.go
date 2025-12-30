package httpimpl

import (
	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-client-service/types"
	httpsservertypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/transport-service/http-impl/https-server-service/types"
	vmgatewaytypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/types"
)

// Config translation functions - convert transport-agnostic config to HTTP-specific config

// ToHTTPHTTPServerConfig converts API HTTPS server config to HTTP-specific config
func ToHTTPHTTPServerConfig(cfg *vmgatewaytypes.HTTPServerConfig) *httpsservertypes.HTTPServerConfig {
	if cfg == nil {
		return nil
	}
	return &httpsservertypes.HTTPServerConfig{
		ListenAddress:                  cfg.ListenAddress,
		ServerCertPath:                 cfg.ServerCertPath,
		ServerKeyPath:                  cfg.ServerKeyPath,
		CACertPath:                     cfg.CACertPath,
		ReadTimeout:                    cfg.ReadTimeout,
		WriteTimeout:                   cfg.WriteTimeout,
		IdleTimeout:                    cfg.IdleTimeout,
		TunnelInterfaceWaitTimeout:     cfg.TunnelInterfaceWaitTimeout,
		TunnelInterfaceCheckInterval:   cfg.TunnelInterfaceCheckInterval,
		MultipartFormMaxMemory:         cfg.MultipartFormMaxMemory,
		CertificatePinning:             cfg.CertificatePinning,
		CertificateRevocation:          cfg.CertificateRevocation,
		RateLimit:                      cfg.RateLimit,
		Timeouts:                       cfg.Timeouts,
	}
}

// ToHTTPHTTPSClientConfig converts API HTTPS client config to HTTP-specific config
func ToHTTPHTTPSClientConfig(cfg *vmgatewaytypes.HTTPSClientConfig) *httpsclienttypes.HTTPSClientConfig {
	if cfg == nil {
		return nil
	}
	return &httpsclienttypes.HTTPSClientConfig{
		VMEndpoint:            cfg.VMEndpoint,
		ClientCertPath:        cfg.ClientCertPath,
		ClientKeyPath:         cfg.ClientKeyPath,
		CACertPath:            cfg.CACertPath,
		Timeout:               cfg.Timeout,
		CertificatePinning:    cfg.CertificatePinning,
		CertificateRevocation: cfg.CertificateRevocation,
		TimeSync:              cfg.TimeSync,
	}
}

// Translation functions between transport-agnostic API types and HTTP-specific types.
// These functions allow the gateway to maintain a clean API boundary while
// delegating to HTTP-specific implementations.

// ToHTTPGetConfigResponse converts API response to HTTP response (types are identical, just copy)
func ToHTTPGetConfigResponse(resp *vmgatewaytypes.GetConfigResponse) *httpsclienttypes.GetConfigResponse {
	if resp == nil {
		return nil
	}
	return &httpsclienttypes.GetConfigResponse{
		Success:      resp.Success,
		ConfigJSON:   resp.ConfigJSON,
		ErrorMessage: resp.ErrorMessage,
	}
}

// FromHTTPGetConfigResponse converts HTTP response to API response
func FromHTTPGetConfigResponse(resp *httpsclienttypes.GetConfigResponse) *vmgatewaytypes.GetConfigResponse {
	if resp == nil {
		return nil
	}
	return &vmgatewaytypes.GetConfigResponse{
		Success:      resp.Success,
		ConfigJSON:   resp.ConfigJSON,
		ErrorMessage: resp.ErrorMessage,
	}
}

// Note: Translation functions for SyncCapabilities, SyncDevices, SyncDataUnits, and SyncAuditLogs
// have been removed. These types are now defined only in vm-gateway/types/api.go and used directly
// throughout the codebase. This eliminates duplicate type definitions and unnecessary translation code.
//
// The following functions were removed as part of type consolidation (Section 1.2.1):
// - ToHTTPSyncCapabilitiesRequest / FromHTTPSyncCapabilitiesResponse
// - ToHTTPSyncDevicesRequest / FromHTTPSyncDevicesResponse
// - ToHTTPSyncDataUnitsRequest / FromHTTPSyncDataUnitsResponse
// - ToHTTPSyncAuditLogsRequest / FromHTTPSyncAuditLogsResponse
//
// All code now uses vmgatewaytypes directly, eliminating the need for translation between
// vm-gateway/types and https-client-service/types for these types.

// Note: Translation functions for HeartbeatRequest, TelemetryData, and Event have been removed.
// These types are now defined only in vm-gateway/types/api.go and used directly throughout the codebase.
// This eliminates duplicate type definitions and unnecessary translation code.
