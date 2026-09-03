package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/session"
)

func newTestSessionStore() *session.Store {
	return session.NewStore(session.Config{
		Enabled:     true,
		IdleTimeout: 30 * time.Minute,
		MaxLifetime: 24 * time.Hour,
	})
}

func newTestSessionMux(store *session.Store) *http.ServeMux {
	mux := http.NewServeMux()
	NewSessionAPI(store, nil).RegisterHandlers(mux)
	return mux
}

func decodeSessionResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return m
}

func TestAgentSessionAPI_Create(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"agentId":"agent-1","label":"ci-run"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	resp := decodeSessionResponse(t, w)
	if resp["id"] == "" || resp["id"] == nil {
		t.Fatal("expected id in response")
	}
	if resp["sessionToken"] == "" || resp["sessionToken"] == nil {
		t.Fatal("expected sessionToken in response")
	}
	if resp["agentId"] != "agent-1" {
		t.Fatalf("agentId = %q, want agent-1", resp["agentId"])
	}
	if resp["label"] != "ci-run" {
		t.Fatalf("label = %q, want ci-run", resp["label"])
	}
	if resp["status"] != "active" {
		t.Fatalf("status = %q, want active", resp["status"])
	}
}

func TestAgentSessionAPI_Create_MissingAgentID(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"label":"ci-run"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAgentSessionAPI_Create_InvalidJSON(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("POST", "/sessions", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAgentSessionAPI_List(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	_, _, _ = store.Create("agent-1", "first", "")
	_, _, _ = store.Create("agent-2", "second", "")

	req := httptest.NewRequest("GET", "/sessions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var sessions []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
}

func TestAgentSessionAPI_List_Empty(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("GET", "/sessions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var sessions []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d, want 0", len(sessions))
	}
}

func TestAgentSessionAPI_Get(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	id, _, _ := store.Create("agent-1", "my-session", "")

	req := httptest.NewRequest("GET", "/sessions/"+id, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	resp := decodeSessionResponse(t, w)
	if resp["id"] != id {
		t.Fatalf("id = %q, want %q", resp["id"], id)
	}
	if resp["agentId"] != "agent-1" {
		t.Fatalf("agentId = %q, want agent-1", resp["agentId"])
	}
}

func TestAgentSessionAPI_Get_NotFound(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("GET", "/sessions/ses_doesnotexist", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestAgentSessionAPI_Me(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	sessionID, token, _ := store.Create("agent-1", "my-session", "")
	sess, ok := store.Get(sessionID)
	if !ok || sess == nil {
		t.Fatal("expected session to exist")
	}

	req := httptest.NewRequest("GET", "/sessions/me", nil)
	req.Header.Set("Authorization", "Session "+token)
	req = session.WithSession(req, sess)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	resp := decodeSessionResponse(t, w)
	if resp["agentId"] != "agent-1" {
		t.Fatalf("agentId = %q, want agent-1", resp["agentId"])
	}
}

func TestAgentSessionAPI_Me_RequiresSessionAuth(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("GET", "/sessions/me", nil)
	req.Header.Set("Authorization", "Bearer some-bearer-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (bearer should not work for /me)", w.Code, http.StatusUnauthorized)
	}
}

func TestAgentSessionAPI_Me_InvalidToken(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("GET", "/sessions/me", nil)
	req.Header.Set("Authorization", "Session ses_invalidtoken")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAgentSessionAPI_Revoke(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	id, token, _ := store.Create("agent-1", "", "")

	req := httptest.NewRequest("POST", "/sessions/"+id+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer dashboard-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if sess, ok := store.Authenticate(token); ok || sess != nil {
		t.Fatal("expected token to be invalidated after revoke")
	}
}

func TestAgentSessionAPI_RevokeReturnsRemainingOwnedTabIDs(t *testing.T) {
	store := newTestSessionStore()
	id, _, _ := store.Create("agent-1", "", "")
	api := NewSessionAPI(store, nil)
	api.SetSessionTabSource(func(gotID string) []string {
		if gotID != id {
			t.Fatalf("tab source session id = %q, want %q", gotID, id)
		}
		if _, ok := store.Get(id); !ok {
			t.Fatal("tab source must be read before the session is revoked")
		}
		return []string{"tab-a", "tab-b"}
	})
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	req := httptest.NewRequest("POST", "/sessions/"+id+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer dashboard-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	resp := decodeSessionResponse(t, w)
	tabs, ok := resp["remainingTabIds"].([]any)
	if !ok || len(tabs) != 2 || tabs[0] != "tab-a" || tabs[1] != "tab-b" {
		t.Fatalf("remainingTabIds = %#v, want [tab-a tab-b]", resp["remainingTabIds"])
	}
}

func TestAgentSessionAPI_Revoke_NotFound(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("POST", "/sessions/ses_doesnotexist/revoke", nil)
	req.Header.Set("Authorization", "Bearer dashboard-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// The defect this covers: `session create` returns a token, every id-taking endpoint
// here rejects it, and the refusal said only "session not found" — which reads as
// already-gone, so the caller shrugs and leaves a live session running. Both id-taking
// endpoints are covered, because the same value reaches both by the same mistake.
func TestSupplyingATokenWhereAnIDGoesExplainsTheDifference(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	_, token, _ := store.Create("agent-1", "", "")

	for _, tc := range []struct {
		name string
		req  *http.Request
	}{
		{"revoke", httptest.NewRequest("POST", "/sessions/"+token+"/revoke", nil)},
		{"get", httptest.NewRequest("GET", "/sessions/"+token, nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.Header.Set("Authorization", "Bearer dashboard-token")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, tc.req)

			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
			}
			details := errorDetails(t, w)
			if !strings.Contains(details["hint"], "TOKEN") || !strings.Contains(details["hint"], "not a session id") {
				t.Errorf("hint does not name the id/token distinction: %q", details["hint"])
			}
			// The listing is the remedy because it is the one command that works with
			// nothing else set up; `session info` needs PINCHTAB_SESSION exported, a
			// precondition a remedy cannot state, so it is named in the hint instead.
			if want := "pinchtab session list"; details["remedy"] != want {
				t.Errorf("remedy = %q, want %q — the id is unreachable without a command that lists it", details["remedy"], want)
			}
			if !strings.Contains(details["hint"], "session info") {
				t.Errorf("hint %q does not mention session info, the other way to reach the id", details["hint"])
			}
		})
	}
}

// The path a caller following the product's own instructions actually takes: with
// PINCHTAB_SESSION exported, revoking "$PINCHTAB_SESSION" authenticates AS that session
// and lands on the 403, whose message — "may only revoke their own session" — is
// actively misleading, because this IS their own session named by the wrong value. The
// caller already holds this session's secret, so the remedy can hand them the id itself.
func TestRevokingYourOwnSessionByTokenNamesYourID(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	id, token, _ := store.Create("agent-1", "", "")
	sess, ok := store.Get(id)
	if !ok || sess == nil {
		t.Fatal("expected session to exist")
	}

	req := httptest.NewRequest("POST", "/sessions/"+token+"/revoke", nil)
	req.Header.Set("Authorization", "Session "+token)
	req = session.WithSession(req, sess)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
	details := errorDetails(t, w)
	if !strings.Contains(details["hint"], "TOKEN") {
		t.Errorf("hint does not name the id/token distinction: %q", details["hint"])
	}
	if want := "pinchtab session revoke " + id; details["remedy"] != want {
		t.Errorf("remedy = %q, want %q — the caller holds this session's secret, so the id is the whole remedy", details["remedy"], want)
	}
	if strings.Contains(details["remedy"], token) {
		t.Error("the remedy repeats the token, which is the value that does not work here")
	}
}

// An unknown ID keeps the plain refusal. The hint claims the caller supplied a token,
// so offering it for anything else would be a guess — and a wrong one for the ordinary
// case of a session that really is gone.
func TestAnUnknownIDGetsNoTokenHint(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	// Shaped like an id the store could have minted, but never minted — a session that
	// really is gone, which is the case the bare refusal is right for.
	const id = "ses_0123456789abcdef"
	if !session.LooksLikeID(id) || session.LooksLikeToken(id) {
		t.Fatalf("precondition: %q must read as an id, or this proves nothing", id)
	}

	req := httptest.NewRequest("POST", "/sessions/"+id+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer dashboard-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if _, ok := decodeSessionResponse(t, w)["details"]; ok {
		t.Error("an already-revoked session id was told it had supplied a token")
	}
}

func errorDetails(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	raw, ok := decodeSessionResponse(t, w)["details"].(map[string]any)
	if !ok {
		t.Fatal("the refusal carries no details, so it explains nothing")
	}
	out := map[string]string{}
	for key, value := range raw {
		text, _ := value.(string)
		out[key] = text
	}
	return out
}

func TestAgentSessionAPI_Revoke_SessionOwnerAllowed(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	id, token, _ := store.Create("agent-1", "", "")
	sess, ok := store.Get(id)
	if !ok || sess == nil {
		t.Fatal("expected session to exist")
	}

	req := httptest.NewRequest("POST", "/sessions/"+id+"/revoke", nil)
	req.Header.Set("Authorization", "Session "+token)
	req = session.WithSession(req, sess)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAgentSessionAPI_Revoke_SessionCallerCannotRevokeOtherSession(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	id, token, _ := store.Create("agent-1", "", "")
	sess, ok := store.Get(id)
	if !ok || sess == nil {
		t.Fatal("expected session to exist")
	}
	otherID, _, _ := store.Create("agent-2", "", "")

	req := httptest.NewRequest("POST", "/sessions/"+otherID+"/revoke", nil)
	req.Header.Set("Authorization", "Session "+token)
	req = session.WithSession(req, sess)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAgentSessionAPI_Revoke_RejectsUnauthenticatedCaller(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	id, _, _ := store.Create("agent-1", "", "")

	req := httptest.NewRequest("POST", "/sessions/"+id+"/revoke", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAgentSessionAPI_Create_WithBrowser(t *testing.T) {
	store := newTestSessionStore()
	mux := http.NewServeMux()
	NewSessionAPI(store, []string{"chrome"}).RegisterHandlers(mux)

	req := httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"agentId":"agent-1","browser":"chrome"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	resp := decodeSessionResponse(t, w)
	if resp["browser"] != "chrome" {
		t.Fatalf("browser = %q, want chrome", resp["browser"])
	}

	id, ok := resp["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected id in response")
	}
	sess, found := store.Get(id)
	if !found {
		t.Fatal("expected session to exist in store")
	}
	if sess.Browser != "chrome" {
		t.Fatalf("stored browser = %q, want chrome", sess.Browser)
	}
}

func TestAgentSessionAPI_Create_InvalidBrowser(t *testing.T) {
	store := newTestSessionStore()
	mux := http.NewServeMux()
	NewSessionAPI(store, []string{"chrome"}).RegisterHandlers(mux)

	req := httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"agentId":"agent-1","browser":"invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAgentSessionAPI_RegisterHandlers_NoOpsWhenDisabled(t *testing.T) {
	store := session.NewStore(session.Config{
		Enabled:     false,
		Mode:        "off",
		IdleTimeout: 30 * time.Minute,
		MaxLifetime: 24 * time.Hour,
	})
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"agentId":"agent-1"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// The grants key used to be discarded silently, which is what made an unscoped
// session look scoped: the caller asked for a narrowing, got a 201, and held a
// credential that reached every non-admin route.
func TestAgentSessionAPI_CreateAppliesAndEchoesGrants(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"agentId":"agent-1","grants":["browse"," Network "]}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	resp := decodeSessionResponse(t, w)
	echoed, _ := resp["grants"].([]any)
	if len(echoed) != 2 || echoed[0] != session.GrantBrowse || echoed[1] != session.GrantNetwork {
		t.Fatalf("grants echoed as %v, want the normalized pair", resp["grants"])
	}

	// Echoed is not enough: the store is what the middleware reads.
	id, _ := resp["id"].(string)
	stored, ok := store.Get(id)
	if !ok {
		t.Fatalf("session %q is not in the store", id)
	}
	if len(stored.Grants) != 2 || stored.Grants[0] != session.GrantBrowse {
		t.Errorf("stored grants = %v, want the pair the caller asked for", stored.Grants)
	}
}

// An unknown grant is refused rather than dropped, and the refusal carries the
// offender and the vocabulary — the whole point of not being silent.
func TestAgentSessionAPI_CreateRefusesAnUnknownGrant(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	req := httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"agentId":"agent-1","grants":["brows"]}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	resp := decodeSessionResponse(t, w)
	if resp["code"] != "invalid_grant" {
		t.Errorf("code = %v, want invalid_grant", resp["code"])
	}
	if msg, _ := resp["error"].(string); !strings.Contains(msg, "brows") || !strings.Contains(msg, "browse") {
		t.Errorf("error = %q, want the offender and the valid names", msg)
	}
	if len(store.List()) != 0 {
		t.Error("a session was created despite the refused grant; a half-applied scope is worse than none")
	}
}

// A session's scope must be readable, not only writable: list and get carry it, so
// an operator can audit what a credential can reach.
func TestAgentSessionAPI_ListAndGetReportGrants(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest("POST", "/sessions", strings.NewReader(`{"agentId":"agent-1","grants":["browse"]}`)))
	id, _ := decodeSessionResponse(t, create)["id"].(string)

	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest("GET", "/sessions/"+id, nil))
	if !strings.Contains(get.Body.String(), `"grants":["browse"]`) {
		t.Errorf("GET /sessions/{id} does not report the scope: %s", get.Body.String())
	}

	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest("GET", "/sessions", nil))
	if !strings.Contains(list.Body.String(), `"grants":["browse"]`) {
		t.Errorf("GET /sessions does not report the scope: %s", list.Body.String())
	}
}
