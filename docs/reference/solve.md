# Solve

Detect and solve browser challenges (Cloudflare Turnstile, CAPTCHAs, interstitials, etc.) on the current page.

PinchTab ships with a pluggable **solver framework**. Each solver targets a specific provider (e.g. Cloudflare). Solvers are registered at startup and can be invoked explicitly by name or discovered automatically.

## Endpoints

```text
GET  /solvers
POST /solve
POST /solve/{name}
POST /tabs/{id}/solve
POST /tabs/{id}/solve/{name}
```

## List Solvers

```bash
curl http://localhost:9867/solvers
```

```json
{
  "solvers": ["cloudflare"]
}
```

## Auto-Detect Solve

When no `solver` field is provided, PinchTab tries each registered solver in order. The first one whose `CanHandle` returns true is used.

```bash
curl -X POST http://localhost:9867/solve \
  -H "Content-Type: application/json" \
  -d '{"maxAttempts": 3, "timeout": 30000}'
```

If no challenge is detected on the page, the response returns immediately with `solved: true` and `attempts: 0`.

## Named Solver

Specify the solver by name in the body or path:

```bash
# Body
curl -X POST http://localhost:9867/solve \
  -H "Content-Type: application/json" \
  -d '{"solver": "cloudflare", "maxAttempts": 3}'

# Path
curl -X POST http://localhost:9867/solve/cloudflare \
  -H "Content-Type: application/json" \
  -d '{"maxAttempts": 3}'
```

## Tab-Scoped Solve

```bash
curl -X POST http://localhost:9867/tabs/{tabId}/solve \
  -H "Content-Type: application/json" \
  -d '{"solver": "cloudflare"}'
```

## Request Body

| Field        | Type   | Default | Description                              |
|--------------|--------|---------|------------------------------------------|
| `tabId`      | string | —       | Tab ID (optional, uses default tab)      |
| `solver`     | string | —       | Solver name (optional, auto-detect)      |
| `maxAttempts`| int    | 3       | Maximum solve attempts                   |
| `timeout`    | float  | 30000   | Overall timeout in milliseconds          |

## Response

```json
{
  "tabId": "DEADBEEF",
  "solver": "cloudflare",
  "solved": false,
  "challengeType": "managed",
  "attempts": 1,
  "title": "Just a moment...",
  "needsHumanHandoff": true,
  "handoffReason": "challenge_requires_manual_intervention"
}
```

| Field           | Type   | Description                                    |
|-----------------|--------|------------------------------------------------|
| `tabId`         | string | Tab the solve ran on                           |
| `solver`        | string | Which solver handled the challenge             |
| `solved`        | bool   | Whether the challenge was resolved             |
| `challengeType` | string | Challenge variant (e.g. `managed`, `embedded`) |
| `attempts`      | int    | Number of attempts made                        |
| `title`         | string | Final page title                               |
| `needsHumanHandoff` | bool | Whether manual intervention is required to continue |
| `handoffReason` | string | Reason for handoff (`challenge_requires_manual_intervention`, `credentials_required`, or empty when not needed) |

## Error Responses

| Code | Meaning                                |
|------|----------------------------------------|
| 400  | Invalid body or unknown solver name    |
| 404  | Tab not found                          |
| 423  | Tab locked by another owner            |
| 500  | CDP/Chrome error                       |

## Built-In Solvers

### Cloudflare (`cloudflare`)

Handles Cloudflare Turnstile and interstitial challenges.

**Detection**: Checks the page title for known Cloudflare indicators ("Just a moment...", "Attention Required", "Checking your browser").

**Challenge types**:

| Type              | Handling                                               |
|-------------------|--------------------------------------------------------|
| `non-interactive` | Waits for auto-resolution (up to 15s)                  |
| `managed`         | Locates Turnstile iframe, clicks checkbox              |
| `interactive`     | Same as managed                                        |
| `embedded`        | Detects via Turnstile script tag, clicks checkbox      |

**Click strategy**: The solver uses human-like mouse input (Bezier curve movement, random delays, press/release offset) to click the Turnstile checkbox. Click coordinates are computed relative to the widget dimensions (not hardcoded pixel offsets) with randomised jitter.

**Stealth requirement**: The Cloudflare solver works best with `stealthLevel: "full"` in the PinchTab config. Cloudflare evaluates browser fingerprints (CDP detection, WebGL, canvas, navigator properties) before and after the checkbox interaction. Without full stealth, the solver may click correctly but the challenge can still fail fingerprint verification. Check stealth status with `GET /stealth/status`.

## Writing a Custom Solver

Implement the `solver.Solver` interface and register it during `init()`:

```go
package mygateway

import (
    "context"
    "github.com/pinchtab/pinchtab/internal/solver"
)

func init() {
    solver.MustRegister("mygateway", &MyGatewaySolver{})
}

type MyGatewaySolver struct{}

func (s *MyGatewaySolver) Name() string { return "mygateway" }

func (s *MyGatewaySolver) CanHandle(ctx context.Context) (bool, error) {
    // Check page markers (title, DOM elements, etc.)
    return false, nil
}

func (s *MyGatewaySolver) Solve(ctx context.Context, opts solver.Options) (*solver.Result, error) {
    // Detect, interact, and resolve the challenge.
    return &solver.Result{Solver: "mygateway", Solved: true}, nil
}
```

The solver has access to the full chromedp context for CDP operations.
