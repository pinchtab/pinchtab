package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func postAction(t *testing.T, h *Handlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/action", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleAction(w, req)
	return w
}

func TestUnknownActionKindAnswersOneCodedRefusalFromEitherPath(t *testing.T) {
	for _, tc := range []struct {
		name      string
		available []string
		execErr   error
	}{
		{"refused up front by the available list", []string{"click", "type"}, nil},
		{"refused after dispatch by the bridge sentinel", []string{}, fmt.Errorf("%w: zap", bridge.ErrUnknownAction)},
		{"survives a reword of the bridge message", []string{}, fmt.Errorf("no such verb %q: %w", "zap", bridge.ErrUnknownAction)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mb := &mockBridge{availableActions: tc.available, executeActionErr: tc.execErr}
			h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
			w := postAction(t, h, `{"kind":"zap","tabId":"tab1"}`)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
			var body struct {
				Code  string `json:"code"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v: %s", err, w.Body.String())
			}
			if body.Code != "unknown_action_kind" {
				t.Fatalf("code = %q, want unknown_action_kind: %s", body.Code, w.Body.String())
			}
			if !strings.Contains(body.Error, "zap") {
				t.Fatalf("message %q does not name the kind", body.Error)
			}
			if valid := mb.AvailableActions(); len(valid) > 0 && !strings.Contains(body.Error, "valid values: "+strings.Join(valid, ", ")) {
				t.Fatalf("message %q does not list the valid kinds", body.Error)
			}
		})
	}
}

func TestUnknownActionKindListsTheValidKindsInAStableOrder(t *testing.T) {
	available := []string{"type", "click", "hover"}
	mb := &mockBridge{availableActions: available}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	w := postAction(t, h, `{"kind":"zap","tabId":"tab1"}`)
	var body struct {
		Error   string `json:"error"`
		Details struct {
			ValidKinds []string `json:"validKinds"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	want := []string{"click", "hover", "type"}
	if strings.Join(body.Details.ValidKinds, ",") != strings.Join(want, ",") {
		t.Fatalf("validKinds = %v, want %v", body.Details.ValidKinds, want)
	}
	if !strings.HasSuffix(body.Error, "valid values: click, hover, type") {
		t.Fatalf("message %q does not list the kinds sorted", body.Error)
	}
	if strings.Join(available, ",") != "type,click,hover" {
		t.Fatalf("the bridge's own list was reordered in place: %v", available)
	}
}

func TestAnUnwrappedUnknownActionMessageIsNoLongerRecoveredByPrefix(t *testing.T) {
	mb := &mockBridge{availableActions: []string{}, executeActionErr: fmt.Errorf("unknown action: zap")}
	h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
	w := postAction(t, h, `{"kind":"zap","tabId":"tab1"}`)
	if strings.Contains(w.Body.String(), "unknown_action_kind") {
		t.Fatalf("a bridge error that merely spells the old prefix was recovered as unknown_action_kind: %s", w.Body.String())
	}
}

func TestBatchStopOnErrorStopsAfterAPreDispatchFailure(t *testing.T) {
	for _, tc := range []struct {
		name        string
		stopOnError bool
		wantResults int
	}{
		{"stops after the failed selector resolution", true, 1},
		{"runs every step without stopOnError", false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := New(&mockBridge{}, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
			body := fmt.Sprintf(`{"tabId":"tab1","stopOnError":%v,"actions":[{"kind":"click","selector":"#missing"},{"kind":"click"}]}`, tc.stopOnError)
			req := httptest.NewRequest(http.MethodPost, "/actions", bytes.NewReader([]byte(body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.HandleActions(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			var envelope batchEnvelope
			if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if len(envelope.Results) != tc.wantResults || envelope.Results[0].Success {
				t.Fatalf("results = %+v, want %d with the first failed", envelope.Results, tc.wantResults)
			}
			if tc.wantResults == 2 && !envelope.Results[1].Success {
				t.Fatalf("second step failed although the first failure must not stop the run: %+v", envelope.Results[1])
			}
		})
	}
}

func TestEachStepTimeoutContextIsCancelledExactlyOnce(t *testing.T) {
	pkg := srccensus.Load(t, ".", handlersSourceFileFloor)
	checked := 0
	for _, name := range []string{"handleActionsBatch", "HandleMacro", "runMultiStep", "runMultiStepAction"} {
		fn, _ := handlerFuncDecl(t, pkg, name)
		checked++
		cancels := map[string]int{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 2 || len(assign.Rhs) != 1 {
				return true
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "WithTimeout" {
				cancels[assign.Lhs[1].(*ast.Ident).Name] = 0
			}
			return true
		})
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); ok {
				if _, tracked := cancels[id.Name]; tracked {
					cancels[id.Name]++
				}
			}
			return true
		})
		for name, count := range cancels {
			if count != 1 {
				t.Errorf("%s calls %s %d times, want exactly one cancellation path per step context", fn.Name.Name, name, count)
			}
		}
		if fn.Name.Name == "runMultiStepAction" {
			if len(cancels) != 0 {
				t.Errorf("%s creates its own timeout context; the caller owns the step budget", fn.Name.Name)
			}
		}
	}
	if checked != 4 {
		t.Fatalf("checked %d functions, want the two handlers, the shared loop and its step helper", checked)
	}
}

func TestNoHandlerRecoversAnUnknownActionByStringPrefix(t *testing.T) {
	fset, files := srccensus.Load(t, ".", handlersSourceFileFloor).Syntax()
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "HasPrefix" || len(call.Args) != 2 {
				return true
			}
			if lit, ok := call.Args[1].(*ast.BasicLit); ok && strings.Contains(lit.Value, "unknown action") {
				t.Errorf("%s still recovers the unknown-action error by string prefix; match bridge.ErrUnknownAction with errors.Is", fset.Position(call.Pos()))
			}
			return true
		})
	}
}
