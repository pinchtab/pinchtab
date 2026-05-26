# Browser Routing Contract

## Overview

Routing in PinchTab decides which browser provider handles a given request.
The system uses a `CanHandle` pattern: each provider reports its capability for a given request intent.

## Decision Types

- **`DecisionHandle`** — "I can serve this request; proceed with this browser"
- **`DecisionSkip`** — "I cannot serve this shape/intent; caller may try another provider if fallback is available. Must NOT be used for security denials."
- **`DecisionFail`** — "Fatal provider error; abort the request immediately"

## Request Shapes

Each shape constant classifies the kind of work a request represents:

| Constant | Value | Operations |
|----------|-------|------------|
| `ShapeStaticRead` | `static-read` | Lightweight DOM read (no rendering needed) |
| `ShapeStaticSnapshot` | `static-snapshot` | Lightweight snapshot capture |
| `ShapeRenderedRead` | `rendered-read` | DOM read requiring full rendering (Chrome) |
| `ShapeVisual` | `visual` | Screenshots, PDF generation |
| `ShapeInteraction` | `interaction` | Click, type, press, scroll, etc. |
| `ShapeSessionState` | `session-state` | Session/cookie management |
| `ShapeNetworkControl` | `network-control` | Network interception, HAR capture |
| `ShapeDownloadUpload` | `download-upload` | File download/upload operations |

### StateChanging Flag

When `RequestIntent.StateChanging` is true, the request mutates browser state. Providers that only handle read-only operations (e.g. ghost-chrome lite) should return `DecisionSkip` for state-changing requests even if the shape would otherwise be acceptable.

## Security Invariant

Security denials (domain block, IDPI content block, private/internal IP block, redirect limit) are:

- Enforced at the **handler level**, before any browser-specific execution
- **Never** fallback-able — a 403 is final, regardless of which browser was selected
- Separate from capability decisions — a browser returning `DecisionHandle` does not override security policy

## Provider Capabilities (current)

| Provider | Shapes Handled | Notes |
|----------|---------------|-------|
| chrome | All shapes | Full CDP browser; handles everything |
| cloak | All shapes | Anti-detection Chrome fork; same capabilities as chrome |
| ghost-chrome | StaticRead, StaticSnapshot (non-state-changing) | Skips all other shapes; has internal escalation to Chrome |

## Ghost-Chrome Escalation

- Ghost-chrome has a special internal `Route()` method that implements lite-first-then-Chrome escalation
- This is NOT a general multi-browser fallback — it's hardcoded ghost-to-chrome escalation within one provider
- When ghost-chrome's lite attempt produces low-quality results (SPA markers, thin content), it escalates to Chrome automatically

## Fallback Policy (current vs future)

- **Current**: `DecisionSkip` returns HTTP 400 immediately. No multi-browser fallback exists in the handler layer.
- **Future**: `DecisionSkip` should trigger fallback to the next available provider in priority order. Security denials must remain non-fallback-able.

## Routing Order

1. Resolve browser provider from request (query param `?browser=`) or config default
2. Call `CanHandle(intent)` on the resolved provider
3. If `DecisionSkip` — return 400 (future: try next provider)
4. If `DecisionFail` — return 400 with error
5. If `DecisionHandle` — proceed to security checks
6. Apply security policy (domain, IDPI, IP, redirects)
7. If security denies — return 403 (never fallback)
8. Execute request with the selected provider
