package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/api/types"
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
	guardTabLease
)

func resolveOwner(r *http.Request, bodyOwner string) string {
	if o := strings.TrimSpace(r.Header.Get("X-Owner")); o != "" {
		return o
	}
	if o := strings.TrimSpace(r.URL.Query().Get("owner")); o != "" {
		return o
	}
	return strings.TrimSpace(bodyOwner)
}

func (h *Handlers) enforceTabLease(tabID, owner string) error {
	if tabID == "" {
		return nil
	}
	lock := h.Bridge.TabLockInfo(tabID)
	if lock == nil {
		return nil
	}
	if owner == "" {
		return fmt.Errorf("tab %s is locked by %s; owner required", tabID, lock.Owner)
	}
	if owner != lock.Owner {
		return fmt.Errorf("tab %s is locked by %s", tabID, lock.Owner)
	}
	return nil
}

func (h *Handlers) stepTabContext(r *http.Request, tabID, bodyOwner string) (context.Context, string, error) {
	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		return nil, "", fmt.Errorf("tab not found: %w", err)
	}
	if err := h.enforceTabLease(resolvedTabID, resolveOwner(r, bodyOwner)); err != nil {
		return nil, "", err
	}
	return ctx, resolvedTabID, nil
}

func (h *Handlers) applyTabGuards(w http.ResponseWriter, r *http.Request, ctx context.Context, tabID string, guards tabGuards) (string, bool) {
	return h.applyTabGuardsAs(w, r, ctx, tabID, "", guards)
}

func (h *Handlers) applyTabGuardsAs(w http.ResponseWriter, r *http.Request, ctx context.Context, tabID, bodyOwner string, guards tabGuards) (string, bool) {
	if guards&guardTabLease != 0 {
		if err := h.enforceTabLease(tabID, resolveOwner(r, bodyOwner)); err != nil {
			httpx.ErrorCode(w, http.StatusLocked, "tab_locked", err.Error(), false, nil)
			return "", false
		}
	}
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

	if provider, ok := bridgeAs[tabPolicyStateProvider](h.Bridge); ok {
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
	if setter, ok := bridgeAs[tabPolicyStateSetter](h.Bridge); ok {
		setter.SetTabPolicyState(tabID, state)
	}
	h.recordResolvedURL(r, currentURL)

	return h.applyTabPolicyState(w, state)
}

func (h *Handlers) applyTabPolicyState(w http.ResponseWriter, state bridge.TabPolicyState) (string, bool) {
	if state.Threat {
		w.Header().Set(types.HeaderIDPIWarning, state.Reason)
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
