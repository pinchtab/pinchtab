package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestActionSelectorResolutionDefaultsToTopmostDialog(t *testing.T) {
	chromePath, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("chromium not installed")
	}
	profile, err := os.MkdirTemp("", "pinchtab-handler-modal-")
	if err != nil {
		t.Fatal(err)
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 20*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
		_ = os.RemoveAll(profile)
	})

	html := `<button id="background">Content contains</button><p>Background only</p>
		<div id="dialog" role="dialog" aria-modal="true" style="position:fixed;inset:10px;z-index:50;background:white">
			<button id="inside">Content contains</button><p>Dialog only</p>
			<div id="hidden" aria-hidden="true">Hidden AX target</div>
		</div>`
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html)))); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RuntimeConfig{ActionTimeout: 5 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := bridge.New(context.Background(), ctx, cfg)
	b.TabManager.RegisterTab("tab-modal", ctx)
	h := New(b, cfg, nil, nil, nil)

	req := bridge.ActionRequest{Selector: "text:Content contains"}
	resolution, err := h.resolveActionRequestSelector(ctx, "tab-modal", &req)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.refMissing || req.NodeID == 0 {
		t.Fatalf("resolution = %+v request = %+v", resolution, req)
	}
	var id string
	if err := b.CallFunctionOnNode(ctx, req.NodeID, `function() { return this.id; }`, nil, &id); err != nil {
		t.Fatal(err)
	}
	if id != "inside" {
		t.Fatalf("handler resolved %q, want inside", id)
	}

	var backgroundNodeID int64
	backgroundNodeID, err = bridge.ResolveCSSToNodeID(ctx, "#background")
	if err != nil {
		t.Fatal(err)
	}
	b.SetRefCache("tab-modal", &bridge.RefCache{Targets: map[string]bridge.RefTarget{
		"e0": {BackendNodeID: backgroundNodeID},
		"e1": {BackendNodeID: req.NodeID},
	}})
	backgroundRef := bridge.ActionRequest{Selector: "ref:e0"}
	resolution, err = h.resolveActionRequestSelector(ctx, "tab-modal", &backgroundRef)
	if err == nil || resolution.httpStatus() != 400 || resolution.refMissing || backgroundRef.NodeID != 0 {
		t.Fatalf("background ref escaped dialog: resolution=%+v request=%+v", resolution, backgroundRef)
	}
	insideRef := bridge.ActionRequest{Selector: "ref:e1"}
	resolution, err = h.resolveActionRequestSelector(ctx, "tab-modal", &insideRef)
	if err != nil || resolution.refMissing || insideRef.NodeID != req.NodeID || insideRef.Ref != "" {
		t.Fatalf("inside ref = resolution=%+v request=%+v error=%v", resolution, insideRef, err)
	}

	t.Run("text handler reads only the modal", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/text?tabId=tab-modal&mode=raw", nil)
		w := httptest.NewRecorder()
		h.HandleText(w, httpReq)
		if w.Code != http.StatusOK {
			t.Fatalf("text status = %d body=%s", w.Code, w.Body.String())
		}
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(payload.Text, "Dialog only") || strings.Contains(payload.Text, "Background only") {
			t.Fatalf("text leaked outside modal: %q", payload.Text)
		}
	})

	t.Run("snapshot handler reads only the modal", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/snapshot?tabId=tab-modal", nil)
		w := httptest.NewRecorder()
		h.HandleSnapshot(w, httpReq)
		if w.Code != http.StatusOK {
			t.Fatalf("snapshot status = %d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "Dialog only") || strings.Contains(w.Body.String(), "Background only") {
			t.Fatalf("snapshot leaked outside modal: %s", w.Body.String())
		}
	})

	t.Run("AX-missing selector returns empty scope", func(t *testing.T) {
		httpReq := httptest.NewRequest(http.MethodGet, "/snapshot?tabId=tab-modal&selector=%23hidden", nil)
		w := httptest.NewRecorder()
		h.HandleSnapshot(w, httpReq)
		if w.Code != http.StatusOK {
			t.Fatalf("snapshot status = %d body=%s", w.Code, w.Body.String())
		}
		var payload struct {
			Count int    `json:"count"`
			Hint  string `json:"hint"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Count != 0 || payload.Hint == "" {
			t.Fatalf("AX-missing selector payload = %+v, want empty with hint", payload)
		}
	})
}

func TestSelectorResolutionHTTPStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "no match", err: bridge.ErrSelectorNoMatch, want: http.StatusNotFound},
		{name: "outside modal", err: bridge.ErrSelectorOutsideScope, want: http.StatusBadRequest},
		{name: "deadline", err: context.DeadlineExceeded, want: http.StatusGatewayTimeout},
		{name: "transport", err: context.Canceled, want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectorResolutionHTTPStatus(tt.err); got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
		})
	}
}
