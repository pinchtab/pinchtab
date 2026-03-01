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

	err := NavigatePage(ctx, "https://example.com")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
