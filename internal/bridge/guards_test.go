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

func TestCheckUnexpectedNavigation(t *testing.T) {
	err := checkUnexpectedNavigation("https://a.example", "https://b.example")
	if !errors.Is(err, ErrUnexpectedNavigation) {
		t.Fatalf("expected ErrUnexpectedNavigation, got %v", err)
	}
}

func TestCheckUnexpectedNavigation_NoChange(t *testing.T) {
	if err := checkUnexpectedNavigation("https://a.example", "https://a.example"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestCheckUnexpectedNavigation_EquivalentURLs(t *testing.T) {
	if err := checkUnexpectedNavigation("https://A.EXAMPLE/path?x=1#section", "https://a.example/path?x=1"); err != nil {
		t.Fatalf("expected nil error for equivalent URLs, got %v", err)
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

	_, err := b.ExecuteAction(context.Background(), ActionClick, ActionRequest{})
	if !errors.Is(err, ErrUnexpectedNavigation) {
		t.Fatalf("expected ErrUnexpectedNavigation, got %v", err)
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

	_, err := b.ExecuteAction(context.Background(), ActionType, ActionRequest{})
	if !errors.Is(err, ErrUnexpectedNavigation) {
		t.Fatalf("expected ErrUnexpectedNavigation, got %v", err)
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
