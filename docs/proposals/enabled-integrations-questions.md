# Integration Visibility — Design Questions

**Status:** Resolved — all decisions made
**Related:** [Design document](enabled-integrations-design.md)

Each question has options with trade-offs and a recommendation. Go through them one by one to form the design, then update the design document.

---

## Q1: Allowlist vs. denylist approach?

When a cluster admin wants to hide a broken integration, the system needs a mechanism. This affects the operational model and how much config admins need to maintain.

### Option C: Denylist approach (`disabledIntegrations`)

List what is disabled. Everything not in the list is enabled.

- **Pro:** Backward compatible — existing deployments show everything.
- **Pro:** Operationally simpler — the common case (everything healthy) is an empty field; admins only add the 1-2 broken integrations.
- **Pro:** Adding a new integration requires no config change — it's enabled by default.
- **Con:** The UI must check each integration against the disabled list (trivial).

**Decision:** Option C — denylist approach. With an allowlist, admins must enumerate every integration; with a denylist the normal state is empty and only broken integrations are listed.

_Considered and rejected: Option A — allowlist when populated (requires listing all integrations, higher operational overhead), Option B — explicit allowlist with nothing enabled by default (breaking change for existing deployments)._

---

## Q2: Should integrations be simple string identifiers or richer structs?

This determines the shape of the array items in the CRD.

### Option A: Simple `[]string`

```yaml
disabledIntegrations:
  - "openshift"
```

- **Pro:** Minimal API surface, easy to understand.
- **Pro:** Follows the pattern used by comma-separated string lists elsewhere in the config (e.g. `domains`, `forbiddenUsernamePrefixes`), but as a proper array.
- **Con:** No room for per-integration metadata (e.g. a display name or URL) without a future API change.
- **Con:** Free-form strings are prone to typos and mismatches between backend and UI — a misspelled identifier is silently ignored.

### Option B: Typed enum (`IntegrationName`) with kubebuilder validation

Define a named string type with a `+kubebuilder:validation:Enum` marker and Go constants for each known integration. The CRD field becomes `[]IntegrationName` instead of `[]string`.

```go
// +kubebuilder:validation:Enum=openshift;openshift-ai;devspaces;ansible-automation-platform;openshift-virtualization
type IntegrationName string

const (
    IntegrationOpenShift                 IntegrationName = "openshift"
    IntegrationOpenShiftAI               IntegrationName = "openshift-ai"
    IntegrationDevSpaces                 IntegrationName = "devspaces"
    IntegrationAnsibleAutomationPlatform IntegrationName = "ansible-automation-platform"
    IntegrationOpenShiftVirtualization   IntegrationName = "openshift-virtualization"
)
```

```yaml
disabledIntegrations:
  - "openshift"
```

- **Pro:** Kubernetes rejects invalid values at admission time — immediate feedback on typos.
- **Pro:** `kubectl explain` shows the valid values — self-documenting.
- **Pro:** Go constants provide a canonical reference for backend and UI teams.
- **Con:** Adding a new integration requires an API module change, CRD regeneration, and rollout.

**Decision:** Option B — typed enum with kubebuilder validation. The safety of admission-time validation outweighs the cost of requiring an API change for new integrations. That cost is actually desirable — new integrations should be a deliberate, reviewable change.

_Considered and rejected: Option A — simple `[]string` (too error-prone, no validation), Option C — array of structs (over-engineered for current needs), Option D — comma-separated string (less idiomatic for CRDs)._

---

## Q3: Where should the field live in the config hierarchy?

The `ToolchainConfig` has a deep nested structure. The field placement affects who "owns" this configuration conceptually.

### Option A: Under `spec.host.registrationService`

```yaml
spec:
  host:
    registrationService:
      disabledIntegrations:
        - "openshift"
```

- **Pro:** The registration service is the component that exposes this data; co-locating it with other registration service config is natural.
- **Pro:** Follows the pattern of `uiCanaryDeploymentWeight` and `workatoWebHookURL` which are UI-facing config under `registrationService`.
- **Con:** Conceptually, "which integrations are disabled" could be considered a broader host-level concern, not specific to the registration service.

**Decision:** Option A — co-locate with other UI-facing config under `registrationService`, following existing precedent.

_Considered and rejected: Option B — under `spec.host` directly (only registration service needs it, would bloat HostConfig)._

---

## Q4: Should this be a new endpoint or added to the existing `/api/v1/uiconfig`?

The registration service already has `GET /api/v1/uiconfig` that returns UI-facing configuration (canary weight, Workato URL).

### Option A: Extend `/api/v1/uiconfig`

Add the integration visibility field to the existing `UIConfigResponse`.

- **Pro:** No new endpoint to maintain; the UI already calls `/api/v1/uiconfig`.
- **Pro:** Keeps UI configuration consolidated in one place.
- **Con:** Makes the uiconfig response grow over time; it's currently JWT-secured, which means unauthenticated callers cannot access it.

**Decision:** Option A — add to existing `UIConfigResponse`. Avoids an extra HTTP round-trip and keeps UI configuration consolidated.

_Considered and rejected: Option B — new dedicated endpoint (extra maintenance burden, extra HTTP call from the UI)._

---

## Q5: Should the endpoint be secured (JWT) or unsecured?

Currently `/api/v1/uiconfig` requires JWT authentication. The list of disabled integrations is not sensitive, but access control is a consideration.

### Option A: Keep it secured (current behavior for `/api/v1/uiconfig`)

- **Pro:** No security posture change.
- **Pro:** Only authenticated users see what integrations are available.
- **Con:** The UI must have a valid token before it can determine what to render, which may delay the initial page load or require the UI to handle a two-phase render.

**Decision:** Option A — keep the endpoint JWT-secured. No security posture change needed; the UI already has a token when it renders integration entry points.

_Considered and rejected: Option B — unsecured (would also expose workatoWebHookURL and canary weight), Option C — split into new unsecured endpoint (contradicts Q4 decision)._

---

## Q6: What should the CRD field be named?

Naming affects clarity, consistency with the codebase, and how the field appears in the CRD YAML.

### Option A: `disabledIntegrations`

```yaml
disabledIntegrations:
  - "openshift"
```
JSON tag: `disabledIntegrations`

- **Pro:** Directly communicates the denylist semantics — these are the integrations that are turned off.
- **Pro:** Consistent with the naming pattern of the approach (listing what's disabled).
- **Con:** Slightly long.

**Decision:** Option A (`disabledIntegrations`) — unambiguous, directly reflects the denylist semantics.

_Considered and rejected: Option B — `integrations` (ambiguous), Option C — `hiddenIntegrations` (UI term out of place in a CRD)._

---

## Q7: What should the REST response JSON property be named?

The CRD field is `disabledIntegrations` (a denylist), but the REST response could either mirror the CRD directly or translate the semantics for the UI's convenience. This affects where the "is this integration disabled?" logic lives.

### Option A: Mirror the CRD — expose `disabledIntegrations` in the JSON response

```json
{
  "uiCanaryDeploymentWeight": 20,
  "workatoWebHookURL": "https://...",
  "disabledIntegrations": ["openshift"]
}
```

- **Pro:** 1:1 mapping with the CRD — simple, transparent, no translation layer.
- **Pro:** The UI can trivially check `disabledIntegrations.includes("x")` before rendering each integration.
- **Pro:** Empty array `[]` unambiguously means "nothing is disabled" (everything enabled).
- **Con:** The UI works with a negative list, which some may find less intuitive than a positive one.

**Decision:** Option A — mirror the CRD directly. Simple, no master list needed, trivial UI check.

_Considered and rejected: Option B — translate to `enabledIntegrations` (requires backend to maintain a hardcoded master list of all integrations, adds a translation layer that can drift)._
