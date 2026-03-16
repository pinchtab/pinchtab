package bridge

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
	Type              string `json:"type"`    // "alert", "confirm", "prompt", "beforeunload"
	Message           string `json:"message"` // dialog message text
	DefaultPrompt     string `json:"defaultPrompt,omitempty"`
	HasBrowserHandler bool   `json:"-"` // whether auto-accept handled it already
}

// DialogManager tracks pending JavaScript dialogs per tab.
type DialogManager struct {
	mu      sync.RWMutex
	pending map[string]*DialogState // tabID → pending dialog
}

// NewDialogManager creates a new DialogManager.
func NewDialogManager() *DialogManager {
	return &DialogManager{
		pending: make(map[string]*DialogState),
	}
}

// SetPending stores a pending dialog for a tab.
func (dm *DialogManager) SetPending(tabID string, state *DialogState) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.pending[tabID] = state
}

// GetPending returns the pending dialog for a tab, or nil.
func (dm *DialogManager) GetPending(tabID string) *DialogState {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.pending[tabID]
}

// ClearPending removes the pending dialog for a tab.
func (dm *DialogManager) ClearPending(tabID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	delete(dm.pending, tabID)
}

// GetAndClear atomically gets and clears the pending dialog.
func (dm *DialogManager) GetAndClear(tabID string) *DialogState {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	state := dm.pending[tabID]
	delete(dm.pending, tabID)
	return state
}

// HandleDialog accepts or dismisses the current dialog on a tab via CDP.
func HandleDialog(ctx context.Context, accept bool, promptText string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.HandleJavaScriptDialog(accept).WithPromptText(promptText).Do(ctx)
	}))
}

// ListenDialogEvents sets up a CDP event listener for JavaScript dialog events
// on the given context. When autoAccept is true, dialogs are automatically accepted.
func ListenDialogEvents(ctx context.Context, tabID string, dm *DialogManager, autoAccept bool) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *page.EventJavascriptDialogOpening:
			state := &DialogState{
				Type:          string(e.Type),
				Message:       e.Message,
				DefaultPrompt: e.DefaultPrompt,
			}
			slog.Debug("dialog opened", "tabId", tabID, "type", e.Type, "message", e.Message)

			if autoAccept {
				state.HasBrowserHandler = true
				// Auto-accept: handle immediately
				if err := HandleDialog(ctx, true, e.DefaultPrompt); err != nil {
					slog.Warn("auto-accept dialog failed", "tabId", tabID, "err", err)
					// Still store it so manual handling can retry
					dm.SetPending(tabID, state)
				} else {
					slog.Debug("dialog auto-accepted", "tabId", tabID, "type", e.Type)
					// Don't store — already handled
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

// EnableDialogEvents enables Page domain events needed for dialog tracking.
func EnableDialogEvents(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return page.Enable().Do(ctx)
	}))
}

// DialogResult is the response returned after handling a dialog.
type DialogResult struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Handled bool   `json:"handled"`
}

// HandlePendingDialog handles the pending dialog for a tab.
// Returns an error if no dialog is pending.
func HandlePendingDialog(ctx context.Context, tabID string, dm *DialogManager, accept bool, promptText string) (*DialogResult, error) {
	state := dm.GetAndClear(tabID)
	if state == nil {
		return nil, fmt.Errorf("no dialog open on tab %s", tabID)
	}

	if err := HandleDialog(ctx, accept, promptText); err != nil {
		// Put it back if handling failed
		dm.SetPending(tabID, state)
		return nil, fmt.Errorf("handle dialog: %w", err)
	}

	return &DialogResult{
		Type:    state.Type,
		Message: state.Message,
		Handled: true,
	}, nil
}
