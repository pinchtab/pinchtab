package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/pinchtab/pinchtab/internal/config"
)

// EvaluateSubtype's whole job is telling a Promise apart from a genuine empty
// object, and that only works against real Chrome: under WithReturnByValue(true)
// CDP serialises the Promise and drops subtype:"promise", so a mock cannot expose
// the bug this pins. It drives a live browser end to end — a Promise reports the
// subtype while still decoding to {} (the prior output), a plain object decodes
// with no promise subtype, a primitive is unaffected, and --await-promise resolves
// the value. Fails against a WithReturnByValue(true) detection path.
func TestEvaluateSubtypeDetectsPromiseAgainstRealChrome(t *testing.T) {
	ctx := newActionTab(t, 20*time.Second)

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte("<html><body></body></html>"))
	if err := chromedp.Run(ctx, chromedp.Navigate(dataURL)); err != nil {
		t.Fatal(err)
	}

	b := New(context.Background(), nil, &config.RuntimeConfig{})

	t.Run("un-awaited promise reports the subtype and still decodes to {}", func(t *testing.T) {
		var result any
		subtype, err := b.EvaluateSubtype(ctx, "Promise.resolve(42)", &result, EvalOpts{})
		if err != nil {
			t.Fatalf("EvaluateSubtype: %v", err)
		}
		if subtype != "promise" {
			t.Fatalf("subtype = %q, want promise; the hint can never fire without it", subtype)
		}
		if m, ok := result.(map[string]any); !ok || len(m) != 0 {
			t.Errorf("result = %#v, want the empty object the prior path returned", result)
		}
	})

	t.Run("plain object decodes with no promise subtype", func(t *testing.T) {
		var result any
		subtype, err := b.EvaluateSubtype(ctx, "({a:1})", &result, EvalOpts{})
		if err != nil {
			t.Fatalf("EvaluateSubtype: %v", err)
		}
		if subtype == "promise" {
			t.Errorf("a plain object reported subtype promise: %q", subtype)
		}
		m, ok := result.(map[string]any)
		if !ok || m["a"] != float64(1) {
			t.Errorf("result = %#v, want {a:1} preserved (no regression)", result)
		}
	})

	t.Run("primitive is unaffected", func(t *testing.T) {
		var result any
		subtype, err := b.EvaluateSubtype(ctx, "42", &result, EvalOpts{})
		if err != nil {
			t.Fatalf("EvaluateSubtype: %v", err)
		}
		if subtype == "promise" {
			t.Errorf("a number reported subtype promise: %q", subtype)
		}
		if result != float64(42) {
			t.Errorf("result = %#v, want 42", result)
		}
	})

	t.Run("await-promise resolves the value", func(t *testing.T) {
		var result any
		subtype, err := b.EvaluateSubtype(ctx, "Promise.resolve(42)", &result, EvalOpts{AwaitPromise: true})
		if err != nil {
			t.Fatalf("EvaluateSubtype: %v", err)
		}
		if subtype == "promise" {
			t.Errorf("an awaited promise still reported subtype promise: %q", subtype)
		}
		if result != float64(42) {
			t.Errorf("awaited result = %#v, want 42", result)
		}
	})
}
