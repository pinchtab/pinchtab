package bridge

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestClassifyActionError_StaleNode(t *testing.T) {
	err := classifyActionError(errors.New("Node with given identifier does not exist"))
	if !errors.Is(err, ErrElementStale) {
		t.Fatalf("expected ErrElementStale, got %v", err)
	}
}

func TestClassifyActionError_PreservesTyped(t *testing.T) {
	err := classifyActionError(fmt.Errorf("wrapped: %w", ErrElementStale))
	if !errors.Is(err, ErrElementStale) {
		t.Fatalf("expected ErrElementStale, got %v", err)
	}
}

func TestNavigationChanged(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after string
		want          bool
	}{
		{"a different page", "https://a.example", "https://b.example", true},
		{"the same page", "https://a.example", "https://a.example", false},
		{"fragment and host case only", "https://A.EXAMPLE/path?x=1#section", "https://a.example/path?x=1", false},
		{"unreadable before", "", "https://b.example", false},
		{"unreadable after", "https://a.example", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := navigationChanged(tc.before, tc.after); got != tc.want {
				t.Errorf("navigationChanged(%q, %q) = %v, want %v", tc.before, tc.after, got, tc.want)
			}
		})
	}
}

func TestNormalizeGuardURL(t *testing.T) {
	got, ok := normalizeGuardURL("https://A.EXAMPLE/path#frag")
	if !ok {
		t.Fatal("expected URL normalization to succeed")
	}
	if got != "https://a.example/path" {
		t.Fatalf("expected normalized URL, got %q", got)
	}
}

func TestShouldCheckUnexpectedNavigation(t *testing.T) {
	if !shouldCheckUnexpectedNavigation(ActionRequest{}) {
		t.Fatal("click should be guarded when WaitNav is false")
	}
	if !shouldCheckUnexpectedNavigation(ActionRequest{}) {
		t.Fatal("press should be guarded when WaitNav is false")
	}
	if shouldCheckUnexpectedNavigation(ActionRequest{WaitNav: true}) {
		t.Fatal("WaitNav=true should disable navigation guard")
	}
	if shouldCheckUnexpectedNavigation(ActionRequest{Kind: ActionClick, Submit: true}) {
		t.Fatal("click submit should treat navigation as an expected post-state")
	}
}

func TestReadActionURL_NoChromeDPContext(t *testing.T) {
	u, err := defaultActionURLReader(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if u != "" {
		t.Fatalf("expected empty URL, got %q", u)
	}
}

func TestExecuteAction_UnexpectedNavigation_WhenEnabled(t *testing.T) {
	call := 0
	readActionURL := func(context.Context) (string, error) {
		call++
		if call == 1 {
			return "https://a.example", nil
		}
		return "https://b.example", nil
	}

	b := &Bridge{
		Config:    &config.RuntimeConfig{EnableActionGuards: true},
		URLReader: readActionURL,
		Actions: map[string]ActionFunc{
			ActionClick: func(context.Context, ActionRequest) (map[string]any, error) {
				return map[string]any{"ok": true}, nil
			},
			ActionType: func(context.Context, ActionRequest) (map[string]any, error) {
				return map[string]any{"ok": true}, nil
			},
		},
	}

	res, err := b.ExecuteAction(context.Background(), ActionClick, ActionRequest{})
	if err != nil {
		t.Fatalf("a click that ran and moved the page reported failure: %v", err)
	}
	assertNavigationOutcome(t, res, "https://a.example", "https://b.example")
}

// The whole defect in one assertion: the action's own result is what the API used
// to throw away when it returned the navigation as an error instead.
func assertNavigationOutcome(t *testing.T, res map[string]any, before, after string) {
	t.Helper()
	if res == nil {
		t.Fatal("no result delivered for an action that succeeded")
	}
	if res["ok"] != true {
		t.Errorf("the action's own result was discarded: %v", res)
	}
	if res[ResultNavigated] != true {
		t.Errorf("result does not report the navigation: %v", res)
	}
	if res[ResultLandedURL] != after {
		t.Errorf("landed url = %v, want %q", res[ResultLandedURL], after)
	}
	if res[ResultPreviousURL] != before {
		t.Errorf("previous url = %v, want %q", res[ResultPreviousURL], before)
	}
	if res[ResultRefsStale] != true {
		t.Errorf("result does not say the caller's refs are dead: %v", res)
	}
}

func TestExecuteAction_UnexpectedNavigationGuardDisabled(t *testing.T) {
	called := 0
	readActionURL := func(context.Context) (string, error) {
		called++
		return "https://a.example", nil
	}

	b := &Bridge{
		Config:    &config.RuntimeConfig{EnableActionGuards: false},
		URLReader: readActionURL,
		Actions: map[string]ActionFunc{
			ActionType: func(context.Context, ActionRequest) (map[string]any, error) {
				return map[string]any{"ok": true}, nil
			},
		},
	}

	if _, err := b.ExecuteAction(context.Background(), ActionType, ActionRequest{}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if called != 0 {
		t.Fatalf("expected readActionURL to not be called when guards are disabled, got %d calls", called)
	}
}

func TestExecuteAction_UnexpectedNavigation_WithNilConfigDefaultsEnabled(t *testing.T) {
	call := 0
	readActionURL := func(context.Context) (string, error) {
		call++
		if call == 1 {
			return "https://a.example", nil
		}
		return "https://b.example", nil
	}

	b := &Bridge{
		URLReader: readActionURL,
		Actions: map[string]ActionFunc{
			ActionType: func(context.Context, ActionRequest) (map[string]any, error) {
				return map[string]any{"ok": true}, nil
			},
		},
	}

	res, err := b.ExecuteAction(context.Background(), ActionType, ActionRequest{})
	if err != nil {
		t.Fatalf("an action that ran and moved the page reported failure: %v", err)
	}
	assertNavigationOutcome(t, res, "https://a.example", "https://b.example")
}

func TestExecuteAction_ClassifiesStaleError_WhenGuardsDisabled(t *testing.T) {
	b := &Bridge{
		Config: &config.RuntimeConfig{EnableActionGuards: false},
		Actions: map[string]ActionFunc{
			ActionType: func(context.Context, ActionRequest) (map[string]any, error) {
				return nil, errors.New("Node with given identifier does not exist")
			},
		},
	}

	_, err := b.ExecuteAction(context.Background(), ActionType, ActionRequest{})
	if !errors.Is(err, ErrElementStale) {
		t.Fatalf("expected ErrElementStale, got %v", err)
	}
}
