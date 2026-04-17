package handlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	coreautosolver "github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/autosolver/adapters"
	"github.com/pinchtab/pinchtab/internal/autosolver/external"
	autosolversemantic "github.com/pinchtab/pinchtab/internal/autosolver/semantic"
	autosolvers "github.com/pinchtab/pinchtab/internal/autosolver/solvers"
)

const (
	autoSolverTriggerNavigate = "navigate"
	autoSolverTriggerAction   = "action"
)

func (h *Handlers) maybeAutoSolve(ctx context.Context, tabID, trigger string) {
	if tabID == "" || h.autoSolverRunner == nil || !h.shouldAutoSolve(trigger) {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if err := h.autoSolverRunner(ctx, tabID); err != nil &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		slog.Warn("autosolver auto-trigger failed",
			"trigger", trigger,
			"tab_id", tabID,
			"error", err)
	}
}

func (h *Handlers) shouldAutoSolve(trigger string) bool {
	if h == nil || h.Config == nil {
		return false
	}

	cfg := h.Config.AutoSolver
	if !cfg.Enabled || !cfg.AutoTrigger {
		return false
	}

	switch trigger {
	case autoSolverTriggerNavigate:
		return cfg.TriggerOnNavigate
	case autoSolverTriggerAction:
		return cfg.TriggerOnAction
	default:
		return false
	}
}

func (h *Handlers) runAutoSolver(ctx context.Context, tabID string) error {
	if h == nil || h.Config == nil || h.Bridge == nil {
		return nil
	}

	page, executor, err := adapters.NewFromBridge(h.Bridge, tabID)
	if err != nil {
		return err
	}

	html, err := page.HTML()
	if err != nil {
		return err
	}

	// Auto-trigger focuses on challenge resolution and should be a no-op on normal pages.
	if coreautosolver.DetectChallengeIntent(page.Title(), page.URL(), html) == nil {
		return nil
	}

	cfg := h.normalizedAutoSolverConfig()
	as := h.buildAutoSolver(cfg, true)

	solveCtx, cancel := context.WithTimeout(ctx, estimateAutoSolverRunTimeout(cfg))
	defer cancel()

	result, err := as.Solve(solveCtx, page, executor)
	if err != nil {
		return err
	}

	if result != nil {
		if result.Solved && result.Attempts > 0 {
			slog.Info("autosolver auto-trigger solved challenge",
				"tab_id", tabID,
				"solver", result.SolverUsed,
				"attempts", result.Attempts)
		} else if !result.Solved && result.Attempts > 0 {
			slog.Warn("autosolver auto-trigger did not solve challenge",
				"tab_id", tabID,
				"attempts", result.Attempts,
				"error", result.Error)
		}
	}

	return nil
}

func (h *Handlers) normalizedAutoSolverConfig() coreautosolver.Config {
	cfg := coreautosolver.DefaultConfig()
	if h == nil || h.Config == nil {
		return cfg
	}

	cfg.Enabled = h.Config.AutoSolver.Enabled
	if h.Config.AutoSolver.MaxAttempts > 0 {
		cfg.MaxAttempts = h.Config.AutoSolver.MaxAttempts
	}
	if h.Config.AutoSolver.SolverTimeoutSec > 0 {
		cfg.SolverTimeout = time.Duration(h.Config.AutoSolver.SolverTimeoutSec) * time.Second
	}
	if h.Config.AutoSolver.RetryBaseDelayMs >= 0 {
		cfg.RetryBaseDelay = time.Duration(h.Config.AutoSolver.RetryBaseDelayMs) * time.Millisecond
	}
	if h.Config.AutoSolver.RetryMaxDelayMs >= 0 {
		cfg.RetryMaxDelay = time.Duration(h.Config.AutoSolver.RetryMaxDelayMs) * time.Millisecond
	}
	if len(h.Config.AutoSolver.Solvers) > 0 {
		configured := make([]string, 0, len(h.Config.AutoSolver.Solvers))
		for _, name := range h.Config.AutoSolver.Solvers {
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				configured = append(configured, trimmed)
			}
		}
		if len(configured) > 0 {
			cfg.Solvers = configured
		}
	}
	cfg.LLMFallback = h.Config.AutoSolver.LLMFallback

	return cfg
}

func (h *Handlers) buildAutoSolver(cfg coreautosolver.Config, includeSemantic bool) *coreautosolver.AutoSolver {
	var semanticEngine coreautosolver.SemanticEngine
	if includeSemantic {
		semanticEngine = autosolversemantic.NewAdapter(h.Matcher)
	}

	as := coreautosolver.New(cfg, semanticEngine, nil)
	as.Registry().MustRegister(&autosolvers.Cloudflare{})
	as.Registry().MustRegister(&autosolvers.JSChallenge{})

	if h != nil && h.Config != nil {
		if key := strings.TrimSpace(h.Config.AutoSolver.CapsolverKey); key != "" {
			as.Registry().MustRegister(external.NewCapsolver(external.CapsolverConfig{APIKey: key}))
		}
		if key := strings.TrimSpace(h.Config.AutoSolver.TwoCaptchaKey); key != "" {
			as.Registry().MustRegister(external.NewTwoCaptcha(external.TwoCaptchaConfig{APIKey: key}))
		}
	}

	return as
}

func (h *Handlers) availableAutoSolverNames() []string {
	cfg := h.normalizedAutoSolverConfig()
	available := map[string]bool{
		"cloudflare":  true,
		"semantic":    true,
		"jschallenge": true,
	}
	if h != nil && h.Config != nil {
		if strings.TrimSpace(h.Config.AutoSolver.CapsolverKey) != "" {
			available["capsolver"] = true
		}
		if strings.TrimSpace(h.Config.AutoSolver.TwoCaptchaKey) != "" {
			available["twocaptcha"] = true
		}
	}

	names := make([]string, 0, len(available))
	seen := make(map[string]struct{}, len(available))
	for _, configured := range cfg.Solvers {
		if !available[configured] {
			continue
		}
		if _, ok := seen[configured]; ok {
			continue
		}
		names = append(names, configured)
		seen[configured] = struct{}{}
	}

	for _, fallback := range []string{"cloudflare", "semantic", "jschallenge", "capsolver", "twocaptcha"} {
		if !available[fallback] {
			continue
		}
		if _, ok := seen[fallback]; ok {
			continue
		}
		names = append(names, fallback)
		seen[fallback] = struct{}{}
	}

	return names
}

func (h *Handlers) isAvailableAutoSolver(name string) bool {
	for _, n := range h.availableAutoSolverNames() {
		if n == name {
			return true
		}
	}
	return false
}

func estimateAutoSolverRunTimeout(cfg coreautosolver.Config) time.Duration {
	attempts := cfg.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	timeout := time.Duration(attempts) * cfg.SolverTimeout
	if attempts > 1 {
		timeout += time.Duration(attempts-1) * cfg.RetryMaxDelay
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return timeout + 2*time.Second
}
