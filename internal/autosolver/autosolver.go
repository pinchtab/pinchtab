package autosolver

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/htmltrim"
)

// AutoSolver orchestrates the challenge-detection and solving pipeline.
// It uses a fallback chain: semantic engine (/find + self-healing) ->
// rule-based solvers -> external solvers -> LLM provider.
type AutoSolver struct {
	registry *Registry
	semantic SemanticEngine
	llm      LLMProvider
	config   Config

	warnedMissingKey sync.Map
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

	intent, err := as.detectIntent(ctx, page)
	if err != nil {
		slog.Warn("autosolver: intent detection failed, proceeding with unknown",
			"err", err, "url", page.URL())
		intent = &Intent{Type: IntentUnknown, Confidence: 0}
	}
	// A SemanticEngine may report (nil, nil): no error, no reading. That is the
	// same "we know nothing" the error branch above substitutes for, so it gets
	// the same substitution here rather than a nil check at every later read.
	if intent == nil {
		intent = &Intent{Type: IntentUnknown, Confidence: 0}
	}
	result.Intent = intentTypeOf(intent)

	if intentTypeOf(intent) == IntentNormal {
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
		"type", intentTypeOf(intent),
		"confidence", intent.Confidence,
		"url", page.URL())

	for attempt := 0; attempt < as.config.MaxAttempts; attempt++ {
		result.Attempts = attempt + 1

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

		solved, entry := as.trySemantic(ctx, page, executor, intent)
		appendAttempt(result, entry)
		if solved {
			return as.finalizeSuccess(result, page, entry.Solver, start), nil
		}

		solved, entries := as.trySolvers(ctx, page, executor)
		result.History = append(result.History, entries...)
		if solved {
			return as.finalizeSuccess(result, page, entries[len(entries)-1].Solver, start), nil
		}

		if as.config.LLMFallback && as.llm != nil {
			solved, entry = as.tryLLM(ctx, page, executor, result.History)
			appendAttempt(result, entry)
			if solved {
				return as.finalizeSuccess(result, page, llmFallbackSolverLabel, start), nil
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

func appendAttempt(result *Result, entry *AttemptEntry) {
	if entry != nil {
		result.History = append(result.History, *entry)
	}
}

// finalizeSuccess records a solved result, logs success + done, and returns it.
func (as *AutoSolver) finalizeSuccess(result *Result, page Page, solver string, start time.Time) *Result {
	result.Solved = true
	result.SolverUsed = solver
	result.FinalTitle = page.Title()
	result.FinalURL = page.URL()
	result.TotalDuration = time.Since(start)
	slog.Info("autosolver_success",
		"solver", solver,
		"attempts", result.Attempts,
		"duration_ms", result.TotalDuration.Milliseconds(),
		"url", page.URL())
	slog.Info("autosolver_done",
		"solved", true,
		"solver", solver,
		"attempts", result.Attempts,
		"duration_ms", result.TotalDuration.Milliseconds())
	return result
}

func (as *AutoSolver) detectIntent(ctx context.Context, page Page) (*Intent, error) {
	if as.semantic != nil {
		return as.semantic.DetectIntent(ctx, page)
	}
	return detectIntentByTitle(page.Title()), nil
}

func (as *AutoSolver) orderSolvers(matching []Solver) []Solver {
	if len(as.config.Solvers) == 0 {
		return matching
	}

	byName := make(map[string]Solver, len(matching))
	for _, s := range matching {
		byName[s.Name()] = s
	}

	filtered := make([]Solver, 0, len(as.config.Solvers))
	missing := make([]string, 0, len(as.config.Solvers))
	for _, name := range as.config.Solvers {
		if s, ok := byName[name]; ok {
			filtered = append(filtered, s)
			continue
		}
		missing = append(missing, name)
	}

	if len(filtered) == 0 {
		available := make([]string, 0, len(byName))
		for name := range byName {
			available = append(available, name)
		}
		sort.Strings(available)
		// Warn, not debug: every configured solver is unavailable, so the run
		// silently uses solvers the operator never listed. Refusing to solve
		// would be the worse failure, so the fallback stays — but it stops
		// being invisible at the default log level.
		slog.Warn("autosolver: no configured solver is available, falling back to priority order",
			"configured", as.config.Solvers,
			"unavailable", missing,
			"running", available)
		return matching
	}

	as.reportMissingSolvers(missing, filtered)
	return filtered
}

// reportMissingSolvers splits the configured-but-unavailable names by whether the
// operator can act on them. Only an unset API key is actionable, and the names
// here are absent from THIS page's matching set, which a working solver is too
// whenever it does not handle this challenge — so the registry decides, never the
// caller's slice.
func (as *AutoSolver) reportMissingSolvers(missing []string, running []Solver) {
	if len(missing) == 0 {
		return
	}

	unactionable := make([]string, 0, len(missing))
	for _, name := range missing {
		gated, ok := KeyGatedSolverNamed(name)
		if !ok || as.isRegistered(name) {
			unactionable = append(unactionable, name)
			continue
		}
		if _, warned := as.warnedMissingKey.LoadOrStore(name, struct{}{}); warned {
			continue
		}
		if gated.Planned {
			slog.Warn(fmt.Sprintf("autosolver: %s is configured but not implemented yet, so it never runs", gated.Name),
				"solver", gated.Name,
				"configured", as.config.Solvers,
				"running", solverNamesOf(running))
			continue
		}
		slog.Warn(fmt.Sprintf("autosolver: %s is configured but its API key is not set, so it never runs", gated.Name),
			"solver", gated.Name,
			"config_key", gated.ConfigKey,
			"configured", as.config.Solvers,
			"running", solverNamesOf(running))
	}

	if len(unactionable) > 0 {
		slog.Debug("autosolver: some configured solvers unavailable; using matched subset",
			"configured", as.config.Solvers,
			"missing", unactionable)
	}
}

func (as *AutoSolver) isRegistered(name string) bool {
	if as.registry == nil {
		return false
	}
	_, ok := as.registry.Get(name)
	return ok
}

func solverNamesOf(solvers []Solver) []string {
	names := make([]string, 0, len(solvers))
	for _, s := range solvers {
		names = append(names, s.Name())
	}
	return names
}

func (as *AutoSolver) trySolvers(ctx context.Context, page Page, executor ActionExecutor) (bool, []AttemptEntry) {
	solvers := as.registry.MatchingSolvers(ctx, page)
	if len(solvers) == 0 {
		return false, []AttemptEntry{{
			Solver: "none",
			Status: StatusSkipped,
		}}
	}

	orderedSolvers := as.orderSolvers(solvers)

	entries := make([]AttemptEntry, 0, len(orderedSolvers))
	for _, s := range orderedSolvers {
		solverCtx, cancel := context.WithTimeout(ctx, as.config.SolverTimeout)
		solverStart := time.Now()

		slog.Info("autosolver_attempt",
			"solver", s.Name(),
			"priority", s.Priority())

		solveResult, err := s.Solve(solverCtx, page, executor)
		cancel()

		entry := AttemptEntry{
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
			entries = append(entries, entry)
			continue
		}

		if solveResult != nil && solveResult.Solved {
			entry.Status = StatusSolved
			return true, append(entries, entry)
		}

		entry.Status = StatusFailed
		if solveResult != nil && solveResult.Error != "" {
			entry.Error = solveResult.Error
		}
		slog.Debug("autosolver: solver returned not-solved",
			"solver", s.Name(),
			"duration_ms", entry.Duration.Milliseconds())
		entries = append(entries, entry)
	}

	return false, entries
}

func (as *AutoSolver) trySemantic(ctx context.Context, page Page, executor ActionExecutor, intent *Intent) (bool, *AttemptEntry) {
	entry := &AttemptEntry{Solver: SemanticSolverName}
	semanticStart := time.Now()

	if as.semantic == nil {
		return as.finishSemantic(entry, semanticStart, StatusSkipped, "semantic engine not configured")
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
			if nextIntent, detectErr := as.detectIntent(semanticCtx, page); detectErr != nil {
				slog.Debug("autosolver: semantic step intent refresh failed",
					"step", step+1,
					"error", detectErr)
			} else {
				currentIntent = nextIntent
			}
		}

		if intentTypeOf(currentIntent) == IntentNormal {
			return as.finishSemantic(entry, semanticStart, StatusSolved, "")
		}

		action, err := as.planSemanticStep(semanticCtx, page, currentIntent, step)
		if err != nil {
			return as.finishSemantic(entry, semanticStart, StatusFailed, err.Error())
		}

		if err := as.executeSemanticStep(semanticCtx, page, executor, currentIntent, step, action); err != nil {
			return as.finishSemantic(entry, semanticStart, StatusFailed, err.Error())
		}

		actionsExecuted++

		if postIntent, detectErr := as.detectIntent(semanticCtx, page); detectErr != nil {
			slog.Debug("autosolver: semantic post-step intent detection failed",
				"step", step+1,
				"error", detectErr)
		} else {
			currentIntent = postIntent
			if intentTypeOf(currentIntent) == IntentNormal {
				return as.finishSemantic(entry, semanticStart, StatusSolved, "")
			}
		}
	}

	if isHighLevelIntent(initialIntentType) && actionsExecuted > 0 {
		return as.finishSemantic(entry, semanticStart, StatusSolved, "")
	}

	return as.finishSemantic(entry, semanticStart, StatusFailed, fmt.Sprintf("semantic flow exhausted for intent %q", initialIntentType))
}

func (as *AutoSolver) finishSemantic(entry *AttemptEntry, start time.Time, status SolverStatus, errMsg string) (bool, *AttemptEntry) {
	entry.Status = status
	entry.Error = errMsg
	entry.Duration = time.Since(start)
	return status == StatusSolved, entry
}

func (as *AutoSolver) planSemanticStep(ctx context.Context, page Page, intent *Intent, step int) (*SuggestedAction, error) {
	suggested, err := as.semantic.SuggestAction(ctx, page, intent)
	if err != nil {
		return nil, fmt.Errorf("semantic suggest action: %w", err)
	}

	planned := as.planSemanticAction(intent, step, suggested)
	action, err := as.prepareSemanticAction(ctx, page, intent, step, planned)
	if err != nil {
		slog.Debug("autosolver: semantic action preparation failed",
			"step", step+1,
			"intent", intentTypeOf(intent),
			"error", err)
		return nil, fmt.Errorf("prepare semantic action: %w", err)
	}

	return action, nil
}

func (as *AutoSolver) executeSemanticStep(ctx context.Context, page Page, executor ActionExecutor, intent *Intent, step int, action *SuggestedAction) error {
	err := executeSuggestedAction(ctx, executor, action)
	if err == nil {
		return nil
	}

	healed, healErr := as.selfHealSemanticAction(ctx, page, intent, step, action)
	if healErr != nil {
		return fmt.Errorf("execute semantic action: %v; self-heal failed: %v", err, healErr)
	}

	if execErr := executeSuggestedAction(ctx, executor, healed); execErr != nil {
		return fmt.Errorf("execute semantic self-heal action: %v", execErr)
	}

	return nil
}

type semanticFlowStep struct {
	Query  string
	Action ActionType
	// Value is the credential the step injects into the matched element when
	// Action == ActionType_. Empty means the step is just a click/wait.
	Value string
}

func (as *AutoSolver) planSemanticAction(intent *Intent, step int, suggested *SuggestedAction) *SuggestedAction {
	planned := &SuggestedAction{Action: ActionNone}
	if suggested != nil {
		copy := *suggested
		planned = &copy
	}

	intentType := intentTypeOf(intent)
	flowStep := semanticFlowStepForIntent(intentType, step, as.config.Credentials)

	if planned.Action == ActionNone || isHighLevelIntent(intentType) {
		planned.Action = flowStep.Action
	}

	if planned.Text == "" && planned.Action == ActionType_ {
		planned.Text = flowStep.Value
	}

	if planned.Reason == "" {
		planned.Reason = fmt.Sprintf("semantic flow step %d", step+1)
	}

	return planned
}

// hydrateSemanticAction maps a resolved semantic match into an actionable
// SuggestedAction: copies the match's selector/ref + coordinates (when present),
// applies the empty-type → click fallback (filling Text from the flow value), and
// validates that a click has a target. clickErr formats the caller-specific
// "click needs a target" error (wording differs between execute and self-heal).
func hydrateSemanticAction(action *SuggestedAction, match *ElementMatch, value string, clickErr func() error) error {
	if match != nil {
		if match.Selector != "" {
			action.Selector = match.Selector
		} else if match.Ref != "" {
			action.Selector = match.Ref
		}
		if match.X != 0 || match.Y != 0 {
			action.X = match.X
			action.Y = match.Y
		}
	}

	if action.Action == ActionType_ && action.Text == "" {
		action.Text = value
		if action.Text == "" {
			action.Action = ActionClick
		}
	}

	if action.Action == ActionClick && action.Selector == "" && action.X == 0 && action.Y == 0 {
		return clickErr()
	}
	return nil
}

func (as *AutoSolver) prepareSemanticAction(ctx context.Context, page Page, intent *Intent, step int, action *SuggestedAction) (*SuggestedAction, error) {
	if action == nil {
		return nil, fmt.Errorf("nil action")
	}

	resolved := *action
	flowStep := semanticFlowStepForIntent(intentTypeOf(intent), step, as.config.Credentials)

	var match *ElementMatch
	if isHighLevelIntent(intentTypeOf(intent)) || actionNeedsTarget(&resolved) {
		var err error
		match, err = as.semantic.FindElement(ctx, page, flowStep.Query)
		if err != nil {
			return nil, fmt.Errorf("semantic find element query %q: %w", flowStep.Query, err)
		}
		if match == nil && actionNeedsTarget(&resolved) {
			return nil, fmt.Errorf("semantic find returned no match for query %q", flowStep.Query)
		}
	}

	if err := hydrateSemanticAction(&resolved, match, flowStep.Value, func() error {
		return fmt.Errorf("semantic action requires selector or coordinates for query %q", flowStep.Query)
	}); err != nil {
		return nil, err
	}

	return &resolved, nil
}

func (as *AutoSolver) selfHealSemanticAction(ctx context.Context, page Page, intent *Intent, step int, original *SuggestedAction) (*SuggestedAction, error) {
	if original == nil {
		return nil, fmt.Errorf("nil action")
	}

	flowStep := semanticFlowStepForIntent(intentTypeOf(intent), step, as.config.Credentials)
	match, err := as.semantic.FindElement(ctx, page, flowStep.Query)
	if err != nil {
		return nil, fmt.Errorf("semantic self-heal find query %q: %w", flowStep.Query, err)
	}
	if match == nil {
		return nil, fmt.Errorf("semantic self-heal returned no match for query %q", flowStep.Query)
	}

	healed := *original
	if err := hydrateSemanticAction(&healed, match, flowStep.Value, func() error {
		return fmt.Errorf("semantic self-heal match for query %q had no actionable selector or coordinates", flowStep.Query)
	}); err != nil {
		return nil, err
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

func semanticFlowStepForIntent(intentType IntentType, step int, creds Credentials) semanticFlowStep {
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
			{Query: "username email input field", Action: ActionType_, Value: creds.Login.User},
			{Query: "password input field", Action: ActionType_, Value: creds.Login.Password},
			{Query: "login submit sign in button", Action: ActionClick},
		}
	case IntentSignup:
		steps = []semanticFlowStep{
			{Query: "name full name input field", Action: ActionType_, Value: creds.Signup.Name},
			{Query: "email input field", Action: ActionType_, Value: creds.Signup.Email},
			{Query: "password create password input field", Action: ActionType_, Value: creds.Signup.Password},
			{Query: "sign up register create account submit button", Action: ActionClick},
		}
	case IntentForm:
		steps = []semanticFlowStep{
			{Query: "first required input field", Action: ActionType_, Value: creds.Form.Field1},
			{Query: "second required input field", Action: ActionType_, Value: firstNonEmpty(creds.Form.Field2, creds.Form.Email)},
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
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

// llmFallbackSolverLabel names the LLM fallback stage in an AttemptEntry and in the
// success result. It is deliberately NOT one of the solver-name constants beside
// SemanticSolverName: those are config-selectable — catalog collects them, so config
// validation accepts them in autoSolver.solvers — while this is only an attempt-entry
// label. Moving it into that group would invite adding it to the catalog, at which
// point autoSolver.solvers: ["llm"] would validate and then match no solver at all.
const llmFallbackSolverLabel = "llm"

func (as *AutoSolver) tryLLM(ctx context.Context, page Page, executor ActionExecutor, history []AttemptEntry) (bool, *AttemptEntry) {
	llmStart := time.Now()
	entry := &AttemptEntry{Solver: llmFallbackSolverLabel}

	html, err := page.HTML()
	if err != nil {
		entry.Status = StatusFailed
		entry.Error = fmt.Sprintf("get HTML: %v", err)
		entry.Duration = time.Since(llmStart)
		return false, entry
	}

	resp, err := as.llm.SuggestNextAction(ctx, LLMRequest{
		PageTitle:    page.Title(),
		PageURL:      page.URL(),
		TrimmedHTML:  htmltrim.TrimHTML(html),
		DetectedType: IntentUnknown,
		PrevAttempts: history,
	})
	if err != nil {
		entry.Status = StatusFailed
		entry.Error = fmt.Sprintf("llm: %v", err)
		entry.Duration = time.Since(llmStart)
		return false, entry
	}

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

// suggestedActionFromLLM adapts an LLMResponse to a SuggestedAction so the LLM
// path runs through the single executeSuggestedAction executor instead of a
// divergent copy. The LLM response carries no X/Y/Expr, so those stay zero.
func suggestedActionFromLLM(resp *LLMResponse) *SuggestedAction {
	return &SuggestedAction{
		Action:   resp.Action,
		Selector: resp.Selector,
		Text:     resp.Text,
		URL:      resp.URL,
		Reason:   resp.Reasoning,
	}
}

func executeAction(ctx context.Context, executor ActionExecutor, resp *LLMResponse) error {
	if resp == nil {
		return fmt.Errorf("nil response")
	}
	return executeSuggestedAction(ctx, executor, suggestedActionFromLLM(resp))
}

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
