package autosolver

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/pinchtab/pinchtab/internal/sanitize"
)

type mockPage struct {
	url   string
	title string
	html  string
}

func (m *mockPage) URL() string           { return m.url }
func (m *mockPage) Title() string         { return m.title }
func (m *mockPage) HTML() (string, error) { return m.html, nil }
func (m *mockPage) HTMLWithin(_ time.Duration) (string, error) {
	return m.html, nil
}
func (m *mockPage) Screenshot() ([]byte, error) { return nil, nil }

type mockExecutor struct {
	clickCalled    int
	typeCalled     int
	navigateCalled int
	evaluateCalled int
	waitCalled     int
	evaluateErr    error
}

func (m *mockExecutor) Click(_ context.Context, _, _ float64) error {
	m.clickCalled++
	return nil
}
func (m *mockExecutor) Type(_ context.Context, _ string) error {
	m.typeCalled++
	return nil
}
func (m *mockExecutor) WaitFor(_ context.Context, _ string, _ time.Duration) error {
	m.waitCalled++
	return nil
}
func (m *mockExecutor) Evaluate(_ context.Context, _ string, _ interface{}) error {
	m.evaluateCalled++
	return m.evaluateErr
}
func (m *mockExecutor) Navigate(_ context.Context, _ string) error {
	m.navigateCalled++
	return nil
}

type mockSolver struct {
	name       string
	priority   int
	canHandle  bool
	solved     bool
	err        error
	solveCalls int

	// waitForClockTick makes Solve occupy at least one tick of the clock the
	// autosolver times attempts with, for the tests that assert a recorded
	// duration is non-zero. A solver that returns immediately can be timed as
	// exactly 0 — time.Since only reports what the clock resolves, and the
	// Windows clock advances about every 15.6ms, so an instant return is
	// genuinely 0ms there. Waiting for the tick asserts what the tests mean
	// (the duration was measured, not left unset) without assuming any
	// particular clock resolution.
	waitForClockTick bool
}

func (m *mockSolver) Name() string  { return m.name }
func (m *mockSolver) Priority() int { return m.priority }
func (m *mockSolver) CanHandle(_ context.Context, _ Page) (bool, error) {
	return m.canHandle, nil
}
func (m *mockSolver) Solve(_ context.Context, _ Page, _ ActionExecutor) (*Result, error) {
	m.solveCalls++
	if m.waitForClockTick {
		waitForClockTick()
	}
	if m.err != nil {
		return &Result{Error: m.err.Error()}, m.err
	}
	return &Result{Solved: m.solved, SolverUsed: m.name}, nil
}

// waitForClockTick blocks until the monotonic clock has advanced at least once,
// which is the smallest non-zero duration this machine can report. It returns
// as soon as that happens, so it costs nothing on a fine-grained clock.
func waitForClockTick() {
	start := time.Now()
	for time.Since(start) <= 0 {
		runtime.Gosched()
	}
}

type mockSemantic struct {
	intent       *Intent
	err          error
	detectSeq    []*Intent
	detectCalls  int
	findMatch    *ElementMatch
	findSeq      []*ElementMatch
	findErr      error
	action       *SuggestedAction
	actionSeq    []*SuggestedAction
	actionErr    error
	findCalls    int
	findQueries  []string
	suggestCalls int
}

func (m *mockSemantic) DetectIntent(_ context.Context, _ Page) (*Intent, error) {
	if len(m.detectSeq) > 0 {
		idx := m.detectCalls
		if idx >= len(m.detectSeq) {
			idx = len(m.detectSeq) - 1
		}
		m.detectCalls++
		return m.detectSeq[idx], m.err
	}
	m.detectCalls++
	return m.intent, m.err
}
func (m *mockSemantic) FindElement(_ context.Context, _ Page, query string) (*ElementMatch, error) {
	m.findCalls++
	m.findQueries = append(m.findQueries, query)
	if len(m.findSeq) > 0 {
		idx := m.findCalls - 1
		if idx >= len(m.findSeq) {
			idx = len(m.findSeq) - 1
		}
		return m.findSeq[idx], m.findErr
	}
	return m.findMatch, m.findErr
}
func (m *mockSemantic) SuggestAction(_ context.Context, _ Page, _ *Intent) (*SuggestedAction, error) {
	m.suggestCalls++
	if len(m.actionSeq) > 0 {
		idx := m.suggestCalls - 1
		if idx >= len(m.actionSeq) {
			idx = len(m.actionSeq) - 1
		}
		return m.actionSeq[idx], m.actionErr
	}
	return m.action, m.actionErr
}

type mockLLM struct {
	resp     *LLMResponse
	err      error
	requests []LLMRequest
}

func (m *mockLLM) SuggestNextAction(_ context.Context, req LLMRequest) (*LLMResponse, error) {
	m.requests = append(m.requests, req)
	return m.resp, m.err
}

func TestSolve_NormalPage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 3

	as := New(cfg, nil, nil)

	page := &mockPage{title: "Google", url: "https://google.com"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Error("expected Solved=true for normal page")
	}
	if result.Intent != IntentNormal {
		t.Errorf("expected intent Normal, got %s", result.Intent)
	}
	if result.Attempts != 0 {
		t.Errorf("expected 0 attempts for normal page, got %d", result.Attempts)
	}
}

func TestSolve_SemanticDetection(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1

	semantic := &mockSemantic{
		intent: &Intent{Type: IntentCaptcha, Confidence: 0.9},
	}

	solver := &mockSolver{
		name:      "test-solver",
		priority:  10,
		canHandle: true,
		solved:    true,
	}

	as := New(cfg, semantic, nil)
	as.Registry().MustRegister(solver)

	page := &mockPage{title: "Challenge Page", url: "https://example.com"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Error("expected Solved=true")
	}
	if result.SolverUsed != "test-solver" {
		t.Errorf("expected solver 'test-solver', got %q", result.SolverUsed)
	}
	if result.Intent != IntentCaptcha {
		t.Errorf("expected intent Captcha, got %s", result.Intent)
	}
}

func TestSolve_SemanticFirst_SuccessSkipsRuleSolvers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1

	semantic := &mockSemantic{
		detectSeq: []*Intent{
			{Type: IntentCaptcha, Confidence: 0.9},
			{Type: IntentNormal, Confidence: 0.9},
		},
		action: &SuggestedAction{
			Action:   ActionClick,
			Selector: "#verify-button",
		},
	}

	solver := &mockSolver{
		name:      "rule-solver",
		priority:  10,
		canHandle: true,
		solved:    true,
	}

	as := New(cfg, semantic, nil)
	as.Registry().MustRegister(solver)

	page := &mockPage{title: "Challenge Page", url: "https://example.com"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Error("expected Solved=true")
	}
	if result.SolverUsed != "semantic" {
		t.Errorf("expected solver 'semantic', got %q", result.SolverUsed)
	}
	if solver.solveCalls != 0 {
		t.Errorf("expected rule solver not to run, got %d calls", solver.solveCalls)
	}
	if semantic.suggestCalls == 0 {
		t.Error("expected semantic SuggestAction to be called")
	}
}

func TestSolve_SemanticFirst_FailureFallsBackToRuleSolvers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1

	semantic := &mockSemantic{
		detectSeq: []*Intent{
			{Type: IntentCaptcha, Confidence: 0.9},
			{Type: IntentCaptcha, Confidence: 0.8},
		},
		action: &SuggestedAction{
			Action:   ActionClick,
			Selector: "#verify-button",
		},
	}

	solver := &mockSolver{
		name:      "rule-solver",
		priority:  10,
		canHandle: true,
		solved:    true,
	}

	as := New(cfg, semantic, nil)
	as.Registry().MustRegister(solver)

	page := &mockPage{title: "Challenge Page", url: "https://example.com"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Error("expected Solved=true via rule solver fallback")
	}
	if result.SolverUsed != "rule-solver" {
		t.Errorf("expected solver 'rule-solver', got %q", result.SolverUsed)
	}
	if solver.solveCalls == 0 {
		t.Error("expected rule solver to run after semantic failure")
	}
	if len(result.History) == 0 {
		t.Fatal("expected non-empty attempt history")
	}
	if result.History[0].Solver != "semantic" {
		t.Errorf("expected first attempt to be semantic, got %q", result.History[0].Solver)
	}
	if result.History[0].Status != StatusFailed {
		t.Errorf("expected semantic attempt to fail before fallback, got %q", result.History[0].Status)
	}
}

func TestSolve_SemanticHighLevel_LoginFlow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.Credentials = Credentials{
		Login: LoginCredentials{
			User:     "user@example.com",
			Password: "secret",
		},
	}

	semantic := &mockSemantic{
		detectSeq: []*Intent{
			{Type: IntentLogin, Confidence: 0.9},
			{Type: IntentLogin, Confidence: 0.9},
			{Type: IntentLogin, Confidence: 0.9},
			{Type: IntentLogin, Confidence: 0.9},
		},
		findMatch: &ElementMatch{Selector: "#login-field"},
	}

	solver := &mockSolver{
		name:      "rule-solver",
		priority:  10,
		canHandle: true,
		solved:    true,
	}

	as := New(cfg, semantic, nil)
	as.Registry().MustRegister(solver)

	page := &mockPage{title: "Sign in", url: "https://example.com/login"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Error("expected Solved=true")
	}
	if result.SolverUsed != "semantic" {
		t.Errorf("expected solver 'semantic', got %q", result.SolverUsed)
	}
	if solver.solveCalls != 0 {
		t.Errorf("expected rule solver not to run, got %d calls", solver.solveCalls)
	}
	if semantic.findCalls < 3 {
		t.Errorf("expected semantic /find to run on multiple flow steps, got %d calls", semantic.findCalls)
	}
	if executor.typeCalled < 2 {
		t.Errorf("expected form-filling type actions, got %d", executor.typeCalled)
	}
}

func TestSolve_SemanticHighLevel_LoginFallbackWhenFindFails(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1

	semantic := &mockSemantic{
		detectSeq: []*Intent{{Type: IntentLogin, Confidence: 0.9}},
		findMatch: nil,
	}

	solver := &mockSolver{
		name:      "rule-solver",
		priority:  10,
		canHandle: true,
		solved:    true,
	}

	as := New(cfg, semantic, nil)
	as.Registry().MustRegister(solver)

	page := &mockPage{title: "Sign in", url: "https://example.com/login"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Error("expected Solved=true via rule solver fallback")
	}
	if result.SolverUsed != "rule-solver" {
		t.Errorf("expected solver 'rule-solver', got %q", result.SolverUsed)
	}
	if semantic.findCalls == 0 {
		t.Error("expected semantic /find attempt before fallback")
	}
	if len(result.History) == 0 {
		t.Fatal("expected non-empty attempt history")
	}
	if result.History[0].Solver != "semantic" {
		t.Errorf("expected first history entry to be semantic, got %q", result.History[0].Solver)
	}
}

func TestSolve_SemanticSelfHealFailureFallsBackToRuleSolvers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1

	semantic := &mockSemantic{
		detectSeq: []*Intent{{Type: IntentCaptcha, Confidence: 0.9}},
		action: &SuggestedAction{
			Action:   ActionClick,
			Selector: "#verify",
		},
		findMatch: &ElementMatch{Selector: "#verify"},
	}

	solver := &mockSolver{
		name:      "rule-solver",
		priority:  10,
		canHandle: true,
		solved:    true,
	}

	as := New(cfg, semantic, nil)
	as.Registry().MustRegister(solver)

	page := &mockPage{title: "Challenge Page", url: "https://example.com"}
	executor := &mockExecutor{evaluateErr: fmt.Errorf("selector lookup failed")}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Error("expected solve success via rule solver fallback")
	}
	if result.SolverUsed != "rule-solver" {
		t.Errorf("expected solver 'rule-solver', got %q", result.SolverUsed)
	}
	if len(result.History) == 0 || result.History[0].Solver != "semantic" {
		t.Fatalf("expected semantic attempt in history, got %+v", result.History)
	}
	if result.History[0].Status != StatusFailed {
		t.Fatalf("expected semantic attempt to fail, got %q", result.History[0].Status)
	}
	if solver.solveCalls == 0 {
		t.Fatal("expected rule solver to run after semantic self-heal failure")
	}
}

func TestSolve_FallbackChain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.RetryBaseDelay = time.Millisecond

	// First solver fails, second succeeds.
	failing := &mockSolver{
		name:      "failing",
		priority:  10,
		canHandle: true,
		solved:    false,
		err:       fmt.Errorf("solver error"),
	}
	succeeding := &mockSolver{
		name:      "succeeding",
		priority:  20,
		canHandle: true,
		solved:    true,
	}

	as := New(cfg, nil, nil)
	as.Registry().MustRegister(failing)
	as.Registry().MustRegister(succeeding)

	// Use a title that triggers captcha detection via heuristics.
	page := &mockPage{title: "Just a moment...", url: "https://example.com"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Error("expected Solved=true from second solver")
	}
	if result.SolverUsed != "succeeding" {
		t.Errorf("expected solver 'succeeding', got %q", result.SolverUsed)
	}
}

func TestSolve_AllSolversFail(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 2
	cfg.RetryBaseDelay = time.Millisecond

	failing := &mockSolver{
		name:      "failing",
		priority:  10,
		canHandle: true,
		solved:    false,
		err:       fmt.Errorf("solver error"),
	}

	as := New(cfg, nil, nil)
	as.Registry().MustRegister(failing)

	page := &mockPage{title: "Just a moment...", url: "https://example.com"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Solved {
		t.Error("expected Solved=false when all solvers fail")
	}
	if result.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", result.Attempts)
	}
	if len(result.History) == 0 {
		t.Error("expected non-empty history")
	}
}

func solverHistory(history []AttemptEntry) []AttemptEntry {
	entries := make([]AttemptEntry, 0, len(history))
	for _, entry := range history {
		if entry.Solver == "semantic" || entry.Solver == "llm" {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func twoFailingSolvers() (*mockSolver, *mockSolver) {
	return &mockSolver{
			name:             "alpha",
			priority:         10,
			canHandle:        true,
			err:              fmt.Errorf("alpha exploded"),
			waitForClockTick: true,
		}, &mockSolver{
			name:             "beta",
			priority:         20,
			canHandle:        true,
			err:              fmt.Errorf("beta exploded"),
			waitForClockTick: true,
		}
}

func TestSolve_HistoryRecordsEverySolverAttempt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.RetryBaseDelay = time.Millisecond

	alpha, beta := twoFailingSolvers()
	as := New(cfg, nil, nil)
	as.Registry().MustRegister(beta)
	as.Registry().MustRegister(alpha)

	page := &mockPage{title: "Just a moment...", url: "https://example.com"}
	result, err := as.Solve(context.Background(), page, &mockExecutor{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := solverHistory(result.History)
	if len(entries) != 2 {
		t.Fatalf("expected one history entry per solver, got %d (%+v)", len(entries), entries)
	}

	wantErrors := map[string]string{
		"alpha": "alpha exploded",
		"beta":  "beta exploded",
	}
	for i, entry := range entries {
		want, ok := wantErrors[entry.Solver]
		if !ok {
			t.Fatalf("entry %d has unexpected solver %q", i, entry.Solver)
		}
		if entry.Error != want {
			t.Errorf("entry %d (%s): expected error %q, got %q", i, entry.Solver, want, entry.Error)
		}
		if entry.Status != StatusFailed {
			t.Errorf("entry %d (%s): expected status %q, got %q", i, entry.Solver, StatusFailed, entry.Status)
		}
		if entry.Duration <= 0 {
			t.Errorf("entry %d (%s): expected non-zero duration, got %v", i, entry.Solver, entry.Duration)
		}
		delete(wantErrors, entry.Solver)
	}
	if len(wantErrors) != 0 {
		t.Errorf("missing history entries for solvers %v", wantErrors)
	}
}

func TestSolve_LLMFallbackReceivesEveryPreviousAttempt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.LLMFallback = true
	cfg.RetryBaseDelay = time.Millisecond

	alpha, beta := twoFailingSolvers()
	llm := &mockLLM{err: fmt.Errorf("llm unavailable")}

	as := New(cfg, nil, llm)
	as.Registry().MustRegister(alpha)
	as.Registry().MustRegister(beta)

	page := &mockPage{title: "Just a moment...", url: "https://example.com", html: "<html></html>"}
	if _, err := as.Solve(context.Background(), page, &mockExecutor{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(llm.requests) != 1 {
		t.Fatalf("expected exactly one LLM request, got %d", len(llm.requests))
	}

	seen := map[string]string{}
	for _, attempt := range llm.requests[0].PrevAttempts {
		seen[attempt.Solver] = attempt.Error
	}
	if seen["alpha"] != "alpha exploded" {
		t.Errorf("expected alpha error in PrevAttempts, got %q", seen["alpha"])
	}
	if seen["beta"] != "beta exploded" {
		t.Errorf("expected beta error in PrevAttempts, got %q", seen["beta"])
	}
}

func TestSolve_HistoryKeepsFailedAttemptBeforeSuccess(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.RetryBaseDelay = time.Millisecond

	alpha := &mockSolver{
		name:      "alpha",
		priority:  10,
		canHandle: true,
		err:       fmt.Errorf("alpha exploded"),
	}
	beta := &mockSolver{
		name:      "beta",
		priority:  20,
		canHandle: true,
		solved:    true,
	}

	as := New(cfg, nil, nil)
	as.Registry().MustRegister(alpha)
	as.Registry().MustRegister(beta)

	page := &mockPage{title: "Just a moment...", url: "https://example.com"}
	result, err := as.Solve(context.Background(), page, &mockExecutor{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Solved {
		t.Fatal("expected Solved=true when the second solver solves")
	}
	if result.SolverUsed != "beta" {
		t.Errorf("expected solver 'beta', got %q", result.SolverUsed)
	}

	entries := solverHistory(result.History)
	if len(entries) != 2 {
		t.Fatalf("expected both solver attempts in history, got %+v", entries)
	}
	if entries[0].Solver != "alpha" || entries[0].Status != StatusFailed || entries[0].Error != "alpha exploded" {
		t.Errorf("expected alpha failure preserved, got %+v", entries[0])
	}
	if entries[1].Solver != "beta" || entries[1].Status != StatusSolved {
		t.Errorf("expected beta solved entry, got %+v", entries[1])
	}
}

func TestSolve_HistoryRecordsNoMatchingSolver(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.RetryBaseDelay = time.Millisecond

	as := New(cfg, nil, nil)
	as.Registry().MustRegister(&mockSolver{name: "idle", priority: 10, canHandle: false})

	page := &mockPage{title: "Just a moment...", url: "https://example.com"}
	result, err := as.Solve(context.Background(), page, &mockExecutor{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := solverHistory(result.History)
	if len(entries) != 1 {
		t.Fatalf("expected a single history entry, got %+v", entries)
	}
	if entries[0].Solver != "none" || entries[0].Status != StatusSkipped {
		t.Errorf("expected none/skipped entry, got %+v", entries[0])
	}
}

func TestSolve_LLMFallback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.LLMFallback = true
	cfg.RetryBaseDelay = time.Millisecond

	llm := &mockLLM{
		resp: &LLMResponse{
			Action:     ActionNone,
			Confidence: 0.8,
		},
	}

	as := New(cfg, nil, llm)

	// No solvers registered, so LLM fallback should activate.
	page := &mockPage{title: "Just a moment...", url: "https://example.com", html: "<html></html>"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Error("expected Solved=true via LLM fallback")
	}
	if result.SolverUsed != "llm" {
		t.Errorf("expected solver 'llm', got %q", result.SolverUsed)
	}
}

func TestSolve_LLMRequestCarriesStrippedRuneSafeHTML(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.LLMFallback = true
	cfg.RetryBaseDelay = time.Millisecond

	llm := &mockLLM{resp: &LLMResponse{Action: ActionNone, Confidence: 0.8}}
	as := New(cfg, nil, llm)

	html := "<html><head><style>body{color:red}</style><script>var secret=1;</script></head><body>" +
		strings.Repeat("héllo wörld ", 1000) + "</body></html>"
	page := &mockPage{title: "Just a moment...", url: "https://example.com", html: html}

	if _, err := as.Solve(context.Background(), page, &mockExecutor{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(llm.requests) != 1 {
		t.Fatalf("expected a single LLM request, got %d", len(llm.requests))
	}

	sent := llm.requests[0].TrimmedHTML
	for _, unwanted := range []string{"<style", "<script", "color:red", "var secret"} {
		if strings.Contains(sent, unwanted) {
			t.Errorf("TrimmedHTML still contains %q", unwanted)
		}
	}
	if len(sent) >= len(html) {
		t.Errorf("TrimmedHTML was not capped: %d bytes", len(sent))
	}
	if !utf8.ValidString(sent) {
		t.Errorf("TrimmedHTML is not valid UTF-8")
	}
	if strings.ContainsRune(sent, utf8.RuneError) {
		t.Errorf("TrimmedHTML contains U+FFFD absent from the source")
	}
	if strings.HasSuffix(sent, sanitize.TruncationSuffix) {
		t.Errorf("TrimmedHTML got a truncation marker appended: %q", sent[len(sent)-8:])
	}
	if !strings.HasPrefix(sent, "<html><head></head><body>héllo wörld") {
		t.Errorf("TrimmedHTML lost the page body: %q", sent[:40])
	}
}

func TestSolve_ContextCancellation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 10
	cfg.RetryBaseDelay = 5 * time.Second

	// Slow solver that never succeeds.
	slow := &mockSolver{
		name:      "slow",
		priority:  10,
		canHandle: true,
		solved:    false,
	}

	as := New(cfg, nil, nil)
	as.Registry().MustRegister(slow)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	page := &mockPage{title: "Just a moment...", url: "https://example.com"}
	executor := &mockExecutor{}

	result, err := as.Solve(ctx, page, executor)
	// Depending on timing, Solve may return a context error directly or
	// terminate with a non-success result after the context is canceled.
	if err == nil {
		if ctx.Err() == nil {
			t.Fatalf("expected context cancellation or solve error; got err=nil and ctx.Err()=nil")
		}
	} else if ctx.Err() == nil {
		t.Fatalf("got error %v but context was not canceled", err)
	}
	_ = result
}

func TestSolve_PriorityOrdering(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.RetryBaseDelay = time.Millisecond

	// Register solvers in reverse priority order.
	var solveOrder []string
	makeSolver := func(name string, priority int) Solver {
		return &trackingSolver{
			name:      name,
			priority:  priority,
			canHandle: true,
			order:     &solveOrder,
		}
	}

	as := New(cfg, nil, nil)
	as.Registry().MustRegister(makeSolver("third", 30))
	as.Registry().MustRegister(makeSolver("first", 10))
	as.Registry().MustRegister(makeSolver("second", 20))

	page := &mockPage{title: "Just a moment...", url: "https://example.com"}
	executor := &mockExecutor{}

	_, _ = as.Solve(context.Background(), page, executor)

	// Verify solvers were tried in priority order.
	if len(solveOrder) < 3 {
		t.Fatalf("expected 3 solver calls, got %d", len(solveOrder))
	}
	if solveOrder[0] != "first" {
		t.Errorf("expected first solver tried, got %q", solveOrder[0])
	}
	if solveOrder[1] != "second" {
		t.Errorf("expected second solver tried, got %q", solveOrder[1])
	}
	if solveOrder[2] != "third" {
		t.Errorf("expected third solver tried, got %q", solveOrder[2])
	}
}

func TestSolve_UsesConfiguredSolverOrder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.RetryBaseDelay = time.Millisecond
	cfg.Solvers = []string{"third", "first", "second"}

	var solveOrder []string
	makeSolver := func(name string, priority int, solved bool) Solver {
		return &trackingSolver{
			name:      name,
			priority:  priority,
			canHandle: true,
			solved:    solved,
			order:     &solveOrder,
		}
	}

	as := New(cfg, nil, nil)
	as.Registry().MustRegister(makeSolver("first", 10, false))
	as.Registry().MustRegister(makeSolver("second", 20, false))
	as.Registry().MustRegister(makeSolver("third", 30, true))

	page := &mockPage{title: "Just a moment...", url: "https://example.com"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Fatal("expected solve success from configured first solver")
	}
	if result.SolverUsed != "third" {
		t.Fatalf("expected solver 'third', got %q", result.SolverUsed)
	}
	if len(solveOrder) != 1 {
		t.Fatalf("expected exactly one solver call, got %d (%v)", len(solveOrder), solveOrder)
	}
	if solveOrder[0] != "third" {
		t.Fatalf("expected configured solver order to try 'third' first, got %q", solveOrder[0])
	}
}

func TestSolve_ConfiguredSolverOrderFallbackWhenNoConfiguredNamesMatch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.RetryBaseDelay = time.Millisecond
	cfg.Solvers = []string{"missing-one", "missing-two"}

	var solveOrder []string
	makeSolver := func(name string, priority int, solved bool) Solver {
		return &trackingSolver{
			name:      name,
			priority:  priority,
			canHandle: true,
			solved:    solved,
			order:     &solveOrder,
		}
	}

	as := New(cfg, nil, nil)
	as.Registry().MustRegister(makeSolver("first", 10, true))
	as.Registry().MustRegister(makeSolver("second", 20, false))

	page := &mockPage{title: "Just a moment...", url: "https://example.com"}
	executor := &mockExecutor{}

	result, err := as.Solve(context.Background(), page, executor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Solved {
		t.Fatal("expected solve success from priority-order fallback")
	}
	if len(solveOrder) != 1 {
		t.Fatalf("expected one solver attempt, got %d (%v)", len(solveOrder), solveOrder)
	}
	if solveOrder[0] != "first" {
		t.Fatalf("expected fallback to priority order starting with 'first', got %q", solveOrder[0])
	}
}

// trackingSolver records the order in which Solve is called.
type trackingSolver struct {
	name      string
	priority  int
	canHandle bool
	solved    bool
	order     *[]string
}

func (s *trackingSolver) Name() string  { return s.name }
func (s *trackingSolver) Priority() int { return s.priority }
func (s *trackingSolver) CanHandle(_ context.Context, _ Page) (bool, error) {
	return s.canHandle, nil
}
func (s *trackingSolver) Solve(_ context.Context, _ Page, _ ActionExecutor) (*Result, error) {
	*s.order = append(*s.order, s.name)
	return &Result{Solved: s.solved}, nil
}

// The LLM path runs through the shared executeSuggestedAction executor, so its
// type/click/navigate semantics match the semantic path (no divergent copy).

// Regression: an LLM type action with a selector must resolve + click (focus)
// the target field before typing — previously it typed without focusing.
func TestExecuteAction_LLMTypeWithSelectorFocusesBeforeTyping(t *testing.T) {
	ex := &mockExecutor{}
	resp := &LLMResponse{Action: ActionType_, Selector: "#email", Text: "a@b.co"}
	if err := executeAction(context.Background(), ex, resp); err != nil {
		t.Fatalf("executeAction: %v", err)
	}
	if ex.evaluateCalled != 1 {
		t.Errorf("expected selector resolve via Evaluate, got %d", ex.evaluateCalled)
	}
	if ex.clickCalled != 1 {
		t.Errorf("expected field focus click before typing, got %d", ex.clickCalled)
	}
	if ex.typeCalled != 1 {
		t.Errorf("expected type, got %d", ex.typeCalled)
	}
}

func TestExecuteAction_LLMTypeWithoutSelectorTypesOnly(t *testing.T) {
	ex := &mockExecutor{}
	resp := &LLMResponse{Action: ActionType_, Text: "hello"}
	if err := executeAction(context.Background(), ex, resp); err != nil {
		t.Fatalf("executeAction: %v", err)
	}
	if ex.clickCalled != 0 || ex.evaluateCalled != 0 {
		t.Errorf("expected no focus click without selector, got click=%d evaluate=%d", ex.clickCalled, ex.evaluateCalled)
	}
	if ex.typeCalled != 1 {
		t.Errorf("expected type, got %d", ex.typeCalled)
	}
}

func TestExecuteAction_LLMClickWithSelector(t *testing.T) {
	ex := &mockExecutor{}
	resp := &LLMResponse{Action: ActionClick, Selector: "#submit"}
	if err := executeAction(context.Background(), ex, resp); err != nil {
		t.Fatalf("executeAction: %v", err)
	}
	if ex.evaluateCalled != 1 || ex.clickCalled != 1 {
		t.Errorf("expected resolve+click, got evaluate=%d click=%d", ex.evaluateCalled, ex.clickCalled)
	}
}

func TestExecuteAction_LLMNavigate(t *testing.T) {
	ex := &mockExecutor{}
	resp := &LLMResponse{Action: ActionNavigate, URL: "https://example.com"}
	if err := executeAction(context.Background(), ex, resp); err != nil {
		t.Fatalf("executeAction: %v", err)
	}
	if ex.navigateCalled != 1 {
		t.Errorf("expected navigate, got %d", ex.navigateCalled)
	}
}

func TestOrderSolvers(t *testing.T) {
	newPriorityOrder := func() []Solver {
		return []Solver{
			&mockSolver{name: "alpha", priority: 10},
			&mockSolver{name: "beta", priority: 20},
			&mockSolver{name: "gamma", priority: 30},
		}
	}

	for _, tc := range []struct {
		name       string
		configured []string
		want       []string
	}{
		{
			name:       "all configured names present, configured order wins",
			configured: []string{"gamma", "alpha", "beta"},
			want:       []string{"gamma", "alpha", "beta"},
		},
		{
			name:       "some configured names missing, matched subset in configured order",
			configured: []string{"gamma", "absent", "alpha"},
			want:       []string{"gamma", "alpha"},
		},
		{
			name:       "no configured name matches, priority order unchanged",
			configured: []string{"absent", "also-absent"},
			want:       []string{"alpha", "beta", "gamma"},
		},
		{
			name:       "empty config, priority order unchanged",
			configured: nil,
			want:       []string{"alpha", "beta", "gamma"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			as := New(Config{Solvers: tc.configured}, nil, nil)

			got := solverNamesOf(as.orderSolvers(newPriorityOrder()))

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("orderSolvers = %v, want %v", got, tc.want)
			}
			for _, name := range got {
				if name == "absent" || name == "also-absent" {
					t.Errorf("unconfigured-but-missing name %q leaked into the result %v", name, got)
				}
			}
		})
	}
}

func TestOrderSolversDoesNotMutateInput(t *testing.T) {
	priorityOrder := []Solver{
		&mockSolver{name: "alpha", priority: 10},
		&mockSolver{name: "beta", priority: 20},
	}
	before := solverNamesOf(priorityOrder)

	as := New(Config{Solvers: []string{"beta", "alpha"}}, nil, nil)
	_ = as.orderSolvers(priorityOrder)

	if after := solverNamesOf(priorityOrder); !reflect.DeepEqual(before, after) {
		t.Fatalf("input reordered in place: %v -> %v", before, after)
	}
}

type levelRecorder struct {
	records []slog.Record
}

func (r *levelRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *levelRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.records = append(r.records, rec.Clone())
	return nil
}
func (r *levelRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *levelRecorder) WithGroup(string) slog.Handler      { return r }

func (r *levelRecorder) find(t *testing.T, minLevel slog.Level, substr string) slog.Record {
	t.Helper()
	for _, rec := range r.records {
		if rec.Level >= minLevel && strings.Contains(rec.Message, substr) {
			return rec
		}
	}
	for _, rec := range r.records {
		if strings.Contains(rec.Message, substr) {
			t.Fatalf("%q was logged at %s, want %s or above — an operator never sees it at the default level",
				rec.Message, rec.Level, minLevel)
		}
	}
	t.Fatalf("no log line containing %q (%d records)", substr, len(r.records))
	return slog.Record{}
}

func recordAttrs(rec slog.Record) string {
	var sb strings.Builder
	rec.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&sb, "%s=%v ", a.Key, a.Value)
		return true
	})
	return sb.String()
}

// After typo rejection, zero-match is still reachable: every configured solver
// known but unavailable, e.g. capsolver named without its API key. The fallback
// stays — refusing to solve is the worse failure — but it must be visible.
func TestOrderSolversWarnsWhenNoConfiguredSolverIsAvailable(t *testing.T) {
	recorder := captureLogs(t)

	priorityOrder := []Solver{
		&mockSolver{name: "cloudflare", priority: 10},
		&mockSolver{name: "jschallenge", priority: 20},
	}
	as := New(Config{Solvers: []string{"capsolver", "twocaptcha"}}, nil, nil)

	got := solverNamesOf(as.orderSolvers(priorityOrder))

	if want := []string{"cloudflare", "jschallenge"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback returned %v, want the priority order %v", got, want)
	}

	rec := recorder.find(t, slog.LevelWarn, "falling back to priority order")
	attrs := recordAttrs(rec)
	for _, want := range []string{"capsolver", "twocaptcha", "cloudflare", "jschallenge"} {
		if !strings.Contains(attrs, want) {
			t.Errorf("warn line does not name %q: %s", want, attrs)
		}
	}
}

// A missing name that is not key-gated is nothing the operator can act on, and
// the shipped default hits this path every run (semantic is a stage, not a
// registry solver, so it never matches). Promoting it would put a warning in
// front of every operator on every run.
func TestOrderSolversDoesNotWarnWhenTheMissingSolverIsNotKeyGated(t *testing.T) {
	recorder := captureLogs(t)

	as := New(Config{Solvers: []string{"cloudflare", SemanticSolverName}}, nil, nil)
	as.orderSolvers([]Solver{&mockSolver{name: "cloudflare", priority: 10}})

	if _, ok := KeyGatedSolverNamed(SemanticSolverName); ok {
		t.Fatalf("%s is key-gated, so this test no longer covers the non-gated case", SemanticSolverName)
	}
	for _, rec := range recorder.records {
		if rec.Level >= slog.LevelWarn {
			t.Errorf("non-gated missing solver logged at %s: %q — the shipped default hits this path every run", rec.Level, rec.Message)
		}
	}
}

// A key-gated solver named without its API key is the one misconfiguration
// solver-name validation deliberately admits, so the runtime is where the
// operator has to learn about it.
func TestOrderSolversWarnsWhenAKeyGatedSolverHasNoAPIKey(t *testing.T) {
	for _, gated := range KeyGatedSolvers() {
		t.Run(gated.Name, func(t *testing.T) {
			recorder := captureLogs(t)

			as := New(Config{Solvers: []string{"cloudflare", gated.Name}}, nil, nil)
			if _, registered := as.Registry().Get(gated.Name); registered {
				t.Fatalf("%s is registered, so this test is not covering the unset-key case", gated.Name)
			}
			got := solverNamesOf(as.orderSolvers([]Solver{
				&mockSolver{name: "cloudflare", priority: 10},
				&mockSolver{name: "jschallenge", priority: 20},
			}))

			if want := []string{"cloudflare"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("orderSolvers = %v, want the matched subset %v — the warning must not change which solvers run", got, want)
			}

			rec := recorder.find(t, slog.LevelWarn, gated.Name)
			if !strings.Contains(rec.Message, gated.Name) {
				t.Errorf("warn message %q does not name the solver", rec.Message)
			}
			if !strings.Contains(rec.Message, "API key") {
				t.Errorf("warn message %q does not state the missing API key as the reason", rec.Message)
			}
			if attrs := recordAttrs(rec); !strings.Contains(attrs, gated.ConfigKey) {
				t.Errorf("warn line does not name the config key %q to set: %s", gated.ConfigKey, attrs)
			}
		})
	}
}

// orderSolvers receives THIS page's matching set, which a registered, working
// solver is absent from whenever it does not handle this challenge. Diagnosing
// that as an unset API key names a config key the operator has already filled in.
func TestOrderSolversDoesNotWarnWhenAKeyGatedSolverIsRegisteredButDoesNotHandleThePage(t *testing.T) {
	for _, gated := range KeyGatedSolvers() {
		t.Run(gated.Name, func(t *testing.T) {
			recorder := captureLogs(t)

			as := New(Config{Solvers: []string{"cloudflare", gated.Name}}, nil, nil)
			as.Registry().MustRegister(&mockSolver{name: gated.Name, priority: 200})
			if _, registered := as.Registry().Get(gated.Name); !registered {
				t.Fatalf("precondition: %s must be registered, meaning its API key IS set", gated.Name)
			}

			as.orderSolvers([]Solver{&mockSolver{name: "cloudflare", priority: 10}})

			for _, rec := range recorder.records {
				if rec.Level >= slog.LevelWarn {
					t.Errorf("keyed %s absent from this page's matching set logged at %s: %q", gated.Name, rec.Level, rec.Message)
				}
			}
		})
	}
}

// Solve loops up to MaxAttempts and calls orderSolvers each time, so an unset
// key stated once per attempt would repeat one static config fact all run.
func TestOrderSolversWarnsOncePerUnregisteredKeyGatedSolver(t *testing.T) {
	recorder := captureLogs(t)

	gated := KeyGatedSolvers()[0]
	as := New(Config{Solvers: []string{"cloudflare", gated.Name}}, nil, nil)
	matching := []Solver{&mockSolver{name: "cloudflare", priority: 10}}
	for i := 0; i < 5; i++ {
		as.orderSolvers(matching)
	}

	warnings := 0
	for _, rec := range recorder.records {
		if rec.Level >= slog.LevelWarn && strings.Contains(rec.Message, gated.Name) {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("five orderSolvers calls produced %d warnings for %s, want 1", warnings, gated.Name)
	}
}

// The dedup above is deliberately per-AutoSolver, and buildAutoSolver makes a
// fresh one per /solve request and per auto-solve trigger — so a real
// misconfiguration is stated once per request rather than once per attempt.
// Moving the guard to package scope would silence it after the first request in
// the process, which also means an operator who sets the key mid-process would
// never see the warning stop being true. This pins the choice so it cannot be
// "simplified" into that.
func TestUnregisteredKeyGatedWarningRepeatsForAFreshAutoSolver(t *testing.T) {
	recorder := captureLogs(t)

	gated := KeyGatedSolvers()[0]
	matching := []Solver{&mockSolver{name: "cloudflare", priority: 10}}
	const requests = 3
	for i := 0; i < requests; i++ {
		as := New(Config{Solvers: []string{"cloudflare", gated.Name}}, nil, nil)
		as.orderSolvers(matching)
		as.orderSolvers(matching)
	}

	warnings := 0
	for _, rec := range recorder.records {
		if rec.Level >= slog.LevelWarn && strings.Contains(rec.Message, gated.Name) {
			warnings++
		}
	}
	if warnings != requests {
		t.Errorf("%d fresh AutoSolvers produced %d warnings for %s, want %d — one per request, deduped within each",
			requests, warnings, gated.Name, requests)
	}
}

func captureLogs(t *testing.T) *levelRecorder {
	t.Helper()
	recorder := &levelRecorder{}
	previous := slog.Default()
	slog.SetDefault(slog.New(recorder))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return recorder
}

// SemanticEngine is exported, so a third-party DetectIntent returning (nil, nil)
// satisfies the contract — the bundled adapter's FindElement already returns that
// shape. Every intent read in trySemantic routes through intentTypeOf except, once,
// the post-step check, which dereferenced the value detectIntent had just handed
// back. This drives the whole loop with nil intents so that guard has a home.
func TestTrySemanticSurvivesANilIntentFromTheEngine(t *testing.T) {
	cfg := DefaultConfig()
	semantic := &mockSemantic{
		detectSeq: []*Intent{nil},
		action:    &SuggestedAction{Action: ActionClick, Selector: "#anything"},
	}

	as := New(cfg, semantic, nil)
	page := &mockPage{title: "Verify your identity", url: "https://example.com/gate"}

	solved, entry := as.trySemantic(context.Background(), page, &mockExecutor{}, nil)

	if solved {
		t.Error("a nil intent cannot be a solved page; the engine reported nothing")
	}
	if entry == nil {
		t.Fatal("no attempt entry recorded")
	}
	if entry.Status != StatusFailed {
		t.Errorf("attempt status = %q, want %q", entry.Status, StatusFailed)
	}
}

// Solve is the exported entry point and reads the intent BEFORE any solving
// starts, so the (nil, nil) engine reaches it earlier than it reaches
// trySemantic. The trySemantic test above walked past this because it calls the
// unexported step directly.
func TestSolveSurvivesANilIntentFromTheEngine(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxAttempts = 1
	semantic := &mockSemantic{
		detectSeq: []*Intent{nil},
		action:    &SuggestedAction{Action: ActionClick, Selector: "#anything"},
	}

	as := New(cfg, semantic, nil)
	page := &mockPage{title: "Verify your identity", url: "https://example.com/gate"}

	result, err := as.Solve(context.Background(), page, &mockExecutor{})
	if err != nil {
		t.Fatalf("Solve returned an error: %v", err)
	}
	if result == nil {
		t.Fatal("Solve returned no result")
	}
	// An engine that read nothing must not be recorded as reporting a normal
	// page: IntentNormal is the one value that short-circuits Solve as solved,
	// which would turn "we know nothing" into "there is no challenge".
	if result.Intent != IntentUnknown {
		t.Errorf("result.Intent = %q, want %q for an engine that reported nothing", result.Intent, IntentUnknown)
	}
	if result.Solved {
		t.Error("a nil intent cannot be a solved page; the engine reported nothing")
	}
}
