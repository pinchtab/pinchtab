package main

import (
	"context"
	"log/slog"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type TabEntry struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type refCache struct {
	refs  map[string]int64
	nodes []A11yNode
}

type Bridge struct {
	allocCtx      context.Context
	browserCtx    context.Context
	*TabManager
	stealthScript string
	actions       map[string]ActionFunc
	locks         *lockManager
}

func (b *Bridge) injectStealth(ctx context.Context) {
	if b.stealthScript == "" {
		return
	}
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(b.stealthScript).Do(ctx)
			return err
		}),
	); err != nil {
		slog.Warn("stealth injection failed", "err", err)
	}
}

// tabSetup is the hook called by TabManager on new tab contexts.
func (b *Bridge) tabSetup(ctx context.Context) {
	b.injectStealth(ctx)
	if cfg.NoAnimations {
		b.injectNoAnimations(ctx)
	}
}

// InitTabManager creates and wires the TabManager into the Bridge.
func (b *Bridge) InitTabManager() {
	b.TabManager = NewTabManager(b.browserCtx, b.tabSetup)
}
