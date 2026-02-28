package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type integrationMockBridge struct {
	bridge.BridgeAPI
	tabs map[string]context.Context
}

func (m *integrationMockBridge) TabContext(tabID string) (context.Context, string, error) {
	if tabID == "" {
		return context.Background(), "tab1", nil
	}
	ctx, ok := m.tabs[tabID]
	if !ok {
		return nil, "", fmt.Errorf("not found")
	}
	return ctx, tabID, nil
}

func (m *integrationMockBridge) ListTargets() ([]*target.Info, error) {
	return []*target.Info{{TargetID: "tab1", Type: "page"}}, nil
}

func (m *integrationMockBridge) TabLockInfo(tabID string) *bridge.LockInfo {
	return nil
}

func (m *integrationMockBridge) EnsureChrome(cfg *config.RuntimeConfig) error {
	return nil
}

func TestIntegration_RoutesRegistration(t *testing.T) {
	b := &integrationMockBridge{tabs: make(map[string]context.Context)}
	cfg := &config.RuntimeConfig{}
	h := New(b, cfg, nil, nil, nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, func() {})

	tests := []struct {
		method string
		path   string
		code   int
	}{
		{"GET", "/health", 200},
		{"GET", "/tabs", 200},
		{"GET", "/welcome", 200},
		{"POST", "/navigate", 400}, // missing body
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != tt.code {
			t.Errorf("%s %s expected %d, got %d", tt.method, tt.path, tt.code, w.Code)
		}
	}
}

func TestIntegration_StealthInjection(t *testing.T) {
	b := bridge.New(context.Background(), context.Background(), &config.RuntimeConfig{})
	b.StealthScript = assets.StealthScript

	if b.StealthScript == "" {
		t.Error("expected stealth script to be populated")
	}
}
