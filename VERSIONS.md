# Component Versions

This file tracks the versions of all components in the meta repository.

## Public Components

Public components are developed directly in the meta repository:

| Component | Version | Location | Notes |
|-----------|---------|----------|-------|
| edge | - | `edge/` | Not yet initialized |
| crypto | - | `crypto/` | Not yet initialized |
| proto | - | `proto/` | Not yet initialized |

## Private Components

Private components are referenced as git submodules:

| Component | Version | Repository | Notes |
|-----------|---------|------------|-------|
| view-guard-kvm-agent | - | view-guard-kvm-agent | Not yet initialized |
| view-guard-saas-backend | - | view-guard-saas-backend | Not yet initialized |
| view-guard-saas-frontend | - | view-guard-saas-frontend | Not yet initialized |
| view-guard-infra | - | view-guard-infra | Not yet initialized |

## Versioning Strategy

All public components follow **Semantic Versioning (SemVer)**:
- **Major version** (v1.0.0): Breaking changes
- **Minor version** (v0.1.0): New features, backward compatible
- **Patch version** (v0.0.1): Bug fixes, backward compatible

**Special considerations:**
- `proto/`: Breaking changes in `.proto` files should bump major version
- `crypto/`: Breaking changes in encryption API should bump major version
- `edge/`: Follows standard SemVer

**Best practice**: Tag the meta repository when releasing public components.

## Updating Versions

When releasing a new version of public components:

1. Make changes in the component directories (`edge/`, `crypto/`, `proto/`)
2. Commit changes to the meta repository
3. Tag the meta repository: `git tag v0.1.0 && git push origin v0.1.0`
4. Update this file with the new version

When updating a private submodule version:

1. Navigate to the submodule directory
2. Checkout the desired tag: `git checkout v0.1.0`
3. Return to meta repo root
4. Commit the submodule reference: `git add <submodule-dir> && git commit -m "Pin <submodule> to v0.1.0"`
5. Update this file with the new version
