package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/routes"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// With the scheduler off the family used to be simply unregistered, so a caller —
// including a session holding the tasks grant — got a bare 404 that cannot be told
// apart from an endpoint that does not exist. The family IS catalogued and appears
// in /openapi.json, so a client that discovered it there is owed an answer naming
// the setting that would enable it.
func TestADisabledSchedulerRefusesWithTheSettingRatherThanA404(t *testing.T) {
	mux := http.NewServeMux()
	registerDisabledSchedulerRoutes(mux)

	if len(routes.SchedulerEndpoints()) < 5 {
		t.Fatalf("the scheduler family holds %d routes; this test would prove little", len(routes.SchedulerEndpoints()))
	}

	for _, ep := range routes.SchedulerEndpoints() {
		t.Run(ep.Route(), func(t *testing.T) {
			path := strings.ReplaceAll(ep.Path, "{id}", "task1")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(ep.Method, path, nil))

			if w.Code == http.StatusNotFound {
				t.Fatalf("answered 404: a disabled subsystem is indistinguishable from one that was never built")
			}
			var body struct {
				Code    string         `json:"code"`
				Error   string         `json:"error"`
				Details map[string]any `json:"details"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not the JSON error envelope (%v): %s", err, w.Body.String())
			}
			if body.Code != routes.SchedulerDisabled {
				t.Errorf("code = %q, want %q", body.Code, routes.SchedulerDisabled)
			}
			if body.Details["setting"] != routes.SchedulerSetting {
				t.Errorf("details.setting = %v, want %q; the caller cannot enable what the refusal does not name", body.Details["setting"], routes.SchedulerSetting)
			}
			if !strings.Contains(body.Error, routes.SchedulerSetting) {
				t.Errorf("message %q does not name the setting", body.Error)
			}
		})
	}
}

// The refusal is the capability gates' own vocabulary, not a second gating style:
// an agent that already handles evaluate_disabled handles this one unchanged.
func TestTheSchedulerRefusalReusesTheCapabilityGateShape(t *testing.T) {
	mux := http.NewServeMux()
	registerDisabledSchedulerRoutes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tasks", nil))

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403, the status every disabled endpoint answers", w.Code)
	}
	var body struct {
		Details map[string]any `json:"details"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	for _, key := range []string{"setting", "remedy", "hint"} {
		if _, ok := body.Details[key]; !ok {
			t.Errorf("details carries no %q; the capability gates publish all three: %v", key, body.Details)
		}
	}
}

// Driving registerDisabledSchedulerRoutes proves what it answers, not that anything
// calls it — and RunDashboard binds a listener, launches a browser and blocks on
// signals, so it cannot be driven from a unit test. The wiring is pinned at the
// source instead, the same guard the not-found envelope carries, because deleting
// the else branch would put the family back on a bare 404 with every assertion
// above still green.
func TestRunDashboardRegistersTheDisabledSchedulerRoutes(t *testing.T) {
	pkg := srccensus.Load(t, ".", 4)

	fn, ok := pkg.Func("RunDashboard")
	if !ok {
		t.Fatal("RunDashboard not found; if it was renamed, re-point this guard at the new entry point rather than deleting it")
	}
	inside := 0
	for _, site := range pkg.Calls(t, "registerDisabledSchedulerRoutes") {
		if pkg.Contains(fn, site) {
			inside++
		}
	}
	if inside == 0 {
		t.Error("RunDashboard never registers the disabled scheduler routes, so with the scheduler off the catalogued family answers a bare 404 that a caller cannot tell from an endpoint that does not exist")
	}
}
