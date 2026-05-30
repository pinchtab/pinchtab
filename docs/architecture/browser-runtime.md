# Browser Runtime Architecture

Owner: bridge, browsers
Related: [browser-abstraction.md](browser-abstraction.md), [routing-contract.md](routing-contract.md)

## Overview

Handlers have zero chromedp/cdproto imports. All browser operations go
through BridgeAPI (~40 methods). CDP usage is contained in the bridge
layer and the cdptk shared toolkit. Each browser provider owns its
post-launch runtime behavior via the `RuntimeInstance` interface.

## Layer diagram

```
┌─────────────────────────────────────────────────────────┐
│  Handlers (54 files)                                    │
│                                                         │
│  Zero chromedp imports. Zero cdproto imports.            │
│  All operations via bridge.BridgeAPI.                    │
│  TabContext() returns *TabHandle (opaque).               │
│                                                         │
│  screenshot → Bridge.CaptureScreenshot(ctx, ...)        │
│  screencast → Bridge.StartScreencast(ctx, opts)         │
│  record     → captureFrame closure (injected at init)   │
│  evaluate   → Bridge.Evaluate(ctx, expr, &out, opts)    │
│  cookies    → Bridge.GetCookies(ctx) / SetCookie(ctx)   │
│  DOM        → Bridge.CallFunctionOnNode(ctx, ...)       │
│  download   → Bridge.DownloadURL(ctx, url, opts)        │
│  emulation  → Bridge.SetViewport / SetGeolocation / ... │
│  navigation → Bridge.CurrentURL / CurrentTitle          │
│  actions    → Bridge.ExecuteAction(ctx, kind, req)      │
└──────────────────────────┬──────────────────────────────┘
                           │ BridgeAPI (domain types, no CDP types)
                           ▼
┌─────────────────────────────────────────────────────────┐
│  Bridge (BridgeAPI — ~40 methods)                       │
│                                                         │
│  Owns: lifecycle, tab routing, locks, auto-close,       │
│        network monitoring, CDP connection               │
│                                                         │
│  Holds a RuntimeInstance for provider-specific behavior. │
│                                                         │
│  Visual:     CaptureScreenshot, StartScreencast         │
│  Evaluate:   Evaluate, CallFunctionOnNode,              │
│              EvaluateInFrame                             │
│  DOM:        DescribeNode, ResolveSelectorToNodeID,      │
│              SetFileInputFiles                           │
│  Cookies:    GetCookies, SetCookie                       │
│  Emulation:  SetViewport, SetGeolocation,                │
│              SetEmulatedMedia                            │
│  Network:    SetNetworkConditions, SetExtraHTTPHeaders,  │
│              EnableNetwork, ListenNetworkEvents           │
│  Navigation: CurrentURL, CurrentTitle, GoBack,           │
│              GoForward, Reload                           │
│  Download:   DownloadURL (Fetch interception + Network)  │
│  Auth:       EnableFetchWithAuth                         │
│  PDF:        PrintToPDF                                  │
│  Stealth:    SetUserAgentOverride,                       │
│              AddScriptToEvaluateOnNewDocument            │
│  Tabs:       ListTargets → []TabTarget (bridge type)     │
│                                                         │
│  BridgeAPI signatures use domain types, not CDP types.   │
│  CDP types never appear in BridgeAPI signatures.         │
└──────────────────────────┬──────────────────────────────┘
                           │ RuntimeInstance + chromedp (internal)
                           ▼
┌─────────────────────────────────────────────────────────┐
│  RuntimeInstance (per-provider runtime)                  │
│                                                         │
│  20-method interface in internal/browsers/.              │
│  Each provider owns HOW it controls the browser.        │
│  CDP is an implementation detail, not a contract.       │
│                                                         │
│  Chrome instance  → uses cdptk (shared toolkit)         │
│  Cloak instance   → embeds Chrome, forces polling       │
│                     screencast (no Page.startScreencast) │
│  Ghost-chrome     → embeds Chrome (pure delegation)     │
│  Future: Firefox  → WebDriver BiDi (no CDP at all)      │
└──────────────────────────┬──────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│  internal/cdptk/ (shared CDP toolkit)                   │
│                                                         │
│  Pure functions. No state. No browser ownership.        │
│  Takes a chromedp context, returns data.                │
│                                                         │
│  cdptk.CaptureScreenshot(ctx, format, quality, clip)    │
│  cdptk.ClipForNode(ctx, backendNodeID) → *ScreenshotClip│
│  cdptk.ScreencastRepaintLoop(ctx) (start/stop)          │
│  cdptk.AnnotatedScreenshot(ctx, ...) → []byte           │
│                                                         │
│  Used by RuntimeInstance impls. Never by handlers.      │
└─────────────────────────────────────────────────────────┘
```

## Browser interface (pre-launch)

The `Browser` interface handles pre-launch concerns: ID, DisplayName,
BuildLaunchArgs, CanHandle, DiscoverBinary, DoctorChecks, GeoAlignment,
Capabilities, ValidateTarget. It also provides `NewRuntimeInstance()` to
create the post-launch runtime.

Each provider package exports both:

```go
// internal/browsers/chrome/chrome.go
type Browser struct{}           // implements browsers.Browser (pre-launch)

// internal/browsers/chrome/instance.go
type Instance struct{}          // implements browsers.RuntimeInstance (post-launch)
```

The Bridge calls `browser.NewRuntimeInstance(browserCtx, headless)`
during `EnsureChrome()` and holds the returned `RuntimeInstance` for the
session lifetime.

## TabHandle

`TabContext()` returns `*TabHandle` instead of `context.Context`.
`TabHandle` implements `context.Context` (Deadline/Done/Err/Value all
delegate to the underlying CDP context) but the return type signals to
handlers that this context should only be passed to Bridge methods.

## Capability-based routing

Browser capabilities (`CapabilitySet`) drive runtime behavior. For
example, `CapEventScreencast` controls screencast strategy:

- Chrome declares `CapEventScreencast` → event-driven screencast
- Cloak omits it → polling-based screencast (same as headless path)

`shouldUsePollingScreencast()` checks both `Config.Headless` and the
browser's capability set.

## Key patterns

- **BridgeAPI signatures use domain types, not CDP types.** Return
  `[]byte` for screenshots, not `*page.CaptureScreenshotReturns`.

- **CallFunctionOnNode** centralizes the DOM.resolveNode →
  Runtime.callFunctionOn pattern. Used by attr, box, checked, enabled,
  visible, value, inspect, and text handlers.

- **ScreencastStream** returns `*ScreencastStream` with
  `Frames <-chan []byte`. Two strategies selected by capability.

- **DownloadURL** encapsulates the full Fetch interception + Network
  monitoring state machine.

- **captureFrame closure** — the recorder receives a `captureFrame`
  function injected at construction time, bound to
  `Bridge.CaptureScreenshot`.

- **TabTarget** — bridge-level type replaces `*target.Info` to avoid
  cdproto imports leaking into handlers.

## CDP usage by layer

| Layer | CDP Domains Used | Purpose |
|---|---|---|
| bridge/ | Page, DOM, Runtime, Network, Fetch, Emulation, Input, Target | All browser operations delegated from handlers |
| cdptk/ | Page, DOM, Runtime | Shared pure-function CDP wrappers |
| browsers/chrome/ | Page, DOM, Runtime, Network, Fetch, Emulation | RuntimeInstance implementation |
| handlers/ | None | All operations via BridgeAPI |

## Non-goals

- Replacing chromedp as the CDP client library.
- Supporting non-Chromium browsers in v1 (but the architecture
  does not prevent it).
