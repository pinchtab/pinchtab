package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleEvaluate_InvalidJSON(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowEvaluate: true}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/evaluate", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.HandleEvaluate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabEvaluate_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowEvaluate: true}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs//evaluate", bytes.NewReader([]byte(`{"expression":"1+1"}`)))
	w := httptest.NewRecorder()
	h.HandleTabEvaluate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabEvaluate_TabIDMismatch(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowEvaluate: true}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/evaluate", bytes.NewReader([]byte(`{"tabId":"tab_other","expression":"1+1"}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabEvaluate(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabEvaluate_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{AllowEvaluate: true}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/evaluate", bytes.NewReader([]byte(`{"expression":"1+1"}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabEvaluate(w, req)
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleEvaluate_Disabled(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/evaluate", bytes.NewReader([]byte(`{"expression":"1+1"}`)))
	w := httptest.NewRecorder()
	h.HandleEvaluate(w, req)
	if w.Code != 403 {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandleTabEvaluate_Disabled(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("POST", "/tabs/tab_abc/evaluate", bytes.NewReader([]byte(`{"expression":"1+1"}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabEvaluate(w, req)
	if w.Code != 403 {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandleEvaluateAwaitPromiseOption(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantAwait    bool
		wantResponse string
	}{
		{
			name:         "default disabled",
			body:         `{"expression":"Promise.resolve(\"ok\")"}`,
			wantAwait:    false,
			wantResponse: `"result":"sync"`,
		},
		{
			name:         "enabled when requested",
			body:         `{"expression":"Promise.resolve(\"ok\")","awaitPromise":true}`,
			wantAwait:    true,
			wantResponse: `"result":"awaited"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(&mockBridge{}, &config.RuntimeConfig{
				AllowEvaluate: true,
				ActionTimeout: time.Second,
			}, nil, nil, nil)

			h.evalRuntime = func(_ context.Context, expression string, out any, opts ...chromedp.EvaluateOption) error {
				if expression != `Promise.resolve("ok")` {
					t.Fatalf("unexpected expression: %s", expression)
				}

				params := runtime.Evaluate(expression)
				for _, opt := range opts {
					params = opt(params)
				}
				if params.AwaitPromise != tt.wantAwait {
					t.Fatalf("expected awaitPromise=%v, got %v", tt.wantAwait, params.AwaitPromise)
				}

				ptr, ok := out.(*any)
				if !ok {
					t.Fatalf("expected *any output, got %T", out)
				}
				if tt.wantAwait {
					*ptr = "awaited"
				} else {
					*ptr = "sync"
				}
				return nil
			}

			req := httptest.NewRequest("POST", "/evaluate", bytes.NewReader([]byte(tt.body)))
			w := httptest.NewRecorder()
			h.HandleEvaluate(w, req)

			if w.Code != 200 {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			if !bytes.Contains(w.Body.Bytes(), []byte(tt.wantResponse)) {
				t.Fatalf("expected response to contain %s, got %s", tt.wantResponse, w.Body.String())
			}
		})
	}
}

func TestHandleTabEvaluateForwardsAwaitPromise(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{
		AllowEvaluate: true,
		ActionTimeout: time.Second,
	}, nil, nil, nil)

	h.evalRuntime = func(_ context.Context, expression string, out any, opts ...chromedp.EvaluateOption) error {
		if expression != `Promise.resolve(1)` {
			t.Fatalf("unexpected expression: %s", expression)
		}
		params := runtime.Evaluate(expression)
		for _, opt := range opts {
			params = opt(params)
		}
		if !params.AwaitPromise {
			t.Fatalf("expected awaitPromise to be forwarded for tab evaluate route")
		}

		ptr, ok := out.(*any)
		if !ok {
			t.Fatalf("expected *any output, got %T", out)
		}
		*ptr = 1
		return nil
	}

	req := httptest.NewRequest("POST", "/tabs/tab_abc/evaluate", bytes.NewReader([]byte(`{"expression":"Promise.resolve(1)","awaitPromise":true}`)))
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabEvaluate(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["result"] != float64(1) {
		t.Fatalf("expected result 1, got %#v", payload["result"])
	}
}
