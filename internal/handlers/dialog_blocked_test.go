package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type dialogBlockedFailure struct {
	Code      string `json:"code"`
	Error     string `json:"error"`
	Retryable bool   `json:"retryable"`
	Details   struct {
		TabID         string `json:"tabId"`
		DialogType    string `json:"dialogType"`
		DialogMessage string `json:"dialogMessage"`
		Hint          string `json:"hint"`
		Remedy        string `json:"remedy"`
	} `json:"details"`
}

func decodeDialogBlocked(t *testing.T, w *httptest.ResponseRecorder, dialog *bridge.DialogState) {
	t.Helper()
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	var failure dialogBlockedFailure
	if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil {
		t.Fatalf("decode failure: %v body=%s", err, w.Body.String())
	}
	if failure.Code != dialogBlockedCode {
		t.Fatalf("code = %q, want %q; body=%s", failure.Code, dialogBlockedCode, w.Body.String())
	}
	if failure.Retryable {
		t.Fatalf("dialog-blocked response advertises retryable: %s", w.Body.String())
	}
	if failure.Details.DialogType != dialog.Type || failure.Details.DialogMessage != dialog.Message {
		t.Fatalf("dialog not named: %s", w.Body.String())
	}
	if failure.Details.Remedy != dialogBlockedRemedy.String() {
		t.Fatalf("remedy = %q, want %q", failure.Details.Remedy, dialogBlockedRemedy)
	}
	if failure.Details.Hint == "" {
		t.Fatalf("missing hint: %s", w.Body.String())
	}
}

func TestDialogBlockedIsTheSameContractOnReadsAndActions(t *testing.T) {
	dialogs := []*bridge.DialogState{
		{Type: "alert", Message: "hello alert"},
		{Type: "confirm", Message: "are you sure?"},
		{Type: "prompt", Message: "your name?", DefaultPrompt: "anon"},
	}

	paths := []struct {
		name string
		call func(h *Handlers, w *httptest.ResponseRecorder)
	}{
		{"url", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandleURL(w, httptest.NewRequest("GET", "/url?tabId=tab1", nil))
		}},
		{"title", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandleTitle(w, httptest.NewRequest("GET", "/title?tabId=tab1", nil))
		}},
		{"text", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandleText(w, httptest.NewRequest("GET", "/text?tabId=tab1", nil))
		}},
		{"snapshot", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandleSnapshot(w, httptest.NewRequest("GET", "/snapshot?tabId=tab1", nil))
		}},
		{"action", func(h *Handlers, w *httptest.ResponseRecorder) {
			body := bytes.NewReader([]byte(`{"kind":"click","tabId":"tab1","nodeId":42}`))
			h.HandleAction(w, httptest.NewRequest("POST", "/action", body))
		}},
		// The visual exports ride resolveBinaryReadContext, the sibling prelude —
		// these rows are what stop the two preludes diverging on the guard again.
		{"screenshot", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandleScreenshot(w, httptest.NewRequest("GET", "/screenshot?tabId=tab1", nil))
		}},
		{"pdf", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandlePDF(w, httptest.NewRequest("GET", "/pdf?tabId=tab1", nil))
		}},
		{"annotate", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandleAnnotate(w, httptest.NewRequest("GET", "/annotate?tabId=tab1", nil))
		}},
	}

	for _, dialog := range dialogs {
		for _, path := range paths {
			t.Run(dialog.Type+"/"+path.name, func(t *testing.T) {
				mb := &mockBridge{}
				mb.GetDialogManager().SetPending("tab1", dialog)
				h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)

				w := httptest.NewRecorder()
				path.call(h, w)
				decodeDialogBlocked(t, w, dialog)
			})
		}
	}
}

func TestBusyTabWithAPendingDialogIsDialogBlockedNotUnresponsive(t *testing.T) {
	dialog := &bridge.DialogState{Type: "alert", Message: "hello alert"}
	b := &busyPolicyBridge{currentURLErr: context.DeadlineExceeded}
	b.GetDialogManager().SetPending("exact-tab", dialog)
	h := New(b, &config.RuntimeConfig{
		AllowedDomains: []string{"example.com"},
		IDPI:           config.IDPIConfig{Enabled: true, StrictMode: true},
	}, nil, nil, nil)

	w := httptest.NewRecorder()
	h.handleWaitCore(w, httptest.NewRequest("POST", "/tabs/exact-tab/wait", nil), waitRequest{TabID: "exact-tab", Text: "ready"})

	decodeDialogBlocked(t, w, dialog)
	if strings.Contains(w.Body.String(), "fresh tab") {
		t.Fatalf("dialog-blocked response suggests a fresh tab: %s", w.Body.String())
	}
}

func TestReadsAreNotDialogBlockedWithoutAPendingDialog(t *testing.T) {
	for _, path := range []struct {
		name string
		call func(h *Handlers, w *httptest.ResponseRecorder)
	}{
		{"url", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandleURL(w, httptest.NewRequest("GET", "/url?tabId=tab1", nil))
		}},
		{"screenshot", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandleScreenshot(w, httptest.NewRequest("GET", "/screenshot?tabId=tab1", nil))
		}},
		{"pdf", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandlePDF(w, httptest.NewRequest("GET", "/pdf?tabId=tab1", nil))
		}},
		{"annotate", func(h *Handlers, w *httptest.ResponseRecorder) {
			h.HandleAnnotate(w, httptest.NewRequest("GET", "/annotate?tabId=tab1", nil))
		}},
	} {
		t.Run(path.name, func(t *testing.T) {
			mb := &mockBridge{}
			h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)

			w := httptest.NewRecorder()
			path.call(h, w)
			if w.Code == http.StatusConflict {
				t.Fatalf("refused as dialog-blocked with no dialog pending: %s", w.Body.String())
			}
		})
	}
}

type dialogOpeningBridge struct {
	mockBridge
	dialog *bridge.DialogState
	result map[string]any
	err    error
}

func (b *dialogOpeningBridge) ExecuteAction(context.Context, string, bridge.ActionRequest) (map[string]any, error) {
	b.GetDialogManager().SetPending("tab1", b.dialog)
	return b.result, b.err
}

func (b *dialogOpeningBridge) AvailableActions() []string {
	return []string{bridge.ActionClick, bridge.ActionDoubleClick}
}

var clickOpenedDialogOutcomes = []struct {
	name   string
	bridge func(*bridge.DialogState) *dialogOpeningBridge
}{
	{"dialog named on the click result", func(d *bridge.DialogState) *dialogOpeningBridge {
		return &dialogOpeningBridge{dialog: d, result: map[string]any{"clicked": true, bridge.OpenedDialogResultKey: d}}
	}},
	{"click timed out behind its dialog", func(d *bridge.DialogState) *dialogOpeningBridge {
		return &dialogOpeningBridge{dialog: d, err: context.DeadlineExceeded}
	}},
}

func assertOpenedDialogResult(t *testing.T, result map[string]any, dialog *bridge.DialogState) {
	t.Helper()
	got, _ := result[bridge.OpenedDialogResultKey].(map[string]any)
	want := dialogBlockedDetails("tab1", dialog)
	for _, key := range []string{"tabId", "dialogType", "dialogMessage", "remedy"} {
		if got[key] != want[key] {
			t.Fatalf("opened dialog %s = %v, want the blocked-case rendering %v; result=%v", key, got[key], want[key], result)
		}
	}
	if got["hint"] != dialogOpenedHint {
		t.Fatalf("hint = %v, want %q", got["hint"], dialogOpenedHint)
	}
}

func TestClickThatOpensADialogSucceedsAndNamesTheDialog(t *testing.T) {
	dialog := &bridge.DialogState{Type: "confirm", Message: "Delete order 1?"}
	for _, kind := range []string{bridge.ActionClick, bridge.ActionDoubleClick} {
		for _, outcome := range clickOpenedDialogOutcomes {
			t.Run(kind+"/"+outcome.name, func(t *testing.T) {
				h := New(outcome.bridge(dialog), &config.RuntimeConfig{}, nil, nil, nil)
				rec := httptest.NewRecorder()
				body := bytes.NewReader([]byte(`{"kind":"` + kind + `","tabId":"tab1","nodeId":42}`))
				h.HandleAction(rec, httptest.NewRequest(http.MethodPost, "/action", body))

				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
				}
				var resp struct {
					Success bool           `json:"success"`
					Result  map[string]any `json:"result"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode: %v body=%s", err, rec.Body.String())
				}
				if !resp.Success {
					t.Fatalf("click that landed is not a success: %s", rec.Body.String())
				}
				assertOpenedDialogResult(t, resp.Result, dialog)
			})
		}
	}
}

func TestBatchClickThatOpensADialogSucceedsAndTheNextStepIsBlocked(t *testing.T) {
	dialog := &bridge.DialogState{Type: "alert", Message: "locked"}
	for _, outcome := range clickOpenedDialogOutcomes {
		t.Run(outcome.name, func(t *testing.T) {
			h := New(outcome.bridge(dialog), &config.RuntimeConfig{}, nil, nil, nil)
			rec := httptest.NewRecorder()
			body := bytes.NewReader([]byte(`{"tabId":"tab1","actions":[{"kind":"click","nodeId":42},{"kind":"click","nodeId":43}]}`))
			req := httptest.NewRequest(http.MethodPost, "/actions", body)
			req.Header.Set("Content-Type", "application/json")
			h.HandleActions(rec, req)

			var envelope struct {
				Results []actionResult `json:"results"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode: %v body=%s", err, rec.Body.String())
			}
			if len(envelope.Results) != 2 {
				t.Fatalf("want the opening click and the step it blocked: %s", rec.Body.String())
			}
			opening, blocked := envelope.Results[0], envelope.Results[1]
			if !opening.Success {
				t.Fatalf("click that landed is not a success: %s", rec.Body.String())
			}
			assertOpenedDialogResult(t, opening.Result, dialog)
			if blocked.Success || blocked.Code != dialogBlockedCode {
				t.Fatalf("step behind the dialog = %+v, want a %s failure", blocked, dialogBlockedCode)
			}
		})
	}
}
