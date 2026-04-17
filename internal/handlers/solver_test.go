package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	coreautosolver "github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/config"
)

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

	foundCloudflare := false
	foundSemantic := false
	for _, s := range solvers {
		if s == "cloudflare" {
			foundCloudflare = true
		}
		if s == "semantic" {
			foundSemantic = true
		}
	}
	if !foundCloudflare {
		t.Errorf("expected cloudflare in solvers list, got %v", solvers)
	}
	if !foundSemantic {
		t.Errorf("expected semantic in solvers list, got %v", solvers)
	}
}

func TestHandleAutoSolverConfig(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		AutoSolver: config.AutoSolverConfig{
			Enabled:           true,
			AutoTrigger:       true,
			TriggerOnNavigate: false,
			TriggerOnAction:   true,
			MaxAttempts:       5,
			SolverTimeoutSec:  42,
			RetryBaseDelayMs:  200,
			RetryMaxDelayMs:   1200,
			Solvers:           []string{"cloudflare", "semantic", "jschallenge"},
			LLMProvider:       "openai",
			LLMFallback:       true,
		},
	}, nil, nil, nil)

	req := httptest.NewRequest("GET", "/config/autosolver", nil)
	w := httptest.NewRecorder()
	h.HandleAutoSolverConfig(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got, ok := resp["enabled"].(bool); !ok || !got {
		t.Fatalf("enabled = %v, want true", resp["enabled"])
	}
	if got, ok := resp["triggerOnNavigate"].(bool); !ok || got {
		t.Fatalf("triggerOnNavigate = %v, want false", resp["triggerOnNavigate"])
	}
	if got, ok := resp["solverTimeoutSec"].(float64); !ok || int(got) != 42 {
		t.Fatalf("solverTimeoutSec = %v, want 42", resp["solverTimeoutSec"])
	}
	if got, ok := resp["llmProvider"].(string); !ok || got != "openai" {
		t.Fatalf("llmProvider = %v, want openai", resp["llmProvider"])
	}
	if got, ok := resp["solvers"].([]any); !ok || len(got) == 0 {
		t.Fatalf("solvers = %v, want non-empty array", resp["solvers"])
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

// Verify the HTTP-exposed solver list includes cloudflare.
func TestCloudflareSolverRegistered(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	names := h.availableAutoSolverNames()
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

func TestDeriveChallengeType_NilPage(t *testing.T) {
	result := &coreautosolver.Result{Intent: coreautosolver.IntentCaptcha}
	if got := deriveChallengeType(result, nil); got != "" {
		t.Fatalf("deriveChallengeType(nil page) = %q, want empty string", got)
	}
}
