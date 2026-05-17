# Browser Abstraction

Status: proposed
Owner: bridge
Related: [browser-targets.md](browser-targets.md)

## Problem

Adding a new browser (e.g. lightpanda, brave, firefox) requires editing at least five unrelated files. Provider-specific logic is scattered across `config/`, `bridge/runtime/`, `browserprobe/`, and `doctor/`, coupled by string switches on `NormalizeBrowserProvider()` and predicates like `IsCloakBrowserProvider()`.

Current branch points:

| Concern | Location | Mechanism |
|---|---|---|
| Launch flags | `internal/bridge/runtime/init.go` | `IsCloakBrowserProvider()` gate + `CloakBrowserFlagArgs()` |
| Geo alignment | `internal/bridge/runtime/geo_align.go` | `switch NormalizeBrowserProvider(...)` |
| Binary discovery | `internal/browserprobe/binary.go` | Parallel `DiscoverChromeBinary`, `DiscoverCloakBrowserBinary` |
| Doctor registry | `internal/doctor/runner.go` | `if IsCloakBrowserProvider()` adds checks |
| Config validation | `internal/config/browser_targets.go` | Hardcoded `{chrome, cloak}` allowlist |

`Engine` (`chrome` / `lite` / `auto`) and `Capabilities` are additional concepts layered on top, partially wired.

## Goal

A new browser implementation should be added by:

1. Creating one sub-package under `internal/browsers/<id>/`.
2. Registering it via `browsers.Register(...)`.

No edits to bridge, doctor, validator, or geo dispatch.

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
    DoctorChecks(cfg TargetConfig) []doctor.Check

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

### Engine as a launch mode, not a provider axis

`Engine` becomes a field on `LaunchConfig`:

```go
type LaunchConfig struct {
    Mode      LaunchMode  // "chrome" | "lite" | "auto"
    Binary    string
    UserDir   string
    Proxy     ProxyConfig
    ExtraFlags []string
    // ...
}
```

- `chrome`: full headful Chromium.
- `lite`: headless + lightweight flags (`--disable-gpu`, `--disable-extensions`, etc.).
- `auto`: resolved by the `Browser` via `Capabilities()` + workload hints.

A provider may reject a mode it cannot serve (e.g. a future `lightpanda` rejects `chrome` mode).

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
2. **Chrome extraction.** Move `DiscoverChromeBinary`, chrome flag builders, chrome geo logic into `browsers/chrome/`. Old call sites delegate to `browsers.Get("chrome")`. Tests stay green.
3. **Cloak extraction.** Move stealth flag builder, cloak geo, cloak binary discovery into `browsers/cloak/` as composition over chrome. Remove `IsCloakBrowserProvider()` callers one at a time.
4. **Doctor migration.** Move provider-specific checks into `browsers/{chrome,cloak}/doctor.go`. `doctor/runner.go` collapses to a registry walk.
5. **Geo migration.** Replace `geo_align.go` switch with `b.GeoAlignment(...)` call. Delete `geo_align.go` body.
6. **Validator migration.** Replace `browser_targets.go` allowlist with `browsers.Get(...)` check.
7. **Engine integration.** Wire `LaunchMode` through `LaunchConfig`. `auto` resolution lives in the registry or a small helper.
8. **Validation by stub.** Add `browsers/lightpanda/` that registers but only stubs `BuildLaunchArgs` (returns "not implemented"). If wiring it requires editing anything outside `browsers/lightpanda/`, the abstraction has leaked — fix before merging.

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
