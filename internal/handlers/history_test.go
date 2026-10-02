package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

type historyEdgeBridge struct {
	mockBridge
	moved bool
}

func (b *historyEdgeBridge) GoBack(context.Context) (bool, error)    { return b.moved, nil }
func (b *historyEdgeBridge) GoForward(context.Context) (bool, error) { return b.moved, nil }
func (b *historyEdgeBridge) CurrentURL(context.Context) (string, error) {
	return "https://pinchtab.com/here", nil
}

func historyResponse(t *testing.T, moved bool, handler func(*Handlers) http.HandlerFunc, path string) map[string]any {
	t.Helper()
	h := New(&historyEdgeBridge{moved: moved}, &config.RuntimeConfig{}, nil, nil, nil)
	w := httptest.NewRecorder()
	handler(h)(w, httptest.NewRequest("POST", path+"?tabId=tab1", nil))
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func TestBackAndForwardReportWhetherHistoryMoved(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		handler func(*Handlers) http.HandlerFunc
	}{
		{name: "back", path: "/back", handler: func(h *Handlers) http.HandlerFunc { return h.HandleBack }},
		{name: "forward", path: "/forward", handler: func(h *Handlers) http.HandlerFunc { return h.HandleForward }},
	} {
		for _, moved := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s moved=%t", tc.name, moved), func(t *testing.T) {
				resp := historyResponse(t, moved, tc.handler, tc.path)
				if got, ok := resp["navigated"].(bool); !ok || got != moved {
					t.Errorf("navigated = %v, want %v", resp["navigated"], moved)
				}
				if resp["url"] != "https://pinchtab.com/here" {
					t.Errorf("url = %v, want the current URL", resp["url"])
				}
			})
		}
	}
}

func TestReloadResponseOmitsNavigated(t *testing.T) {
	resp := historyResponse(t, false, func(h *Handlers) http.HandlerFunc { return h.HandleReload }, "/reload")
	if _, present := resp["navigated"]; present {
		t.Errorf("reload response carries navigated = %v, want the field absent", resp["navigated"])
	}
}
