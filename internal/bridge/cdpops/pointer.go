package cdpops

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/chromedp/chromedp"
)

func normalizeMouseButton(button string) string {
	switch strings.ToLower(strings.TrimSpace(button)) {
	case "right":
		return "right"
	case "middle":
		return "middle"
	default:
		return "left"
	}
}

func validatePointerCoordinates(x, y float64) error {
	if x < 0 || y < 0 {
		return fmt.Errorf("x/y coordinates must be >= 0")
	}
	return nil
}

func dispatchMouseEvent(ctx context.Context, payload map[string]any) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", payload, nil)
	}))
}

func MouseMoveByCoordinate(ctx context.Context, x, y float64) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	return dispatchMouseEvent(ctx, map[string]any{
		"type": "mouseMoved",
		"x":    x,
		"y":    y,
	})
}

func MouseDownByCoordinate(ctx context.Context, x, y float64, button string) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	return dispatchMouseEvent(ctx, map[string]any{
		"type":       "mousePressed",
		"button":     normalizeMouseButton(button),
		"clickCount": 1,
		"x":          x,
		"y":          y,
	})
}

func MouseUpByCoordinate(ctx context.Context, x, y float64, button string) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	return dispatchMouseEvent(ctx, map[string]any{
		"type":       "mouseReleased",
		"button":     normalizeMouseButton(button),
		"clickCount": 1,
		"x":          x,
		"y":          y,
	})
}

func MouseWheelByCoordinate(ctx context.Context, x, y float64, deltaX, deltaY int) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved",
				"x":    x,
				"y":    y,
			}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":   "mouseWheel",
				"x":      x,
				"y":      y,
				"deltaX": deltaX,
				"deltaY": deltaY,
			}, nil)
		}),
	)
}

func ClickByCoordinate(ctx context.Context, x, y float64) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}
	if err := MouseDownByCoordinate(ctx, x, y, "left"); err != nil {
		return err
	}
	return MouseUpByCoordinate(ctx, x, y, "left")
}

func ClickByNodeID(ctx context.Context, nodeID int64) error {
	x, y, err := PointerPointForNode(ctx, nodeID, true)
	if err != nil {
		return err
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
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
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"button":     "left",
				"clickCount": 1,
				"x":          x, "y": y,
			}, nil)
		}),
	)
}

func DoubleClickByCoordinate(ctx context.Context, x, y float64) error {
	if err := validatePointerCoordinates(x, y); err != nil {
		return err
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mousePressed",
				"button":     "left",
				"clickCount": 2,
				"x":          x,
				"y":          y,
			}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"button":     "left",
				"clickCount": 2,
				"x":          x,
				"y":          y,
			}, nil)
		}),
	)
}

func DoubleClickByNodeID(ctx context.Context, nodeID int64) error {
	x, y, err := PointerPointForNode(ctx, nodeID, true)
	if err != nil {
		return err
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": nodeID}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mousePressed",
				"button":     "left",
				"clickCount": 2,
				"x":          x, "y": y,
			}, nil)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"button":     "left",
				"clickCount": 2,
				"x":          x, "y": y,
			}, nil)
		}),
	)
}

// DragByNodeID drags an element by (dx, dy) pixels using mousePressed → mouseMoved → mouseReleased.
func DragByNodeID(ctx context.Context, nodeID int64, dx, dy int) error {
	x, y, err := PointerPointForNode(ctx, nodeID, true)
	if err != nil {
		return err
	}

	endX := x + float64(dx)
	endY := y + float64(dy)
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

func HoverByCoordinate(ctx context.Context, x, y float64) error {
	return MouseMoveByCoordinate(ctx, x, y)
}

func ScrollByCoordinate(ctx context.Context, x, y float64, deltaX, deltaY int) error {
	return MouseWheelByCoordinate(ctx, x, y, deltaX, deltaY)
}

func HoverByNodeID(ctx context.Context, nodeID int64) error {
	x, y, err := PointerPointForNode(ctx, nodeID, true)
	if err != nil {
		return err
	}

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved",
				"x":    x, "y": y,
			}, nil)
		}),
	)
}
