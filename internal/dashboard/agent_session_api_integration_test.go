package dashboard_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/session"
)

func newIntegrationSessionStore() *session.Store {
	return session.NewStore(session.Config{
		Enabled:     true,
		IdleTimeout: 30 * time.Minute,
		MaxLifetime: 24 * time.Hour,
	})
}

func decodeIntegrationResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return m
}

func TestAgentSessionAPI_Me_UsesContextSessionAfterMiddlewareAuth(t *testing.T) {
	store := newIntegrationSessionStore()
	mux := http.NewServeMux()
	dashboard.NewSessionAPI(store).RegisterHandlers(mux)

	sessionID, token, _ := store.Create("agent-1", "my-session")
	handler := handlers.AuthMiddlewareWithSessions(
		&config.RuntimeConfig{Token: "dashboard-token"},
		nil,
		store,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !store.Revoke(sessionID) {
				t.Fatal("expected middleware-authenticated session to exist")
			}
			mux.ServeHTTP(w, r)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/sessions/me", nil)
	req.Header.Set("Authorization", "Session "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	resp := decodeIntegrationResponse(t, w)
	if resp["id"] != sessionID {
		t.Fatalf("id = %q, want %q", resp["id"], sessionID)
	}
}

func TestAgentSessionAPI_Revoke_UsesContextSessionAfterMiddlewareAuth(t *testing.T) {
	store := newIntegrationSessionStore()
	mux := http.NewServeMux()
	dashboard.NewSessionAPI(store).RegisterHandlers(mux)

	sessionID, token, _ := store.Create("agent-1", "")
	handler := handlers.AuthMiddlewareWithSessions(
		&config.RuntimeConfig{Token: "dashboard-token"},
		nil,
		store,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !store.Revoke(sessionID) {
				t.Fatal("expected middleware-authenticated session to exist")
			}
			mux.ServeHTTP(w, r)
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/sessions/"+sessionID+"/revoke", nil)
	req.Header.Set("Authorization", "Session "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
