package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type offlineRecordingBridge struct {
	*mockBridge
	conditions []bridge.NetworkConditions
}

func (b *offlineRecordingBridge) SetNetworkConditions(_ context.Context, _ string, params bridge.NetworkConditions) error {
	b.conditions = append(b.conditions, params)
	return nil
}

func TestOfflineStaysAvailableWhileNetworkInterceptIsGated(t *testing.T) {
	cfg := &config.RuntimeConfig{AllowNetworkIntercept: false}

	b := &offlineRecordingBridge{mockBridge: &mockBridge{}}
	h := New(b, cfg, nil, nil, nil)
	w := httptest.NewRecorder()
	h.HandleSetOffline(w, httptest.NewRequest(http.MethodPost, "/emulation/offline", bytes.NewReader([]byte(`{"offline":true}`))))
	if w.Code != http.StatusOK {
		t.Fatalf("offline with allowNetworkIntercept=false answered %d, want 200: %s", w.Code, w.Body.String())
	}
	if len(b.conditions) != 1 || !b.conditions[0].Offline {
		t.Fatalf("offline was not installed on the tab: %+v", b.conditions)
	}

	route := New(newRouteMockBridge(), cfg, nil, nil, nil)
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tabs/tab1/network/route", bytes.NewReader([]byte(`{"pattern":"x","action":"continue"}`)))
	req.SetPathValue("id", "tab1")
	route.HandleTabNetworkRoute(w, req)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "network_intercept_disabled") {
		t.Fatalf("route rule with allowNetworkIntercept=false answered %d, want 403 network_intercept_disabled: %s", w.Code, w.Body.String())
	}
}
