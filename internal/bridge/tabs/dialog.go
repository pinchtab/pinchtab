package tabs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// DialogState represents a pending JavaScript dialog.
type DialogState struct {
	Type              string `json:"type"`
	Message           string `json:"message"`
	DefaultPrompt     string `json:"defaultPrompt,omitempty"`
	HasBrowserHandler bool   `json:"-"`
}

// DialogManager tracks pending JavaScript dialogs per tab.
type DialogManager struct {
	mu      sync.RWMutex
	pending map[string]*DialogState
}

func NewDialogManager() *DialogManager {
	return &DialogManager{
		pending: make(map[string]*DialogState),
	}
}

func (dm *DialogManager) SetPending(tabID string, state *DialogState) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.pending[tabID] = state
}

func (dm *DialogManager) GetPending(tabID string) *DialogState {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.pending[tabID]
}

func (dm *DialogManager) ClearPending(tabID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	delete(dm.pending, tabID)
}

func (dm *DialogManager) GetAndClear(tabID string) *DialogState {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	state := dm.pending[tabID]
	delete(dm.pending, tabID)
	return state
}

func HandleDialog(ctx context.Context, accept bool, promptText string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.HandleJavaScriptDialog(accept).WithPromptText(promptText).Do(ctx)
	}))
}

func ListenDialogEvents(ctx context.Context, tabID string, dm *DialogManager, autoAccept bool) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *page.EventJavascriptDialogOpening:
			state := &DialogState{
				Type:          string(e.Type),
				Message:       e.Message,
				DefaultPrompt: e.DefaultPrompt,
			}
			slog.Debug("dialog opened", "tabId", tabID, "type", e.Type)

			if autoAccept {
				state.HasBrowserHandler = true
				if err := HandleDialog(ctx, true, e.DefaultPrompt); err != nil {
					slog.Warn("auto-accept dialog failed", "tabId", tabID, "err", err)
					dm.SetPending(tabID, state)
				} else {
					slog.Debug("dialog auto-accepted", "tabId", tabID, "type", e.Type)
				}
			} else {
				dm.SetPending(tabID, state)
			}

		case *page.EventJavascriptDialogClosed:
			slog.Debug("dialog closed", "tabId", tabID, "result", e.Result)
			dm.ClearPending(tabID)
		}
	})
}

func EnableDialogEvents(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.Enable().Do(ctx)
	}))
}

type DialogResult struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Handled bool   `json:"handled"`
}

func HandlePendingDialog(ctx context.Context, tabID string, dm *DialogManager, accept bool, promptText string) (*DialogResult, error) {
	state := dm.GetAndClear(tabID)
	if state == nil {
		return nil, fmt.Errorf("no dialog open on tab %s", tabID)
	}

	if err := HandleDialog(ctx, accept, promptText); err != nil {
		dm.SetPending(tabID, state)
		return nil, fmt.Errorf("handle dialog: %w", err)
	}

	return &DialogResult{
		Type:    state.Type,
		Message: state.Message,
		Handled: true,
	}, nil
}
