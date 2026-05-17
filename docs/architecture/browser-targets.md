# Browser Targets

PinchTab currently treats the browser backend as a single global provider:
`browser.provider=chrome|cloak`. That works for first-class CloakBrowser support,
but it makes browser type mutually exclusive. The next architecture should let
one PinchTab server know about multiple browser targets and select one per
request, per instance, or by fallback policy.

This spec uses "browser target" to mean a named runnable browser backend such as
`chrome-local`, `cloak-us`, or `lightpanda-fast`. A target has a provider type,
launch or attach settings, profile storage, capabilities, and health state.

## Goals

- Configure multiple browser targets in one PinchTab config.
- Keep a default browser target for existing clients.
- Let requests select a target explicitly when they need one.
- Let PinchTab fall back to another configured target when startup or
  acquisition fails.
- Support multiple targets of the same provider type.
- Support future provider types with different capability surfaces, including
  lightweight browsers such as Lightpanda-style runtimes.
- Preserve the current Chrome behavior when no multi-target config is present.

## Non-Goals

- Do not make PinchTab download third-party browser binaries at runtime.
- Do not silently move an existing tab from one browser target to another.
- Do not fallback after a request has already performed page side effects unless
  the request explicitly opts into retry semantics.
- Do not require every browser provider to support every endpoint.

## Configuration

Introduce named targets under `browser.targets`. Keep current single-provider
fields as a compatibility shorthand that migrates to a target named `default`.

Example:

```json
{
  "browser": {
    "defaultTarget": "chrome-local",
    "fallbackOrder": ["cloak-primary"],
    "targets": {
      "chrome-local": {
        "provider": "chrome",
        "binary": "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
      },
      "cloak-primary": {
        "provider": "cloak",
        "binary": "/opt/cloakbrowser/chrome",
        "cloak": {
          "fingerprintSeed": "42069",
          "platform": "linux",
          "locale": "en-US",
          "timezone": "UTC",
          "disableDefaultStealthArgs": true
        }
      }
    }
  }
}
```

Rules:

- `browser.defaultTarget` is required once `browser.targets` is present.
- Target names are stable API identifiers. They must be lowercase, URL-safe,
  and not derived from binary paths.
- `browser.fallbackOrder` is optional. If absent, fallback is disabled unless a
  request supplies an explicit fallback list. It must not repeat targets or
  include `browser.defaultTarget`.
- Current `browser.provider`, `browser.binary`, `browser.extraFlags`, and
  `browser.cloak` remain valid and are interpreted as `browser.targets.default`
  during config load.
- Provider-specific blocks live inside each target, not globally.
- Profile directories are target-scoped:
  `profiles.baseDir/<target-name>/<profile-name>`.

## Request Selection

Add a canonical request option named `browserTarget`.

Examples:

```http
POST /navigate
{
  "url": "https://example.com",
  "browserTarget": "cloak-primary"
}
```

```http
POST /instances/start
{
  "browserTarget": "lightpanda-fast",
  "mode": "headless"
}
```

Selection order:

1. If a request references an existing `tabId` or `instanceId`, the existing
   instance's browser target wins. A conflicting `browserTarget` is a `409`.
2. If `browserTarget` is present, resolve that named target.
3. If a request supplies a provider type such as `browserProvider=cloak`, resolve
   the configured default target for that provider only when unambiguous.
4. Otherwise use `browser.defaultTarget`.
5. If acquisition fails and fallback is enabled, walk the request fallback list
   first, then `browser.fallbackOrder`.

Surfaces:

- HTTP JSON bodies use `browserTarget`.
- HTTP query params may use `browserTarget` for GET-style endpoints.
- CLI can expose `--browser <target>` as the user-facing alias.
- MCP tools should expose `browserTarget` for explicitness.
- `/instances/start` should accept `browserTarget` because instances are the
  natural browser ownership boundary.

## Fallback

Fallback is only safe before a browser instance has performed request side
effects.

Fallback is allowed for:

- binary missing or not executable
- startup timeout
- CDP connect failure
- browser process exits during startup
- target unhealthy or in cooldown
- target lacks a required capability and the request allows capability fallback

Fallback is not allowed for:

- security policy denials
- IDPI or allowed-domain blocks
- auth/session failures
- existing `tabId` or `instanceId` requests
- action/navigation after the target page has already changed state, unless a
  future endpoint explicitly declares idempotent retry behavior

Request-level controls:

```json
{
  "url": "https://example.com",
  "browserTarget": "cloak-primary",
  "fallbackTargets": ["chrome-local"],
  "allowBrowserFallback": true
}
```

Response metadata should expose the actual target:

```json
{
  "tabId": "...",
  "url": "https://example.com",
  "browserTarget": "chrome-local",
  "browserProvider": "chrome",
  "fallbackFrom": "cloak-primary",
  "fallbackReason": "startup_timeout"
}
```

If fallback is disabled or exhausted, return a structured error:

```json
{
  "code": "browser_target_unavailable",
  "error": "browser target cloak-primary failed to start",
  "details": {
    "browserTarget": "cloak-primary",
    "browserProvider": "cloak",
    "reason": "startup_timeout",
    "fallbackTried": ["chrome-local"]
  }
}
```

## Capabilities

Each provider adapter declares a capability set. PinchTab uses capabilities to
route requests, decide fallback, and return clear unsupported-operation errors.

Initial capability names:

- `cdp`
- `headless`
- `headed`
- `persistentProfiles`
- `extensions`
- `domSnapshot`
- `actions`
- `evaluate`
- `cookies`
- `screenshots`
- `pdf`
- `upload`
- `download`
- `networkIntercept`
- `emulation`
- `nativeStealth`
- `remoteAttach`

Capability resolution:

- Provider adapters expose default capabilities.
- Runtime probes can downgrade capabilities when a binary is missing a feature.
- Endpoint handlers declare required capabilities.
- If the selected target lacks a required capability, return `409
  browser_capability_unsupported` unless fallback is enabled and another target
  can satisfy the request.

This is what makes Lightpanda-style providers viable. A lightweight provider can
be configured beside Chrome and CloakBrowser, but only endpoints backed by its
declared capabilities are routed to it. Missing features should be explicit,
not discovered as late CDP errors.

## Provider Adapter Contract

Introduce a provider registry instead of hard-coding `chrome|cloak` in launch
paths.

Adapter responsibilities:

- validate target config
- resolve binary or remote endpoint
- report capabilities
- build launch args or remote attach config
- probe runtime health
- redact provider-specific secrets
- map provider-specific launch errors to normalized failure reasons

Sketch:

```go
type BrowserProvider interface {
    Type() string
    ValidateTarget(target BrowserTargetConfig) []error
    Capabilities(target BrowserTargetConfig) CapabilitySet
    BuildLaunchPlan(target BrowserTargetConfig, runtime RuntimeOptions) (LaunchPlan, error)
    Probe(ctx context.Context, target BrowserTargetConfig) (ProbeResult, error)
}
```

The first implementation can keep Chrome and CloakBrowser on the existing
chromedp allocator path. Future non-Chromium providers can either implement a
CDP-compatible adapter or force a larger engine abstraction when CDP is not
enough.

## Instance Model

Instances become target-bound:

- Every instance records `browserTarget` and `browserProvider`.
- `/instances` includes those fields.
- `/health` includes default target health and degraded target summaries.
- `/stealth/status` continues to report provider information for the active
  instance, and should add `browserTarget`.
- Tab lifecycle, leases, locks, handoff, and active-tab routing remain
  instance-scoped.

Auto-start behavior:

- `simple` strategy starts or reuses an instance for the selected target.
- `always-on` starts one default instance per configured always-on target.
- Allocation policy must never hand a request to an instance from a different
  target than the resolver selected.

## Security

Browser target selection is not a security bypass.

- Existing auth, sessions, IDPI, allowed domains, and endpoint capability gates
  still run after target selection.
- Fallback must not bypass a security block. A blocked navigation should stay
  blocked on every target.
- Agent/session policy can later restrict allowed targets per token or agent.
- Browser target names should be logged in activity and audit trails.
- Provider-specific secrets, especially proxy credentials, must be redacted.
- Hosted third-party browser targets require separate licensing and trust review
  before being documented as supported.

## Migration

Phase 1 keeps behavior identical:

- Existing config without `browser.targets` produces one target named `default`.
- Existing `browser.provider=chrome` remains the default.
- Existing tests for Chrome launch args stay unchanged.

Phase 2 adds named target config and request selection:

- Add config structs, validation, JSON schema, editor get/set, and dashboard UI.
- Add `browserTarget` request parsing to `/navigate`, `/instances/start`, and
  routes that can auto-acquire an instance.
- Add target fields to instance metadata and health output.

Phase 3 adds fallback:

- Add startup failure classification.
- Add target health/cooldown state.
- Add global and request-level fallback order.
- Add response metadata for fallback selection.

Phase 4 adds capability routing:

- Add provider capability declarations.
- Gate endpoint acquisition by required capabilities.
- Convert unsupported-provider failures into structured `409` responses.

Phase 5 adds future providers:

- Remote CDP-backed targets.
- Lightpanda-style lightweight targets with explicit partial capability support.
- Provider-specific E2E smoke lanes selected by capability rather than provider
  name.

## Open Questions

- Should `browserProvider=cloak` be accepted on requests, or should all requests
  use only target names to avoid ambiguity?
- Should fallback be opt-in per request, globally configured, or both?
- Should target-level security policy exist, or should security stay purely
  instance/request scoped?
- Should CLI defaults be `--browser <target>` only, or should commands also
  expose `--browser-provider <provider>`?
- How should target health be surfaced in dashboard without encouraging users to
  run every target all the time?
