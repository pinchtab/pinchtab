package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const TargetTypePage = "page"

// NavigatePage uses raw CDP Page.navigate + polls document.readyState for completion.
func NavigatePage(ctx context.Context, url string) error {
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, _, _, err := page.Navigate(url).Do(ctx)
			return err
		}),
	)
	if err != nil {
		return err
	}

	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("navigation timeout")
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var state string
			err = chromedp.Run(ctx,
				chromedp.Evaluate("document.readyState", &state),
			)
			if err == nil && (state == "interactive" || state == "complete") {
				return nil
			}
		}
	}
}

var ImageBlockPatterns = []string{
	"*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "*.svg", "*.ico",
}

var MediaBlockPatterns = append(ImageBlockPatterns,
	"*.mp4", "*.webm", "*.ogg", "*.mp3", "*.wav", "*.flac", "*.aac",
)

// SetResourceBlocking uses Network.setBlockedURLs to block resources by URL pattern.
func SetResourceBlocking(ctx context.Context, patterns []string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			if len(patterns) == 0 {
				return network.SetBlockedURLs([]string{}).Do(ctx)
			}
			return network.SetBlockedURLs(patterns).Do(ctx)
		}),
	)
}

func ClickByNodeID(ctx context.Context, nodeID int64) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mousePressed",
				"button":     "left",
				"clickCount": 1,
				"x":          0, "y": 0,
			}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"button":     "left",
				"clickCount": 1,
				"x":          0, "y": 0,
			}, nil)
		}),
	)
}

func TypeByNodeID(ctx context.Context, nodeID int64, text string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.KeyEvent(text),
	)
}

func HoverByNodeID(ctx context.Context, nodeID int64) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved",
				"x":    0, "y": 0,
			}, nil)
		}),
	)
}

func SelectByNodeID(ctx context.Context, nodeID int64, value string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var result json.RawMessage
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
				"backendNodeId": nodeID,
			}, &result); err != nil {
				return err
			}
			var resolved struct {
				Object struct {
					ObjectID string `json:"objectId"`
				} `json:"object"`
			}
			if err := json.Unmarshal(result, &resolved); err != nil {
				return err
			}
			js := `function(v) { this.value = v; this.dispatchEvent(new Event('input', {bubbles: true})); this.dispatchEvent(new Event('change', {bubbles: true})); }`
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
				"functionDeclaration": js,
				"objectId":            resolved.Object.ObjectID,
				"arguments":           []map[string]any{{"value": value}},
			}, nil)
		}),
	)
}

func ScrollByNodeID(ctx context.Context, nodeID int64) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": nodeID}, nil)
		}),
	)
}

func WaitForTitle(ctx context.Context, timeout time.Duration) string {
	if timeout <= 0 {
		var title string
		_ = chromedp.Run(ctx, chromedp.Title(&title))
		return title
	}

	start := time.Now()
	for time.Since(start) < timeout {
		var title string
		_ = chromedp.Run(ctx, chromedp.Title(&title))
		if title != "" && title != "about:blank" {
			return title
		}
		time.Sleep(200 * time.Millisecond)
	}
	var title string
	_ = chromedp.Run(ctx, chromedp.Title(&title))
	return title
}
