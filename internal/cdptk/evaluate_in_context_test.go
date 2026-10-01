package cdptk_test

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cdptk"
)

func TestEvaluateInContextSurfacesAThrownException(t *testing.T) {
	ctx := newHijackFixture(t)
	execID, err := cdptk.IsolatedContextID(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var got string
	err = cdptk.EvaluateInContext(ctx, execID, `(() => { throw new Error("boom"); })()`, false, &got)
	if err == nil {
		t.Fatalf("a throwing expression returned no error and decoded %q", got)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not carry the exception text", err)
	}
}

func TestEvaluateInContextAwaitsAPromiseWhenAsked(t *testing.T) {
	ctx := newHijackFixture(t)
	execID, err := cdptk.IsolatedContextID(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var got string
	if err := cdptk.EvaluateInContext(ctx, execID, `Promise.resolve(document.getElementById("target").id)`, true, &got); err != nil {
		t.Fatal(err)
	}
	if got != "target" {
		t.Errorf("decoded %q, want %q", got, "target")
	}
}
