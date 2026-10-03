package bridge

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/selector"
)

func (b *Bridge) SetFileInputFiles(ctx context.Context, backendNodeID int64, paths []string) error {
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return dom.SetFileInputFiles(paths).WithBackendNodeID(cdp.BackendNodeID(backendNodeID)).Do(ctx)
	}))
	if err == nil {
		return nil
	}
	if refusal, probeErr := bridgecdpops.NotFileInput(ctx, backendNodeID); probeErr == nil && refusal != nil {
		return fmt.Errorf("%w: %w", ErrInvalidActionRequest, refusal)
	}
	return err
}

func (b *Bridge) ResolveSelectorToNodeID(ctx context.Context, raw string, refCache *RefCache, frameID string) (int64, error) {
	return ResolveUnifiedSelectorInFrame(ctx, selector.Parse(raw), refCache, frameID)
}
