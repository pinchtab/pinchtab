package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

type dialogRaisingBridge struct {
	mockBridge
	dialog *bridge.DialogState
	err    error
}

func (b *dialogRaisingBridge) ExecuteAction(context.Context, string, bridge.ActionRequest) (map[string]any, error) {
	b.GetDialogManager().SetPending("tab1", b.dialog)
	return nil, b.err
}

var dialogBlockedBranches = []struct {
	name    string
	err     func(*bridge.DialogState) error
	message string
}{
	{"click timeout", func(*bridge.DialogState) error { return context.DeadlineExceeded }, "timed out"},
	{"dialog blocking", func(d *bridge.DialogState) error {
		return &bridge.ErrDialogBlocking{DialogType: d.Type, DialogMessage: d.Message}
	}, "click blocked"},
}

func TestBatchAndMacroStepsCarryTheDialogRemedyExactlyOnce(t *testing.T) {
	dialog := &bridge.DialogState{Type: "confirm", Message: "leave page?"}
	for _, branch := range dialogBlockedBranches {
		for _, tc := range []struct{ name, path, body string }{
			{"batch", "/actions", `{"tabId":"tab1","actions":[{"kind":"click"}]}`},
			{"macro", "/macro", `{"tabId":"tab1","steps":[{"kind":"click"}]}`},
		} {
			t.Run(branch.name+"/"+tc.name, func(t *testing.T) {
				b := &dialogRaisingBridge{dialog: dialog, err: branch.err(dialog)}
				h := New(b, &config.RuntimeConfig{AllowMacro: true}, nil, nil, nil)
				req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader([]byte(tc.body)))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				if tc.path == "/macro" {
					h.HandleMacro(rec, req)
				} else {
					h.HandleActions(rec, req)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("%s answered %d: %s", tc.path, rec.Code, rec.Body.String())
				}
				var envelope batchEnvelope
				if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
					t.Fatalf("decode: %v: %s", err, rec.Body.String())
				}
				if len(envelope.Results) != 1 || envelope.Results[0].Success {
					t.Fatalf("want one failed result: %s", rec.Body.String())
				}
				got := envelope.Results[0].Error
				if !strings.Contains(got, branch.message) || !strings.Contains(got, dialog.Message) {
					t.Fatalf("step error does not name the condition: %q", got)
				}
				if n := strings.Count(got, dialogBlockedStepRemedy); n != 1 {
					t.Fatalf("remedy sentence appears %d times, want 1: %q", n, got)
				}
				if strings.Contains(got, "accept|dismiss") {
					t.Fatalf("step error uses the pipeline form: %q", got)
				}
			})
		}
	}
}

func TestSingleActionDialogBlockedMessageCarriesNoRemedy(t *testing.T) {
	dialog := &bridge.DialogState{Type: "alert", Message: "saved"}
	for _, branch := range dialogBlockedBranches {
		t.Run(branch.name, func(t *testing.T) {
			b := &dialogRaisingBridge{dialog: dialog, err: branch.err(dialog)}
			h := New(b, &config.RuntimeConfig{}, nil, nil, nil)
			rec := httptest.NewRecorder()
			body := bytes.NewReader([]byte(`{"kind":"click","tabId":"tab1"}`))
			h.HandleAction(rec, httptest.NewRequest(http.MethodPost, "/action", body))
			decodeDialogBlocked(t, rec, dialog)
			var failure dialogBlockedFailure
			if err := json.Unmarshal(rec.Body.Bytes(), &failure); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(failure.Error, branch.message) {
				t.Fatalf("message does not name the condition: %q", failure.Error)
			}
			for _, remedy := range []string{dialogBlockedStepRemedy, "--dialog-action", "pinchtab dialog"} {
				if strings.Contains(failure.Error, remedy) {
					t.Fatalf("message carries the remedy %q: %q", remedy, failure.Error)
				}
			}
			if failure.Details.Hint != dialogBlockedHint {
				t.Fatalf("hint = %q, want %q", failure.Details.Hint, dialogBlockedHint)
			}
		})
	}
}

func TestErrDialogBlockingCarriesNoRemedy(t *testing.T) {
	got := (&bridge.ErrDialogBlocking{DialogType: "prompt", DialogMessage: "name?"}).Error()
	for _, remedy := range []string{"--dialog-action", "pinchtab dialog", dialogBlockedStepRemedy} {
		if strings.Contains(got, remedy) {
			t.Fatalf("bridge error carries the remedy %q: %q", remedy, got)
		}
	}
	if !strings.Contains(got, `prompt: "name?"`) {
		t.Fatalf("bridge error does not name the dialog: %q", got)
	}
}

func TestDialogActionRemedyLiteralHasOneOwnerBesideTheFlagHelp(t *testing.T) {
	const literal = "dialog-action accept"
	allowed := map[string]bool{
		"internal/handlers/dialog_blocked.go":     true,
		"cmd/pinchtab/cmd_cli_browser_actions.go": true,
	}
	found := map[string]bool{}
	for _, source := range srccensus.Tree(t, filepath.Join("..", ".."), 200) {
		if !strings.HasPrefix(source.Name, "internal/") && !strings.HasPrefix(source.Name, "cmd/") {
			continue
		}
		f := srccensus.Parse(t, token.NewFileSet(), source.Name, source.Text, 0)
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(lit.Value, literal) {
				found[source.Name] = true
			}
			return true
		})
	}
	if !found["internal/handlers/dialog_blocked.go"] {
		t.Fatalf("dialog_blocked.go no longer owns the %q remedy: %v", literal, found)
	}
	for file := range found {
		if !allowed[file] {
			t.Errorf("%s spells the %q remedy; dialog_blocked.go is the one owner", file, literal)
		}
	}
}
