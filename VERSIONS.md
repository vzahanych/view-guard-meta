# Component Versions

This file tracks the pinned versions of all submodules and dependencies in the meta repository.

## Public Components

| Component | Version | Repository | Notes |
|-----------|---------|------------|-------|
| view-guard-edge | - | [view-guard-edge](https://github.com/yourorg/view-guard-edge) | Not yet initialized |
| view-guard-crypto | - | [view-guard-crypto](https://github.com/yourorg/view-guard-crypto) | Not yet initialized |
| view-guard-proto | - | [view-guard-proto](https://github.com/yourorg/view-guard-proto) | Not yet initialized |

## Private Components

| Component | Version | Repository | Notes |
|-----------|---------|------------|-------|
| view-guard-kvm-agent | - | view-guard-kvm-agent | Not yet initialized |
| view-guard-saas-backend | - | view-guard-saas-backend | Not yet initialized |
| view-guard-saas-frontend | - | view-guard-saas-frontend | Not yet initialized |
| view-guard-infra | - | view-guard-infra | Not yet initialized |

## Versioning Strategy

All public repositories follow **Semantic Versioning (SemVer)**:
- **Major version** (v1.0.0): Breaking changes
- **Minor version** (v0.1.0): New features, backward compatible
- **Patch version** (v0.0.1): Bug fixes, backward compatible

**Special considerations:**
- `view-guard-proto`: Breaking changes in `.proto` files should bump major version
- `view-guard-crypto`: Breaking changes in encryption API should bump major version
- `view-guard-edge`: Follows standard SemVer

**Best practice**: Pin submodules to **tags**, not arbitrary commits, whenever possible.

## Updating Versions

When updating a submodule version:

1. Navigate to the submodule directory
2. Checkout the desired tag: `git checkout v0.1.0`
3. Return to meta repo root
4. Commit the submodule reference: `git add <submodule-dir> && git commit -m "Pin <submodule> to v0.1.0"`
5. Update this file with the new version

