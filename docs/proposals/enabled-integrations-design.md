# Integration Visibility Configuration

**Status:** Final

## Overview

Developer Sandbox integrates with several downstream services (e.g. OpenShift Lightspeed, DevSpaces, Workato, etc.). When one of these downstream integrations experiences issues, there is currently no way to hide it from customers without a code change and redeployment.

This design adds a `disabledIntegrations` field to `ToolchainConfig` so that operators can dynamically hide broken integrations from users. The registration service exposes this list via the existing `/api/v1/uiconfig` endpoint, allowing the UI to conditionally hide integration entry points at runtime.

The field uses a **denylist** approach: the normal state is an empty field (all integrations enabled), and admins only add the 1-2 broken integrations when something goes wrong. This avoids the operational burden of maintaining a full allowlist of every integration.

## Design Principles

- **Minimal API surface change**: add a single new field to the existing `ToolchainConfig` CRD rather than introducing a new CRD.
- **Safe defaults**: when the field is not set, all integrations are enabled (backward compatible with existing deployments).
- **Operational simplicity**: the common case (everything healthy) requires zero configuration; only broken integrations are listed.
- **Consistency with existing patterns**: follows the same conventions used by `UICanaryDeploymentWeight` and `WorkatoWebHookURL` for CRD placement and REST exposure.
- **Decoupled from feature toggles**: feature toggles control per-Space probabilistic template rendering; this mechanism controls UI-level visibility of integrations for all users.

## Architecture / How It Works

```
┌──────────────┐       ┌──────────────────┐       ┌─────────────────────┐
│  Cluster     │       │  Registration    │       │  Sandbox UI         │
│  Admin       │──────▶│  Service         │──────▶│  (Browser)          │
│              │       │                  │       │                     │
│ applies      │       │ GET /api/v1/     │       │  fetches uiconfig,  │
│ ToolchainCfg │       │   uiconfig       │       │  reads disabled-    │
└──────────────┘       │ (JWT-secured)    │       │  Integrations,      │
       │               └──────────────────┘       │  hides those        │
       │                        ▲                 └─────────────────────┘
       ▼                        │
┌──────────────────────────┐    │
│ToolchainConfig CR        │    │
│  spec.host               │    │
│    .registrationService  │    │
│    .disabledIntegrations:│    │
│      - "openshift"       │────┘
└──────────────────────────┘
     (loaded via commonconfig.LoadLatest)
```

1. **Cluster admin** updates the `ToolchainConfig` CR, adding broken integrations to `disabledIntegrations`.
2. **Registration service** picks up the change via its cached client (same mechanism used for all other config — `commonconfig.LoadLatest`).
3. **`GET /api/v1/uiconfig`** (JWT-secured) serves the disabled list alongside existing fields (`uiCanaryDeploymentWeight`, `workatoWebHookURL`).
4. **UI** fetches the endpoint and hides any integration present in the `disabledIntegrations` array.

## Core Concepts

### Integration identifier

Each integration is represented by a value of the `IntegrationName` type, a named string with kubebuilder enum validation. The valid identifiers are defined as Go constants in the API module:

| Constant | Value |
|----------|-------|
| `IntegrationOpenShift` | `"openshift"` |
| `IntegrationOpenShiftAI` | `"openshift-ai"` |
| `IntegrationDevSpaces` | `"devspaces"` |
| `IntegrationAnsibleAutomationPlatform` | `"ansible-automation-platform"` |
| `IntegrationOpenShiftVirtualization` | `"openshift-virtualization"` |

Kubernetes rejects any value not in this enum at admission time, preventing typos and misconfiguration. Adding a new integration requires an API module change and CRD regeneration.

### Default behavior

When `disabledIntegrations` is nil or empty, **all integrations are enabled**. This preserves backward compatibility — existing deployments without the field continue to work with everything visible. Admins only populate the field when they need to hide something.

### Field location

The `disabledIntegrations` field lives under `spec.host.registrationService` in the `ToolchainConfig` CRD, co-located with other UI-facing configuration (`uiCanaryDeploymentWeight`, `workatoWebHookURL`).

### CRD field definition

```go
// In api/v1alpha1/toolchainconfig_types.go:

// +kubebuilder:validation:Enum=openshift;openshift-ai;devspaces;ansible-automation-platform;openshift-virtualization
type IntegrationName string

const (
    IntegrationOpenShift                 IntegrationName = "openshift"
    IntegrationOpenShiftAI               IntegrationName = "openshift-ai"
    IntegrationDevSpaces                 IntegrationName = "devspaces"
    IntegrationAnsibleAutomationPlatform IntegrationName = "ansible-automation-platform"
    IntegrationOpenShiftVirtualization   IntegrationName = "openshift-virtualization"
)

// In RegistrationServiceConfig:

// DisabledIntegrations specifies the list of integrations that should be
// hidden/disabled in the UI. When nil or empty, all integrations are
// considered enabled. Only listed integrations are hidden.
// +optional
// +listType=set
DisabledIntegrations []IntegrationName `json:"disabledIntegrations,omitempty"`
```

### REST response

The existing `UIConfigResponse` in the registration service is extended with a `disabledIntegrations` field that mirrors the CRD 1:1:

```go
type UIConfigResponse struct {
    UICanaryDeploymentWeight int                                `json:"uiCanaryDeploymentWeight"`
    WorkatoWebHookURL        string                             `json:"workatoWebHookURL"`
    DisabledIntegrations     []toolchainv1alpha1.IntegrationName `json:"disabledIntegrations"`
}
```

Example JSON response from `GET /api/v1/uiconfig` when OpenShift is down:

```json
{
  "uiCanaryDeploymentWeight": 20,
  "workatoWebHookURL": "https://...",
  "disabledIntegrations": ["openshift"]
}
```

Example when everything is healthy (field not set in CR):

```json
{
  "uiCanaryDeploymentWeight": 20,
  "workatoWebHookURL": "https://...",
  "disabledIntegrations": []
}
```

The handler normalizes nil to an empty slice before serialization, so the response always contains `"disabledIntegrations": []` (never absent or `null`). The UI checks `disabledIntegrations.includes("x")` before rendering each integration entry point.

## Implementation Plan

Changes span **three repositories** in the codeready-toolchain ecosystem:

### Phase 1: API types (`codeready-toolchain/api`)

1. Define the `IntegrationName` type with a `+kubebuilder:validation:Enum` marker and the associated constants in `api/v1alpha1/toolchainconfig_types.go`.
2. Add `DisabledIntegrations []IntegrationName` to `RegistrationServiceConfig` with `+listType=set` and `+optional` markers.
3. Run code generation (`make generate`) to update DeepCopy methods and CRD manifests.

### Phase 2: Host operator (`codeready-toolchain/host-operator`)

1. Bump the `codeready-toolchain/api` dependency in `go.mod` to pick up the new field, then run `make generate` to regenerate the CRD YAML in `config/crd/bases/`.

### Phase 3: Registration service (`codeready-toolchain/registration-service`)

1. Add a `DisabledIntegrations() []IntegrationName` accessor method on `RegistrationServiceConfig` in `pkg/configuration/configuration.go`. Note: unlike pointer-based fields (e.g. `UICanaryDeploymentWeight`, `WorkatoWebHookURL`) that use `commonconfig.Get*` helpers, this accessor returns the `[]IntegrationName` field directly — nil and empty both mean "nothing disabled," so no default-value wrapper is needed.
2. Extend `UIConfigResponse` in `pkg/controller/uiconfig.go` to include the `DisabledIntegrations` field (typed as `[]toolchainv1alpha1.IntegrationName`).
3. Update the `GetHandler` to populate the new field from configuration, normalizing nil to an empty slice.
4. Add unit tests for the updated handler.

### Phase 4: UI (out of scope for this design)

The UI team will consume the `disabledIntegrations` field from the `/api/v1/uiconfig` response and hide any integration present in the array.

## Decisions Summary

| # | Question | Decision |
|---|----------|----------|
| Q1 | Allowlist vs. denylist | Denylist (`disabledIntegrations`) — only list broken integrations |
| Q2 | Field type | Typed enum `[]IntegrationName` with kubebuilder validation |
| Q3 | Field placement | Under `spec.host.registrationService` |
| Q4 | Endpoint | Extend existing `GET /api/v1/uiconfig` |
| Q5 | Authentication | Keep JWT-secured (no change) |
| Q6 | CRD field name | `disabledIntegrations` |
| Q7 | REST response property | Mirror CRD — `disabledIntegrations` |

Full decision rationale: [enabled-integrations-questions.md](enabled-integrations-questions.md)
