# Edge Orchestrator State Transition

This document describes the state transitions of the Edge Orchestrator during startup and normal operation.

## Overview

The Edge Orchestrator manages the lifecycle of the Edge Appliance, coordinating the establishment of secure communication with the VM and transitioning through various states until it becomes fully operational.

## States

The Edge Orchestrator can be in one of the following states:

1. **`disconnected`** - Initial state when Edge starts. No network connection to VM.
2. **`wireguard_connected`** - WireGuard tunnel is established to VM.
3. **`https_connected`** - HTTPS connection is established over WireGuard tunnel.
4. **`authenticated`** - Edge has successfully authenticated with VM.
5. **`ready`** - Edge is fully operational and ready for normal operations.
6. **`error`** - Error state when something goes wrong.

## State Transition Flow

### Initial State: `disconnected`

When the Edge Orchestrator starts, it begins in the `disconnected` state. In this state:
- Edge has no network connection to the VM
- All VM-related services are not yet operational
- Edge is waiting to establish a WireGuard tunnel

**Actions in this state:**
- Edge reads WireGuard configuration from config file
- Edge initializes WireGuard client service
- Edge attempts to establish WireGuard tunnel to VM endpoint

### Transition: `disconnected` → `wireguard_connected`

**Trigger:** WireGuard tunnel is successfully established.

**Event:** `network.wireguard.connected`

**What happens:**
1. WireGuard client service starts and configures the WireGuard interface
2. WireGuard tunnel is established to the VM endpoint (configured via `KVMEndpoint` in config)
3. Edge can now communicate with VM over the encrypted WireGuard tunnel
4. State transitions to `wireguard_connected`
5. `NetworkConnected` flag is set to `true`

**State properties:**
- `Status`: `wireguard_connected`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `false`

### Transition: `wireguard_connected` → `https_connected`

**Trigger:** HTTPS services are started and ready over WireGuard tunnel.

**Event:** `network.https.connected`

**What happens:**
1. HTTPS server service starts (listens on WireGuard interface for VM → Edge communication)
2. HTTPS client service starts (ready for Edge → VM communication)
3. State transitions to `https_connected`

**State properties:**
- `Status`: `https_connected`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `false`

### Transition: `https_connected` → `authenticated`

**Trigger:** Edge successfully authenticates with VM using the HTTPS client.

**Event:** `edge.authenticated`

**What happens:**
1. HTTPS client service automatically attempts authentication after startup (with 2-second delay)
2. Edge sends authentication request to VM endpoint: `POST https://{vm_endpoint}/api/v1/auth/authenticate`
   - Request body: `{"edge_id": "<edge_id>"}`
   - Uses mTLS (mutual TLS) with client certificates
3. VM validates the Edge credentials and responds with success
4. State transitions to `authenticated`
5. `VMAuthenticated` flag is set to `true`

**State properties:**
- `Status`: `authenticated`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `true`

**Authentication Details:**
- Authentication uses the VM Gateway HTTP client service
- The authentication request is sent over the WireGuard tunnel using HTTPS
- Client certificates are used for mTLS authentication
- Edge ID is sent in the authentication request body

### Transition: `authenticated` → `ready`

**Trigger:** All conditions are met for Edge to be fully operational.

**Conditions:**
- `Status` is `authenticated`
- `NetworkConnected` is `true`
- `VMAuthenticated` is `true`

**What happens:**
1. State automatically transitions to `ready` when all conditions are met
2. Edge is now fully operational
3. Services are initialized (cameras, AI, etc.)
4. Edge can now:
   - Sync capabilities to VM
   - Send heartbeats and telemetry
   - Send events
   - Receive commands from VM
   - Process camera streams
   - Run AI inference

**State properties:**
- `Status`: `ready`
- `NetworkConnected`: `true`
- `VMAuthenticated`: `true`

## Reverse Transitions (Error Handling)

### `ready` / `authenticated` / `https_connected` → `wireguard_connected`

**Trigger:** HTTPS connection is lost.

**Event:** `network.https.disconnected`

**What happens:**
- State transitions back to `wireguard_connected`
- `VMAuthenticated` is set to `false`
- Edge will attempt to re-establish HTTPS connection

### Any state → `disconnected`

**Trigger:** WireGuard tunnel is lost.

**Event:** `network.wireguard.disconnected`

**What happens:**
- State transitions to `disconnected`
- `NetworkConnected` is set to `false`
- `VMAuthenticated` is set to `false`
- Edge will attempt to re-establish WireGuard tunnel

## State Diagram

```
┌──────────────┐
│ disconnected │ (Initial State)
└──────┬───────┘
       │ WireGuard tunnel established
       ▼
┌──────────────────────┐
│ wireguard_connected   │
└──────┬────────────────┘
       │ HTTPS services started
       ▼
┌──────────────────┐
│ https_connected   │
└──────┬───────────┘
       │ Authentication successful
       ▼
┌──────────────────┐
│ authenticated    │
└──────┬───────────┘
       │ All conditions met
       ▼
┌──────────────┐
│ ready        │ (Operational)
└──────────────┘

Reverse transitions (on errors):
- HTTPS disconnected → wireguard_connected
- WireGuard disconnected → disconnected
```

## Implementation Details

### VM Gateway Startup Sequence

The VM Gateway (`VMGateway`) coordinates three services in the following order:

1. **WireGuard Client Service** - Establishes tunnel first
2. **HTTPS Server Service** - Starts after WireGuard (for VM → Edge communication)
3. **HTTPS Client Service** - Starts after WireGuard (for Edge → VM communication)

The HTTPS Client Service automatically attempts authentication 2 seconds after startup.

### State Manager

The State Manager (`StateManager`) listens to events from the event bus and updates the Edge state accordingly. It:
- Tracks state transitions
- Executes workflows based on state changes
- Coordinates service initialization
- Handles error recovery

### Events

Key events that trigger state transitions:
- `network.wireguard.connected` - WireGuard tunnel established
- `network.wireguard.disconnected` - WireGuard tunnel lost
- `network.https.connected` - HTTPS connection established
- `network.https.disconnected` - HTTPS connection lost
- `edge.authenticated` - Edge authenticated with VM

## Error Handling

If authentication fails:
- Edge remains in `https_connected` state
- Authentication can be retried (e.g., on next heartbeat)
- Edge will continue attempting to authenticate

If WireGuard tunnel is lost:
- Edge transitions to `disconnected`
- All services dependent on the tunnel are stopped
- Edge will attempt to re-establish the tunnel

## Notes

- The authentication process uses the VM Gateway HTTP client service, which communicates over the WireGuard tunnel
- All communication between Edge and VM is encrypted via WireGuard
- HTTPS communication uses mTLS for additional security
- State transitions are logged for debugging and monitoring

