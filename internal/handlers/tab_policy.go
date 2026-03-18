package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"
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

func (h *Handlers) currentTabDomainPolicyEnabled() bool {
	return h != nil &&
		h.Config != nil &&
		h.Config.IDPI.Enabled &&
		len(h.Config.IDPI.AllowedDomains) > 0
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

	var currentURL string
	if err := chromedp.Run(lookupCtx, chromedp.Location(&currentURL)); err != nil {
		httpx.Error(w, 500, fmt.Errorf("resolve current tab url: %w", err))
		return "", false
	}

	state := bridge.EvaluateTabPolicy(currentURL, h.Config.IDPI)
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
		httpx.ErrorCode(w, http.StatusForbidden, "idpi_domain_blocked",
			fmt.Sprintf("current tab blocked by IDPI: %s", state.Reason), false, map[string]any{
				"url": state.CurrentURL,
			})
		return state.CurrentURL, false
	}
	return state.CurrentURL, true
}
