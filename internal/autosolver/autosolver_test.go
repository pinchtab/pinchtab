package autosolver

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// --- Mock implementations ---

type mockPage struct {
	url   string
	title string
	html  string
}

func (m *mockPage) URL() string                 { return m.url }
func (m *mockPage) Title() string               { return m.title }
func (m *mockPage) HTML() (string, error)       { return m.html, nil }
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
}

func (m *mockSolver) Name() string  { return m.name }
func (m *mockSolver) Priority() int { return m.priority }
func (m *mockSolver) CanHandle(_ context.Context, _ Page) (bool, error) {
	return m.canHandle, nil
}
func (m *mockSolver) Solve(_ context.Context, _ Page, _ ActionExecutor) (*Result, error) {
	m.solveCalls++
	if m.err != nil {
		return &Result{Error: m.err.Error()}, m.err
	}
	return &Result{Solved: m.solved, SolverUsed: m.name}, nil
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
	resp *LLMResponse
	err  error
}

func (m *mockLLM) SuggestNextAction(_ context.Context, _ LLMRequest) (*LLMResponse, error) {
	return m.resp, m.err
}

// --- Tests ---

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
	t.Setenv("PINCHTAB_AUTOSOLVER_LOGIN_USER", "user@example.com")
	t.Setenv("PINCHTAB_AUTOSOLVER_LOGIN_PASSWORD", "secret")

	cfg := DefaultConfig()
	cfg.MaxAttempts = 1

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
