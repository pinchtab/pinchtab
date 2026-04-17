// Package adapters provides runtime-specific implementations of the
// autosolver interfaces. The pinchtab adapter is the ONLY place that
// imports chromedp and connects to the Pinchtab bridge runtime.
package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/bridge"
)

// PinchtabPage implements autosolver.Page by wrapping a chromedp tab context.
type PinchtabPage struct {
	ctx   context.Context
	tabID string
	b     bridge.BridgeAPI
}

// NewPinchtabPage creates a Page backed by a Pinchtab bridge tab.
func NewPinchtabPage(ctx context.Context, tabID string, b bridge.BridgeAPI) *PinchtabPage {
	return &PinchtabPage{ctx: ctx, tabID: tabID, b: b}
}

func (p *PinchtabPage) URL() string {
	var url string
	_ = chromedp.Run(p.ctx, chromedp.Location(&url))
	return url
}

func (p *PinchtabPage) Title() string {
	var title string
	_ = chromedp.Run(p.ctx, chromedp.Title(&title))
	return title
}

func (p *PinchtabPage) HTML() (string, error) {
	var html string
	err := chromedp.Run(p.ctx, chromedp.Evaluate(
		`document.documentElement.outerHTML`, &html))
	if err != nil {
		return "", fmt.Errorf("get HTML: %w", err)
	}
	return html, nil
}

func (p *PinchtabPage) Screenshot() ([]byte, error) {
	var buf []byte
	err := chromedp.Run(p.ctx, chromedp.CaptureScreenshot(&buf))
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	return buf, nil
}

// PinchtabExecutor implements autosolver.ActionExecutor by delegating
// to the Pinchtab bridge's human-like input system.
type PinchtabExecutor struct {
	ctx   context.Context
	tabID string
	b     bridge.BridgeAPI
}

// NewPinchtabExecutor creates an ActionExecutor backed by a Pinchtab bridge.
func NewPinchtabExecutor(ctx context.Context, tabID string, b bridge.BridgeAPI) *PinchtabExecutor {
	return &PinchtabExecutor{ctx: ctx, tabID: tabID, b: b}
}

func (e *PinchtabExecutor) Click(ctx context.Context, x, y float64) error {
	return bridge.Click(ctx, x, y)
}

func (e *PinchtabExecutor) Type(ctx context.Context, text string) error {
	actions := bridge.Type(text, false)
	return chromedp.Run(ctx, actions...)
}

func (e *PinchtabExecutor) WaitFor(ctx context.Context, selector string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return chromedp.Run(waitCtx, chromedp.WaitVisible(selector))
}

func (e *PinchtabExecutor) Evaluate(ctx context.Context, expr string, result interface{}) error {
	return chromedp.Run(ctx, chromedp.Evaluate(expr, result))
}

func (e *PinchtabExecutor) Navigate(ctx context.Context, url string) error {
	return chromedp.Run(ctx, chromedp.Navigate(url))
}

// NewFromBridge creates both a Page and ActionExecutor from a Bridge
// and tab ID. This is the primary factory for integration with Pinchtab.
func NewFromBridge(b bridge.BridgeAPI, tabID string) (autosolver.Page, autosolver.ActionExecutor, error) {
	tabCtx, resolvedID, err := b.TabContext(tabID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve tab %q: %w", tabID, err)
	}

	page := NewPinchtabPage(tabCtx, resolvedID, b)
	executor := NewPinchtabExecutor(tabCtx, resolvedID, b)
	return page, executor, nil
}
