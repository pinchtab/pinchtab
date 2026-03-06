package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleScreencast_AuthRejectsNoToken(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret-token-123"}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	req := httptest.NewRequest("GET", "/screencast", nil)
	w := httptest.NewRecorder()
	handler := AuthMiddleware(cfg, http.HandlerFunc(h.HandleScreencast))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized without token, got %d", w.Code)
	}
}

func TestHandleScreencast_AuthRejectsWrongToken(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret-token-123"}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	req := httptest.NewRequest("GET", "/screencast?token=wrong-token", nil)
	w := httptest.NewRecorder()
	handler := AuthMiddleware(cfg, http.HandlerFunc(h.HandleScreencast))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized with wrong token, got %d", w.Code)
	}
}

func TestHandleScreencast_AuthRejectsWrongHeader(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret-token-123"}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	req := httptest.NewRequest("GET", "/screencast", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler := AuthMiddleware(cfg, http.HandlerFunc(h.HandleScreencast))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized with wrong Bearer header, got %d", w.Code)
	}
}

func TestHandleScreencast_NoTokenConfigSkipsAuth(t *testing.T) {
	cfg := &config.RuntimeConfig{} // No token configured
	h := New(&mockBridge{failTab: true}, cfg, nil, nil, nil)

	// With no token configured, auth is skipped. The handler will then
	// fail at TabContext (failTab=true) and return 404 — proving auth
	// didn't block the request.
	req := httptest.NewRequest("GET", "/screencast", nil)
	w := httptest.NewRecorder()
	handler := AuthMiddleware(cfg, http.HandlerFunc(h.HandleScreencast))
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Errorf("should not get 401 when no token is configured, got %d", w.Code)
	}
}
