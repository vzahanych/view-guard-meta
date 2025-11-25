# Phase 3: SaaS Control Plane Backend

**Duration**: 2-3 weeks  
**Goal**: Build core SaaS backend services - authentication, event inventory, basic VM management

**Scope**: Simplified for PoC - manual VM provisioning, basic auth, essential event storage

**Note**: This phase is **deferred for PoC**. The PoC focuses on Edge Appliance ↔ User VM API communication, with User VM API running in Docker Compose. SaaS components will be implemented post-PoC.

---

### Epic 3.1: SaaS Backend Project Setup

**Priority: P0**

#### Step 3.1.1: Project Structure
- **Substep 3.1.1.1**: Create SaaS backend directory structure
  - **Status**: ⬜ TODO
  - Note: SaaS backend is a private repository (git submodule in meta repo)
  - `saas/api/` - REST API service
  - `saas/auth/` - Authentication service
  - `saas/events/` - Event inventory service
  - `saas/provisioning/` - VM provisioning service
  - `saas/billing/` - Billing service
  - `saas/shared/` - Shared libraries
  - Note: gRPC proto definitions for KVM VM ↔ SaaS are in meta repo `proto/proto/kvm/` (imported as Go module)
- **Substep 3.1.1.2**: Go modules and dependencies
  - **Status**: ⬜ TODO
  - Initialize Go modules
  - Import `proto/go` from meta repo as Go module dependency
  - Database drivers (PostgreSQL, Redis)
  - External service clients

#### Step 3.1.2: Database Setup
- **Substep 3.1.2.1**: PostgreSQL schema design
  - **Status**: ⬜ TODO
  - Users table
  - Tenants table
  - KVM VM assignments table
  - Event metadata table
  - Subscriptions table
  - Billing records table
- **Substep 3.1.2.2**: Database migration system
  - **Status**: ⬜ TODO
  - Migration tool setup (golang-migrate)
  - Initial migrations
  - Migration rollback capability
- **Substep 3.1.2.3**: Unit tests for SaaS backend project setup
  - **Status**: ⬜ TODO
  - **P0**: Test PostgreSQL schema initialization
  - **P0**: Test database migration system
  - **P1**: Test Go module dependencies

### Epic 3.2: Authentication & User Management

**Priority: P0**

#### Step 3.2.1: Auth0 Integration
- **Substep 3.2.1.1**: Auth0 application setup
  - **Status**: ⬜ TODO
  - **P0**: Create Auth0 application
  - **P0**: Configure OAuth2/OIDC settings
  - **P0**: Set up callback URLs
  - **P1**: Configure user roles (single "tenant admin" role is P0)
- **Substep 3.2.1.2**: Backend authentication service
  - **Status**: ⬜ TODO
  - **P0**: JWT token validation middleware
  - **P0**: User session management
  - **P0**: Simple tenant mapping (single role: "tenant admin")
  - **P1**: Full RBAC implementation with multiple roles
  - **P0**: Token refresh handling
- **Substep 3.2.1.3**: User service
  - **Status**: ⬜ TODO
  - User CRUD operations
  - User profile management
  - User preferences storage
  - User-tenant association

#### Step 3.2.2: Tenant Management
- **Substep 3.2.2.1**: Tenant service
  - **Status**: ⬜ TODO
  - Tenant creation and management
  - Tenant settings
  - Tenant-KVM VM assignment
  - Tenant subscription association
- **Substep 3.2.2.2**: Multi-tenancy isolation
  - **Status**: ⬜ TODO
  - Tenant context middleware
  - Data isolation enforcement
  - Cross-tenant access prevention
- **Substep 3.2.2.3**: Unit tests for authentication and user management
  - **Status**: ⬜ TODO
  - **P0**: Test JWT token validation middleware
  - **P0**: Test user session management
  - **P0**: Test tenant mapping and isolation
  - **P0**: Test user CRUD operations
  - **P0**: Test tenant service (creation, VM assignment)
  - **P1**: Test RBAC implementation (if implemented)

### Epic 3.3: Event Inventory Service

**Priority: P0**

#### Step 3.3.1: Event Storage & Querying
- **Substep 3.3.1.1**: Event service implementation
  - **Status**: ⬜ TODO
  - **P0**: Store summarized event metadata in PostgreSQL
  - **P0**: Basic event querying API
  - **P0**: Basic filtering (camera, type, date range)
  - **P2**: Advanced indexing and full-text search
- **Substep 3.3.1.2**: Event search functionality
  - **Status**: ⬜ TODO
  - **P1**: Basic search by metadata fields
  - **P2**: Full-text search (PostgreSQL pg_trgm)
- **Substep 3.3.1.3**: Event retention policies
  - **Status**: ⬜ TODO
  - **P1**: Basic retention (simple cleanup)
  - **P2**: Configurable retention periods, archive status tracking

#### Step 3.3.2: Real-time Event Updates
- **Substep 3.3.2.1**: Event updates mechanism
  - **Status**: ⬜ TODO
  - **P0**: Basic polling (`/events` endpoint with periodic refresh)
  - **P1**: Server-Sent Events (SSE) for live updates
  - **P2**: Advanced SSE reconnection handling
- **Substep 3.3.2.2**: Event aggregation
  - **Status**: ⬜ TODO
  - **P1**: Basic event counts
  - **P2**: Advanced statistics and dashboard data
- **Substep 3.3.2.3**: Unit tests for event inventory service
  - **Status**: ⬜ TODO
  - **P0**: Test event storage and retrieval
  - **P0**: Test event querying and filtering
  - **P0**: Test event updates mechanism (polling)
  - **P1**: Test event search functionality
  - **P1**: Test Server-Sent Events (if implemented)
  - **P1**: Test event aggregation

### Epic 3.4: KVM VM Management Service (Basic)

**Priority: P0** (Simplified for PoC)

#### Step 3.4.1: Basic VM Assignment
- **Substep 3.4.1.1**: Manual VM provisioning (PoC)
  - **Status**: ⬜ TODO
  - **P0**: Pre-provision 1-2 KVM VMs manually
  - Simple CLI script or manual setup
  - Store VM connection details in database
  - **P2**: Full Terraform automation (post-PoC)
- **Substep 3.4.1.2**: VM assignment service (basic)
  - **Status**: ⬜ TODO
  - Assign pre-provisioned VM to tenant on signup
  - Store tenant-VM mapping in database
  - Basic VM status tracking
  - **P2**: VM lifecycle management (start/stop/delete, scaling)

#### Step 3.4.2: VM Communication
- **Substep 3.4.2.1**: gRPC server for VM agents
  - **Status**: ⬜ TODO
  - gRPC server setup
  - mTLS configuration
  - Command handling
- **Substep 3.4.2.2**: VM agent management
  - **Status**: ⬜ TODO
  - Agent registration
  - Agent health monitoring
  - Agent command execution
  - Agent configuration updates
- **Substep 3.4.2.3**: Unit tests for KVM VM management service
  - **Status**: ⬜ TODO
  - **P0**: Test VM assignment service
  - **P0**: Test tenant-VM mapping storage
  - **P0**: Test gRPC server for VM agents (mTLS, command handling)
  - **P0**: Test agent registration and health monitoring
  - **P1**: Test VM lifecycle management (if implemented)

### Epic 3.5: ISO Generation Service (Basic)

**Priority: P1** (Can use generic ISO for early PoC)

#### Step 3.5.1: Basic ISO Setup (PoC)
- **Substep 3.5.1.1**: Generic ISO preparation
  - **Status**: ⬜ TODO
  - **P0**: Single generic ISO with hard-coded config or manual bootstrap script
  - Base Ubuntu 24.04 LTS ISO
  - Manual configuration editing for PoC
  - **P2**: Full Packer pipeline with tenant-specific generation
- **Substep 3.5.1.2**: Basic bootstrap (PoC)
  - **Status**: ⬜ TODO
  - **P0**: Manual bootstrap token generation and configuration
  - Simple script-based configuration injection
  - **P2**: Automated tenant-specific ISO generation
- **Substep 3.5.1.3**: ISO download (basic)
  - **Status**: ⬜ TODO
  - **P0**: Simple download endpoint or manual distribution
  - **P2**: Secure download API with CDN integration
- **Substep 3.5.1.4**: Unit tests for ISO generation service
  - **Status**: ⬜ TODO
  - **P1**: Test generic ISO preparation
  - **P1**: Test bootstrap token generation
  - **P1**: Test ISO download endpoint

### Epic 3.6: Billing & Subscription Service (Basic)

**Priority: P2** (Defer to post-PoC, use free plan for PoC)

#### Step 3.6.1: Basic Plan Management (PoC)
- **Substep 3.6.1.1**: Simple plan model
  - **Status**: ⬜ TODO
  - **P0**: Hard-coded "free plan" for PoC
  - Basic plan assignment to tenants
  - **P2**: Full Stripe integration with webhooks
- **Substep 3.6.1.2**: Quota management (basic)
  - **Status**: ⬜ TODO
  - **P0**: Hard-coded quota limits for PoC
  - Basic quota tracking
  - **P2**: Full quota service with plan-based limits
- **Substep 3.6.1.3**: Unit tests for billing and subscription service
  - **Status**: ⬜ TODO
  - **P2**: Test plan management (if implemented)
  - **P2**: Test quota management (if implemented)

### Epic 3.7: REST API Service

**Priority: P0**

#### Step 3.7.1: API Framework
- **Substep 3.7.1.1**: Gin framework setup
  - **Status**: ⬜ TODO
  - Router configuration
  - Middleware setup (auth, logging, CORS)
  - Error handling
- **Substep 3.7.1.2**: API endpoints
  - **Status**: ⬜ TODO
  - User endpoints
  - Event endpoints
  - Camera endpoints
  - Subscription endpoints
  - VM management endpoints
- **Substep 3.7.1.3**: API documentation
  - **Status**: ⬜ TODO
  - OpenAPI/Swagger specification
  - API endpoint documentation
  - Request/response examples

#### Step 3.7.2: API Features (Basic)
- **Substep 3.7.2.1**: Rate limiting
  - **Status**: ⬜ TODO
  - **P1**: Basic rate limiting middleware
  - **P2**: Advanced per-user rate limits
- **Substep 3.7.2.2**: Caching
  - **Status**: ⬜ TODO
  - **P1**: Basic Redis caching for critical data
  - **P2**: Advanced caching strategies
- **Substep 3.7.2.3**: Unit tests for REST API service
  - **Status**: ⬜ TODO
  - **P0**: Test API endpoints (user, event, camera, subscription, VM management)
  - **P0**: Test authentication middleware
  - **P0**: Test error handling
  - **P1**: Test rate limiting middleware
  - **P1**: Test Redis caching (if implemented)

---

## Success Criteria

### Phase 3 Success Criteria (SaaS Backend)

**PoC Must-Have:**
- ✅ Authentication service working (Auth0 integration)
- ✅ Users can sign up and authenticate
- ✅ Event inventory service storing and querying events
- ✅ Basic filtering (date, camera, event type)
- ✅ REST API functional for core endpoints
- ✅ Manual KVM VM assignment working

**Stretch Goals:**
- Automated VM provisioning (Terraform)
- Full Stripe billing integration
- Advanced VM lifecycle management

