package tabs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/sanitize"
)

const maxDialogTextBytes = 8 * 1024

var handleDialogAction = HandleDialog

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
	armed   map[string]*ArmedDialogHandler
}

// ArmedDialogHandler is a one-shot auto-handler for the next dialog on a tab.
type ArmedDialogHandler struct {
	Action string // "accept" or "dismiss"
	Text   string // optional prompt text
}

func NewDialogManager() *DialogManager {
	return &DialogManager{
		pending: make(map[string]*DialogState),
		armed:   make(map[string]*ArmedDialogHandler),
	}
}

// ArmAutoHandler sets a one-shot auto-handler for the next dialog on the
// given tab. The handler is consumed when the dialog fires, or remains armed
// if no dialog appears.
func (dm *DialogManager) ArmAutoHandler(tabID, action, text string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.armed[tabID] = &ArmedDialogHandler{Action: action, Text: text}
}

// TakeAutoHandler retrieves and removes the armed auto-handler for a tab.
func (dm *DialogManager) TakeAutoHandler(tabID string) *ArmedDialogHandler {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	h := dm.armed[tabID]
	delete(dm.armed, tabID)
	return h
}

// HasAutoHandler reports whether a one-shot auto-handler is armed for the tab.
func (dm *DialogManager) HasAutoHandler(tabID string) bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	_, ok := dm.armed[tabID]
	return ok
}

func (dm *DialogManager) SetPending(tabID string, state *DialogState) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.pending[tabID] = normalizeDialogState(state)
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
			state := normalizeDialogState(&DialogState{
				Type:          string(e.Type),
				Message:       e.Message,
				DefaultPrompt: e.DefaultPrompt,
			})
			slog.Debug("dialog opened", "tabId", tabID, "type", e.Type)

			// Check for a one-shot armed handler first (set via ArmAutoHandler
			// before a click that is expected to open a dialog).
			if armed := dm.TakeAutoHandler(tabID); armed != nil {
				accept := armed.Action == "accept"
				promptText := armed.Text
				if promptText == "" {
					promptText = e.DefaultPrompt
				}
				state.HasBrowserHandler = true
				// Dispatch in a goroutine because HandleDialog sends a CDP
				// command, and the enclosing ListenTarget callback runs on
				// the CDP event loop — doing CDP work here would deadlock.
				go func() {
					if err := handleDialogAction(ctx, accept, promptText); err != nil {
						slog.Warn("armed dialog handler failed", "tabId", tabID, "err", err)
						dm.SetPending(tabID, state)
					} else {
						slog.Debug("dialog auto-handled", "tabId", tabID, "type", e.Type, "action", armed.Action)
					}
				}()
				return
			}

			if autoAccept {
				state.HasBrowserHandler = true
				// Same reasoning as above — dispatch in a goroutine.
				go func() {
					if err := handleDialogAction(ctx, true, e.DefaultPrompt); err != nil {
						slog.Warn("auto-accept dialog failed", "tabId", tabID, "err", err)
						dm.SetPending(tabID, state)
					} else {
						slog.Debug("dialog auto-accepted", "tabId", tabID, "type", e.Type)
					}
				}()
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
		// Best-effort fallback when dialog-open events were missed.
		if err := handleDialogAction(ctx, accept, promptText); err != nil {
			if isNoDialogOpenError(err) || isDialogContextUnavailableError(err) {
				return nil, fmt.Errorf("no dialog open on tab %s", tabID)
			}
			return nil, fmt.Errorf("handle dialog: %w", err)
		}
		return &DialogResult{Type: "unknown", Message: "", Handled: true}, nil
	}

	if err := handleDialogAction(ctx, accept, promptText); err != nil {
		if isNoDialogOpenError(err) {
			// Dialog may already be handled/closed by browser auto-handler.
			return &DialogResult{Type: state.Type, Message: state.Message, Handled: true}, nil
		}
		dm.SetPending(tabID, state)
		return nil, fmt.Errorf("handle dialog: %w", err)
	}

	return &DialogResult{
		Type:    state.Type,
		Message: state.Message,
		Handled: true,
	}, nil
}

func isNoDialogOpenError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no dialog open") ||
		strings.Contains(msg, "no dialog is showing") ||
		strings.Contains(msg, "not showing a dialog")
}

func isDialogContextUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid context")
}

func normalizeDialogState(state *DialogState) *DialogState {
	if state == nil {
		return nil
	}

	copyState := *state
	copyState.Type = sanitize.TruncateUTF8Bytes(copyState.Type, 32)
	copyState.Message = sanitize.TruncateUTF8Bytes(copyState.Message, maxDialogTextBytes)
	copyState.DefaultPrompt = sanitize.TruncateUTF8Bytes(copyState.DefaultPrompt, maxDialogTextBytes)
	return &copyState
}
