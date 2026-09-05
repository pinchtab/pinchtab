package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) (code, message string) {
	t.Helper()
	var resp struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body is not the JSON error envelope (%v): %s", err, w.Body.String())
	}
	return resp.Code, resp.Error
}

// An unrouted path used to escape the JSON contract as net/http's plain-text
// "404 page not found". Every other failure on the same server answers the
// envelope, so an unrouted one must too.
func TestUnroutedPathAnswersTheJSONEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {})
	handler := notFoundEnvelope(mux)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if strings.Contains(w.Body.String(), "404 page not found") {
		t.Fatalf("still the bare mux 404, which breaks the JSON contract: %s", w.Body.String())
	}
	code, message := decodeEnvelope(t, w)
	if code != "not_found" {
		t.Errorf("code = %q, want not_found", code)
	}
	if message == "" {
		t.Error("the not-found refusal carries no message")
	}
}

// The generic fallback is a floor, not a replacement: a registered coded refusal
// still matches a pattern and must keep winning. The bridge-mode session family
// is the codebase's own example of a known-absent family answered with a code
// and a remedy, and it must not be flattened into the generic not_found body.
func TestCodedRefusalsWinOverTheGenericFallback(t *testing.T) {
	mux := http.NewServeMux()
	RegisterSessionsUnavailableInBridgeMode(mux)
	handler := notFoundEnvelope(mux)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/sessions", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	code, _ := decodeEnvelope(t, w)
	if code != CodeSessionsUnavailableInBridgeMode {
		t.Fatalf("code = %q, want %q — the coded refusal was flattened into the generic fallback", code, CodeSessionsUnavailableInBridgeMode)
	}
}

// notFoundEnvelope is unit-tested against a bare mux, but that cannot show either
// MODE wraps its mux in it — and a mode that stops wrapping is silently back to the
// plain-text "404 page not found" this card exists to remove. Neither entry point is
// drivable from a unit test: RunDashboard and RunBridgeServer bind a listener, launch
// a browser and block on signals. So the wiring is pinned at the source, inside the
// enclosing function, following the same guard the session family already carries.
func TestBothModesWrapTheMuxInTheNotFoundEnvelope(t *testing.T) {
	pkg := srccensus.Load(t, ".", 4)

	for _, mode := range []string{"RunDashboard", "RunBridgeServer"} {
		t.Run(mode, func(t *testing.T) {
			fn, ok := pkg.Func(mode)
			if !ok {
				t.Fatalf("%s not found in %s; if it was renamed, re-point this guard at the new entry point rather than deleting it", mode, pkg.Dir())
			}
			inside := 0
			for _, site := range pkg.Calls(t, "notFoundEnvelope") {
				if pkg.Contains(fn, site) {
					inside++
				}
			}
			if inside == 0 {
				t.Errorf("%s never wraps its mux in notFoundEnvelope, so an unrouted path on that front answers net/http's bare plain-text 404 instead of the JSON envelope", mode)
			}
		})
	}
}

// A matched route serves normally; the wrapper only fires when nothing is routed.
func TestMatchedRouteIsUntouched(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("live"))
	})
	handler := notFoundEnvelope(mux)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code != http.StatusOK || w.Body.String() != "live" {
		t.Fatalf("matched route not served: status=%d body=%q", w.Code, w.Body.String())
	}
}

// A request for a real path under an unregistered method is not "no such path":
// the wrapper preserves the 405 the mux would have written rather than reporting
// a 404, while still answering the JSON envelope.
func TestWrongMethodStaysA405Envelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {})
	handler := notFoundEnvelope(mux)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/openapi.json", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
	if strings.Contains(w.Body.String(), "method not allowed\n") {
		t.Fatalf("still the bare mux 405 plain text: %s", w.Body.String())
	}
	code, _ := decodeEnvelope(t, w)
	if code != "method_not_allowed" {
		t.Errorf("code = %q, want method_not_allowed", code)
	}
}

// The wrapper used to dispatch the handler http.ServeMux.Handler returned, which
// reports WHICH pattern matched but leaves the request's wildcard matches unset.
// Every {id} route on both front doors therefore saw an empty id. A literal-path
// fixture cannot see that, so the mux here registers a wildcard.
func TestWildcardPathValuesSurviveTheEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tabs/{id}/x", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.PathValue("id")))
	})
	handler := notFoundEnvelope(mux)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tabs/TAB123/x", nil))

	if got := w.Body.String(); got != "TAB123" {
		t.Fatalf("r.PathValue(\"id\") = %q, want TAB123: the wrapper must route the request the way mux.ServeHTTP would, wildcards included", got)
	}
}

// Both front doors wrap their own mux, so the loss has to be pinned on each
// registration rather than on a fixture mux. Every assertion here is POSITIVE —
// the named id reached the handler — because a refusal cannot tell "correctly
// refused" from "the id never arrived": with an empty id every tab and every
// instance is unknown, which is why the e2e guards asserting refusals stayed
// green through the whole outage.
func TestBothFrontDoorsRouteATabScopedRequestWithItsID(t *testing.T) {
	t.Run("full server", func(t *testing.T) {
		mux := http.NewServeMux()
		(&orchestrator.Orchestrator{}).RegisterHandlers(mux)

		// The shape internal/mcp sends whenever an agent supplies tabId.
		w := httptest.NewRecorder()
		notFoundEnvelope(mux).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tabs/TAB123/reload", strings.NewReader("{}")))

		_, message := decodeEnvelope(t, w)
		if !strings.Contains(message, "TAB123") {
			t.Fatalf("the proxy answered %q; the tab id never reached it, so every tab-scoped route on this front door is dead", message)
		}
	})

	t.Run("bridge", func(t *testing.T) {
		mux := http.NewServeMux()
		(&handlers.Handlers{Config: config.Load()}).RegisterRoutes(mux, func() {})

		w := httptest.NewRecorder()
		notFoundEnvelope(mux).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tabs/TAB123/metrics", nil))

		_, message := decodeEnvelope(t, w)
		if w.Code == http.StatusBadRequest {
			t.Fatalf("the bridge answered 400 %q; that is the empty-id gate, so the id never reached the handler", message)
		}
		if !strings.Contains(message, "bridge not initialized") {
			t.Fatalf("message = %q, want the handler's own answer past the id gate", message)
		}
	})
}

// An instance-scoped id is lost the same way, and this is the pair the CLI's
// instance verbs and the e2e readiness latch poll.
func TestInstanceScopedRequestsCarryTheirID(t *testing.T) {
	mux := http.NewServeMux()
	(&orchestrator.Orchestrator{}).RegisterHandlers(mux)
	handler := notFoundEnvelope(mux)

	for _, probe := range []struct{ method, path string }{
		{http.MethodGet, "/instances/inst_abc"},
		{http.MethodPost, "/instances/inst_abc/stop"},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(probe.method, probe.path, strings.NewReader("{}")))
		if _, message := decodeEnvelope(t, w); !strings.Contains(message, "inst_abc") {
			t.Errorf("%s %s answered %q, naming no instance: the id was dropped", probe.method, probe.path, message)
		}
	}
}
