## Edge ↔ VM Gateway Refactor Plan

### Goals

- **Single edge boundary**: `tunnelgateway.EdgeGateway` is *the* object that encapsulates all VM↔Edge responsibilities (WireGuard, HTTPS server/client, connection tracking, capabilities, health).
- **SaaS-only API server**: `services.SaaSAPIServer` exposes HTTP endpoints for SaaS components only and never talks to low-level edge plumbing directly.
- **Orchestrator simplicity**: `Server` owns just:
  - `saasAdminHTTPServer *services.SaaSAPIServer`
  - `edgeGateway tunnelgateway.EdgeGateway`
  - core infra (`stateStore`, DB, MinIO, `stateCoordinator`, etc.)

### Current State (simplified)

- `Server.Start`:
  - Creates `WireGuardServer`, `EdgeAPIServer`, `VMHTTPServer`, `EdgeHTTPSClient`, `ConnectionMonitor`.
  - Creates dataset receiver, model catalog/storage/deployment.
  - Creates `services.APIServer` (SaaS HTTP API) and passes:
    - `capStore` (from `EdgeAPIServer`),
    - `edgeGateway` (currently `*VMHTTPServer`),
    - `edgeHTTPSClient`.
- `services.APIServer`:
  - Exposes `/api/*` endpoints for SaaS.
  - Contains **both SaaS logic and Edge-facing logic** (training proxy, edges list/health, deployments to edge, etc.).

### Target Architecture

1. **EdgeGateway**
   - Lives in `internal/tunnel-gateway`.
   - Interface (conceptually):
     - Lifecycle: `Start(ctx)`, `Stop(ctx)`, `Name()`.
     - Connectivity: `GetConnectedEdges()`, `GetConnection(edgeID)`, `GetWireGuardServer()`.
     - State/DB: `GetDB()`, `GetCapabilityStore()`.
     - Edge ops: `RequestSnapshotCapture(...)`, `SyncCapabilities(...)`, `GetEdgeHealth(...)`, etc.
   - Implementation:
     - Wrap existing `WireGuardServer`, `EdgeAPIServer` (or successor), `VMHTTPServer`, `EdgeHTTPSClient`.
     - Own `ConnectionMonitor` and any other edge-only services.

2. **SaaSAPIServer**
   - Lives in `internal/orchestrator/services/api.go`.
   - Responsibilities:
     - HTTP surface for SaaS:
       - Camera, dataset, model, deployment, training endpoints.
     - Translate SaaS requests into **high-level calls** on:
       - `EdgeGateway` (for anything that touches Edge).
       - Model/deployment services, dataset receiver, etc.
   - Must **not** use:
     - `WireGuardServer`, `VMHTTPServer`, `EdgeAPIServer`, `EdgeHTTPSClient` directly.
     - Any low-level tunnel or connection structs.

3. **Orchestrator Server**

```go
type Server struct {
    ...
    saasAdminHTTPServer *services.SaaSAPIServer
    edgeGateway         tunnelgateway.EdgeGateway
}
```

- `Init`:
  - Creates infra (DB, MinIO, Python AI, state store).
- `Start`:
  - Builds `edgeGateway` via a factory in `tunnel-gateway` (e.g. `NewEdgeGateway(cfg, logger, db)`).
  - Builds `SaaSAPIServer` with:
    - `capStore` (from `edgeGateway.GetCapabilityStore()`),
    - `edgeGateway`,
    - model/deployment services, dataset receiver, etc.
  - Registers and starts both as services via `service.Manager`.

### Step-by-Step Refactor

#### Phase 1 – Solidify EdgeGateway contract

1. Extend `tunnelgateway.EdgeGateway` so it exposes the **minimal high‑level edge API** that SaaS code needs, without leaking low‑level details:
   - Connectivity:
     - `GetConnectedEdges()`, `GetConnection(edgeID)`, `GetWireGuardServer()`.
   - State/DB:
     - `GetDB()`, `GetCapabilityStore()`.
   - Edge operations:
     - Methods that wrap existing `EdgeHTTPSClient` calls (e.g. snapshot capture, config fetch/push) and any gRPC helpers that are still required.
2. Add a concrete implementation (e.g. `type HTTPSWireGuardGateway struct { ... }`) that:
   - Is built from `WireGuardServer`, `EdgeAPIServer` (or its successor), `VMHTTPServer`, and `EdgeHTTPSClient`.
   - Owns any edge‑only helpers internally (e.g. connection bookkeeping), so callers never touch `ConnectionMonitor` or other legacy structs directly.

#### Phase 2 – Make SaaSAPIServer edge-agnostic

3. In `services.APIServer`:
   - Replace all direct uses of:
     - `tunnelgateway.EdgeAPIServer`,
     - `tunnelgateway.EdgeHTTPSClient`,
     - connection structs,
   - with calls to `edgeGateway` methods (new interface methods from Phase 1).
4. Ensure that handler responsibilities read as:
   - Parse/validate SaaS HTTP request.
   - Call domain services + `edgeGateway`.
   - Map results/errors to HTTP responses.

#### Phase 3 – Simplify Server wiring

5. Move all Edge-wire‑up code out of `Server.Start` into a `tunnel-gateway` factory:

```go
// in tunnel-gateway
func NewEdgeGateway(cfg *config.Config, logger *logging.Logger, db *database.DB, bus *service.EventBus) (EdgeGateway, error) {
    // construct WireGuardServer, EdgeAPIServer, VMHTTPServer, EdgeHTTPSClient, ConnectionMonitor...
}
```

6. In `Server.Start`:
   - Replace the explicit `WireGuardServer`/`EdgeAPIServer`/`VMHTTPServer`/`EdgeHTTPSClient` creation with:

```go
edgeGateway, err := tunnelgateway.NewEdgeGateway(s.config, s.logger, db, eventBus)
if err != nil { return err }
s.edgeGateway = edgeGateway
s.manager.Register(edgeGateway)
```

7. Use `edgeGateway.GetCapabilityStore()` and other getters to build `SaaSAPIServer` and domain services, instead of touching tunnel structs directly.

#### Phase 4 – Cleanup & Legacy Removal

8. Once all consumers use `EdgeGateway`, mark `EdgeAPIServer` gRPC APIs as legacy and:
   - Either delete them if no longer used, or
   - Keep them internal to the gateway implementation only.
9. Remove any remaining references to:
   - `ConnectionMonitor` outside of `tunnel-gateway`.
   - Raw `WireGuardServer`/`VMHTTPServer`/`EdgeHTTPSClient` from orchestrator and SaaS code.


