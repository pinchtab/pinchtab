package bridge

import (
	"context"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/selector"
)

func (b *Bridge) SetFileInputFiles(ctx context.Context, backendNodeID int64, paths []string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return dom.SetFileInputFiles(paths).WithBackendNodeID(cdp.BackendNodeID(backendNodeID)).Do(ctx)
	}))
}

// ResolveSelectorToNodeID finds a DOM node by a unified selector string and returns its NodeID.
// Supports CSS (default), XPath (xpath: prefix or // auto-detect), and text (text: prefix).
func (b *Bridge) ResolveSelectorToNodeID(ctx context.Context, raw string) (int64, error) {
	return ResolveUnifiedSelectorInFrame(ctx, selector.Parse(raw), nil, "")
}
