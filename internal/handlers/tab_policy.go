package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const cachedTabPolicyTTL = 1 * time.Second

type tabPolicyStateProvider interface {
	GetTabPolicyState(tabID string) (bridge.TabPolicyState, bool)
}

type tabPolicyStateSetter interface {
	SetTabPolicyState(tabID string, state bridge.TabPolicyState)
}

type tabGuards uint8

const (
	guardNone tabGuards = 1 << iota
	guardDialogBlocked
	guardDomainPolicy
	guardHandoffPause
)

func (h *Handlers) applyTabGuards(w http.ResponseWriter, r *http.Request, ctx context.Context, tabID string, guards tabGuards) (string, bool) {
	if guards&guardDialogBlocked != 0 && h.refuseIfDialogBlocked(w, tabID) {
		return "", false
	}
	var currentURL string
	if guards&guardDomainPolicy != 0 {
		url, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, tabID)
		if !ok {
			return "", false
		}
		currentURL = url
	}
	if guards&guardHandoffPause != 0 && !h.enforceTabNotPausedForHandoffOrRespond(w, tabID) {
		return "", false
	}
	return currentURL, true
}

func (h *Handlers) guardTabContext(w http.ResponseWriter, r *http.Request, ctx context.Context, tabID string, err error, guards tabGuards) (context.Context, string, bool) {
	if err != nil {
		WriteTabContextError(w, err, http.StatusNotFound)
		return nil, "", false
	}
	if _, ok := h.applyTabGuards(w, r, ctx, tabID, guards); !ok {
		return nil, "", false
	}
	return ctx, tabID, true
}

func (h *Handlers) guardedTabContext(w http.ResponseWriter, r *http.Request, tabID string, guards tabGuards) (context.Context, string, bool) {
	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	return h.guardTabContext(w, r, ctx, resolvedTabID, err, guards)
}

func (h *Handlers) guardedTabContextWithHeader(w http.ResponseWriter, r *http.Request, tabID string, guards tabGuards) (context.Context, string, bool) {
	ctx, resolvedTabID, err := h.tabContextWithHeader(w, r, tabID)
	return h.guardTabContext(w, r, ctx, resolvedTabID, err, guards)
}

func (h *Handlers) currentTabDomainPolicyEnabled() bool {
	return h != nil &&
		h.Config != nil &&
		h.Config.IDPI.Enabled &&
		len(h.Config.AllowedDomains) > 0
}

func (h *Handlers) enforceCurrentTabDomainPolicy(w http.ResponseWriter, r *http.Request, ctx context.Context, tabID string) (string, bool) {
	if !h.currentTabDomainPolicyEnabled() {
		return "", true
	}

	if provider, ok := h.Bridge.(tabPolicyStateProvider); ok {
		if state, ok := provider.GetTabPolicyState(tabID); ok && !state.UpdatedAt.IsZero() {
			if state.CurrentURL != "" {
				h.recordResolvedURL(r, state.CurrentURL)
			}
			if time.Since(state.UpdatedAt) <= cachedTabPolicyTTL {
				return h.applyTabPolicyState(w, state)
			}
		}
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	currentURL, err := h.Bridge.CurrentURL(lookupCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			if h.refuseIfDialogBlocked(w, tabID) {
				return "", false
			}
			httpx.ErrorCode(
				w,
				http.StatusServiceUnavailable,
				"tab_unresponsive",
				fmt.Sprintf("tab %s is busy, suspended, or unresponsive; activate it or open a fresh tab, then retry the same explicit tab", tabID),
				true,
				map[string]any{
					"tabId":               tabID,
					"classification":      "suspended_or_busy",
					"requiresStateChange": true,
				},
			)
			return "", false
		}
		httpx.Error(w, 500, fmt.Errorf("resolve current tab url: %w", err))
		return "", false
	}

	state := bridge.EvaluateTabPolicy(currentURL, h.Config.IDPI, h.Config.AllowedDomains)
	if setter, ok := h.Bridge.(tabPolicyStateSetter); ok {
		setter.SetTabPolicyState(tabID, state)
	}
	h.recordResolvedURL(r, currentURL)

	return h.applyTabPolicyState(w, state)
}

func (h *Handlers) applyTabPolicyState(w http.ResponseWriter, state bridge.TabPolicyState) (string, bool) {
	if state.Threat {
		w.Header().Set("X-IDPI-Warning", state.Reason)
	}
	if state.Blocked {
		writeIDPIDomainBlocked(w,
			fmt.Sprintf("current tab blocked by IDPI: %s", state.Reason),
			idpiDriftedTabDetails(state.CurrentURL))
		return state.CurrentURL, false
	}
	return state.CurrentURL, true
}

func (h *Handlers) enforceURLDomainPolicy(w http.ResponseWriter, url string) bool {
	if !h.currentTabDomainPolicyEnabled() {
		return true
	}
	if url == "" {
		return true
	}

	state := bridge.EvaluateTabPolicy(url, h.Config.IDPI, h.Config.AllowedDomains)
	if state.Blocked {
		writeIDPIDomainBlocked(w,
			fmt.Sprintf("url blocked by IDPI: %s", state.Reason),
			idpiRefusedURLDetails(url))
		return false
	}
	return true
}
