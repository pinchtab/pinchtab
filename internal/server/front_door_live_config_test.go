package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/session"
)

const frontDoorLiveToken = "front-door-live-token"

// frontDoor stands up RunDashboard's wiring: one publication point reaching the
// orchestrator, the config API and the front-door chain, over a real config file
// the save writes to. The chain is the real one — an isolated middleware already
// behaved correctly, which is why nothing caught the boot pointer.
type frontDoor struct {
	handler       http.Handler
	api           *dashboard.ConfigAPI
	sessions      *browsersession.Manager
	agentSessions *session.Store
	live          *config.Live
}

func newFrontDoor(t *testing.T, boot func(*config.FileConfig)) frontDoor {
	t.Helper()

	fc := config.DefaultFileConfig()
	fc.Server.Token = frontDoorLiveToken
	fc.Server.StateDir = t.TempDir()
	if boot != nil {
		boot(&fc)
	}
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Load()
	orch := orchestrator.NewOrchestrator(t.TempDir())
	orch.ApplyRuntimeConfig(cfg)
	live := orch.LiveConfig()

	sessions := browsersession.NewManager(dashboard.BrowserSessionConfig(cfg))
	agentSessions := session.NewStore(dashboard.AgentSessionConfig(cfg, dashboard.AgentSessionStatePath(cfg)))
	api := dashboard.NewConfigAPI(live, orch, nil, orch, nil, "test", time.Now())
	api.SetSessionManager(sessions)
	api.SetAgentSessionStore(agentSessions)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/tabs", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tabs":[]}`))
	})
	// A second route in a different grant group, so a scoped session can be shown
	// reaching one and refused the other through the real chain.
	mux.HandleFunc("/clipboard/read", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"text":""}`))
	})
	api.RegisterHandlers(mux)
	if agentSessions.Enabled() {
		dashboard.NewSessionAPI(agentSessions, cfg.BrowsersAvailable).RegisterHandlers(mux)
	} else {
		RegisterSessionsDisabled(mux, agentSessions.DisabledBy())
	}

	return frontDoor{
		handler:       FrontDoorHandler(live, nil, sessions, agentSessions, mux),
		api:           api,
		sessions:      sessions,
		agentSessions: agentSessions,
		live:          live,
	}
}

// save drives a real PUT /api/config through the config API, the same way the
// dashboard does, and asserts the server reported it as applied rather than as
// needing a restart — which is the claim this card says the front door broke.
func (f frontDoor) save(t *testing.T, mutate func(*config.FileConfig)) {
	t.Helper()

	if reasons := f.saveReportingReasons(t, mutate); len(reasons) > 0 {
		t.Fatalf("the save reported restartRequired=true (%v); this card must not enlarge that list", reasons)
	}
}

// saveReportingReasons drives the same real PUT and hands back the restart
// reasons the server reported, for the one transition that has any.
func (f frontDoor) saveReportingReasons(t *testing.T, mutate func(*config.FileConfig)) []string {
	t.Helper()

	current, _, err := config.LoadFileConfig()
	if err != nil || current == nil {
		t.Fatalf("load current config: %v", err)
	}
	payload := *current
	payload.Server.Token = ""
	mutate(&payload)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	f.api.HandlePutConfig(w, httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("save returned %d: %s", w.Code, w.Body.String())
	}

	var env struct {
		RestartRequired bool     `json:"restartRequired"`
		RestartReasons  []string `json:"restartReasons"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode save envelope: %v", err)
	}
	if env.RestartRequired && len(env.RestartReasons) == 0 {
		t.Fatal("the save reported restartRequired=true with no reason naming what is frozen")
	}
	return env.RestartReasons
}

func proxiedPreflight() *http.Request {
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "https://proxy.example")
	req.Header.Set("X-Forwarded-Host", "proxy.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	return req
}

// server.trustProxyHeaders decides whether X-Forwarded-* is believed for the
// cookie-origin and CORS same-origin checks. It sits in the Server section, which
// restartReasonsFor does not flag, so the save reports applied — and with the boot
// pointer captured in the chain it went on being enforced at its boot value for
// the life of the process, in either direction.
func TestSavingTrustProxyHeadersTakesEffectWithoutARestart(t *testing.T) {
	f := newFrontDoor(t, func(fc *config.FileConfig) {
		trust := false
		fc.Server.TrustProxyHeaders = &trust
	})

	before := httptest.NewRecorder()
	f.handler.ServeHTTP(before, proxiedPreflight())
	if before.Code != http.StatusForbidden {
		t.Fatalf("with trustProxyHeaders off the proxied origin should be refused, got %d: %s", before.Code, before.Body.String())
	}

	f.save(t, func(fc *config.FileConfig) {
		trust := true
		fc.Server.TrustProxyHeaders = &trust
	})

	after := httptest.NewRecorder()
	f.handler.ServeHTTP(after, proxiedPreflight())
	if after.Code != http.StatusNoContent {
		t.Fatalf("after the save the proxied origin is still refused (%d: %s); the chain is serving the boot config",
			after.Code, after.Body.String())
	}
	if got := after.Header().Get("Access-Control-Allow-Origin"); got != "https://proxy.example" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the forwarded origin echoed back", got)
	}
}

// The field the card missed, and the reason the direction is what it is: an
// authorization control on PUT /api/config and POST /shutdown. An operator who
// turns elevation ON is told the save applied; with the boot pointer the front
// door went on not requiring it.
func TestSavingRequireElevationTakesEffectWithoutARestart(t *testing.T) {
	f := newFrontDoor(t, func(fc *config.FileConfig) {
		require := false
		fc.Sessions.Dashboard.RequireElevation = &require
	})

	sessionID, err := f.sessions.Create(frontDoorLiveToken)
	if err != nil {
		t.Fatal(err)
	}
	covered := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Origin", "http://example.com")
		req.Host = "example.com"
		req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: sessionID})
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		return w
	}

	if got := covered().Code; got == http.StatusForbidden {
		t.Fatalf("elevation was demanded before it was turned on (%d); the fixture does not start from the state this test needs", got)
	}

	f.save(t, func(fc *config.FileConfig) {
		require := true
		fc.Sessions.Dashboard.RequireElevation = &require
	})

	after := covered()
	if after.Code != http.StatusForbidden {
		t.Fatalf("after saving requireElevation=true the request was not challenged (%d: %s); the chain is serving the boot config",
			after.Code, after.Body.String())
	}
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(after.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode challenge: %v (body %s)", err, after.Body.String())
	}
	if resp.Code != "elevation_required" {
		t.Errorf("code = %q, want elevation_required — the refusal must name the control that produced it", resp.Code)
	}
}

// BackgroundMarker is the one field the chain reads per request that no save can
// carry: cmd_server sets it on the runtime config at boot and no config file has a
// place for it. Reading it through the Live therefore depends on every published
// value carrying it forward, which is a property of the clone rather than of this
// chain — so if it ever stops holding, `pinchtab server` loses its own health probe
// and nothing else says so.
//
// The assertion is on what the auth chain DECIDES, not on the response: this
// fixture registers no /health/background route, so a probe the chain admits ends
// at the mux as a 404. Admitted-or-refused is the property; the route existing is
// a different one.
func TestABackgroundProbeIsStillAdmittedAfterASave(t *testing.T) {
	const marker = "background-marker-under-test"

	f := newFrontDoor(t, nil)
	published := config.CloneRuntimeConfig(f.live.Get())
	published.BackgroundMarker = marker
	f.live.Publish(published)

	admitted := func(header string) bool {
		req := httptest.NewRequest(http.MethodGet, "/health/background", nil)
		if header != "" {
			req.Header.Set("PinchTab-Background-Marker", header)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		return w.Code != http.StatusUnauthorized
	}

	// Without the marker the chain refuses, so being admitted after the save
	// cannot come from the endpoint being reachable by anyone.
	if admitted("") {
		t.Fatal("an unmarked background probe was admitted; this endpoint must not be open")
	}
	if !admitted(marker) {
		t.Fatal("the marked probe was refused before any save, so this test never reached the state it is about")
	}

	f.save(t, func(fc *config.FileConfig) {
		trust := true
		fc.Server.TrustProxyHeaders = &trust
	})

	if !admitted(marker) {
		t.Error("the marked probe was refused after a save; the published value dropped BackgroundMarker")
	}
	if got := f.live.Get().BackgroundMarker; got != marker {
		t.Errorf("BackgroundMarker after a save = %q, want %q", got, marker)
	}
}
