package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func pausedHandoffActionHandlers(t *testing.T) *Handlers {
	t.Helper()

	b := &handoffRecordingBridge{
		state: bridge.TabHandoffState{
			Status:        "paused_handoff",
			Reason:        "captcha_manual",
			PausedAt:      time.Now().UTC(),
			LastUpdatedAt: time.Now().UTC(),
		},
		has: true,
	}
	return New(b, &config.RuntimeConfig{AllowMacro: true}, nil, nil, nil)
}

type batchEnvelope struct {
	Results []struct {
		Index   int            `json:"index"`
		Success bool           `json:"success"`
		Error   string         `json:"error"`
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	} `json:"results"`
}

func TestBatchAndMacroCarryThePausedTabRefusalPerItem(t *testing.T) {
	for _, tc := range []struct{ name, path, body string }{
		{
			name: "batch",
			path: "/actions",
			body: `{"tabId":"tab1","actions":[{"kind":"click","selector":"button"}]}`,
		},
		{
			name: "macro",
			path: "/macro",
			body: `{"tabId":"tab1","steps":[{"kind":"click","selector":"button"}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := pausedHandoffActionHandlers(t)
			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			if tc.path == "/macro" {
				h.HandleMacro(rec, req)
			} else {
				h.HandleActions(rec, req)
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("%s answered %d, want the 200 result-list envelope: %s", tc.path, rec.Code, rec.Body.String())
			}

			var envelope batchEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("%s response is not the result envelope: %v: %s", tc.path, err, rec.Body.String())
			}
			if len(envelope.Results) != 1 {
				t.Fatalf("%s returned %d results, want one per step: %s", tc.path, len(envelope.Results), rec.Body.String())
			}

			entry := envelope.Results[0]
			if entry.Success {
				t.Errorf("%s reported the step as successful against a paused tab", tc.path)
			}
			if entry.Code != "tab_paused_handoff" {
				t.Errorf("%s per-item code = %q, want %q — a client must detect this by code, not by matching the message", tc.path, entry.Code, "tab_paused_handoff")
			}
			hint, _ := entry.Details["hint"].(string)
			if !strings.Contains(hint, "pinchtab resume tab1") || !strings.Contains(hint, "POST /tabs/tab1/resume") {
				t.Errorf("%s per-item details.hint = %q, want the remedy naming both runnable resume forms for the paused tab", tc.path, hint)
			}
			if entry.Error == "" {
				t.Errorf("%s dropped the human-readable reason; the code is added beside it, not instead of it", tc.path)
			}
		})
	}
}

func TestThePerItemRefusalMatchesTheResponseLevelOne(t *testing.T) {
	h := pausedHandoffActionHandlers(t)

	single := httptest.NewRecorder()
	singleReq := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader([]byte(`{"tabId":"tab1","kind":"click","selector":"button"}`)))
	singleReq.Header.Set("Content-Type", "application/json")
	h.HandleAction(single, singleReq)

	if single.Code != http.StatusConflict {
		t.Fatalf("POST /action answered %d, want the response-level 409: %s", single.Code, single.Body.String())
	}
	var refusal struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(single.Body.Bytes(), &refusal); err != nil {
		t.Fatalf("409 body is not JSON: %v: %s", err, single.Body.String())
	}

	batch := httptest.NewRecorder()
	batchReq := httptest.NewRequest(http.MethodPost, "/actions", bytes.NewReader([]byte(`{"tabId":"tab1","actions":[{"kind":"click","selector":"button"}]}`)))
	batchReq.Header.Set("Content-Type", "application/json")
	h.HandleActions(batch, batchReq)

	var envelope batchEnvelope
	if err := json.Unmarshal(batch.Body.Bytes(), &envelope); err != nil || len(envelope.Results) != 1 {
		t.Fatalf("batch response unusable: %v: %s", err, batch.Body.String())
	}

	if envelope.Results[0].Code != refusal.Code {
		t.Errorf("per-item code %q and response-level code %q have drifted apart; they are the same condition", envelope.Results[0].Code, refusal.Code)
	}
	if got, want := envelope.Results[0].Details["hint"], refusal.Details["hint"]; got != want {
		t.Errorf("per-item hint %v and response-level hint %v differ; a client following one is misled by the other", got, want)
	}
}
