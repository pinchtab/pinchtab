package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

const knownInterstitialTimeout = time.Second

type knownInterstitialResult struct {
	Action    string `json:"action"`
	CatalogID string `json:"catalogId,omitempty"`
	Label     string `json:"label,omitempty"`
}

// knownInterstitialDismissJS is intentionally a small allowlist, not another
// generic modal heuristic. A catalog entry must match the current host, text in
// a visible modal, and the exact dismissal label before one click is allowed.
const knownInterstitialDismissJS = `(async () => {
  const normalize = (value) => String(value || "").replace(/\s+/g, " ").trim().toLowerCase();
  const visible = (el) => {
    if (!el || !el.isConnected) return false;
    const rect = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return rect.width > 0 && rect.height > 0 && style.display !== "none" &&
      style.visibility !== "hidden" && style.opacity !== "0";
  };
  const catalog = [{
    id: "m365_purview_pay_as_you_go",
    hosts: ["purview.microsoft.com", "compliance.microsoft.com"],
    phrases: ["pay-as-you-go", "pay as you go"],
    dismissLabel: "not now"
  }];
  const hostname = normalize(location.hostname).replace(/\.$/, "");
  for (const entry of catalog) {
    if (!entry.hosts.some(host => hostname === host || hostname.endsWith("." + host))) continue;
    const modals = Array.from(document.querySelectorAll(
      'dialog[open], [role="dialog"][aria-modal="true"], [aria-modal="true"]'
    )).filter(visible);
    for (const modal of modals) {
      const modalText = normalize(modal.innerText || modal.textContent);
      if (!entry.phrases.some(phrase => modalText.includes(phrase))) continue;
      const control = Array.from(modal.querySelectorAll('button, [role="button"]')).find(el =>
        visible(el) && normalize(el.innerText || el.textContent || el.getAttribute("aria-label")) === entry.dismissLabel
      );
      if (!control) return {action: "blocked", catalogId: entry.id, label: entry.dismissLabel};
      control.click();
      const deadline = Date.now() + 700;
      while (Date.now() < deadline) {
        if (!visible(modal)) return {action: "dismissed", catalogId: entry.id, label: entry.dismissLabel};
        await new Promise(resolve => setTimeout(resolve, 25));
      }
      return {action: "blocked", catalogId: entry.id, label: entry.dismissLabel};
    }
  }
  return {action: "none"};
})()`

func (h *Handlers) dismissKnownInterstitials(ctx context.Context, tabID string) (knownInterstitialResult, error) {
	if ctx == nil || tabID == "" {
		return knownInterstitialResult{}, fmt.Errorf("known interstitial dismissal requires a live tab")
	}
	tCtx, cancel := context.WithTimeout(ctx, knownInterstitialTimeout)
	defer cancel()
	var result knownInterstitialResult
	if err := h.Bridge.Evaluate(tCtx, knownInterstitialDismissJS, &result, bridge.EvalOpts{AwaitPromise: true}); err != nil {
		return result, fmt.Errorf("inspect known interstitials: %w", err)
	}
	switch result.Action {
	case "none", "dismissed":
		return result, nil
	case "blocked":
		return result, fmt.Errorf("recognized interstitial %q could not be dismissed with %q", result.CatalogID, result.Label)
	default:
		return result, fmt.Errorf("known interstitial probe returned invalid action %q", result.Action)
	}
}
