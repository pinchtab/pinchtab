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
	ErrTooManyRedirects = bridgecdpops.ErrTooManyRedirects
	ErrElementOccluded  = bridgecdpops.ErrElementOccluded
	ErrElementBlocked   = bridgecdpops.ErrElementBlocked
	ErrElementOffscreen = bridgecdpops.ErrElementOffscreen
)

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
