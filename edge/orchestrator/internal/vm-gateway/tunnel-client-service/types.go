package tunnelclient

// This package no longer contains duplicate type definitions.
// All VM API types (HeartbeatRequest, TelemetryData, Event) are now defined in
// vm-gateway/types/api.go to maintain a single source of truth.
//
// These types were moved to vm-gateway/types because:
// 1. They are not tunnel-specific - they represent VM API payloads
// 2. Having duplicates in tunnel-client-service caused naming mismatches (EdgeId vs EdgeID)
// 3. Required unnecessary translation code in http-impl/translate.go
//
// All code should now import and use types from vm-gateway/types for these types.

