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

// The forms that used to be excluded, and the reason the exclusion was wrong: a
// caller that DECLARES a navigation is the one most certain to need where it landed
// and that its refs are dead, and it was the one form that never got told. The
// exclusion made sense only while the check raised an error.
func TestEveryFormOfANavigatingActionReportsWhereItLanded(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  ActionRequest
	}{
		{"plain click", ActionRequest{Kind: ActionClick}},
		{"click that declares the navigation", ActionRequest{Kind: ActionClick, WaitNav: true}},
		{"submit click", ActionRequest{Kind: ActionClick, Submit: true, Ref: "e1"}},
		{"non-click action", ActionRequest{Kind: ActionType}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := 0
			b := &Bridge{
				Config: &config.RuntimeConfig{EnableActionGuards: true},
				URLReader: func(context.Context) (string, error) {
					call++
					if call == 1 {
						return "https://a.example", nil
					}
					return "https://b.example", nil
				},
				Actions: map[string]ActionFunc{
					ActionClick: func(context.Context, ActionRequest) (map[string]any, error) {
						return map[string]any{"ok": true}, nil
					},
					ActionType: func(context.Context, ActionRequest) (map[string]any, error) {
						return map[string]any{"ok": true}, nil
					},
				},
			}

			res, err := b.ExecuteAction(context.Background(), tc.req.Kind, tc.req)
			if err != nil {
				t.Fatalf("an action that ran and moved the page reported failure: %v", err)
			}
			assertNavigationOutcome(t, res, "https://a.example", "https://b.example")
		})
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

func TestExecuteAction_ReportsTheNavigationOutcomeWhateverEnableActionGuardsSays(t *testing.T) {
	settings := map[string]*config.RuntimeConfig{
		"guards on":  {EnableActionGuards: true},
		"guards off": {EnableActionGuards: false},
		"nil config": nil,
	}
	for name, cfg := range settings {
		t.Run(name, func(t *testing.T) {
			t.Run("a navigating click reports where it landed", func(t *testing.T) {
				res := executeClickAcross(t, cfg, "https://a.example", "https://b.example")
				assertNavigationOutcome(t, res, "https://a.example", "https://b.example")
			})
			t.Run("a click that stays put reports none of it", func(t *testing.T) {
				res := executeClickAcross(t, cfg, "https://a.example", "https://a.example")
				assertNoNavigationOutcome(t, res)
			})
		})
	}
}

func executeClickAcross(t *testing.T, cfg *config.RuntimeConfig, before, after string) map[string]any {
	t.Helper()
	call := 0
	b := &Bridge{
		Config: cfg,
		URLReader: func(context.Context) (string, error) {
			call++
			if call == 1 {
				return before, nil
			}
			return after, nil
		},
		Actions: map[string]ActionFunc{
			ActionClick: func(context.Context, ActionRequest) (map[string]any, error) {
				return map[string]any{"ok": true}, nil
			},
		},
	}
	res, err := b.ExecuteAction(context.Background(), ActionClick, ActionRequest{})
	if err != nil {
		t.Fatalf("a click that ran reported failure: %v", err)
	}
	if call != 2 {
		t.Fatalf("the page url was read %d times, want before and after the click", call)
	}
	return res
}

func assertNoNavigationOutcome(t *testing.T, res map[string]any) {
	t.Helper()
	if res["ok"] != true {
		t.Errorf("the action's own result was discarded: %v", res)
	}
	for _, key := range []string{ResultNavigated, ResultLandedURL, ResultPreviousURL, ResultRefsStale} {
		if _, present := res[key]; present {
			t.Errorf("a click that did not move the page reports %s: %v", key, res)
		}
	}
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
