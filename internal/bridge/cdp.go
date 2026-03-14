package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
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

// ErrTooManyRedirects is returned when a navigation exceeds the configured redirect limit.
var ErrTooManyRedirects = fmt.Errorf("too many redirects")

// NavigatePageWithRedirectLimit navigates to a URL and enforces a maximum number of
// HTTP redirects using the Fetch domain to intercept requests. If maxRedirects < 0,
// redirects are unlimited. If maxRedirects == 0, any redirect is blocked.
// If maxRedirects > 0, up to that many hops are allowed.
func NavigatePageWithRedirectLimit(ctx context.Context, url string, maxRedirects int) error {
	// Unlimited: just navigate normally.
	if maxRedirects < 0 {
		return NavigatePage(ctx, url)
	}

	// Use Fetch domain to intercept and count redirects.
	// This pauses each request so we can decide to continue or fail it.
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().Do(ctx)
	})); err != nil {
		return fmt.Errorf("fetch enable: %w", err)
	}

	var redirectCount atomic.Int32
	var blocked atomic.Bool

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		e, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		// Handle in goroutine to avoid deadlocking the event dispatcher.
		go func() {
			reqID := e.RequestID
			// A non-empty RedirectedRequestID means this request is a redirect.
			if e.RedirectedRequestID != "" {
				count := int(redirectCount.Add(1))
				if count > maxRedirects {
					blocked.Store(true)
					_ = fetch.FailRequest(reqID, network.ErrorReasonBlockedByClient).Do(cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Target))
					return
				}
			}
			// Allow the request to proceed.
			_ = fetch.ContinueRequest(reqID).Do(cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Target))
		}()
	})

	// Start navigation.
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, _, _, err := page.Navigate(url).Do(ctx)
			return err
		}),
	)

	// Disable fetch interception regardless of outcome.
	_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Disable().Do(ctx)
	}))

	if blocked.Load() {
		return fmt.Errorf("%w: got %d, max %d", ErrTooManyRedirects, redirectCount.Load(), maxRedirects)
	}
	if err != nil {
		return err
	}

	// Wait for page to finish loading.
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var state string
			err := chromedp.Run(ctx,
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

func ClickByCoordinate(ctx context.Context, x, y float64) error {
	if x < 0 || y < 0 {
		return fmt.Errorf("x/y coordinates must be >= 0")
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx,
				"Input.dispatchMouseEvent", map[string]any{
					"type":       "mousePressed",
					"button":     "left",
					"clickCount": 1,
					"x":          x,
					"y":          y,
				}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx,
				"Input.dispatchMouseEvent", map[string]any{
					"type":       "mouseReleased",
					"button":     "left",
					"clickCount": 1,
					"x":          x,
					"y":          y,
				}, nil)
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

	// Some nodes (e.g. Svelte5 snippet child nodes) have a zero-size bounding box
	// in the accessibility tree. Fall back to getBoundingClientRect() for accurate
	// viewport coordinates.
	if x == 0 && y == 0 {
		return getElementCenterJS(ctx, backendNodeID)
	}

	return x, y, nil
}

// getElementCenterJS resolves the DOM node and evaluates getBoundingClientRect()
// to determine the centre of its rendered area. It is used as a fallback when
// DOM.getBoxModel returns a zero bounding box.
func getElementCenterJS(ctx context.Context, backendNodeID int64) (float64, float64, error) {
	var resolveResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &resolveResult)
	})); err != nil {
		return 0, 0, fmt.Errorf("DOM.resolveNode: %w", err)
	}

	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(resolveResult, &resolved); err != nil {
		return 0, 0, err
	}
	if resolved.Object.ObjectID == "" {
		return 0, 0, fmt.Errorf("element not found in DOM (backendNodeId=%d)", backendNodeID)
	}

	const rectFn = `function() {
		var r = this.getBoundingClientRect();
		return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
	}`

	var callResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": rectFn,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}, &callResult)
	})); err != nil {
		return 0, 0, fmt.Errorf("getBoundingClientRect: %w", err)
	}

	var callRes struct {
		Result struct {
			Value struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(callResult, &callRes); err != nil {
		return 0, 0, err
	}

	return callRes.Result.Value.X, callRes.Result.Value.Y, nil
}

// DragByNodeID drags an element by (dx, dy) pixels using mousePressed → mouseMoved → mouseReleased.
func DragByNodeID(ctx context.Context, nodeID int64, dx, dy int) error {
	x, y, err := getElementCenter(ctx, nodeID)
	if err != nil {
		return err
	}

	endX := x + float64(dx)
	endY := y + float64(dy)

	// Number of intermediate mouseMoved events — proportional to distance,
	// clamped to [5, 40] to keep the drag smooth without flooding CDP.
	dist := math.Sqrt(float64(dx*dx + dy*dy))
	steps := int(dist / 10)
	if steps < 5 {
		steps = 5
	}
	if steps > 40 {
		steps = 40
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved",
				"x":    x, "y": y,
			}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mousePressed",
				"button":     "left",
				"clickCount": 1,
				"x":          x, "y": y,
			}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 1; i <= steps; i++ {
				t := float64(i) / float64(steps)
				mx := x + t*float64(dx)
				my := y + t*float64(dy)
				if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
					"type":    "mouseMoved",
					"buttons": 1,
					"x":       mx, "y": my,
				}, nil); err != nil {
					return err
				}
			}
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"button":     "left",
				"clickCount": 1,
				"x":          endX, "y": endY,
			}, nil)
		}),
	)
}

// namedKeyDefs maps friendly key names (as accepted by the CLI "press" command)
// to their CDP Input.dispatchKeyEvent parameters. Keys not in this table fall
// through to chromedp.KeyEvent so that single printable characters still work.
var namedKeyDefs = map[string]struct {
	code       string
	virtualKey int64
	insertText string // non-empty for keys that produce a character (Enter→\r, Tab→\t)
}{
	"Enter":      {"Enter", 13, "\r"},
	"Return":     {"Enter", 13, "\r"},
	"Tab":        {"Tab", 9, "\t"},
	"Escape":     {"Escape", 27, ""},
	"Backspace":  {"Backspace", 8, ""},
	"Delete":     {"Delete", 46, ""},
	"ArrowLeft":  {"ArrowLeft", 37, ""},
	"ArrowRight": {"ArrowRight", 39, ""},
	"ArrowUp":    {"ArrowUp", 38, ""},
	"ArrowDown":  {"ArrowDown", 40, ""},
	"Home":       {"Home", 36, ""},
	"End":        {"End", 35, ""},
	"PageUp":     {"PageUp", 33, ""},
	"PageDown":   {"PageDown", 34, ""},
	"Insert":     {"Insert", 45, ""},
	"F1":         {"F1", 112, ""},
	"F2":         {"F2", 113, ""},
	"F3":         {"F3", 114, ""},
	"F4":         {"F4", 115, ""},
	"F5":         {"F5", 116, ""},
	"F6":         {"F6", 117, ""},
	"F7":         {"F7", 118, ""},
	"F8":         {"F8", 119, ""},
	"F9":         {"F9", 120, ""},
	"F10":        {"F10", 121, ""},
	"F11":        {"F11", 122, ""},
	"F12":        {"F12", 123, ""},
}

// DispatchNamedKey sends proper CDP keyDown / keyUp events for well-known key
// names (e.g. "Enter", "Tab", "Escape", "ArrowLeft") so that JavaScript event
// handlers receive a KeyboardEvent with the correct key property.
//
// Unlike chromedp.KeyEvent, which treats multi-character strings as text
// sequences and would type "Enter" as five separate characters, this function
// consults namedKeyDefs and emits a single logical keystroke. Unrecognised keys
// fall back to chromedp.KeyEvent so that single printable characters still work.
func DispatchNamedKey(ctx context.Context, key string) error {
	def, ok := namedKeyDefs[key]
	if !ok {
		return chromedp.Run(ctx, chromedp.KeyEvent(key))
	}

	// Normalise "Return" → "Enter" for the W3C key value.
	w3cKey := key
	if key == "Return" {
		w3cKey = "Enter"
	}

	dispatchEvent := func(evType string) chromedp.ActionFunc {
		return chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchKeyEvent", map[string]any{
				"type":                  evType,
				"key":                   w3cKey,
				"code":                  def.code,
				"windowsVirtualKeyCode": def.virtualKey,
				"nativeVirtualKeyCode":  def.virtualKey,
			}, nil)
		})
	}

	actions := chromedp.Tasks{dispatchEvent("rawKeyDown")}
	if def.insertText != "" {
		insertText := def.insertText
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.insertText", map[string]any{
				"text": insertText,
			}, nil)
		}))
	}
	actions = append(actions, dispatchEvent("keyUp"))

	return chromedp.Run(ctx, actions...)
}

func TypeByNodeID(ctx context.Context, nodeID int64, text string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.KeyEvent(text),
	)
}

func HoverByCoordinate(ctx context.Context, x, y float64) error {
	if x < 0 || y < 0 {
		return fmt.Errorf("x/y coordinates must be >= 0")
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved",
				"x":    x,
				"y":    y,
			}, nil)
		}),
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

func FillByNodeID(ctx context.Context, nodeID int64, value string) error {
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
