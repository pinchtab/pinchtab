package cdptk_test

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cdptk"
)

// A thrown script used to decode as a zero value at every hand-rolled call site,
// so the caller saw "not visible" or an empty box instead of the cause.
func TestCallFunctionOnSurfacesAThrownException(t *testing.T) {
	ctx := newHijackFixture(t)
	objectID, err := cdptk.IsolatedNodeObjectID(ctx, targetNodeID(t, ctx))
	if err != nil {
		t.Fatal(err)
	}

	var got struct{ X float64 }
	err = cdptk.CallFunctionOn(ctx, objectID, `function() { throw new Error("probe exploded"); }`, nil, &got)
	if err == nil {
		t.Fatalf("a throwing function returned no error and decoded %+v", got)
	}
	if !strings.Contains(err.Error(), "probe exploded") {
		t.Errorf("error %q does not carry the exception text", err)
	}
}

func TestCallFunctionOnDecodesTheReturnedValue(t *testing.T) {
	ctx := newHijackFixture(t)
	objectID, err := cdptk.IsolatedNodeObjectID(ctx, targetNodeID(t, ctx))
	if err != nil {
		t.Fatal(err)
	}

	var got string
	if err := cdptk.CallFunctionOn(ctx, objectID, `function(suffix) { return this.id + suffix; }`,
		[]map[string]any{{"value": "!"}}, &got); err != nil {
		t.Fatal(err)
	}
	if got != "target!" {
		t.Errorf("decoded %q, want %q", got, "target!")
	}
}

func TestCallFunctionOnForHandleErrorsOnAThrowInsteadOfReturningTheErrorHandle(t *testing.T) {
	ctx := newHijackFixture(t)
	objectID, err := cdptk.IsolatedNodeObjectID(ctx, targetNodeID(t, ctx))
	if err != nil {
		t.Fatal(err)
	}

	handle, err := cdptk.CallFunctionOnForHandle(ctx, objectID, `function() { throw new Error("resolver exploded"); }`, nil)
	if err == nil {
		t.Fatalf("a throwing function returned no error and handle %q", handle)
	}
	if !strings.Contains(err.Error(), "resolver exploded") {
		t.Errorf("error %q does not carry the exception text", err)
	}
}

func TestCallFunctionOnForHandleReturnsEmptyForNull(t *testing.T) {
	ctx := newHijackFixture(t)
	objectID, err := cdptk.IsolatedNodeObjectID(ctx, targetNodeID(t, ctx))
	if err != nil {
		t.Fatal(err)
	}

	handle, err := cdptk.CallFunctionOnForHandle(ctx, objectID, `function() { return this.querySelector(".absent"); }`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if handle != "" {
		t.Errorf("null result gave handle %q, want empty", handle)
	}

	handle, err = cdptk.CallFunctionOnForHandle(ctx, objectID, `function() { return this; }`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if handle == "" {
		t.Error("a node result gave no handle")
	}
}
