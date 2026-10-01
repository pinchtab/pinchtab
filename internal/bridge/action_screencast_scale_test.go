package bridge

import (
	"context"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestCoordinateClicksScaleScreencastFrameCoordsToTheViewport(t *testing.T) {
	tabCtx := serverCreatedTab(t, "<html><body></body></html>")
	vw, vh := fetchViewportSize(tabCtx)
	if vw <= 0 || vh <= 0 {
		t.Fatalf("viewport = %vx%v, want a real size", vw, vh)
	}

	for _, action := range []string{ActionClick, ActionDoubleClick} {
		t.Run(action, func(t *testing.T) {
			var gotX, gotY float64
			record := func(x, y float64) error {
				gotX, gotY = x, y
				return nil
			}
			ops := defaultPointerOps()
			ops.clickByCoordinate = func(_ context.Context, x, y float64, _ int) error { return record(x, y) }
			ops.doubleClickByCoordinate = func(_ context.Context, x, y float64) error { return record(x, y) }
			b := New(context.TODO(), nil, &config.RuntimeConfig{})
			b.pointerOps = ops

			req := ActionRequest{HasXY: true, X: 100, Y: 50, FrameW: vw * 2, FrameH: vh * 2}
			if _, err := b.Actions[action](tabCtx, req); err != nil {
				t.Fatalf("%s: %v", action, err)
			}
			if gotX != 50 || gotY != 25 {
				t.Fatalf("%s dispatched at (%v, %v), want the frame point (100, 50) scaled to the viewport (50, 25)", action, gotX, gotY)
			}
		})
	}
}
