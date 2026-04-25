# AutoSolver Architecture

## Overview

The AutoSolver system provides modular, semantic-first browser automation for Pinchtab. It evolves the existing `internal/solver` framework (PR #395) into a general-purpose automation agent capable of handling CAPTCHAs, login flows, signup flows, multi-step navigation, and onboarding sequences.

### Design Principles

1. **Isolation First** — The autosolver module (`internal/autosolver/`) has zero coupling to chromedp or the bridge runtime. All browser interactions go through `Page` and `ActionExecutor` interfaces.

2. **Semantic First** — The `pinchtab/semantic` package is the primary intelligence layer. LLM is used only as a last-resort fallback.

3. **Pluggable Architecture** — Solvers are registered at runtime via a `Registry`. External solvers (Capsolver, 2Captcha) are optional plugins enabled via configuration.

4. **Behavior > Spoofing** — Solvers interact with pages through legitimate browser actions (clicking, typing) rather than API hacks or fake tokens.

5. **Extensible Beyond CAPTCHA** — The `IntentType` system supports login, signup, onboarding, and navigation flows alongside CAPTCHA solving.

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────┐
│                    AutoSolver                        │
│                                                      │
│  ┌──────────┐    ┌──────────┐    ┌──────────────┐   │
│  │ Registry │───▶│Core Loop │───▶│ Fallback     │   │
│  │ (solvers)│    │(detect + │    │ Chain:       │   │
│  └──────────┘    │ dispatch)│    │ semantic →   │   │
│                  └────┬─────┘    │ rule-based → │   │
│                       │          │ external →   │   │
│                       ▼          │ LLM          │   │
│              ┌────────────────┐  └──────────────┘   │
│              │  Interfaces    │                      │
│              │  Page          │                      │
│              │  ActionExecutor│                      │
│              │  SemanticEngine│                      │
│              │  LLMProvider   │                      │
│              └────────┬───────┘                      │
└───────────────────────┼──────────────────────────────┘
                        │ (interface boundary)
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
┌──────────────┐ ┌─────────────┐ ┌──────────────┐
│adapters/     │ │semantic/    │ │external/     │
│pinchtab.go   │ │adapter.go   │ │capsolver.go  │
│(chromedp)    │ │(semantic pkg)│ │twocaptcha.go │
└──────────────┘ └─────────────┘ └──────────────┘
```

## Module Structure

```
internal/autosolver/
├── interfaces.go          # Page, ActionExecutor, Solver, SemanticEngine, LLMProvider
├── types.go               # Result, Intent, Config, enums
├── autosolver.go          # Core orchestrator with fallback chain
├── challenge_detection.go # Shared challenge classification (title/URL/HTML)
├── heuristics.go          # Title-based intent detection fallback
├── registry.go            # Instance-level solver registry with priority ordering
├── autosolver_test.go     # Core loop tests
├── challenge_detection_test.go
├── registry_test.go
├── adapters/
│   └── pinchtab.go        # Bridge adapter (ONLY chromedp import)
├── semantic/
│   └── adapter.go         # Wraps pinchtab/semantic ElementMatcher
├── external/
│   ├── capsolver.go       # Capsolver API skeleton
│   └── twocaptcha.go      # 2Captcha API skeleton
├── llm/
│   ├── llm.go             # LLM provider skeleton with structured prompts
│   └── trim.go            # HTML trimming for token efficiency
└── solvers/
    ├── cloudflare.go      # Cloudflare Turnstile (new interface, no chromedp)
    ├── jschallenge.go     # Generic JavaScript challenge/interstitial solver
    ├── jschallenge_test.go
    └── legacy.go          # Compatibility shim for existing solver.Solver
```

## Core Interfaces

### Page

Read-only view of the current browser page:

```go
type Page interface {
    URL() string
    Title() string
    HTML() (string, error)
    Screenshot() ([]byte, error)
}
```

### ActionExecutor

Performs browser actions with human-like behavior:

```go
type ActionExecutor interface {
    Click(ctx context.Context, x, y float64) error
    Type(ctx context.Context, text string) error
    WaitFor(ctx context.Context, selector string, timeout time.Duration) error
    Evaluate(ctx context.Context, expr string, result interface{}) error
    Navigate(ctx context.Context, url string) error
}
```

### Solver

Handles a specific class of challenge:

```go
type Solver interface {
    Name() string
    Priority() int  // Lower = tried first
    CanHandle(ctx context.Context, page Page) (bool, error)
    Solve(ctx context.Context, page Page, executor ActionExecutor) (*Result, error)
}
```

**Priority ranges:**
| Range | Category |
|-------|----------|
| 0–99 | Built-in solvers (Cloudflare, etc.) |
| 100–199 | Semantic-based solvers |
| 200–299 | External API solvers (Capsolver, 2Captcha) |
| 900+ | LLM fallback |

## Fallback Chain

The core loop executes this chain per attempt:

```
1. Detect intent (semantic engine → title heuristics)
2. If intent = normal → return solved
3. Try semantic-first action planning (`/find` + self-healing)
4. If still unresolved, find matching solvers (CanHandle = true)
5. Execute solvers in configured order (`autoSolver.solvers`), falling back to
    priority order when configuration does not match available solvers
6. If all fail AND LLM enabled:
   a. Trim HTML to ~4KB
   b. Build structured prompt with attempt history
   c. Execute LLM-suggested action
7. Retry with exponential backoff (500ms → 10s cap)
8. Stop after MaxAttempts (default: 8)
```

## Configuration

### Config File (`config.json`)

```json
{
  "autoSolver": {
    "enabled": true,
    "autoTrigger": true,
    "triggerOnNavigate": true,
    "triggerOnAction": true,
    "maxAttempts": 8,
    "solverTimeoutSec": 30,
    "retryBaseDelayMs": 500,
    "retryMaxDelayMs": 10000,
    "solvers": ["cloudflare", "semantic", "capsolver", "twocaptcha"],
    "llmProvider": "openai",
    "llmFallback": false,
    "external": {
      "capsolverKey": "CAP-xxx",
      "twoCaptchaKey": "xxx"
    }
  }
}
```

External provider API keys are configured only in `autoSolver.external` in the
config file.

## Extension Guide

### Adding a New Solver

1. Create a new file in `internal/autosolver/solvers/`:

```go
package solvers

type MySolver struct{}

func (s *MySolver) Name() string  { return "myservice" }
func (s *MySolver) Priority() int { return 150 }

func (s *MySolver) CanHandle(ctx context.Context, page autosolver.Page) (bool, error) {
    // Check if this solver can handle the current page
    return strings.Contains(page.Title(), "my-challenge"), nil
}

func (s *MySolver) Solve(ctx context.Context, page autosolver.Page, executor autosolver.ActionExecutor) (*autosolver.Result, error) {
    // Implement solving logic using Page + ActionExecutor
    result := &autosolver.Result{SolverUsed: "myservice"}
    // ...
    return result, nil
}
```

2. Register it with the AutoSolver:

```go
as := autosolver.New(cfg, semanticEngine, nil)
as.Registry().Register(&solvers.MySolver{})
```

### Using with Pinchtab Bridge

```go
// Create Page + Executor from a bridge tab
page, executor, err := adapters.NewFromBridge(bridge, tabID)

// Run the autosolver
as := autosolver.New(autosolver.DefaultConfig(), semanticAdapter, nil)
as.Registry().MustRegister(&solvers.Cloudflare{})

result, err := as.Solve(ctx, page, executor)
if result.Solved {
    log.Printf("Solved by %s in %d attempts", result.SolverUsed, result.Attempts)
}
```

## Comparison with browser-use

| Aspect | browser-use | Pinchtab AutoSolver |
|--------|-------------|-------------------|
| Decision engine | LLM per step | Semantic first, LLM fallback |
| DOM handling | Full DOM/screenshot each step | Trimmed HTML, a11y tree |
| Cost | High (LLM every step) | Low (LLM only on failure) |
| Speed | Slow (LLM latency) | Fast (local semantic matching) |
| Determinism | Low (LLM non-deterministic) | High (rule-based + semantic) |
| Modularity | Monolithic | Interface-driven, pluggable |

## Backward Compatibility

The existing `internal/solver` package (PR #395) is still present for compatibility, but challenge APIs are now backed by `internal/autosolver`. The `CloudflareSolver` in `bridge/cloudflare.go` remains available, and a `LegacyAdapter` shim (`solvers/legacy.go`) can wrap legacy `solver.Solver` implementations into the new `autosolver.Solver` interface.
