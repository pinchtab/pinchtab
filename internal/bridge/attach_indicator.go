package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

const attachIndicatorCleanupTimeout = 500 * time.Millisecond

type attachIndicatorState struct {
	ctx      context.Context
	scriptID page.ScriptIdentifier
}

func attachIndicatorScript(port string) (string, error) {
	value, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil || value < 1 || value > 65535 {
		return "", fmt.Errorf("invalid bridge port %q", port)
	}
	portJSON, _ := json.Marshal(value)
	return fmt.Sprintf(`(() => {
  const key = "__pinchtabAttachIndicators";
  const port = %s;
  const prefixPattern = /^\[PinchTab :\d+(?:,:\d+)*\]\s*/;
  let state = globalThis[key];
  if (!state || !(state.ports instanceof Set)) {
    state = {ports: new Set(), observer: null, start: null, disposed: false};
    globalThis[key] = state;
  }
  state.disposed = false;
  state.ports.add(port);
  const apply = () => {
    const clean = document.title.replace(prefixPattern, "");
    const ports = Array.from(state.ports).sort((a, b) => a - b);
    const prefix = ports.length ? "[PinchTab " + ports.map(p => ":" + p).join(",") + "] " : "";
    if (document.title !== prefix + clean) document.title = prefix + clean;
  };
  if (!state.observer) state.observer = new MutationObserver(apply);
  if (!state.start) state.start = () => {
    if (state.disposed || !state.ports.size) return;
    apply();
    if (document.head) state.observer.observe(document.head, {subtree:true, childList:true, characterData:true});
  };
  if (document.head) state.start(); else document.addEventListener("DOMContentLoaded", state.start, {once:true});
})()`, portJSON), nil
}

func clearAttachIndicatorScript(port string) (string, error) {
	value, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil || value < 1 || value > 65535 {
		return "", fmt.Errorf("invalid bridge port %q", port)
	}
	portJSON, _ := json.Marshal(value)
	return fmt.Sprintf(`(() => {
  const key = "__pinchtabAttachIndicators";
  const port = %s;
  const prefixPattern = /^\[PinchTab :\d+(?:,:\d+)*\]\s*/;
  const state = globalThis[key];
  if (!state || !(state.ports instanceof Set)) {
    document.title = document.title.replace(prefixPattern, "");
    return;
  }
  state.ports.delete(port);
  const clean = document.title.replace(prefixPattern, "");
  const ports = Array.from(state.ports).sort((a, b) => a - b);
  if (!ports.length) {
    state.disposed = true;
    if (state.observer) state.observer.disconnect();
    if (state.start) document.removeEventListener("DOMContentLoaded", state.start);
    delete globalThis[key];
    document.title = clean;
    return;
  }
  document.title = "[PinchTab " + ports.map(p => ":" + p).join(",") + "] " + clean;
})()`, portJSON), nil
}

func (b *Bridge) installAttachIndicator(ctx context.Context, tabID string) error {
	if b == nil || b.Config == nil {
		return fmt.Errorf("bridge configuration is required")
	}
	script, err := attachIndicatorScript(b.Config.Port)
	if err != nil {
		return err
	}
	b.attachIndicatorMu.Lock()
	defer b.attachIndicatorMu.Unlock()
	if b.attachIndicators != nil {
		if _, exists := b.attachIndicators[tabID]; exists {
			return nil
		}
	}
	var scriptID page.ScriptIdentifier
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var addErr error
		scriptID, addErr = page.AddScriptToEvaluateOnNewDocument(script).Do(ctx)
		if addErr != nil {
			return addErr
		}
		if b.attachIndicators == nil {
			b.attachIndicators = make(map[string]attachIndicatorState)
		}
		b.attachIndicators[tabID] = attachIndicatorState{ctx: ctx, scriptID: scriptID}
		_, exception, evalErr := cdpruntime.Evaluate(script).Do(ctx)
		if evalErr != nil {
			_ = page.RemoveScriptToEvaluateOnNewDocument(scriptID).Do(ctx)
			delete(b.attachIndicators, tabID)
			return evalErr
		}
		if exception != nil {
			_ = page.RemoveScriptToEvaluateOnNewDocument(scriptID).Do(ctx)
			delete(b.attachIndicators, tabID)
			return exception
		}
		return nil
	})); err != nil {
		return fmt.Errorf("install attach indicator: %w", err)
	}
	return nil
}

func (b *Bridge) clearAttachIndicators() {
	if b == nil || b.Config == nil {
		return
	}
	cleanupScript, err := clearAttachIndicatorScript(b.Config.Port)
	if err != nil {
		return
	}
	b.attachIndicatorMu.Lock()
	states := b.attachIndicators
	b.attachIndicators = nil
	b.attachIndicatorMu.Unlock()
	for _, state := range states {
		tabCtx := state.ctx
		if tabCtx == nil {
			continue
		}
		if tabCtx.Err() != nil {
			continue
		}
		cleanupCtx, cancel := context.WithTimeout(tabCtx, attachIndicatorCleanupTimeout)
		if err := chromedp.Run(cleanupCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			removeErr := page.RemoveScriptToEvaluateOnNewDocument(state.scriptID).Do(ctx)
			_, _, evalErr := cdpruntime.Evaluate(cleanupScript).Do(ctx)
			if evalErr != nil {
				return evalErr
			}
			return removeErr
		})); err != nil {
			slog.Warn("attach indicator cleanup failed", "err", err)
		}
		cancel()
	}
}
