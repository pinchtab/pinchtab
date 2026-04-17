package autosolver

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// AutoSolver orchestrates the challenge-detection and solving pipeline.
// It uses a fallback chain: semantic engine (/find + self-healing) ->
// rule-based solvers -> external solvers -> LLM provider.
type AutoSolver struct {
	registry *Registry
	semantic SemanticEngine
	llm      LLMProvider
	config   Config
}

// New creates an AutoSolver with the given configuration.
// The semantic engine and LLM provider are optional (can be nil).
func New(cfg Config, semantic SemanticEngine, llm LLMProvider) *AutoSolver {
	return &AutoSolver{
		registry: NewRegistry(),
		semantic: semantic,
		llm:      llm,
		config:   cfg,
	}
}

// Registry returns the solver registry for external registration.
func (as *AutoSolver) Registry() *Registry {
	return as.registry
}

// Solve runs the autosolver pipeline on the current page.
//
// Steps:
//  1. Detect intent via semantic engine (or title-based heuristics)
//  2. If no challenge detected, return immediately
//  3. Try semantic-first action (/find + self-healing)
//  4. If semantic fails, try matching solvers in priority order
//  5. If all fail and LLM is enabled, try LLM fallback
//  6. Return result with full attempt history
func (as *AutoSolver) Solve(ctx context.Context, page Page, executor ActionExecutor) (*Result, error) {
	start := time.Now()
	result := &Result{
		FinalTitle: page.Title(),
		FinalURL:   page.URL(),
	}

	slog.Info("autosolver_start",
		"url", page.URL(),
		"title", page.Title(),
		"max_attempts", as.config.MaxAttempts,
		"llm_fallback", as.config.LLMFallback)

	// Detect what kind of page we're dealing with.
	intent, err := as.detectIntent(ctx, page)
	if err != nil {
		slog.Warn("autosolver: intent detection failed, proceeding with unknown",
			"err", err, "url", page.URL())
		intent = &Intent{Type: IntentUnknown, Confidence: 0}
	}
	result.Intent = intent.Type

	// No challenge - nothing to solve.
	if intent.Type == IntentNormal {
		result.Solved = true
		result.TotalDuration = time.Since(start)
		slog.Info("autosolver_done",
			"solved", true,
			"reason", "no_challenge_detected",
			"url", page.URL(),
			"duration_ms", result.TotalDuration.Milliseconds())
		return result, nil
	}

	slog.Info("autosolver: challenge detected",
		"type", intent.Type,
		"confidence", intent.Confidence,
		"url", page.URL())

	// Run the fallback chain with retry logic.
	for attempt := 0; attempt < as.config.MaxAttempts; attempt++ {
		result.Attempts = attempt + 1

		// Apply backoff between retries (skip first attempt).
		if attempt > 0 {
			delay := as.backoffDelay(attempt)
			slog.Info("autosolver_retry",
				"attempt", attempt+1,
				"delay_ms", delay.Milliseconds(),
				"url", page.URL())
			select {
			case <-ctx.Done():
				result.TotalDuration = time.Since(start)
				result.Error = ctx.Err().Error()
				slog.Warn("autosolver_done",
					"solved", false,
					"reason", "context_cancelled",
					"attempts", result.Attempts,
					"duration_ms", result.TotalDuration.Milliseconds())
				return result, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Try semantic-first action before any rule-based solver.
		solved, entry := as.trySemantic(ctx, page, executor, intent)
		if entry != nil {
			result.History = append(result.History, *entry)
		}
		if solved {
			result.Solved = true
			result.SolverUsed = entry.Solver
			result.FinalTitle = page.Title()
			result.FinalURL = page.URL()
			result.TotalDuration = time.Since(start)
			slog.Info("autosolver_success",
				"solver", entry.Solver,
				"attempts", result.Attempts,
				"duration_ms", result.TotalDuration.Milliseconds(),
				"url", page.URL())
			slog.Info("autosolver_done",
				"solved", true,
				"solver", entry.Solver,
				"attempts", result.Attempts,
				"duration_ms", result.TotalDuration.Milliseconds())
			return result, nil
		}

		// Try registered solvers in priority order.
		solved, entry = as.trySolvers(ctx, page, executor)
		if entry != nil {
			result.History = append(result.History, *entry)
		}
		if solved {
			result.Solved = true
			result.SolverUsed = entry.Solver
			result.FinalTitle = page.Title()
			result.FinalURL = page.URL()
			result.TotalDuration = time.Since(start)
			slog.Info("autosolver_success",
				"solver", entry.Solver,
				"attempts", result.Attempts,
				"duration_ms", result.TotalDuration.Milliseconds(),
				"url", page.URL())
			slog.Info("autosolver_done",
				"solved", true,
				"solver", entry.Solver,
				"attempts", result.Attempts,
				"duration_ms", result.TotalDuration.Milliseconds())
			return result, nil
		}

		// Try LLM fallback if enabled and all solvers failed.
		if as.config.LLMFallback && as.llm != nil {
			solved, entry = as.tryLLM(ctx, page, executor, result.History)
			if entry != nil {
				result.History = append(result.History, *entry)
			}
			if solved {
				result.Solved = true
				result.SolverUsed = "llm"
				result.FinalTitle = page.Title()
				result.FinalURL = page.URL()
				result.TotalDuration = time.Since(start)
				slog.Info("autosolver_success",
					"solver", "llm",
					"attempts", result.Attempts,
					"duration_ms", result.TotalDuration.Milliseconds(),
					"url", page.URL())
				slog.Info("autosolver_done",
					"solved", true,
					"solver", "llm",
					"attempts", result.Attempts,
					"duration_ms", result.TotalDuration.Milliseconds())
				return result, nil
			}
		}
	}

	result.TotalDuration = time.Since(start)
	result.Error = fmt.Sprintf("all %d attempts exhausted", as.config.MaxAttempts)
	slog.Warn("autosolver_failure",
		"attempts", result.Attempts,
		"duration_ms", result.TotalDuration.Milliseconds(),
		"url", page.URL(),
		"error", result.Error)
	slog.Info("autosolver_done",
		"solved", false,
		"reason", "max_attempts_exhausted",
		"attempts", result.Attempts,
		"duration_ms", result.TotalDuration.Milliseconds())
	return result, nil
}

// detectIntent uses the semantic engine if available, otherwise falls
// back to basic title-based heuristics.
func (as *AutoSolver) detectIntent(ctx context.Context, page Page) (*Intent, error) {
	if as.semantic != nil {
		return as.semantic.DetectIntent(ctx, page)
	}
	return detectIntentByTitle(page.Title()), nil
}

// trySolvers iterates through matching solvers and returns on first success.
func (as *AutoSolver) trySolvers(ctx context.Context, page Page, executor ActionExecutor) (bool, *AttemptEntry) {
	solvers := as.registry.MatchingSolvers(ctx, page)
	if len(solvers) == 0 {
		return false, &AttemptEntry{
			Solver: "none",
			Status: StatusSkipped,
		}
	}

	orderedSolvers := solvers
	if len(as.config.Solvers) > 0 {
		byName := make(map[string]Solver, len(solvers))
		for _, s := range solvers {
			byName[s.Name()] = s
		}

		filtered := make([]Solver, 0, len(as.config.Solvers))
		for _, name := range as.config.Solvers {
			if s, ok := byName[name]; ok {
				filtered = append(filtered, s)
			}
		}

		// If config names don't match available solvers, preserve default behavior.
		if len(filtered) > 0 {
			orderedSolvers = filtered
		}
	}

	for _, s := range orderedSolvers {
		solverCtx, cancel := context.WithTimeout(ctx, as.config.SolverTimeout)
		solverStart := time.Now()

		slog.Info("autosolver_attempt",
			"solver", s.Name(),
			"priority", s.Priority())

		solveResult, err := s.Solve(solverCtx, page, executor)
		cancel()

		entry := &AttemptEntry{
			Solver:   s.Name(),
			Duration: time.Since(solverStart),
		}

		if err != nil {
			entry.Status = StatusFailed
			entry.Error = err.Error()
			slog.Warn("autosolver_failure",
				"solver", s.Name(),
				"error", err,
				"duration_ms", entry.Duration.Milliseconds())
			continue
		}

		if solveResult != nil && solveResult.Solved {
			entry.Status = StatusSolved
			return true, entry
		}

		entry.Status = StatusFailed
		if solveResult != nil && solveResult.Error != "" {
			entry.Error = solveResult.Error
		}
		slog.Debug("autosolver: solver returned not-solved",
			"solver", s.Name(),
			"duration_ms", entry.Duration.Milliseconds())
	}

	return false, &AttemptEntry{
		Solver: orderedSolvers[len(orderedSolvers)-1].Name(),
		Status: StatusFailed,
		Error:  "all matching solvers failed",
	}
}

// trySemantic executes semantic /find-driven action planning first.
// For high-level intents it runs a small multi-step semantic flow.
func (as *AutoSolver) trySemantic(ctx context.Context, page Page, executor ActionExecutor, intent *Intent) (bool, *AttemptEntry) {
	entry := &AttemptEntry{Solver: "semantic"}
	semanticStart := time.Now()

	if as.semantic == nil {
		entry.Status = StatusSkipped
		entry.Error = "semantic engine not configured"
		entry.Duration = time.Since(semanticStart)
		return false, entry
	}

	semanticCtx, cancel := context.WithTimeout(ctx, as.config.SolverTimeout)
	defer cancel()

	initialIntentType := intentTypeOf(intent)
	stepBudget := semanticStepBudget(initialIntentType)
	if stepBudget < 1 {
		stepBudget = 1
	}

	currentIntent := intent
	actionsExecuted := 0

	for step := 0; step < stepBudget; step++ {
		if step > 0 {
			nextIntent, detectErr := as.detectIntent(semanticCtx, page)
			if detectErr != nil {
				slog.Debug("autosolver: semantic step intent refresh failed",
					"step", step+1,
					"error", detectErr)
			} else {
				currentIntent = nextIntent
			}
		}

		if intentTypeOf(currentIntent) == IntentNormal {
			entry.Status = StatusSolved
			entry.Duration = time.Since(semanticStart)
			return true, entry
		}

		suggested, err := as.semantic.SuggestAction(semanticCtx, page, currentIntent)
		if err != nil {
			entry.Status = StatusFailed
			entry.Error = fmt.Sprintf("semantic suggest action: %v", err)
			entry.Duration = time.Since(semanticStart)
			return false, entry
		}

		planned := as.planSemanticAction(currentIntent, step, suggested)
		action, err := as.prepareSemanticAction(semanticCtx, page, currentIntent, step, planned)
		if err != nil {
			entry.Status = StatusFailed
			entry.Error = fmt.Sprintf("prepare semantic action: %v", err)
			entry.Duration = time.Since(semanticStart)
			return false, entry
		}

		if err := executeSuggestedAction(semanticCtx, executor, action); err != nil {
			healedAction, healErr := as.selfHealSemanticAction(semanticCtx, page, currentIntent, step, action)
			if healErr != nil {
				entry.Status = StatusFailed
				entry.Error = fmt.Sprintf("execute semantic action: %v; self-heal failed: %v", err, healErr)
				entry.Duration = time.Since(semanticStart)
				return false, entry
			}

			if err := executeSuggestedAction(semanticCtx, executor, healedAction); err != nil {
				entry.Status = StatusFailed
				entry.Error = fmt.Sprintf("execute semantic self-heal action: %v", err)
				entry.Duration = time.Since(semanticStart)
				return false, entry
			}
		}

		actionsExecuted++

		postIntent, detectErr := as.detectIntent(semanticCtx, page)
		if detectErr != nil {
			slog.Debug("autosolver: semantic post-step intent detection failed",
				"step", step+1,
				"error", detectErr)
		} else {
			currentIntent = postIntent
			if currentIntent.Type == IntentNormal {
				entry.Status = StatusSolved
				entry.Duration = time.Since(semanticStart)
				return true, entry
			}
		}
	}

	if isHighLevelIntent(initialIntentType) && actionsExecuted > 0 {
		entry.Status = StatusSolved
		entry.Duration = time.Since(semanticStart)
		return true, entry
	}

	entry.Status = StatusFailed
	entry.Error = fmt.Sprintf("semantic flow exhausted for intent %q", initialIntentType)
	entry.Duration = time.Since(semanticStart)
	return false, entry
}

type semanticFlowStep struct {
	Query   string
	Action  ActionType
	EnvKeys []string
}

func (as *AutoSolver) planSemanticAction(intent *Intent, step int, suggested *SuggestedAction) *SuggestedAction {
	planned := &SuggestedAction{Action: ActionNone}
	if suggested != nil {
		copy := *suggested
		planned = &copy
	}

	intentType := intentTypeOf(intent)
	flowStep := semanticFlowStepForIntent(intentType, step)

	if planned.Action == ActionNone || isHighLevelIntent(intentType) {
		planned.Action = flowStep.Action
	}

	if planned.Text == "" && planned.Action == ActionType_ {
		planned.Text = firstNonEmptyEnv(flowStep.EnvKeys...)
	}

	if planned.Reason == "" {
		planned.Reason = fmt.Sprintf("semantic flow step %d", step+1)
	}

	return planned
}

func (as *AutoSolver) prepareSemanticAction(ctx context.Context, page Page, intent *Intent, step int, action *SuggestedAction) (*SuggestedAction, error) {
	if action == nil {
		return nil, fmt.Errorf("nil action")
	}

	resolved := *action
	flowStep := semanticFlowStepForIntent(intentTypeOf(intent), step)

	shouldResolveTarget := isHighLevelIntent(intentTypeOf(intent)) || actionNeedsTarget(&resolved)
	if shouldResolveTarget {
		match, err := as.semantic.FindElement(ctx, page, flowStep.Query)
		if err != nil {
			return nil, fmt.Errorf("semantic find element query %q: %w", flowStep.Query, err)
		}
		if match != nil {
			if match.Selector != "" {
				resolved.Selector = match.Selector
			} else if match.Ref != "" {
				resolved.Selector = match.Ref
			}
			if match.X != 0 || match.Y != 0 {
				resolved.X = match.X
				resolved.Y = match.Y
			}
		} else if actionNeedsTarget(&resolved) {
			return nil, fmt.Errorf("semantic find returned no match for query %q", flowStep.Query)
		}
	}

	if resolved.Action == ActionType_ && resolved.Text == "" {
		resolved.Text = firstNonEmptyEnv(flowStep.EnvKeys...)
		if resolved.Text == "" {
			resolved.Action = ActionClick
		}
	}

	if resolved.Action == ActionClick && resolved.Selector == "" && resolved.X == 0 && resolved.Y == 0 {
		return nil, fmt.Errorf("semantic action requires selector or coordinates for query %q", flowStep.Query)
	}

	return &resolved, nil
}

func (as *AutoSolver) selfHealSemanticAction(ctx context.Context, page Page, intent *Intent, step int, original *SuggestedAction) (*SuggestedAction, error) {
	if original == nil {
		return nil, fmt.Errorf("nil action")
	}

	flowStep := semanticFlowStepForIntent(intentTypeOf(intent), step)
	match, err := as.semantic.FindElement(ctx, page, flowStep.Query)
	if err != nil {
		return nil, fmt.Errorf("semantic self-heal find query %q: %w", flowStep.Query, err)
	}
	if match == nil {
		return nil, fmt.Errorf("semantic self-heal returned no match for query %q", flowStep.Query)
	}

	healed := *original
	if match.Selector != "" {
		healed.Selector = match.Selector
	} else if match.Ref != "" {
		healed.Selector = match.Ref
	}
	if match.X != 0 || match.Y != 0 {
		healed.X = match.X
		healed.Y = match.Y
	}

	if healed.Action == ActionType_ && healed.Text == "" {
		healed.Text = firstNonEmptyEnv(flowStep.EnvKeys...)
		if healed.Text == "" {
			healed.Action = ActionClick
		}
	}

	if healed.Action == ActionClick && healed.Selector == "" && healed.X == 0 && healed.Y == 0 {
		return nil, fmt.Errorf("semantic self-heal match for query %q had no actionable selector or coordinates", flowStep.Query)
	}

	return &healed, nil
}

func intentTypeOf(intent *Intent) IntentType {
	if intent == nil {
		return IntentUnknown
	}
	return intent.Type
}

func isHighLevelIntent(intentType IntentType) bool {
	switch intentType {
	case IntentLogin, IntentSignup, IntentForm, IntentOnboarding, IntentNavigation:
		return true
	default:
		return false
	}
}

func semanticStepBudget(intentType IntentType) int {
	switch intentType {
	case IntentLogin:
		return 3
	case IntentSignup:
		return 4
	case IntentForm:
		return 3
	case IntentOnboarding, IntentNavigation:
		return 3
	case IntentCaptcha, IntentBlocked:
		return 2
	default:
		return 1
	}
}

func semanticFlowStepForIntent(intentType IntentType, step int) semanticFlowStep {
	steps := []semanticFlowStep{{Query: "primary continue submit button", Action: ActionClick}}

	switch intentType {
	case IntentCaptcha:
		steps = []semanticFlowStep{
			{Query: "captcha checkbox verify button challenge widget", Action: ActionClick},
			{Query: "verification challenge status text", Action: ActionWait},
		}
	case IntentBlocked:
		steps = []semanticFlowStep{
			{Query: "verify continue button", Action: ActionClick},
			{Query: "body", Action: ActionWait},
		}
	case IntentLogin:
		steps = []semanticFlowStep{
			{Query: "username email input field", Action: ActionType_, EnvKeys: []string{"PINCHTAB_AUTOSOLVER_LOGIN_USER", "PINCHTAB_AUTOSOLVER_LOGIN_EMAIL"}},
			{Query: "password input field", Action: ActionType_, EnvKeys: []string{"PINCHTAB_AUTOSOLVER_LOGIN_PASS", "PINCHTAB_AUTOSOLVER_LOGIN_PASSWORD"}},
			{Query: "login submit sign in button", Action: ActionClick},
		}
	case IntentSignup:
		steps = []semanticFlowStep{
			{Query: "name full name input field", Action: ActionType_, EnvKeys: []string{"PINCHTAB_AUTOSOLVER_SIGNUP_NAME"}},
			{Query: "email input field", Action: ActionType_, EnvKeys: []string{"PINCHTAB_AUTOSOLVER_SIGNUP_EMAIL"}},
			{Query: "password create password input field", Action: ActionType_, EnvKeys: []string{"PINCHTAB_AUTOSOLVER_SIGNUP_PASSWORD"}},
			{Query: "sign up register create account submit button", Action: ActionClick},
		}
	case IntentForm:
		steps = []semanticFlowStep{
			{Query: "first required input field", Action: ActionType_, EnvKeys: []string{"PINCHTAB_AUTOSOLVER_FORM_FIELD1"}},
			{Query: "second required input field", Action: ActionType_, EnvKeys: []string{"PINCHTAB_AUTOSOLVER_FORM_FIELD2", "PINCHTAB_AUTOSOLVER_FORM_EMAIL"}},
			{Query: "primary submit button", Action: ActionClick},
		}
	case IntentOnboarding:
		steps = []semanticFlowStep{
			{Query: "next continue button", Action: ActionClick},
			{Query: "skip button", Action: ActionClick},
			{Query: "done finish submit button", Action: ActionClick},
		}
	case IntentNavigation:
		steps = []semanticFlowStep{
			{Query: "primary navigation link", Action: ActionClick},
			{Query: "continue next button", Action: ActionClick},
			{Query: "submit confirm button", Action: ActionClick},
		}
	}

	if step < 0 {
		step = 0
	}
	if step >= len(steps) {
		step = len(steps) - 1
	}

	return steps[step]
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func actionNeedsTarget(action *SuggestedAction) bool {
	if action == nil {
		return false
	}

	switch action.Action {
	case ActionClick:
		return action.Selector == "" && action.X == 0 && action.Y == 0
	case ActionType_:
		return action.Selector == "" && action.X == 0 && action.Y == 0
	default:
		return false
	}
}

func executeSuggestedAction(ctx context.Context, executor ActionExecutor, action *SuggestedAction) error {
	if action == nil {
		return fmt.Errorf("nil action")
	}

	switch action.Action {
	case ActionClick:
		if action.Selector != "" {
			x, y, err := resolveSelectorCenter(ctx, executor, action.Selector)
			if err != nil {
				return err
			}
			return executor.Click(ctx, x, y)
		}
		if action.X != 0 || action.Y != 0 {
			return executor.Click(ctx, action.X, action.Y)
		}
		return fmt.Errorf("click action requires selector or coordinates")

	case ActionType_:
		if action.Selector != "" {
			x, y, err := resolveSelectorCenter(ctx, executor, action.Selector)
			if err != nil {
				return err
			}
			if err := executor.Click(ctx, x, y); err != nil {
				return err
			}
		} else if action.X != 0 || action.Y != 0 {
			if err := executor.Click(ctx, action.X, action.Y); err != nil {
				return err
			}
		}
		return executor.Type(ctx, action.Text)

	case ActionNavigate:
		return executor.Navigate(ctx, action.URL)

	case ActionWait:
		selector := action.Selector
		if selector == "" {
			selector = "body"
		}
		return executor.WaitFor(ctx, selector, 5*time.Second)

	case ActionEvaluate:
		if action.Expr == "" {
			return fmt.Errorf("evaluate action requires expr")
		}
		var out interface{}
		return executor.Evaluate(ctx, action.Expr, &out)

	case ActionNone:
		return nil

	default:
		return fmt.Errorf("unsupported action: %s", action.Action)
	}
}

func resolveSelectorCenter(ctx context.Context, executor ActionExecutor, selector string) (float64, float64, error) {
	var coords struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}

	expr := fmt.Sprintf(`(() => {
		const el = document.querySelector(%q);
		if (!el) return null;
		const r = el.getBoundingClientRect();
		return {x: r.x + r.width/2, y: r.y + r.height/2};
	})()`, selector)

	if err := executor.Evaluate(ctx, expr, &coords); err != nil {
		return 0, 0, fmt.Errorf("resolve selector %q: %w", selector, err)
	}

	return coords.X, coords.Y, nil
}

// tryLLM builds a trimmed request and asks the LLM for the next action.
func (as *AutoSolver) tryLLM(ctx context.Context, page Page, executor ActionExecutor, history []AttemptEntry) (bool, *AttemptEntry) {
	llmStart := time.Now()
	entry := &AttemptEntry{Solver: "llm"}

	html, err := page.HTML()
	if err != nil {
		entry.Status = StatusFailed
		entry.Error = fmt.Sprintf("get HTML: %v", err)
		entry.Duration = time.Since(llmStart)
		return false, entry
	}

	// Trim HTML to reduce token usage (max ~4000 chars).
	if len(html) > 4000 {
		html = html[:4000]
	}

	resp, err := as.llm.SuggestNextAction(ctx, LLMRequest{
		PageTitle:    page.Title(),
		PageURL:      page.URL(),
		TrimmedHTML:  html,
		DetectedType: IntentUnknown,
		PrevAttempts: history,
	})
	if err != nil {
		entry.Status = StatusFailed
		entry.Error = fmt.Sprintf("llm: %v", err)
		entry.Duration = time.Since(llmStart)
		return false, entry
	}

	// Execute the LLM's suggested action.
	if err := executeAction(ctx, executor, resp); err != nil {
		entry.Status = StatusFailed
		entry.Error = fmt.Sprintf("execute llm action: %v", err)
		entry.Duration = time.Since(llmStart)
		return false, entry
	}

	entry.Status = StatusSolved
	entry.Duration = time.Since(llmStart)
	return true, entry
}

// executeAction translates an LLMResponse into an ActionExecutor call.
func executeAction(ctx context.Context, executor ActionExecutor, resp *LLMResponse) error {
	if resp == nil {
		return fmt.Errorf("nil response")
	}

	switch resp.Action {
	case ActionClick:
		if resp.Selector != "" {
			x, y, err := resolveSelectorCenter(ctx, executor, resp.Selector)
			if err != nil {
				return err
			}
			return executor.Click(ctx, x, y)
		}
		return fmt.Errorf("click action requires selector")

	case ActionType_:
		return executor.Type(ctx, resp.Text)

	case ActionNavigate:
		return executor.Navigate(ctx, resp.URL)

	case ActionNone:
		return nil

	default:
		return fmt.Errorf("unsupported action: %s", resp.Action)
	}
}

// backoffDelay calculates exponential backoff with jitter.
func (as *AutoSolver) backoffDelay(attempt int) time.Duration {
	base := as.config.RetryBaseDelay
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	maxDelay := as.config.RetryMaxDelay
	if maxDelay <= 0 {
		maxDelay = 10 * time.Second
	}

	delay := base * time.Duration(1<<uint(attempt-1))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}
