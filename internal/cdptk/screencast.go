package cdptk

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/assets"
)

const ScreencastRepaintWorldName = "__pinchtab_screencast"

var GetFrameTree = func(ctx context.Context) (*page.FrameTree, error) {
	return page.GetFrameTree().Do(ctx)
}

var CreateIsolatedWorld = func(ctx context.Context, params *page.CreateIsolatedWorldParams) (runtime.ExecutionContextID, error) {
	return params.Do(ctx)
}

var EvaluateInWorld = func(ctx context.Context, params *runtime.EvaluateParams) (*runtime.RemoteObject, *runtime.ExceptionDetails, error) {
	return params.Do(ctx)
}

func CaptureScreenshotJPEG(ctx context.Context, quality int) ([]byte, error) {
	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			var err error
			buf, err = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(int64(quality)).
				Do(c)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func StartRepaintLoop(ctx context.Context) func() {
	execCtxID, err := CreateExecutionContext(ctx)
	if err != nil {
		slog.Warn("enable screencast repaint loop failed", "err", err)
		return func() {}
	}

	if err := EvaluateJS(ctx, execCtxID, assets.ScreencastRepaintStartJS); err != nil {
		slog.Warn("enable screencast repaint loop failed", "err", err)
		return func() {}
	}

	return func() {
		if err := EvaluateJS(ctx, execCtxID, assets.ScreencastRepaintStopJS); err != nil {
			slog.Warn("disable screencast repaint loop failed", "err", err)
		}
	}
}

func CreateExecutionContext(ctx context.Context) (runtime.ExecutionContextID, error) {
	frameTree, err := GetFrameTree(ctx)
	if err != nil {
		return 0, fmt.Errorf("get frame tree: %w", err)
	}
	if frameTree == nil || frameTree.Frame == nil {
		return 0, errors.New("missing top frame")
	}

	execCtxID, err := CreateIsolatedWorld(ctx, newScreencastIsolatedWorldParams(frameTree.Frame.ID))
	if err != nil {
		return 0, fmt.Errorf("create isolated world: %w", err)
	}
	return execCtxID, nil
}

func EvaluateJS(ctx context.Context, execCtxID runtime.ExecutionContextID, expression string) error {
	_, exceptionDetails, err := EvaluateInWorld(ctx, newScreencastEvaluateParams(execCtxID, expression))
	if err != nil {
		return fmt.Errorf("evaluate in isolated world: %w", err)
	}
	if exceptionDetails != nil {
		return fmt.Errorf("evaluate in isolated world: %w", exceptionDetails)
	}
	return nil
}

func newScreencastIsolatedWorldParams(frameID cdp.FrameID) *page.CreateIsolatedWorldParams {
	return page.CreateIsolatedWorld(frameID).WithWorldName(ScreencastRepaintWorldName)
}

func newScreencastEvaluateParams(execCtxID runtime.ExecutionContextID, expression string) *runtime.EvaluateParams {
	return runtime.Evaluate(expression).WithContextID(execCtxID)
}
