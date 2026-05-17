package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/chromedp/chromedp"
)

// bannerDismissJS is a best-effort routine that clears cookie/consent/login
// overlays after a navigation completes. Runs entirely in the page context;
// returns a small JSON-shaped result for telemetry but the Go side ignores it.
//
// Strategy:
//  1. Phase 1 — click the most permissive labelled dismissal button
//     ("Accept all", "I agree", "OK", "Got it", "Close", "Dismiss", ...).
//     Permissive labels are tried first because rejection paths often gate
//     further interaction behind another modal.
//  2. Phase 2 — if no button matched, hard-remove DOM nodes whose id, class,
//     or aria-label contains "cookie"/"consent". Re-enables document scrolling
//     that banners commonly lock with overflow:hidden.
//
// The script is wrapped in try/catch and bounded to a short timeout from the
// caller so it can never wedge a navigation response.
const bannerDismissJS = `(() => {
  try {
    const labels = [
      "accept all", "accept", "i agree", "agree",
      "got it", "ok", "okay", "close", "dismiss", "no thanks",
      "continue", "allow all"
    ];
    const matchLabel = (s) => {
      const t = (s || "").trim().toLowerCase();
      if (t.length < 2 || t.length > 40) return false;
      return labels.includes(t) || labels.some(l => t === l || t.startsWith(l + " ") || t.endsWith(" " + l));
    };

    const isVisible = (el) => {
      if (!el || !el.getBoundingClientRect) return false;
      const r = el.getBoundingClientRect();
      if (r.width <= 0 || r.height <= 0) return false;
      const cs = getComputedStyle(el);
      if (cs.visibility === "hidden" || cs.display === "none" || cs.opacity === "0") return false;
      return true;
    };

    // Phase 1: try a labelled dismissal button.
    const clickables = document.querySelectorAll('button, [role=button], a[href]');
    for (const el of clickables) {
      const text = (el.innerText || el.textContent || el.getAttribute("aria-label") || "").trim();
      if (!matchLabel(text)) continue;
      if (!isVisible(el)) continue;
      try { el.click(); return JSON.stringify({action: "click", label: text}); } catch (_) {}
    }

    // Phase 2: hard-remove obvious overlay containers.
    const sel = '[id*="cookie" i], [class*="cookie" i], [id*="consent" i], [class*="consent" i],' +
                '[aria-label*="cookie" i], [aria-label*="consent" i]';
    const removed = [];
    document.querySelectorAll(sel).forEach((el) => {
      if (el === document.body || el === document.documentElement) return;
      const cs = getComputedStyle(el);
      // Only remove if positioned like an overlay or covers most of the viewport.
      const looksOverlay = cs.position === "fixed" || cs.position === "absolute";
      const r = el.getBoundingClientRect();
      const covers = r.width >= window.innerWidth * 0.5 && r.height >= window.innerHeight * 0.3;
      if (!looksOverlay && !covers) return;
      removed.push((el.tagName || "").toLowerCase() + (el.id ? "#" + el.id : "") + (el.className ? "." + String(el.className).slice(0, 30) : ""));
      try { el.remove(); } catch (_) {}
    });

    // Re-enable scroll if a banner locked it.
    document.documentElement.style.overflow = "";
    if (document.body) document.body.style.overflow = "";

    return JSON.stringify({action: removed.length ? "remove" : "none", removed: removed.slice(0, 5)});
  } catch (e) {
    return JSON.stringify({action: "error", message: String(e && e.message || e)});
  }
})();`

// bannerDismissTimeout caps how long the dismissal script can run end-to-end.
// Hard upper bound — if the page is unresponsive we silently skip.
const bannerDismissTimeout = 750 * time.Millisecond

// dismissBanners runs the dismissal script on the current page of the given
// chromedp context. Errors are swallowed: dismissal is purely best-effort.
//
// Pass enabled=false to no-op (lets call-sites wire the helper unconditionally
// and decide based on the request flag).
func (h *Handlers) dismissBanners(ctx context.Context, tabID string, enabled bool) {
	if !enabled || ctx == nil || tabID == "" {
		return
	}

	tCtx, cancel := context.WithTimeout(ctx, bannerDismissTimeout)
	defer cancel()

	var result string
	if err := chromedp.Run(tCtx, chromedp.Evaluate(bannerDismissJS, &result)); err != nil {
		slog.Debug("banner dismiss skipped", "tab_id", tabID, "error", err)
		return
	}
	if result != "" {
		slog.Debug("banner dismiss ran", "tab_id", tabID, "result", result)
	}
}
