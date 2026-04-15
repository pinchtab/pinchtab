package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/solver"
)

type testStaticSolver struct {
	name   string
	result *solver.Result
	err    error
}

func (s *testStaticSolver) Name() string { return s.name }
func (s *testStaticSolver) CanHandle(context.Context) (bool, error) {
	return true, nil
}
func (s *testStaticSolver) Solve(context.Context, solver.Options) (*solver.Result, error) {
	return s.result, s.err
}

func TestHandleListSolvers(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/solvers", nil)
	w := httptest.NewRecorder()
	h.HandleListSolvers(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string][]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	solvers, ok := resp["solvers"]
	if !ok {
		t.Fatal("expected 'solvers' key in response")
	}

	// cloudflare solver is registered via bridge init
	found := false
	for _, s := range solvers {
		if s == "cloudflare" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected cloudflare in solvers list, got %v", solvers)
	}
}

func TestHandleSolve_InvalidBody(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	h.HandleSolve(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for invalid body, got %d", w.Code)
	}
}

func TestHandleSolve_EmptyBody(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/solve", nil)
	w := httptest.NewRecorder()
	h.HandleSolve(w, req)

	// Empty body should use defaults (auto-detect), not 400.
	if w.Code == 400 {
		t.Errorf("expected non-400 for empty body, got 400: %s", w.Body.String())
	}
}

func TestHandleSolve_UnknownSolver(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"solver": "nonexistent"}`
	req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleSolve(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for unknown solver, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSolve_TabNotFound(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"tabId": "nonexistent"}`
	req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleSolve(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404 for bad tab, got %d", w.Code)
	}
}

func TestHandleSolve_AutoDetect(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"maxAttempts": 1}`
	req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleSolve(w, req)

	// With a mock chromedp context the solver may fail inside chromedp.Run,
	// but the handler should not panic.  Accept 200 (no challenge on blank
	// page) or 500 (CDP error with mock context).
	if w.Code != 200 && w.Code != 500 {
		t.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleTabSolve(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	body := `{"maxAttempts": 1}`
	req := httptest.NewRequest("POST", "/tabs/tab1/solve", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 && w.Code != 500 {
		t.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSolve_NamedSolver(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	body := `{"solver": "cloudflare", "maxAttempts": 1}`
	req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	h.HandleSolve(w, req)

	if w.Code != 200 && w.Code != 500 {
		t.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSolve_PathSolver(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	body := `{"maxAttempts": 1}`
	req := httptest.NewRequest("POST", "/solve/cloudflare", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 && w.Code != 500 {
		t.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSolve_PathUnknownSolver(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	body := `{}`
	req := httptest.NewRequest("POST", "/solve/bogus", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for unknown path solver, got %d: %s", w.Code, w.Body.String())
	}
}

// Verify solver.Names includes cloudflare (registered by bridge init).
func TestCloudflareSolverRegistered(t *testing.T) {
	names := solver.Names()
	found := false
	for _, n := range names {
		if n == "cloudflare" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cloudflare solver not registered: %v", names)
	}
}

func TestDeriveHumanHandoff(t *testing.T) {
	tests := []struct {
		name       string
		result     *solver.Result
		wantNeeded bool
		wantReason string
	}{
		{
			name:       "solved result does not require handoff",
			result:     &solver.Result{Solved: true, ChallengeType: "turnstile"},
			wantNeeded: false,
			wantReason: "",
		},
		{
			name:       "captcha challenge requires handoff",
			result:     &solver.Result{Solved: false, ChallengeType: "cloudflare-turnstile"},
			wantNeeded: true,
			wantReason: "challenge_requires_manual_intervention",
		},
		{
			name:       "credential gate requires handoff",
			result:     &solver.Result{Solved: false, ChallengeType: "login"},
			wantNeeded: true,
			wantReason: "credentials_required",
		},
		{
			name:       "title heuristics detect credentials requirement",
			result:     &solver.Result{Solved: false, Title: "Sign In - Example"},
			wantNeeded: true,
			wantReason: "credentials_required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNeeded, gotReason := deriveHumanHandoff(tt.result)
			if gotNeeded != tt.wantNeeded || gotReason != tt.wantReason {
				t.Fatalf("deriveHumanHandoff() = (%v, %q), want (%v, %q)", gotNeeded, gotReason, tt.wantNeeded, tt.wantReason)
			}
		})
	}
}

func TestHandleSolve_AndTabSolve_IncludeHandoffFields(t *testing.T) {
	name := "test-handoff-static"
	if err := solver.Register(name, &testStaticSolver{
		name: name,
		result: &solver.Result{
			Solver:        name,
			Solved:        false,
			ChallengeType: "turnstile",
			Attempts:      1,
			Title:         "Just a moment...",
		},
	}); err != nil {
		t.Fatalf("register test solver: %v", err)
	}
	t.Cleanup(func() { solver.Unregister(name) })

	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	t.Run("solve route", func(t *testing.T) {
		body := fmt.Sprintf(`{"solver": %q, "maxAttempts": 1}`, name)
		req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte(body)))
		w := httptest.NewRecorder()
		h.HandleSolve(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if _, ok := resp["needsHumanHandoff"]; !ok {
			t.Fatalf("expected needsHumanHandoff field in response: %#v", resp)
		}
		if _, ok := resp["handoffReason"]; !ok {
			t.Fatalf("expected handoffReason field in response: %#v", resp)
		}
		if needed, _ := resp["needsHumanHandoff"].(bool); !needed {
			t.Fatalf("expected needsHumanHandoff=true, got: %#v", resp["needsHumanHandoff"])
		}
		if reason, _ := resp["handoffReason"].(string); reason != "challenge_requires_manual_intervention" {
			t.Fatalf("expected handoffReason=challenge_requires_manual_intervention, got: %#v", resp["handoffReason"])
		}
	})

	t.Run("tab solve route", func(t *testing.T) {
		mux := http.NewServeMux()
		h.RegisterRoutes(mux, nil)

		body := fmt.Sprintf(`{"solver": %q, "maxAttempts": 1}`, name)
		req := httptest.NewRequest("POST", "/tabs/tab1/solve", bytes.NewReader([]byte(body)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if _, ok := resp["needsHumanHandoff"]; !ok {
			t.Fatalf("expected needsHumanHandoff field in response: %#v", resp)
		}
		if _, ok := resp["handoffReason"]; !ok {
			t.Fatalf("expected handoffReason field in response: %#v", resp)
		}
	})
}
