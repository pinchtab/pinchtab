package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type submitActionBridge struct {
	*mockBridge
	tabCtx          context.Context
	urls            []string
	currentURLErr   error
	urlCalls        int
	urlContexts     []context.Context
	urlContextErrs  []error
	actionCalls     int
	actionCtx       context.Context
	executeActionFn func(context.Context, string, bridge.ActionRequest) (map[string]any, error)
}

func newSubmitActionBridge(ctx context.Context) *submitActionBridge {
	return &submitActionBridge{
		mockBridge: &mockBridge{},
		tabCtx:     ctx,
		urls:       []string{"https://example.test/wizard"},
	}
}

func (b *submitActionBridge) TabContext(string) (*bridge.TabHandle, string, error) {
	return bridge.NewTabHandle(b.tabCtx), "tab1", nil
}

func (b *submitActionBridge) CurrentURL(ctx context.Context) (string, error) {
	b.urlContexts = append(b.urlContexts, ctx)
	b.urlContextErrs = append(b.urlContextErrs, ctx.Err())
	if b.currentURLErr != nil {
		return "", b.currentURLErr
	}
	index := b.urlCalls
	b.urlCalls++
	if index >= len(b.urls) {
		index = len(b.urls) - 1
	}
	return b.urls[index], nil
}

func (b *submitActionBridge) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	b.actionCalls++
	b.actionCtx = ctx
	if b.executeActionFn != nil {
		return b.executeActionFn(ctx, kind, req)
	}
	return map[string]any{"clicked": true}, nil
}

func installSubmitModalReader(t *testing.T, fn func(context.Context, string) (int64, bool, error)) {
	t.Helper()
	original := readTopmostSubmitModal
	readTopmostSubmitModal = fn
	t.Cleanup(func() { readTopmostSubmitModal = original })
}

func postSubmitRequest(t *testing.T, h *Handlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleAction(w, req)
	return w
}

func TestHandleClickSubmitTimeoutUsesFreshTabContextAndReportsDialogClose(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	b := newSubmitActionBridge(parentCtx)
	b.executeActionFn = func(ctx context.Context, _ string, _ bridge.ActionRequest) (map[string]any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	modalReads := 0
	installSubmitModalReader(t, func(context.Context, string) (int64, bool, error) {
		modalReads++
		if modalReads == 1 {
			return 71, true, nil
		}
		return 0, false, nil
	})

	h := New(b, &config.RuntimeConfig{ActionTimeout: 10 * time.Millisecond}, nil, nil, nil)
	w := postSubmitRequest(t, h, `{"kind":"click","tabId":"tab1","nodeId":42,"submit":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Result  struct {
			PostState submitPostState `json:"postState"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	post := response.Result.PostState
	if !response.Success || post.Status != "succeeded" || post.Signal != "dialog_closed" ||
		post.Dispatch != "unconfirmed" || !post.ActionTimedOut {
		t.Fatalf("unexpected post-state: %+v", post)
	}
	if b.actionCalls != 1 {
		t.Fatalf("action calls = %d, want 1", b.actionCalls)
	}
	if !errors.Is(b.actionCtx.Err(), context.DeadlineExceeded) {
		t.Fatalf("action context error = %v, want deadline", b.actionCtx.Err())
	}
	if len(b.urlContexts) < 2 {
		t.Fatalf("URL context calls = %d, want baseline and post-state", len(b.urlContexts))
	}
	if b.urlContexts[0] != b.actionCtx {
		t.Fatal("pre-state was not captured with the action context")
	}
	if b.urlContexts[1] == b.actionCtx || b.urlContextErrs[1] != nil {
		t.Fatalf("post-state reused expired action context: same=%v errAtRead=%v", b.urlContexts[1] == b.actionCtx, b.urlContextErrs[1])
	}
	if parentCtx.Err() != nil {
		t.Fatalf("live parent tab context was canceled: %v", parentCtx.Err())
	}
}

func TestPollClickSubmitReportsURLChange(t *testing.T) {
	b := newSubmitActionBridge(context.Background())
	b.urls = []string{"https://example.test/done"}
	installSubmitModalReader(t, func(context.Context, string) (int64, bool, error) {
		return 0, false, nil
	})
	h := &Handlers{Bridge: b}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	post, err := h.pollSubmitPostState(ctx, "tab1", submitStateSnapshot{
		URL: "https://example.test/wizard",
	}, "acknowledged", false)
	if err != nil {
		t.Fatalf("poll post-state: %v", err)
	}
	if post.Status != "succeeded" || post.Signal != "url_changed" || post.Dispatch != "acknowledged" || post.ActionTimedOut {
		t.Fatalf("unexpected post-state: %+v", post)
	}
}

func TestHandleClickSubmitGETParsesSubmitFlag(t *testing.T) {
	b := newSubmitActionBridge(context.Background())
	b.urls = []string{"https://example.test/wizard", "https://example.test/done"}
	installSubmitModalReader(t, func(context.Context, string) (int64, bool, error) {
		return 0, false, nil
	})
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/action?kind=click&tabId=tab1&nodeId=42&submit=true", nil)
	w := httptest.NewRecorder()
	h.HandleAction(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"signal":"url_changed"`) {
		t.Fatalf("GET submit status=%d response=%s", w.Code, w.Body.String())
	}
	if b.actionCalls != 1 {
		t.Fatalf("action calls = %d, want 1", b.actionCalls)
	}
}

func TestPollClickSubmitReportsPendingWithoutTerminalChange(t *testing.T) {
	b := newSubmitActionBridge(context.Background())
	installSubmitModalReader(t, func(context.Context, string) (int64, bool, error) {
		return 8, true, nil
	})
	h := &Handlers{Bridge: b}
	before := submitStateSnapshot{URL: "https://example.test/wizard", DialogOpen: true}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	post, err := h.pollSubmitPostState(ctx, "tab1", before, "unconfirmed", true)
	if err != nil {
		t.Fatalf("poll post-state: %v", err)
	}
	if post.Status != "pending" || post.Signal != "no_terminal_change" || post.After != before {
		t.Fatalf("unexpected post-state: %+v", post)
	}
}

func TestPollClickSubmitCancellationAndUnobservableStateAreErrors(t *testing.T) {
	t.Run("canceled tab context", func(t *testing.T) {
		b := newSubmitActionBridge(context.Background())
		installSubmitModalReader(t, func(context.Context, string) (int64, bool, error) {
			return 8, true, nil
		})
		h := &Handlers{Bridge: b}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := h.pollSubmitPostState(ctx, "tab1", submitStateSnapshot{URL: b.urls[0], DialogOpen: true}, "unconfirmed", true)
		if err == nil || !strings.Contains(err.Error(), "observation canceled") {
			t.Fatalf("canceled observation error = %v", err)
		}
	})

	t.Run("no readable post-state", func(t *testing.T) {
		b := newSubmitActionBridge(context.Background())
		b.currentURLErr = errors.New("target closed")
		installSubmitModalReader(t, func(context.Context, string) (int64, bool, error) {
			return 0, false, nil
		})
		h := &Handlers{Bridge: b}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err := h.pollSubmitPostState(ctx, "tab1", submitStateSnapshot{}, "unconfirmed", true)
		if err == nil || !strings.Contains(err.Error(), "never observable") || !strings.Contains(err.Error(), "target closed") {
			t.Fatalf("unobservable post-state error = %v", err)
		}
	})
}

func TestHandleClickSubmitPostDispatchErrorIsNotRetryable(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	b := newSubmitActionBridge(parentCtx)
	b.executeActionFn = func(context.Context, string, bridge.ActionRequest) (map[string]any, error) {
		return nil, bridge.ErrElementStale
	}
	installSubmitModalReader(t, func(context.Context, string) (int64, bool, error) {
		return 11, true, nil
	})
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)

	w := postSubmitRequest(t, h, `{"kind":"click","tabId":"tab1","ref":"e1","nodeId":42,"submit":true}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
	}
	var failure struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
		Details   struct {
			DoNotRetry bool `json:"doNotRetry"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil {
		t.Fatalf("decode failure: %v", err)
	}
	if failure.Code != "action_failed" || failure.Retryable || !failure.Details.DoNotRetry {
		t.Fatalf("unexpected failure contract: %+v body=%s", failure, w.Body.String())
	}
	if b.actionCalls != 1 {
		t.Fatalf("action calls = %d, want 1", b.actionCalls)
	}
}

func TestHandleClickSubmitNativeDialogKeepsDialogBlockingContract(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	b := newSubmitActionBridge(parentCtx)
	b.executeActionFn = func(ctx context.Context, _ string, _ bridge.ActionRequest) (map[string]any, error) {
		b.GetDialogManager().SetPending("tab1", &bridge.DialogState{Type: "confirm", Message: "Continue?"})
		<-ctx.Done()
		return nil, ctx.Err()
	}
	installSubmitModalReader(t, func(context.Context, string) (int64, bool, error) {
		return 0, false, nil
	})
	h := New(b, &config.RuntimeConfig{ActionTimeout: 10 * time.Millisecond}, nil, nil, nil)

	w := postSubmitRequest(t, h, `{"kind":"click","tabId":"tab1","nodeId":42,"submit":true}`)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), `"code":"dialog_blocking"`) {
		t.Fatalf("native dialog contract changed: status=%d body=%s", w.Code, w.Body.String())
	}
	if b.urlCalls != 1 {
		t.Fatalf("URL reads = %d, want baseline only (no DOM poll while native dialog blocks)", b.urlCalls)
	}
}

func TestHandleClickSubmitTabClosureIsPostStateFailureNotPending(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	b := newSubmitActionBridge(parentCtx)
	b.executeActionFn = func(context.Context, string, bridge.ActionRequest) (map[string]any, error) {
		parentCancel()
		return map[string]any{"clicked": true}, nil
	}
	installSubmitModalReader(t, func(context.Context, string) (int64, bool, error) {
		return 11, true, nil
	})
	h := New(b, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)

	w := postSubmitRequest(t, h, `{"kind":"click","tabId":"tab1","nodeId":42,"submit":true}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
	}
	var failure struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
		Details   struct {
			DoNotRetry bool `json:"doNotRetry"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil {
		t.Fatalf("decode failure: %v", err)
	}
	if failure.Code != "submit_post_state_failed" || failure.Retryable || !failure.Details.DoNotRetry || strings.Contains(w.Body.String(), `"status":"pending"`) {
		t.Fatalf("tab closure was softened: %+v body=%s", failure, w.Body.String())
	}
}

func TestClickSubmitRejectsConflictingSingleActionOptions(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	tests := []string{
		`{"kind":"click","submit":true,"x":1,"hasXY":true}`,
		`{"kind":"click","submit":true,"waitNav":true,"nodeId":1}`,
		`{"kind":"click","submit":true,"mode":"dom","nodeId":1}`,
		`{"kind":"click","submit":true,"humanize":true,"nodeId":1}`,
		`{"kind":"type","submit":true,"nodeId":1}`,
	}
	for _, body := range tests {
		w := postSubmitRequest(t, h, body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_submit_action") {
			t.Fatalf("body %s: status=%d response=%s", body, w.Code, w.Body.String())
		}
	}
}

func TestClickSubmitRejectedInBatchAndMacroButFillSubmitAllowed(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{ActionTimeout: time.Second, AllowMacro: true}, nil, nil, nil)

	batchReq := httptest.NewRequest(http.MethodPost, "/actions", bytes.NewBufferString(`{"actions":[{"kind":"click","submit":true,"nodeId":1}]}`))
	batch := httptest.NewRecorder()
	h.HandleActions(batch, batchReq)
	if batch.Code != http.StatusBadRequest || !strings.Contains(batch.Body.String(), "click_submit_requires_single_action") {
		t.Fatalf("batch status=%d response=%s", batch.Code, batch.Body.String())
	}

	macroReq := httptest.NewRequest(http.MethodPost, "/macro", bytes.NewBufferString(`{"steps":[{"kind":"click","submit":true,"nodeId":1}]}`))
	macro := httptest.NewRecorder()
	h.HandleMacro(macro, macroReq)
	if macro.Code != http.StatusBadRequest || !strings.Contains(macro.Body.String(), "click_submit_requires_single_action") {
		t.Fatalf("macro status=%d response=%s", macro.Code, macro.Body.String())
	}

	if !rejectMultiStepSubmitClicks(httptest.NewRecorder(), []bridge.ActionRequest{{Kind: bridge.ActionFill, Submit: true}}, "batch", "actions") {
		t.Fatal("fill submit should retain its existing multi-step behavior")
	}
}
