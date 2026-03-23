package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type HealthTabsHandler struct {
	Bridge *bridge.Bridge
}

func (h *HealthTabsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targets, err := h.Bridge.ListTargets()
	if err != nil {
		slog.Error("list targets", "err", err)
		httpx.JSON(w, 500, map[string]any{"error": "failed to list targets"})
		return
	}

	currentTabID := ""
	if _, resolvedID, err := h.Bridge.TabContext(""); err == nil {
		currentTabID = resolvedID
	}

	tabs := make([]map[string]any, 0, len(targets))
	appendTab := func(t *target.Info) {
		// Skip the initial about:blank tab that Chrome creates on launch
		// or other transient internal tabs.
		if bridge.IsTransientURL(t.URL) {
			return
		}
		tabID := string(t.TargetID)
		entry := map[string]any{
			"id":    tabID,
			"url":   t.URL,
			"title": t.Title,
			"type":  t.Type,
		}
		if lock := h.Bridge.TabLockInfo(tabID); lock != nil {
			entry["owner"] = lock.Owner
			entry["lockedUntil"] = lock.ExpiresAt.Format(time.RFC3339)
		}
		tabs = append(tabs, entry)
	}

	// First pass: add the current focused tab
	for _, t := range targets {
		if string(t.TargetID) == currentTabID {
			appendTab(t)
		}
	}
	// Second pass: add all other tabs
	for _, t := range targets {
		if string(t.TargetID) == currentTabID {
			continue
		}
		appendTab(t)
	}

	httpx.JSON(w, 200, map[string]any{"tabs": tabs})
}
