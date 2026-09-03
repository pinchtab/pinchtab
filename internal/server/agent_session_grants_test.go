package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/session"
)

// createSessionWithGrants drives the real POST /sessions through the real chain
// with the server token, which is the only way a scope can be requested. Nothing
// here calls SetGrants: the point of this test is the ingress that did not exist.
func (f frontDoor) createSessionWithGrants(t *testing.T, grants []string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"agentId": "grant-test", "grants": grants})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+frontDoorLiveToken)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		return w, ""
	}
	var created struct {
		Token string `json:"sessionToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created session: %v (body %s)", err, w.Body.String())
	}
	return w, created.Token
}

func (f frontDoor) requestAs(t *testing.T, token, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Session "+token)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}

// The whole capability, end to end and over HTTP: a session asked for browse and
// only browse, and the front door holds it to that. Every layer used to agree that
// it could not — the DTO dropped the key, so eleven matchers never ran.
func TestASessionScopedAtCreationIsHeldToItsGrants(t *testing.T) {
	f := newFrontDoor(t, nil)

	_, token := f.createSessionWithGrants(t, []string{"browse"})
	if token == "" {
		t.Fatal("no session token came back")
	}

	if inside := f.requestAs(t, token, http.MethodGet, "/tabs"); inside.Code != http.StatusOK {
		t.Fatalf("a browse session was refused GET /tabs (%d: %s)", inside.Code, inside.Body.String())
	}

	outside := f.requestAs(t, token, http.MethodGet, "/clipboard/read")
	if outside.Code != http.StatusForbidden {
		t.Fatalf("a browse-only session reached GET /clipboard/read (%d: %s)", outside.Code, outside.Body.String())
	}
	var refusal struct {
		Code    string `json:"code"`
		Details struct {
			Hint   string `json:"hint"`
			Remedy string `json:"remedy"`
		} `json:"details"`
	}
	if err := json.Unmarshal(outside.Body.Bytes(), &refusal); err != nil {
		t.Fatalf("decode refusal: %v (body %s)", err, outside.Body.String())
	}
	if refusal.Code != "session_scope_forbidden" {
		t.Errorf("code = %q, want session_scope_forbidden", refusal.Code)
	}
	for _, want := range []string{"browse", "clipboard"} {
		if !strings.Contains(refusal.Details.Hint, want) {
			t.Errorf("hint = %q, want it to name %q", refusal.Details.Hint, want)
		}
	}
	if !strings.Contains(refusal.Details.Remedy, "--grant") {
		t.Errorf("remedy = %q, want the command that creates a session carrying the grant", refusal.Details.Remedy)
	}

	// The control: an unscoped session reaches the same route, so the refusal above
	// is the grant and not the route being unreachable.
	_, open := f.createSessionWithGrants(t, nil)
	if w := f.requestAs(t, open, http.MethodGet, "/clipboard/read"); w.Code != http.StatusOK {
		t.Errorf("an unscoped session was refused GET /clipboard/read (%d: %s)", w.Code, w.Body.String())
	}
}

// The admin denylist answers before any grant, so its refusal must name the other
// cause: a different credential, not a different session.
func TestAnAdminRouteRefusalNamesTheCredentialRatherThanAGrant(t *testing.T) {
	f := newFrontDoor(t, nil)
	_, token := f.createSessionWithGrants(t, []string{"browse"})

	w := f.requestAs(t, token, http.MethodGet, "/api/config")
	if w.Code != http.StatusForbidden {
		t.Fatalf("a session reached GET /api/config (%d: %s)", w.Code, w.Body.String())
	}
	var refusal struct {
		Details struct {
			Hint   string `json:"hint"`
			Remedy string `json:"remedy"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(refusal.Details.Hint, "PINCHTAB_SESSION") || !strings.Contains(refusal.Details.Hint, "server token") {
		t.Errorf("hint = %q, want it to say which credential arrived and which is needed", refusal.Details.Hint)
	}
	if strings.Contains(refusal.Details.Remedy, "--grant") {
		t.Errorf("the admin refusal prescribes a grant (%q); no grant reaches an admin route", refusal.Details.Remedy)
	}
}

// A mistyped grant is refused at the door with the vocabulary, and no session is
// created — a half-applied scope would be worse than none.
func TestAnUnknownGrantIsRefusedAtCreation(t *testing.T) {
	f := newFrontDoor(t, nil)

	w, _ := f.createSessionWithGrants(t, []string{"brows"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "brows") || !strings.Contains(w.Body.String(), "browse") {
		t.Errorf("the refusal does not name the offender and the vocabulary: %s", w.Body.String())
	}
}

// The store already round-tripped Grants; with an ingress that is finally
// checkable. A scope that evaporated on restart would be a security control that
// silently widens.
func TestGrantsSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/sessions.json"
	cfg := session.Config{Enabled: true, IdleTimeout: 30 * time.Minute, MaxLifetime: 24 * time.Hour, PersistPath: path}

	before := session.NewStore(cfg)
	id, _, err := before.Create("grant-test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !before.SetGrants(id, []string{session.GrantBrowse, session.GrantNetwork}) {
		t.Fatal("the session vanished before it could be scoped")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nothing was persisted (%v), so the reload below would prove nothing", err)
	}

	after := session.NewStore(cfg)
	reloaded, ok := after.Get(id)
	if !ok {
		t.Fatal("the session did not survive the restart")
	}
	if len(reloaded.Grants) != 2 || reloaded.Grants[0] != session.GrantBrowse || reloaded.Grants[1] != session.GrantNetwork {
		t.Errorf("grants after restart = %v, want the pair set before it", reloaded.Grants)
	}
}
