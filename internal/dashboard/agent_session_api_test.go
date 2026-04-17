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
	NewSessionAPI(store).RegisterHandlers(mux)
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

	_, _, _ = store.Create("agent-1", "first")
	_, _, _ = store.Create("agent-2", "second")

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

	id, _, _ := store.Create("agent-1", "my-session")

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

	sessionID, token, _ := store.Create("agent-1", "my-session")
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

	id, token, _ := store.Create("agent-1", "")

	req := httptest.NewRequest("POST", "/sessions/"+id+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer dashboard-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Token should no longer authenticate
	if sess, ok := store.Authenticate(token); ok || sess != nil {
		t.Fatal("expected token to be invalidated after revoke")
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

func TestAgentSessionAPI_Revoke_SessionOwnerAllowed(t *testing.T) {
	store := newTestSessionStore()
	mux := newTestSessionMux(store)

	id, token, _ := store.Create("agent-1", "")
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

	id, token, _ := store.Create("agent-1", "")
	sess, ok := store.Get(id)
	if !ok || sess == nil {
		t.Fatal("expected session to exist")
	}
	otherID, _, _ := store.Create("agent-2", "")

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

	id, _, _ := store.Create("agent-1", "")

	req := httptest.NewRequest("POST", "/sessions/"+id+"/revoke", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
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
