// Package autosolver provides a modular, interface-driven system for
// automated browser challenge solving and multi-step web automation.
//
// The system uses a semantic-first approach: the SemanticEngine handles
// page understanding and element discovery without LLM calls. LLM is
// used only as a last-resort fallback when all other strategies fail.
//
// All browser interactions go through the Page and ActionExecutor
// interfaces, ensuring the core autosolver logic has zero coupling
// to any specific browser runtime (chromedp, playwright, etc.).
package autosolver

import (
	"context"
	"time"
)

// Page provides a read-only view of the current browser page.
// Implementations bridge to the actual browser runtime without
// exposing runtime-specific types.
type Page interface {
	// URL returns the current page URL.
	URL() string

	// Title returns the current page title.
	Title() string

	// HTML returns the outer HTML of the page, trimmed to reduce size.
	// Implementations should strip unnecessary whitespace and comments.
	HTML() (string, error)

	// HTMLWithin is HTML() bounded by timeout (0 = no bound). Implementations
	// must cancel the underlying fetch when the deadline fires so callers do not
	// leak workers under a stalled page.
	HTMLWithin(timeout time.Duration) (string, error)

	// Screenshot captures the current page as a PNG image.
	Screenshot() ([]byte, error)
}

// ActionExecutor performs browser actions on the current page.
// Each method blocks until the action completes or the context expires.
type ActionExecutor interface {
	// Click performs a human-like click at the given coordinates.
	Click(ctx context.Context, x, y float64) error

	// Type enters text with human-like keystroke timing.
	Type(ctx context.Context, text string) error

	// WaitFor waits until a CSS selector matches an element or timeout.
	WaitFor(ctx context.Context, selector string, timeout time.Duration) error

	// Evaluate executes JavaScript and unmarshals the result.
	Evaluate(ctx context.Context, expr string, result interface{}) error

	// Navigate loads a URL and waits for the page to settle.
	Navigate(ctx context.Context, url string) error
}

// Solver handles a specific class of browser challenge or automation task.
// Solvers are registered with a Registry and selected based on CanHandle.
type Solver interface {
	// Name returns a unique identifier (e.g., "cloudflare", "capsolver").
	Name() string

	// Priority determines execution order; lower values run first.
	// Built-in solvers use 0-99, semantic 100-199, external 200-299, LLM 900+.
	Priority() int

	// CanHandle reports whether this solver can address the current page.
	// Must be lightweight (title/URL checks) with no side-effects.
	CanHandle(ctx context.Context, page Page) (bool, error)

	// Solve attempts to resolve the challenge or complete the task.
	Solve(ctx context.Context, page Page, executor ActionExecutor) (*Result, error)
}

// SemanticEngine provides AI-driven page understanding using structured
// matching instead of LLM inference. This is the primary intelligence
// layer for the autosolver system.
type SemanticEngine interface {
	// DetectIntent classifies the current page state (challenge, login,
	// signup, navigation block, normal content).
	DetectIntent(ctx context.Context, page Page) (*Intent, error)

	// FindElement locates a UI element by natural-language description,
	// returning the best match with confidence score.
	FindElement(ctx context.Context, page Page, query string) (*ElementMatch, error)

	// SuggestAction determines the next action to take based on the
	// current page state and detected intent.
	SuggestAction(ctx context.Context, page Page, intent *Intent) (*SuggestedAction, error)
}

// LLMProvider is a last-resort fallback that uses a language model
// to determine the next action when all other strategies fail.
// Implementations should minimize token usage.
type LLMProvider interface {
	// SuggestNextAction asks the LLM for the next action given page context.
	SuggestNextAction(ctx context.Context, req LLMRequest) (*LLMResponse, error)
}
