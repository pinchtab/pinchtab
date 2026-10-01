package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	coreautosolver "github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/autosolver/catalog"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
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
	found := slices.Contains(names, "cloudflare")
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

// Every solver name has an owner — a solver type's Name(), or the exported constant
// for the semantic stage — and this package must spell all of them through it. The
// banned set is DERIVED from catalog.Names(), so a solver added later is covered here
// without anyone remembering to extend a list, and the scan covers every non-test file
// in the package rather than the one file that happened to be dirty: a guard reading a
// single file reports the rule as enforced while its siblings go unchecked.
func TestNoHandlerSpellsASolverNameAsALiteral(t *testing.T) {
	names := catalog.Names()
	if len(names) < 2 {
		t.Fatalf("catalog.Names() returned %v; the banned set is derived from it, so this guard would check almost nothing", names)
	}

	referencing := 0
	for name, body := range srccensus.Load(t, ".", 80).Sources() {
		if strings.Contains(body, "coreautosolver.SemanticSolverName") ||
			strings.Contains(body, "coreautosolver.CapsolverSolverName") ||
			strings.Contains(body, "coreautosolver.TwoCaptchaSolverName") {
			referencing++
		}
		for _, solver := range names {
			if strings.Contains(body, `"`+solver+`"`) {
				t.Errorf("%s spells the solver name %q as a literal; use the constant that owns it (internal/autosolver) so the two cannot drift", name, solver)
			}
		}
	}

	if referencing == 0 {
		t.Error("no file in this package references a solver-name constant; the ban above would pass vacuously on a package that stopped naming solvers at all")
	}
}

func solveErrorPayload(t *testing.T, w *httptest.ResponseRecorder) (code, message string) {
	t.Helper()
	var payload struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error payload %q: %v", w.Body.String(), err)
	}
	return payload.Code, payload.Error
}

// capsolver with no API key is a known solver whose key is unset, not a typo.
// Config validation accepts that exact name (TestValidateFileConfig_KnownSolverNamesAccepted,
// row "key-gated without any key configured"), so the API must not call it unknown.
func TestHandleSolveKeylessGatedSolverNamesTheMissingKey(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	restore := coreautosolver.SetKeyGatedSolversForTest([]coreautosolver.KeyGatedSolver{
		{Name: "fakegated", ConfigKey: "autoSolver.external.fakeKey"},
	})
	defer restore()

	for _, gated := range coreautosolver.KeyGatedSolvers() {
		t.Run(gated.Name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte(`{"solver": "`+gated.Name+`"}`)))
			w := httptest.NewRecorder()
			h.HandleSolve(w, req)

			if w.Code != 400 {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
			code, message := solveErrorPayload(t, w)
			if code != "solver_key_missing" {
				t.Errorf("code = %q, want solver_key_missing — a keyless known solver is not an unknown name", code)
			}
			if !strings.Contains(message, gated.ConfigKey) {
				t.Errorf("message %q does not name the missing key %q", message, gated.ConfigKey)
			}
		})
	}
}

// The misspelling must keep the old code and keep listing what is available.
func TestHandleSolveMisspelledSolverStaysUnknown(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte(`{"solver": "cloudlfare"}`)))
	w := httptest.NewRecorder()
	h.HandleSolve(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	code, message := solveErrorPayload(t, w)
	if code != "unknown_solver" {
		t.Errorf("code = %q, want unknown_solver", code)
	}
	if !strings.Contains(message, "cloudlfare") {
		t.Errorf("message %q does not name the rejected value", message)
	}
	for _, available := range h.availableAutoSolverNames() {
		if !strings.Contains(message, available) {
			t.Errorf("message %q omits available solver %q", message, available)
		}
	}
}

// Neither rejection may run a solver. Asserted by ordering rather than status
// alone: this bridge cannot resolve a tab, so any path that got as far as
// resolving one — let alone solving — would answer 404 the way
// TestHandleSolve_TabNotFound does. A 400 proves the reject came first.
func TestHandleSolveRejectionPrecedesAnySolveWork(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)

	for _, name := range []string{coreautosolver.CapsolverSolverName, "cloudlfare"} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte(`{"solver": "`+name+`"}`)))
			w := httptest.NewRecorder()
			h.HandleSolve(w, req)

			if w.Code == 404 {
				t.Fatalf("got 404: the tab was resolved before the solver name was rejected")
			}
			if w.Code != 400 {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleSolvePlannedSolverIsNotImplementedEvenWithAKey(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AutoSolver: config.AutoSolverConfig{
		CapsolverKey:  "k",
		TwoCaptchaKey: "k",
	}}, nil, nil, nil)

	for _, gated := range coreautosolver.KeyGatedSolvers() {
		if !gated.Planned {
			continue
		}
		t.Run(gated.Name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte(`{"solver": "`+gated.Name+`"}`)))
			w := httptest.NewRecorder()
			h.HandleSolve(w, req)

			if w.Code != 400 {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
			if code, _ := solveErrorPayload(t, w); code != "solver_not_implemented" {
				t.Errorf("code = %q, want solver_not_implemented", code)
			}
		})
	}
}
