package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/routes"
)

// TestDefaultProxyShorthandsAreCatalogRoutes guards against the default-proxy
// allowlist drifting from the shared route catalog: every forwarded route must
// be a real catalog route (or the management GET /tabs handled separately), so a
// rename/removal/typo can never leave the landing proxy forwarding a dead route.
func TestDefaultProxyShorthandsAreCatalogRoutes(t *testing.T) {
	catalog := map[string]bool{}
	for _, ep := range routes.Core() {
		catalog[ep.Route()] = true
		if ep.TabScoped {
			catalog[ep.TabRoute()] = true
		}
	}
	// GET /tabs is a management route registered outside the catalog (it has a
	// bespoke empty-instances response); allow it explicitly.
	managementAllow := map[string]bool{"GET /tabs": true}

	for _, route := range DefaultProxyShorthands {
		if !catalog[route] && !managementAllow[route] {
			t.Errorf("default-proxy route %q is not a catalog route", route)
		}
	}
	if managementAllow["GET /tabs"] && catalog["GET /tabs"] {
		t.Error("GET /tabs is now a catalog route; drop it from managementAllow")
	}
}

func TestHealthProbeReason(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "health reason", body: `{"reason":"browser initialization failed","status":"error"}`, want: "browser initialization failed"},
		{name: "trimmed", body: `{"reason":"  spaced out  "}`, want: "spaced out"},
		{name: "no reason field", body: `{"status":"error"}`, want: ""},
		{name: "not json", body: `<html>nginx</html>`, want: ""},
		{name: "empty body", body: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := HealthProbe{Reachable: true, StatusCode: 503, Body: []byte(tt.body)}
			if got := probe.Reason(); got != tt.want {
				t.Fatalf("Reason() = %q, want %q", got, tt.want)
			}
		})
	}
}

// CheckPinchTabRunning answers a narrower question than the CLI ensure path:
// only a 200 counts, so a listener answering anything else is not "running".
func TestCheckPinchTabRunningRequiresExactly200(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{status: http.StatusOK, want: true},
		{status: http.StatusNoContent, want: false},
		{status: http.StatusUnauthorized, want: false},
		{status: http.StatusServiceUnavailable, want: false},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer ts.Close()

			port := ts.URL[strings.LastIndex(ts.URL, ":")+1:]
			if got := CheckPinchTabRunning(port, ""); got != tt.want {
				t.Fatalf("CheckPinchTabRunning(status %d) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestProbeHealthWithTokenCarriesAuthorizationAndBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"reason":"no browser","status":"error"}`))
	}))
	defer ts.Close()

	probe := ProbeHealthWithToken(ts.URL+"/health", time.Second, "tok")
	if !probe.Reachable || probe.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("probe = %+v, want a reachable 503", probe)
	}
	if probe.Reason() != "no browser" {
		t.Fatalf("Reason() = %q, want the server's reason", probe.Reason())
	}

	unreachable := ProbeHealthWithToken("http://127.0.0.1:1/health", time.Second, "tok")
	if unreachable.Reachable {
		t.Fatalf("probe = %+v, want unreachable", unreachable)
	}
}
