# Solve

Detect and solve browser challenges (Cloudflare Turnstile, CAPTCHAs, interstitials, etc.) on the current page.

These endpoints are powered by the `internal/autosolver` pipeline. In auto mode, PinchTab runs semantic intent detection first, then tries configured solvers in order, and optionally falls back to LLM when enabled.

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
  "solvers": ["cloudflare", "semantic", "jschallenge"]
}
```

`capsolver` and `twocaptcha` are planned but not implemented, so they are never listed, even with an API key configured.

## Auto-Detect Solve

When no `solver` field is provided, PinchTab runs the autosolver chain using the configured order (`autoSolver.solvers`).

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
| `maxAttempts`| int    | config (`autoSolver.maxAttempts`, default 8) | Maximum solve attempts |
| `timeout`    | float  | auto-estimated from the solver chain (never below 30000) | Overall timeout in milliseconds; an explicit value is used as-is |

The path form (`/solve/{name}`) overrides a body `solver`. On `/tabs/{id}/solve`, a body `tabId` that disagrees with the path is rejected.

## Response

```json
{
  "tabId": "DEADBEEF",
  "solver": "cloudflare",
  "solved": true,
  "challengeType": "turnstile",
  "attempts": 1,
  "title": "thuisbezorgd.nl"
}
```

| Field           | Type   | Description                                    |
|-----------------|--------|------------------------------------------------|
| `tabId`         | string | Tab the solve ran on                           |
| `solver`        | string | Which solver handled the challenge             |
| `solved`        | bool   | Whether the challenge was resolved             |
| `challengeType` | string | Challenge variant (`turnstile`, `recaptcha-v2`, `hcaptcha`) or broad intent (`captcha`, `blocked`) |
| `attempts`      | int    | Number of attempts made                        |
| `title`         | string | Final page title                               |
| `handoff`       | string | `paused_handoff` when a challenge was detected but not solved — the tab is paused for [handoff](./handoff.md) |
| `hint`          | string | Present with `handoff`: return control to the user, then resume the tab |

## Error Responses

| Code | Meaning                                |
|------|----------------------------------------|
| 400  | Invalid body; `unknown_solver` for an unregistered name; `solver_not_implemented` for the planned `capsolver`/`twocaptcha` |
| 404  | Tab not found                          |
| 409  | `dialog_blocked` (a JavaScript dialog is open) or `tab_paused_handoff` |
| 423  | `tab_locked`: tab leased by another owner |
| 500  | CDP/Chrome error                       |

## Built-In Solvers

### Semantic (`semantic`)

Semantic-first solver that uses `/find`-style matching and multi-step action planning for challenge and flow resolution.

### JS Challenge (`jschallenge`)

Generic JavaScript anti-bot/interstitial solver that waits, probes common verification controls, and polls for challenge resolution.

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

**Stealth requirement**: The Cloudflare solver works best with `instanceDefaults.stealthLevel: "full"` in the PinchTab config. Cloudflare evaluates browser fingerprints (CDP detection, WebGL, canvas, navigator properties) before and after the checkbox interaction. Without full stealth, the solver may click correctly but the challenge can still fail fingerprint verification. Check stealth status with `GET /stealth/status`.

### External Solvers (planned)

`capsolver` and `twocaptcha` have no API client yet. Their names and keys
(`autoSolver.external.capsolverKey`, `autoSolver.external.twoCaptchaKey`) are
accepted in config, but neither solver registers or runs, key or not.

## Writing a Custom Solver

Implement the `autosolver.Solver` interface and register it where the autosolver registry is constructed:

```go
package mygateway

import (
    "context"
  "github.com/pinchtab/pinchtab/internal/autosolver"
)

type MyGatewaySolver struct{}

func (s *MyGatewaySolver) Name() string { return "mygateway" }

func (s *MyGatewaySolver) Priority() int { return 150 }

func (s *MyGatewaySolver) CanHandle(ctx context.Context, page autosolver.Page) (bool, error) {
    // Check page markers (title, DOM elements, etc.)
    return false, nil
}

func (s *MyGatewaySolver) Solve(ctx context.Context, page autosolver.Page, exec autosolver.ActionExecutor) (*autosolver.Result, error) {
    // Detect, interact, and resolve the challenge.
  return &autosolver.Result{SolverUsed: "mygateway", Solved: true}, nil
}
```

Then add it to `buildAll` in `internal/autosolver/catalog/catalog.go`, which the handler registry setup reads through `catalog.Registrable`.
