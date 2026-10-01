package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const pendingAlertTab = "tab-alert"

func handlersOverAPendingAlert(t *testing.T) *Handlers {
	t.Helper()
	ctx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	t.Cleanup(cancelTimeout)
	html := `<button id="b" onclick="alert('hi')">b</button><button id="c">c</button>`
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html)))); err != nil {
		t.Fatal(err)
	}
	opened := make(chan struct{}, 1)
	chromedp.ListenTarget(ctx, func(ev any) {
		if _, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			select {
			case opened <- struct{}{}:
			default:
			}
		}
	})
	if err := chromedp.Run(ctx, chromedp.Evaluate(`setTimeout(() => document.getElementById('b').click(), 0)`, nil)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("the alert never opened")
	}
	cfg := &config.RuntimeConfig{ActionTimeout: time.Second, AllowMacro: true, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := browserBridge(ctx, cfg)
	b.RegisterTab(pendingAlertTab, ctx)
	b.GetDialogManager().SetPending(pendingAlertTab, &bridge.DialogState{Type: "alert", Message: "hi"})
	return New(b, cfg, nil, nil, nil)
}

func TestABatchOverAPendingDialogIsRefusedBeforeAnyStep(t *testing.T) {
	h := handlersOverAPendingAlert(t)
	for _, tc := range []struct{ name, path, body string }{
		{"batch", "/actions", `{"tabId":"` + pendingAlertTab + `","actions":[{"kind":"click","selector":"#c"}]}`},
		{"macro", "/macro", `{"tabId":"` + pendingAlertTab + `","steps":[{"kind":"click","selector":"#c"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			if tc.path == "/macro" {
				h.HandleMacro(rec, req)
			} else {
				h.HandleActions(rec, req)
			}
			var envelope struct {
				Code    string         `json:"code"`
				Error   string         `json:"error"`
				Details map[string]any `json:"details"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode: %v: %s", err, rec.Body.String())
			}
			if rec.Code != http.StatusConflict || envelope.Code != dialogBlockedCode {
				t.Fatalf("want 409 %s before any step runs, got %d: %s", dialogBlockedCode, rec.Code, rec.Body.String())
			}
			if remedy, _ := envelope.Details["remedy"].(string); !strings.Contains(envelope.Error, `"hi"`) || remedy == "" {
				t.Fatalf("refusal does not name the dialog with its remedy: %s", rec.Body.String())
			}
		})
	}
}
