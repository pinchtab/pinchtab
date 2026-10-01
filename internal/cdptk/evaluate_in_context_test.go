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

func TestEvaluateForHandleErrorsOnAThrow(t *testing.T) {
	ctx := newHijackFixture(t)
	execID, err := cdptk.IsolatedContextID(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	handle, err := cdptk.EvaluateForHandle(ctx, execID, `document.querySelector("[[bad")`)
	if err == nil {
		t.Fatalf("an invalid selector returned no error and handle %q", handle)
	}
	if !strings.Contains(err.Error(), "[[bad") {
		t.Errorf("error %q does not carry the exception text", err)
	}
}

func TestEvaluateValueSurfacesAThrownException(t *testing.T) {
	ctx := newHijackFixture(t)

	got, err := cdptk.EvaluateValue[float64](ctx, `(() => { throw new Error("measure exploded"); })()`)
	if err == nil {
		t.Fatalf("a throwing expression returned no error and decoded %v", got)
	}
	if !strings.Contains(err.Error(), "measure exploded") {
		t.Errorf("error %q does not carry the exception text", err)
	}
}
