package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

// pausedTabBridge reports one tab paused for human handoff and records every
// page-mutating call, so a test can assert both the refusal and that the page
// never moved.
type pausedTabBridge struct {
	handoffRecordingBridge
	pageURL   string
	navigated []string
	reloads   int
	backs     int
	uploads   int
}

func newPausedTabBridge() *pausedTabBridge {
	b := &pausedTabBridge{pageURL: "https://pinchtab.com/"}
	b.state = bridge.TabHandoffState{
		Status:        "paused_handoff",
		Reason:        "manual_handoff",
		PausedAt:      time.Now().UTC(),
		LastUpdatedAt: time.Now().UTC(),
	}
	b.has = true
	b.navigateResult = &bridge.NavigateResult{TabID: "tab1", URL: "https://pinchtab.com/docs/"}
	return b
}

func (b *pausedTabBridge) Navigate(ctx context.Context, url string, params bridge.NavigateParams) (*bridge.NavigateResult, error) {
	b.navigated = append(b.navigated, url)
	b.pageURL = url
	return b.handoffRecordingBridge.Navigate(ctx, url, params)
}

func (b *pausedTabBridge) Reload(context.Context) error {
	b.reloads++
	b.pageURL = "https://pinchtab.com/reloaded"
	return nil
}

func (b *pausedTabBridge) GoBack(context.Context) (bool, error) {
	b.backs++
	b.pageURL = "https://pinchtab.com/previous"
	return true, nil
}

func (b *pausedTabBridge) CurrentURL(context.Context) (string, error) { return b.pageURL, nil }

func (b *pausedTabBridge) EvaluateInFrame(_ context.Context, _, _ string, result any, _ bridge.EvalOpts) error {
	if payload, ok := result.(*inspectPayload); ok {
		payload.URL = b.pageURL
	}
	return nil
}

func (b *pausedTabBridge) SetFileInputFiles(context.Context, int64, []string) error {
	b.uploads++
	return nil
}

func pausedTabHandlers(b *pausedTabBridge, tmpDir string) *Handlers {
	return New(b, &config.RuntimeConfig{
		ActionTimeout: time.Second,
		AllowEvaluate: true,
		AllowUpload:   true,
		StateDir:      tmpDir,
	}, nil, nil, nil)
}

// assertHandoffRefusal pins the one refusal shape every gated endpoint shares:
// 409, the tab_paused_handoff code, and the remedy hint in the details.
func assertHandoffRefusal(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error   string         `json:"error"`
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	if resp.Code != "tab_paused_handoff" {
		t.Fatalf("code = %q, want tab_paused_handoff: %s", resp.Code, w.Body.String())
	}
	if !strings.Contains(resp.Error, "paused for human handoff") {
		t.Fatalf("error should name the pause: %s", resp.Error)
	}
	if resp.Details["hint"] != handoffHint("tab1") {
		t.Fatalf("details hint = %v, want the shared handoff hint", resp.Details["hint"])
	}
	if resp.Details["reason"] != "manual_handoff" {
		t.Fatalf("details reason = %v, want manual_handoff", resp.Details["reason"])
	}
}

func TestPausedTabRefusesNavigate(t *testing.T) {
	b := newPausedTabBridge()
	h := pausedTabHandlers(b, t.TempDir())

	req := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"tabId":"tab1","url":"about:blank"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleNavigate(w, req)

	assertHandoffRefusal(t, w)
	if len(b.navigated) != 0 {
		t.Fatalf("navigate reached the bridge: %v", b.navigated)
	}
	if b.pageURL != "https://pinchtab.com/" {
		t.Fatalf("page moved to %q", b.pageURL)
	}
}

func TestPausedTabRefusesReload(t *testing.T) {
	b := newPausedTabBridge()
	h := pausedTabHandlers(b, t.TempDir())

	w := httptest.NewRecorder()
	h.HandleReload(w, httptest.NewRequest("POST", "/reload?tabId=tab1", nil))

	assertHandoffRefusal(t, w)
	if b.reloads != 0 {
		t.Fatalf("reload reached the bridge %d times", b.reloads)
	}
	if b.pageURL != "https://pinchtab.com/" {
		t.Fatalf("page moved to %q", b.pageURL)
	}
}

func TestPausedTabRefusesBack(t *testing.T) {
	b := newPausedTabBridge()
	h := pausedTabHandlers(b, t.TempDir())

	w := httptest.NewRecorder()
	h.HandleBack(w, httptest.NewRequest("POST", "/back?tabId=tab1", nil))

	assertHandoffRefusal(t, w)
	if b.backs != 0 {
		t.Fatalf("back reached the bridge %d times", b.backs)
	}
	if b.pageURL != "https://pinchtab.com/" {
		t.Fatalf("page moved to %q", b.pageURL)
	}
}

func TestPausedTabRefusesEvaluate(t *testing.T) {
	b := newPausedTabBridge()
	h := pausedTabHandlers(b, t.TempDir())

	req := httptest.NewRequest("POST", "/evaluate", bytes.NewReader([]byte(`{"tabId":"tab1","expression":"location.href='/gone'"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleEvaluate(w, req)

	assertHandoffRefusal(t, w)
	if b.evaluateCalls != 0 {
		t.Fatalf("evaluate ran %d expressions on a paused tab", b.evaluateCalls)
	}
}

func TestPausedTabRefusesDialog(t *testing.T) {
	b := newPausedTabBridge()
	h := pausedTabHandlers(b, t.TempDir())

	req := httptest.NewRequest("POST", "/dialog", bytes.NewReader([]byte(`{"tabId":"tab1","action":"accept"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleDialog(w, req)

	assertHandoffRefusal(t, w)
}

func TestPausedTabRefusesSolve(t *testing.T) {
	b := newPausedTabBridge()
	h := pausedTabHandlers(b, t.TempDir())

	req := httptest.NewRequest("POST", "/solve", bytes.NewReader([]byte(`{"tabId":"tab1"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleSolve(w, req)

	assertHandoffRefusal(t, w)
}

func TestPausedTabRefusesUpload(t *testing.T) {
	b := newPausedTabBridge()
	h := pausedTabHandlers(b, t.TempDir())

	req := httptest.NewRequest("POST", "/upload?tabId=tab1", bytes.NewReader([]byte(`{"files":["aGVsbG8="]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleUpload(w, req)

	assertHandoffRefusal(t, w)
	if b.uploads != 0 {
		t.Fatalf("upload attached files %d times on a paused tab", b.uploads)
	}
}

// The watch-and-resume flow depends on reads and resume staying open while a
// human works, so gating them would be as much a defect as the missing guard.
func TestPausedTabAllowsReadsAndResume(t *testing.T) {
	b := newPausedTabBridge()
	h := pausedTabHandlers(b, t.TempDir())

	w := httptest.NewRecorder()
	h.HandleURL(w, httptest.NewRequest("GET", "/url?tabId=tab1", nil))
	if w.Code != 200 {
		t.Fatalf("url on a paused tab = %d: %s", w.Code, w.Body.String())
	}

	statusReq := httptest.NewRequest("GET", "/tabs/tab1/handoff", nil)
	statusReq.SetPathValue("id", "tab1")
	w = httptest.NewRecorder()
	h.HandleTabHandoffStatus(w, statusReq)
	if w.Code != 200 {
		t.Fatalf("handoff-status on a paused tab = %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "paused_handoff") {
		t.Fatalf("handoff-status should still report the pause: %s", w.Body.String())
	}

	resumeReq := httptest.NewRequest("POST", "/tabs/tab1/resume", nil)
	resumeReq.SetPathValue("id", "tab1")
	w = httptest.NewRecorder()
	h.HandleTabResume(w, resumeReq)
	if w.Code != 200 {
		t.Fatalf("resume on a paused tab = %d: %s", w.Code, w.Body.String())
	}
}

// snap and text cannot be driven through the handlers mock (they need a real
// CDP context), so their exemption is pinned at the source: the guard must not
// appear in the read handlers.
func TestReadHandlersDoNotGateOnHandoff(t *testing.T) {
	for _, file := range []string{"text.go", "snapshot.go", "inspect.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if strings.Contains(string(src), "enforceTabNotPausedForHandoff") {
			t.Fatalf("%s gates a read on handoff; reads must stay allowed while a human works", file)
		}
	}
}

func TestResumedTabAllowsNavigateReloadAndBack(t *testing.T) {
	b := newPausedTabBridge()
	h := pausedTabHandlers(b, t.TempDir())

	if err := b.ResumeTabHandoff("tab1"); err != nil {
		t.Fatalf("resume: %v", err)
	}

	navReq := httptest.NewRequest("POST", "/navigate", bytes.NewReader([]byte(`{"tabId":"tab1","url":"about:blank"}`)))
	navReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleNavigate(w, navReq)
	if w.Code == 409 {
		t.Fatalf("navigate still refused after resume: %s", w.Body.String())
	}
	if len(b.navigated) == 0 {
		t.Fatal("navigate did not reach the bridge after resume")
	}

	w = httptest.NewRecorder()
	h.HandleReload(w, httptest.NewRequest("POST", "/reload?tabId=tab1", nil))
	if w.Code != 200 || b.reloads != 1 {
		t.Fatalf("reload after resume: status %d reloads %d: %s", w.Code, b.reloads, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.HandleBack(w, httptest.NewRequest("POST", "/back?tabId=tab1", nil))
	if w.Code != 200 || b.backs != 1 {
		t.Fatalf("back after resume: status %d backs %d: %s", w.Code, b.backs, w.Body.String())
	}
}

func TestHandoffHintNamesBothRunnableFormsWithTheRealTabID(t *testing.T) {
	hint := handoffHint("93FEEE70")
	for _, want := range []string{"return control to the user", "solve the challenge in the browser window", "pinchtab resume 93FEEE70", "POST /tabs/93FEEE70/resume"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint %q does not carry %q", hint, want)
		}
	}
	if strings.Contains(hint, "{id}") {
		t.Fatalf("hint %q carries an unsubstituted placeholder the caller has to edit before running it", hint)
	}
}
