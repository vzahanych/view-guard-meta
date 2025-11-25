# Phase 5: ISO Building & Deployment Automation

**Duration**: 1-2 weeks  
**Goal**: Basic ISO generation and simple deployment automation

**Scope**: Simplified for PoC - generic ISO or simple build script, manual deployment acceptable

**Note**: This phase is **deferred for PoC**. The PoC focuses on Edge Appliance ↔ User VM API communication, with Docker Compose for local development. ISO building and deployment automation will be implemented post-PoC.

---

### Epic 5.1: ISO Build Pipeline (Basic)

**Priority: P1** (Can use generic ISO for early PoC)

**Note**: As of November 2025, Ubuntu 24.04 LTS is the current LTS (supported until 2029). Ubuntu 22.04 LTS remains supported until 2027 and is also acceptable for PoC.

#### Step 5.1.1: Basic ISO Setup
- **Substep 5.1.1.1**: Generic ISO preparation
  - **Status**: ⬜ TODO
  - **P0**: Single generic Ubuntu 24.04 LTS Server ISO
  - Manual configuration or simple bootstrap script
  - Basic auto-install configuration
  - **P2**: Full Packer automation with tenant-specific generation
- **Substep 5.1.1.2**: Software pre-installation (basic)
  - **Status**: ⬜ TODO
  - **P0**: Manual installation of Edge Appliance software
  - Or simple installation script
  - **P2**: Automated packaging and pre-installation

#### Step 5.1.2: Basic Configuration
- **Substep 5.1.2.1**: Bootstrap configuration (simple)
  - **Status**: ⬜ TODO
  - **P0**: Manual bootstrap token generation
  - Manual KVM VM connection details configuration
  - Simple first-boot script
  - **P2**: Automated tenant-specific configuration injection
- **Substep 5.1.2.2**: Build automation (basic)
  - **Status**: ⬜ TODO
  - **P0**: Simple build script on developer machine
  - **P2**: Full CI/CD pipeline with Packer
- **Substep 5.1.2.3**: Unit tests for ISO build pipeline
  - **Status**: ⬜ TODO
  - **P1**: Test ISO preparation scripts
  - **P1**: Test bootstrap configuration generation
  - **P1**: Test build automation scripts
  - **P2**: Test Packer automation (if implemented)

### Epic 5.2: Deployment Automation (Basic)

**Priority: P1** (Manual deployment acceptable for PoC)

#### Step 5.2.1: Basic Deployment
- **Substep 5.2.1.1**: Manual deployment (PoC)
  - **Status**: ⬜ TODO
  - **P0**: Manual KVM VM setup (1-2 VMs)
  - Manual agent installation and configuration
  - Manual SaaS deployment (Docker Compose or simple K8s)
  - **P2**: Full Terraform automation
- **Substep 5.2.1.2**: Basic automation scripts
  - **Status**: ⬜ TODO
  - **P1**: Simple deployment scripts
  - Basic configuration management
  - **P2**: Full Infrastructure as Code

#### Step 5.2.2: SaaS Deployment (Basic)
- **Substep 5.2.2.1**: Simple deployment
  - **Status**: ⬜ TODO
  - **P0**: Docker Compose for local PoC or simple K8s deployment
  - Basic service configuration
  - **P2**: Full EKS setup with advanced configuration
- **Substep 5.2.2.2**: Database setup
  - **Status**: ⬜ TODO
  - **P0**: Manual PostgreSQL setup or managed database
  - Basic migration execution
  - **P2**: Automated database deployment and backups
- **Substep 5.2.2.3**: Unit tests for deployment automation
  - **Status**: ⬜ TODO
  - **P1**: Test deployment scripts
  - **P1**: Test configuration management
  - **P2**: Test Infrastructure as Code (if implemented)

### Epic 5.3: Update & Maintenance Automation

**Priority: P2** (Defer to post-PoC)

#### Step 5.3.1: Update Mechanisms
- **Substep 5.3.1.1**: Manual updates for PoC
  - **Status**: ⬜ TODO
  - **P0**: Manual update process for PoC
  - **P2**: Automated update delivery, signed packages, rollback mechanisms

---

## Success Criteria

### Phase 5 Success Criteria (ISO & Deployment)

**PoC Must-Have:**
- ✅ Generic ISO or simple build script working
- ✅ ISO can be installed on Mini PC
- ✅ Edge Appliance boots and connects to KVM VM
- ✅ Basic deployment scripts or manual deployment working

**Stretch Goals:**
- Tenant-specific ISO generation
- Full Packer automation
- Automated deployment pipeline

