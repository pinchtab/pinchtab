package bridge

import (
	"context"
	"time"
)

// TabHandle wraps a chromedp-enriched context, providing an opaque return
// type from TabContext(). Handlers receive *TabHandle instead of a raw
// context.Context, signaling that the context should only be passed to
// Bridge methods — not to chromedp directly.
//
// TabHandle fully implements context.Context, delegating all methods
// (including Value) to the underlying CDP context.
type TabHandle struct {
	cdpCtx context.Context
}

func NewTabHandle(cdpCtx context.Context) *TabHandle {
	return &TabHandle{cdpCtx: cdpCtx}
}

func (h *TabHandle) Deadline() (time.Time, bool) { return h.cdpCtx.Deadline() }
func (h *TabHandle) Done() <-chan struct{}       { return h.cdpCtx.Done() }
func (h *TabHandle) Err() error                  { return h.cdpCtx.Err() }
func (h *TabHandle) Value(key any) any           { return h.cdpCtx.Value(key) }
