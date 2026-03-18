package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestWaitForTitle_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := WaitForTitle(ctx, 5*time.Second)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestWaitForTitle_NoTimeout(t *testing.T) {
	ctx, _ := chromedp.NewContext(context.Background())

	// With timeout <= 0, should return immediately
	title, _ := WaitForTitle(ctx, 0)
	if title != "" {
		t.Errorf("expected empty title without browser, got %q", title)
	}
}

func TestNavigatePage_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NavigatePage(ctx, "https://pinchtab.com")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestShouldReplaceBlankHistoryEntry(t *testing.T) {
	tests := []struct {
		name       string
		curURL     string
		cur        int64
		entryCount int
		want       bool
	}{
		{name: "fresh blank tab", curURL: "about:blank", cur: 0, entryCount: 1, want: true},
		{name: "already navigated", curURL: "https://example.com", cur: 0, entryCount: 1, want: false},
		{name: "blank with history", curURL: "about:blank", cur: 1, entryCount: 2, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldReplaceBlankHistoryEntry(tc.curURL, tc.cur, tc.entryCount); got != tc.want {
				t.Fatalf("shouldReplaceBlankHistoryEntry(%q, %d, %d) = %v, want %v", tc.curURL, tc.cur, tc.entryCount, got, tc.want)
			}
		})
	}
}
