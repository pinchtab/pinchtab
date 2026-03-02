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

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
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
	// Get element position via box model
	x, y, err := getElementCenter(ctx, nodeID)
	if err != nil {
		return err
	}

	return chromedp.Run(ctx,
		// Scroll element into view first
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		// Focus the element
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		// Mouse down at element center
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mousePressed",
				"button":     "left",
				"clickCount": 1,
				"x":          x, "y": y,
			}, nil)
		}),
		// Mouse up at element center
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"button":     "left",
				"clickCount": 1,
				"x":          x, "y": y,
			}, nil)
		}),
	)
}

// getElementCenter returns the center coordinates of an element using DOM.getBoxModel.
func getElementCenter(ctx context.Context, backendNodeID int64) (x, y float64, err error) {
	var result json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getBoxModel", map[string]any{
			"backendNodeId": backendNodeID,
		}, &result)
	}))
	if err != nil {
		return 0, 0, err
	}

	// Parse the box model response
	// The "content" quad is [x1,y1, x2,y2, x3,y3, x4,y4] - four corners
	var box struct {
		Model struct {
			Content []float64 `json:"content"`
		} `json:"model"`
	}
	if err = json.Unmarshal(result, &box); err != nil {
		return 0, 0, err
	}

	if len(box.Model.Content) < 4 {
		return 0, 0, fmt.Errorf("invalid box model: expected at least 4 coordinates")
	}

	// Content quad: [x1,y1, x2,y2, x3,y3, x4,y4]
	// Calculate center as average of all four corners
	x = (box.Model.Content[0] + box.Model.Content[2] + box.Model.Content[4] + box.Model.Content[6]) / 4
	y = (box.Model.Content[1] + box.Model.Content[3] + box.Model.Content[5] + box.Model.Content[7]) / 4

	return x, y, nil
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
	// Get element position via box model
	x, y, err := getElementCenter(ctx, nodeID)
	if err != nil {
		return err
	}

	return chromedp.Run(ctx,
		// Scroll element into view first
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		// Move mouse to element center
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved",
				"x":    x, "y": y,
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

func WaitForTitle(ctx context.Context, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		var title string
		if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
			return "", err
		}
		return title, nil
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline:
			var title string
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
				return "", err
			}
			return title, nil
		case <-ticker.C:
			var title string
			if err := chromedp.Run(ctx, chromedp.Title(&title)); err != nil {
				continue
			}
			if title != "" && title != "about:blank" {
				return title, nil
			}
		}
	}
}
