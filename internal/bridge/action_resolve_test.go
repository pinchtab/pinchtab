package bridge

import (
	"context"
	"errors"
	"testing"

	"github.com/pinchtab/pinchtab/internal/selector"
)

// Only a genuine no-match must carry ErrSelectorNoMatch (→ 404); unsupported
// kinds and internal routing errors must NOT (→ 5xx).
func TestResolveUnifiedSelector_ErrorClassification(t *testing.T) {
	ctx := context.Background()

	if _, err := ResolveUnifiedSelectorInFrame(ctx, selector.Selector{Kind: selector.KindRef, Value: "e99"}, nil, ""); !errors.Is(err, ErrSelectorNoMatch) {
		t.Errorf("ref-not-found should be ErrSelectorNoMatch (404), got %v", err)
	}
	if _, err := ResolveUnifiedSelectorInFrame(ctx, selector.Selector{Kind: selector.KindSemantic, Value: "Save"}, nil, ""); err == nil || errors.Is(err, ErrSelectorNoMatch) {
		t.Errorf("semantic-at-resolver should be a non-no-match internal error (5xx), got %v", err)
	}
	if _, err := ResolveUnifiedSelectorInFrame(ctx, selector.Selector{Kind: "bogus", Value: "x"}, nil, ""); err == nil || errors.Is(err, ErrSelectorNoMatch) {
		t.Errorf("unknown selector kind should be non-no-match (5xx), got %v", err)
	}
}

func TestParseNthSelectorValue(t *testing.T) {
	index, raw, err := parseNthSelectorValue("2:role:button Save")
	if err != nil {
		t.Fatalf("parseNthSelectorValue returned error: %v", err)
	}
	if index != 2 || raw != "role:button Save" {
		t.Fatalf("got index=%d raw=%q, want 2 role selector", index, raw)
	}
}

func TestParseNthSelectorValueRejectsInvalidIndex(t *testing.T) {
	if _, _, err := parseNthSelectorValue("-1:button"); err == nil {
		t.Fatal("expected negative index to fail")
	}
	if _, _, err := parseNthSelectorValue("button"); err == nil {
		t.Fatal("expected missing nested selector to fail")
	}
}

func TestResolveUnifiedSelector_FirstRef(t *testing.T) {
	cache := &RefCache{Refs: map[string]int64{"e5": 42}}
	got, err := ResolveUnifiedSelectorInFrame(context.Background(), selector.Parse("first:e5"), cache, "")
	if err != nil {
		t.Fatalf("ResolveUnifiedSelectorInFrame returned error: %v", err)
	}
	if got != 42 {
		t.Fatalf("node id = %d, want 42", got)
	}
}

func TestResolveUnifiedSelector_LastRefRejected(t *testing.T) {
	cache := &RefCache{Refs: map[string]int64{"e5": 42}}
	if _, err := ResolveUnifiedSelectorInFrame(context.Background(), selector.Parse("last:e5"), cache, ""); err == nil {
		t.Fatal("expected last:ref to fail")
	}
}
