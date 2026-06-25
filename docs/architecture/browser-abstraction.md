# Browser Abstraction

Status: implemented (BridgeAPI encapsulation and browser registry extraction complete)
Owner: bridge

## Problem (historical)

Before the registry extraction, adding a new browser required editing at least
five unrelated files: provider-specific logic was scattered across `config/`,
`bridge/runtime/`, `browserprobe/`, and `doctor/`, coupled by string switches
and per-provider predicates. Those branch points now route through one
abstraction each:

| Concern | Location | Mechanism today |
|---|---|---|
| Launch flags | `internal/browsers/<id>/` + `internal/browsers/runtimekit/` | per-provider `BuildLaunchArgs` via `ResolveProviderLaunchPlan` |
| Geo alignment | `internal/browsers/` config | provider capabilities + `GeoConfig` on the launch plan |
| Binary discovery | `internal/browsers/<id>/` | per-provider `DiscoverBinary()` |
| Doctor checks | `internal/doctor/browsers.go` | per-provider `DoctorChecks()` from the registry |
| Config validation | `internal/config/browser_targets.go` | registry-driven (`browsers.Get`/`browsers.IDs`) |
| Lifecycle cleanup | `internal/browsers/providerhooks/` | registered `Hooks` (decorate, cleanup, shutdown) |

`Engine` (`chrome` / `lite` / `auto`) — **deprecated**; replaced by the browser provider model (`chrome` / `cloak` / `ghost-chrome`). See [routing-contract.md](routing-contract.md) and [terminology.md](terminology.md) for the canonical provider definitions. `Capabilities` are additional concepts layered on top, partially wired.

## Goal

A new browser implementation should be added by:

1. Creating one sub-package under `internal/browsers/<id>/`.
2. Registering it via `browsers.Register(...)`.

Adding a provider should not require runtime dispatch edits (bridge,
doctor, validator, geo dispatch). Public config schema, documentation,
and UI may still need updates for a new provider.

## Design

### Package layout

```
internal/browsers/
  browser.go           // Browser interface + registry
  config.go            // LaunchConfig, GeoConfig, TargetConfig (provider-neutral)
  capabilities.go      // CapabilitySet (moved from config/)
  common/              // shared chrome-family helpers (proxy auth, profile, CDP url discovery)
  chrome/              // chrome implementation, registers itself
  cloak/               // cloak implementation, composes chrome
  lightpanda/          // future
  brave/               // future
```

`browserprobe/`, `bridge/runtime/init.go` provider branches, and `doctor` provider gates are removed in favor of delegating to the registered `Browser`.

### Interface

```go
package browsers

type Browser interface {
    ID() string                              // "chrome", "cloak", "lightpanda"
    DisplayName() string                     // human-readable
    Capabilities() CapabilitySet

    // Discovery & doctor
    DiscoverBinary() BinaryDiscovery
    DoctorChecks(cfg TargetConfig) []DoctorCheck

    // Launch
    BuildLaunchArgs(cfg LaunchConfig) (args []string, env []string, err error)
    SupportsRemoteCDP() bool

    // Strategy hooks
    GeoAlignment(geo GeoConfig) GeoStrategy
    ValidateTarget(cfg TargetConfig) error
}

type GeoStrategy struct {
    Flags          []string
    Env            []string
    OperatorWins   bool  // explicit user config overrides geo-derived values
}

var registry = map[string]Browser{}

func Register(b Browser)               { registry[b.ID()] = b }
func Get(id string) (Browser, bool)    { b, ok := registry[id]; return b, ok }
func IDs() []string                    { /* sorted list */ }
```

Each implementation registers in its package `init()`:

```go
// internal/browsers/chrome/chrome.go
func init() { browsers.Register(&Browser{}) }
```

Importers wire providers via a barrel file (e.g. `internal/browsers/all/all.go`) so callers get all built-ins with one blank import.

### Launch mode

`LaunchMode` controls **headed vs headless** browser launch state. It is
purely about the display mode of the browser process — not about which
provider handles a request (that is the provider model's job).

> **Deprecation note:** The `chrome` / `lite` / `auto` engine values that
> previously occupied this field are fully superseded by the browser provider
> model (`chrome` / `cloak` / `ghost-chrome`). The `Engine` field is
> deprecated and will be removed. See [terminology.md](terminology.md).

`Mode` is a field on `LaunchConfig`:

```go
type LaunchConfig struct {
    Mode      LaunchMode  // headed | headless (internal only)
    Binary    string
    UserDir   string
    Proxy     ProxyConfig
    ExtraFlags []string
    // ...
}
```

- `headed`: full headful Chromium with GUI.
- `headless`: headless Chromium (`--headless=new` + lightweight flags).

A provider may reject a mode it cannot serve (e.g. a future `lightpanda` rejects `headed` mode).

### Cloak as composition over Chrome

Cloak is Chrome plus stealth flags and stricter geo precedence. Don't duplicate — embed:

```go
// internal/browsers/cloak/cloak.go
type Browser struct{ chrome.Browser }

func (b Browser) ID() string          { return "cloak" }
func (b Browser) DisplayName() string { return "CloakBrowser" }

func (b Browser) BuildLaunchArgs(cfg browsers.LaunchConfig) ([]string, []string, error) {
    args, env, err := b.Browser.BuildLaunchArgs(cfg)
    if err != nil { return nil, nil, err }
    return append(args, cloakFlagArgs(cfg)...), env, nil
}

func (b Browser) GeoAlignment(geo browsers.GeoConfig) browsers.GeoStrategy {
    s := cloakGeoFlags(geo)
    s.OperatorWins = true  // explicit user config wins
    return s
}

func (b Browser) DiscoverBinary() browsers.BinaryDiscovery {
    return discoverCloakBinary()  // separate paths from chrome
}
```

### Doctor

`doctor/runner.go` becomes a thin orchestrator:

```go
func Registry(cfg config.Config) []Check {
    base := []Check{configFileCheck, binaryExistsCheck, binaryExecutableCheck, binaryStartsCheck}
    for _, id := range browsers.IDs() {
        b, _ := browsers.Get(id)
        base = append(base, b.DoctorChecks(cfg.ResolveTarget())...)
    }
    return base
}
```

Provider-specific checks (`cdp_reachable`, `fingerprint_flags_accepted`, `linux_fonts_present`) move into `browsers/cloak/doctor.go`.

### Config validation

`browser_targets.go` replaces its hardcoded allowlist with:

```go
if _, ok := browsers.Get(target.Provider); !ok {
    return fmt.Errorf("unknown browser provider %q (known: %v)", target.Provider, browsers.IDs())
}
return browsers.MustGet(target.Provider).ValidateTarget(target)
```

### Bridge integration

`bridge/runtime/init.go` no longer branches on provider. It resolves the `Browser` once, then delegates:

```go
b, ok := browsers.Get(cfg.Provider)
if !ok { return fmt.Errorf("unsupported provider %q", cfg.Provider) }

args, env, err := b.BuildLaunchArgs(launchCfg)
geo := b.GeoAlignment(geoCfg)
// apply args + env + geo.Flags + geo.Env
```

`InitRemoteCDP` checks `b.SupportsRemoteCDP()` before attaching.

## Migration plan

Extract-then-rewire. Each step lands independently with no behavior change.

1. **Skeleton.** Add `internal/browsers/` with the interface, registry, and empty `chrome` package. Wire a barrel import in one place (`cmd/pinchtab/root.go`). No call sites change yet.
2. **Chrome extraction.** Move Chrome-specific launch helpers, geo logic, and provider discovery behind `browsers/chrome/` plus shared `runtimekit` entry points. Old call sites delegate through `browsers.Get("chrome")` / `FindBrowserBinary(...)`. Tests stay green.
3. **Cloak extraction.** Move stealth flag builder, cloak geo, cloak binary discovery into `browsers/cloak/` as composition over chrome. Remove `IsCloakBrowserProvider()` callers one at a time.
4. **Doctor migration.** Move provider-specific checks into `browsers/{chrome,cloak}/doctor.go`. `doctor/runner.go` collapses to a registry walk.
5. **Geo migration.** Replace `geo_align.go` switch with `b.GeoAlignment(...)` call. Delete `geo_align.go` body.
6. **Validator migration.** Replace `browser_targets.go` allowlist with `browsers.Get(...)` check.
7. **Launch-mode integration.** Wire `LaunchMode` through `LaunchConfig` (internal only; the public-facing engine concept is replaced by browser provider routing). `auto` resolution lives in the registry or a small helper.
8. **Validation by stub.** Add `browsers/lightpanda/` that registers but only stubs `BuildLaunchArgs` (returns "not implemented"). If wiring it requires editing anything outside `browsers/lightpanda/`, the abstraction has leaked — fix before merging.

## Handler-Layer Routing Leakage

### Problem

The `browsers.Browser` interface (above) solved the launch/discovery/doctor
axis — adding a new browser no longer requires editing five files. But there
is a second abstraction boundary that is still leaking: **request routing
at the handler layer**.

Ghost-chrome's internal routing (static browser vs Chrome) is spread across
handlers, the server wiring layer, and a routing package. The handler code
contains explicit `if browser == "ghost-chrome" && h.StaticBrowser != nil`
branches, the `Handlers` struct carries a `StaticBrowser` field, and the
routing package type-asserts `*ghostchrome.Browser`. Chrome and Cloak are
simple (everything goes to CDP). Ghost-chrome has internal routing (static
first, Chrome on escalation). That distinction should be invisible to
callers.

### Target Architecture

```
┌──────────────────────────────────────────────────────┐
│                     Handlers                         │
│                                                      │
│  Only knows about BridgeAPI.                         │
│  No StaticBrowser field. No shouldUseStaticAction.   │
│  No ghost-chrome conditionals. No routing package.   │
│                                                      │
│  navigate() → Bridge.Navigate(url)                   │
│  snapshot() → Bridge.Snapshot(tabID, filter)          │
│  text()     → Bridge.Text(tabID)                     │
│  action()   → Bridge.TabContext(tabID)               │
│              → Bridge.ExecuteAction(ctx, kind, req)   │
│                                                      │
└──────────────────┬───────────────────────────────────┘
                   │ BridgeAPI interface
                   ▼
┌──────────────────────────────────────────────────────┐
│              BridgeAPI implementations               │
│                                                      │
│  ┌──────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │ Chrome       │  │ Cloak       │  │ GhostChrome  │ │
│  │ (bridge.New) │  │ (bridge.New │  │ Adapter      │ │
│  │              │  │  + stealth) │  │              │ │
│  │ All calls go │  │ All calls   │  │ Internally   │ │
│  │ to CDP       │  │ go to CDP   │  │ routes each  │ │
│  │              │  │ + stealth   │  │ call to      │ │
│  │              │  │ injection   │  │ static or    │ │
│  │              │  │             │  │ Chrome       │ │
│  └──────────────┘  └─────────────┘  └──────┬───────┘ │
└──────────────────────────────────────────────┼───────┘
                                               │
                          ┌────────────────────┴──────────────────┐
                          │         GhostChrome Adapter           │
                          │         (internal detail)             │
                          │                                      │
                          │  ┌─────────────┐  ┌───────────────┐  │
                          │  │ staticfetch  │  │ Chrome bridge │  │
                          │  │ (gost-dom)   │  │ (real CDP)    │  │
                          │  └─────────────┘  └───────────────┘  │
                          │                                      │
                          │  Navigate → static first, Chrome if  │
                          │             quality too low           │
                          │  Snapshot → static if tab is static,  │
                          │             Chrome if escalated       │
                          │  Text     → static if tab is static   │
                          │  Action   → static for click/type     │
                          │             with ref, Chrome otherwise │
                          │  Evaluate → always Chrome (escalate)  │
                          │  Screenshot → always Chrome (escalate)│
                          │                                      │
                          │  Ref mapping, quality gates, tab      │
                          │  escalation all happen HERE, not in   │
                          │  the handler layer.                   │
                          └──────────────────────────────────────┘
```

#### Current State (post-Phase 6)

The target architecture is implemented through Phase 6. Key components:

- **`bridge.BridgeAPI`** (`internal/bridge/api.go`) — interface with
  `Navigate`, `Snapshot`, `Text` methods alongside existing tab/action ops.
- **`bridgekit.BridgeAdapter`** (`internal/browsers/ghostchrome/bridgekit/bridge_adapter.go`)
  — wraps `BridgeAPI` for ghost-chrome routing. Embeds the Chrome bridge
  and delegates to `ghostchrome.BridgeProxy` for static-first routing.
  Enforces `NavigateParams` as network policy and `ContentParams.ContentGuard`
  as IDPI scanning on static paths.
- **Handlers** — use only `BridgeAPI`. Navigate and Snapshot handlers call
  `Bridge.Navigate()` and `Bridge.Snapshot()` respectively. No `StaticBrowser`
  field, no ghost-chrome conditionals, no routing package imports.
- **Deferred-launch two-phase navigate** — for fresh new-tab navigates the
  handler probes `StaticFirstNavigate()` and runs phase 1 with
  `NavigateParams.NoEscalate`: the adapter attempts the static browser only
  and signals `*bridge.StaticEscalateError` instead of escalating internally,
  so Chrome is launched only when actually needed. Phase 2 re-runs with
  `SkipStatic`. The timeout budget is per phase (each phase gets
  `NavigateTimeout`, default 30s), so a navigate that escalates can take up
  to twice the configured timeout.
- **Route metadata** — `usedProvider` is consistently `"ghost-chrome"` for all
  adapter paths. Static vs Chrome routing recorded in `Attempts[]`.
- **`internal/routing/`** — deleted.
- **`internal/handlers/routing.go`** — deleted.
- **`bridgeProxyAdapter`** — deleted from `server/bridge.go`.

#### Key Principles

1. **Handlers only know BridgeAPI.** No `StaticBrowser` field, no
   `shouldUseStaticAction`, no `useStaticBrowser`, no
   `if browser == "ghost-chrome"` conditionals.

2. **BridgeAPI gains read operations.** `Navigate`, `Snapshot`, `Text`
   become methods on BridgeAPI so the ghost-chrome adapter can intercept
   them the same way it intercepts `TabContext` and `ExecuteAction`.

3. **Ghost-chrome adapter is a BridgeAPI wrapper.** It wraps the real
   Chrome bridge and a `staticfetch.Browser`. Every BridgeAPI method is
   intercepted: read-only operations try static first, Chrome-only
   operations escalate, actions route by kind+ref. All routing logic
   lives inside the adapter.

4. **No separate routing package for browsers.** The `routing.Route()`
   function and its `ghostchrome.StaticFetcher` adapter exist only
   because handlers can't call `Browser.Route()` through BridgeAPI. Once
   BridgeAPI has Navigate/Snapshot/Text, routing decisions happen inside
   the adapter.

5. **The server layer is thin wiring.** `configureBridgeRouter` creates
   the adapter and sets `h.Bridge = adapter`. Nothing else. No
   `h.StaticBrowser = ...`.

### Violations

#### Resolved (Phases 1-5)

| ID | Description | Resolved by |
|----|-------------|-------------|
| ~~V1~~ | `Handlers.StaticBrowser` field | Phase 3 — field deleted; handlers use only `BridgeAPI` |
| ~~V2~~ | Ghost-chrome fast paths in snapshot handler | Phase 1+3 — `BridgeAPI.Snapshot()` encapsulates routing |
| ~~V3~~ | Ghost-chrome fast paths in text handler | Phase 1+3 — `BridgeAPI.Text()` encapsulates routing |
| ~~V4~~ | Duplicate `useStaticBrowser` fast paths | Phase 3 — helper and all call sites deleted |
| ~~V5~~ | `shouldUseStaticAction` / `executeStaticAction` | Phase 3 — deleted; adapter routes actions internally |
| ~~V6~~ | Tab resolution skipped for static actions | Phase 3 — handlers always call `tabContext()`; adapter handles static tabs |
| ~~V7~~ | `staticFetcher` adapter in handlers | Phase 4 — `internal/handlers/routing.go` deleted |
| ~~V8~~ | `routing.Route()` type-asserts ghostchrome | Phase 4 — `internal/routing/` package deleted |
| ~~V9~~ | Health handler short-circuits for StaticBrowser | Phase 3 — `StaticBrowser` field removed; health uses BridgeAPI |
| ~~V10~~ | `populateEscalatedRefCache` in server layer | Phase 5 — ref cache logic moved into `bridgekit.BridgeAdapter` |
| ~~V11~~ | `bridgeProxyAdapter` too thick | Phase 5 — replaced by `bridgekit.BridgeAdapter`; server adapter deleted |

#### Open

**~~V12: BridgeAPI read methods do not own security-policy enforcement.~~** RESOLVED
The ghost-chrome adapter now enforces `NavigateParams` as a
`staticfetch.NavigateNetworkPolicy` (SSRF, redirect limits, trusted CIDRs/IPs)
and scans static Snapshot/Text content through `ContentParams.ContentGuard`
(IDPI blocking/warnings). Handlers retain their own pre-flight checks for
the Chrome path; the adapter ensures static paths have equivalent enforcement.

**~~V13: Route metadata constructed by handlers.~~** RESOLVED
`NavigateResult`, `SnapshotResult`, and `TextResult` carry route metadata
from the adapter. `usedProvider` is consistently `"ghost-chrome"` for all
adapter paths (static accepted, escalated, or fallback). Static vs Chrome
routing details are captured in the `Attempts` array. Handlers propagate
the adapter-provided route metadata when available.

**V14: Target/provider bookkeeping split (Phase 6 items).**
Target resolution and provider selection still involve scattered config
lookups. The browser registry extraction (migration plan steps 1-8) is
partially complete but not yet wired end-to-end.

### Migration Path (BridgeAPI Encapsulation)

Incremental. Each phase lands independently.

**Phase 1 — Move read operations into BridgeAPI.** COMPLETE
`Navigate`, `Snapshot`, `Text` added to `BridgeAPI` interface
(`internal/bridge/api.go`). `bridge.Bridge` delegates to Chrome CDP.
`NavigateParams` and `ContentParams` carry per-request security policy.

**Phase 2 — Ghost-chrome adapter implements BridgeAPI directly.** COMPLETE
`bridgekit.BridgeAdapter` (`internal/browsers/ghostchrome/bridgekit/bridge_adapter.go`)
wraps `bridge.BridgeAPI` and a `ghostchrome.BridgeProxy`. All
ghost-chrome static-vs-Chrome routing decisions are encapsulated here.
Server layer creates the adapter via `bridgekit.NewBridgeAdapter(chromeBridge, cfg)`.

**Phase 3 — Remove StaticBrowser from Handlers.** COMPLETE
`StaticBrowser` field, `useStaticBrowser()`, `shouldUseStaticAction()`,
`executeStaticAction()`, ghost-chrome fast paths in snapshot/text
handlers, and the `staticFetcher` adapter all deleted. Handlers have
zero ghost-chrome awareness.

**Phase 4 — Remove routing package.** COMPLETE
`internal/routing/` package and `internal/handlers/routing.go` deleted.

**Phase 5 — Clean up BridgeProxy API surface.** COMPLETE
`populateEscalatedRefCache` and `bridgeProxyAdapter` removed from
`server/bridge.go`. Ref cache logic moved into the adapter.
`configureBridgeRouter` is now thin wiring:

```go
func configureBridgeRouter(h *handlers.Handlers, cfg *config.RuntimeConfig) {
    if cfg.DefaultBrowser != config.BrowserGhostChrome {
        return
    }
    h.Bridge = bridgekit.NewBridgeAdapter(h.Bridge, cfg)
}
```

**Phase 6 — Security policy ownership + route metadata.** MOSTLY COMPLETE
- Ghost-chrome adapter enforces `NavigateParams` as network policy and
  `ContentParams.ContentGuard` as IDPI scanning on static paths (V12 resolved).
- Route metadata returned by adapter, consumed by handlers (V13 resolved).
- Remaining: target/provider bookkeeping extraction not yet wired end-to-end (V14).

## Non-goals

- Replacing chromedp / CDP client choice.
- Per-target capability gating at the action layer. Capabilities stay advisory.
- Sandboxing or process-isolation changes.
- Cross-provider profile interop.

## Open questions

1. **Shared chrome-family code.** Cloak, Brave, and lightpanda-with-chromium-mode all share most flag building. Lives in `browsers/common/` or as exported helpers on `chrome.Browser`? Lean toward exporting from `chrome` to avoid a third package.
2. **Doctor check identity.** Today checks have stable string IDs used by CI. Moving them into provider packages must preserve IDs. Audit before step 4.
3. **Geo `OperatorWins` semantics.** Currently encoded implicitly in cloak geo. Lift to an explicit field as proposed, or keep inside the strategy's flag list?
4. **Engine `auto`.** Resolved by the provider, or by a registry helper using `Capabilities()`? Probably the latter so policy stays in one place.

## Out of scope for v1

- Brave, Firefox, lightpanda actual implementations.
- WebDriver fallback for non-CDP browsers.
- Per-provider telemetry namespacing.
