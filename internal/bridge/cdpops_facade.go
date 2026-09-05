package bridge

import (
	"context"
	"fmt"
	"math"
	"time"

	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

const TargetTypePage = bridgecdpops.TargetTypePage

var (
	ImageBlockPatterns  = bridgecdpops.ImageBlockPatterns
	MediaBlockPatterns  = bridgecdpops.MediaBlockPatterns
	ErrElementOccluded  = bridgecdpops.ErrElementOccluded
	ErrElementHidden    = bridgecdpops.ErrElementHidden
	ErrElementBlocked   = bridgecdpops.ErrElementBlocked
	ErrElementOffscreen = bridgecdpops.ErrElementOffscreen
)

func NavigatePage(ctx context.Context, url string) error {
	return bridgecdpops.NavigatePage(ctx, url)
}

func DispatchNavigation(ctx context.Context, url string) error {
	return bridgecdpops.DispatchNavigation(ctx, url)
}

func shouldReplaceBlankHistoryEntry(curURL string, cur int64, entryCount int) bool {
	return bridgecdpops.ShouldReplaceBlankHistoryEntry(curURL, cur, entryCount)
}

func WaitForTitle(ctx context.Context, timeout time.Duration) (string, error) {
	return bridgecdpops.WaitForTitle(ctx, timeout)
}

func SetResourceBlocking(ctx context.Context, patterns []string) error {
	return bridgecdpops.SetResourceBlocking(ctx, patterns)
}

// ScrollIntoViewAndGetBox scrolls the element into view and reports the box it
// ended up at, in the same top-level viewport space /box and /capture report.
func ScrollIntoViewAndGetBox(ctx context.Context, nodeID int64) (map[string]any, error) {
	if err := bridgecdpops.ScrollIntoViewIfNeeded(ctx, nodeID); err != nil {
		return nil, err
	}
	box, ok := ElementBorderBox(ctx, nodeID)
	if !ok {
		return nil, fmt.Errorf("element has no box model (backendNodeId=%d)", nodeID)
	}
	return map[string]any{
		"scrolled": true,
		"box": map[string]any{
			"x":      math.Round(box.X),
			"y":      math.Round(box.Y),
			"width":  math.Round(box.W),
			"height": math.Round(box.H),
		},
	}, nil
}

func PointerPointForNode(ctx context.Context, nodeID int64, requireTopMost bool) (float64, float64, error) {
	return bridgecdpops.PointerPointForNode(ctx, nodeID, requireTopMost)
}

func ClickByCoordinate(ctx context.Context, x, y float64, modifiers int) error {
	return bridgecdpops.ClickByCoordinate(ctx, x, y, modifiers)
}

func ClickByNodeID(ctx context.Context, nodeID int64) error {
	return bridgecdpops.ClickByNodeID(ctx, nodeID)
}

func JSClickByBackendNode(ctx context.Context, nodeID int64) error {
	return bridgecdpops.JSClickByBackendNode(ctx, nodeID)
}

func JSDispatchClickByBackendNode(ctx context.Context, nodeID int64) error {
	return bridgecdpops.JSDispatchClickByBackendNode(ctx, nodeID)
}

func DoubleClickByCoordinate(ctx context.Context, x, y float64) error {
	return bridgecdpops.DoubleClickByCoordinate(ctx, x, y)
}

func DoubleClickByNodeID(ctx context.Context, nodeID int64) error {
	return bridgecdpops.DoubleClickByNodeID(ctx, nodeID)
}

func JSDoubleClickByBackendNode(ctx context.Context, nodeID int64) error {
	return bridgecdpops.JSDoubleClickByBackendNode(ctx, nodeID)
}

func DragByNodeID(ctx context.Context, nodeID int64, dx, dy int, button string) error {
	return bridgecdpops.DragByNodeID(ctx, nodeID, dx, dy, button)
}

func DragBetweenPoints(ctx context.Context, x, y, endX, endY float64, button string) error {
	return bridgecdpops.DragBetweenPoints(ctx, x, y, endX, endY, button)
}

func HoverByCoordinate(ctx context.Context, x, y float64) error {
	return bridgecdpops.HoverByCoordinate(ctx, x, y)
}

func MouseMoveByCoordinate(ctx context.Context, x, y float64) error {
	return bridgecdpops.MouseMoveByCoordinate(ctx, x, y)
}

func MouseDownByCoordinate(ctx context.Context, x, y float64, button string, modifiers int) error {
	return bridgecdpops.MouseDownByCoordinate(ctx, x, y, button, modifiers)
}

func MouseUpByCoordinate(ctx context.Context, x, y float64, button string, modifiers int) error {
	return bridgecdpops.MouseUpByCoordinate(ctx, x, y, button, modifiers)
}

func MouseWheelByCoordinate(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) error {
	return bridgecdpops.MouseWheelByCoordinate(ctx, x, y, deltaX, deltaY, modifiers)
}

func ScrollByCoordinate(ctx context.Context, x, y float64, deltaX, deltaY, modifiers int) error {
	return bridgecdpops.ScrollByCoordinate(ctx, x, y, deltaX, deltaY, modifiers)
}

func HoverByNodeID(ctx context.Context, nodeID int64) error {
	return bridgecdpops.HoverByNodeID(ctx, nodeID)
}

func FillByNodeID(ctx context.Context, nodeID int64, value string) error {
	return bridgecdpops.FillByNodeID(ctx, nodeID, value)
}

func SelectByNodeID(ctx context.Context, nodeID int64, value string) error {
	return bridgecdpops.SelectByNodeID(ctx, nodeID, value)
}

func ReadInputValue(ctx context.Context, nodeID int64) (string, error) {
	return bridgecdpops.ReadInputValue(ctx, nodeID)
}

func ScrollByNodeID(ctx context.Context, nodeID int64) error {
	return bridgecdpops.ScrollByNodeID(ctx, nodeID)
}
