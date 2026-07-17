package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const tabActionNew = "new"

type browserContextTabCreator interface {
	CreateTabInBrowserContext(url, browserContextID string) (string, context.Context, context.CancelFunc, error)
}

func (h *Handlers) HandleTab(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action           string `json:"action"`
		TabID            string `json:"tabId"`
		URL              string `json:"url"`
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	switch req.Action {
	case tabActionNew:
		// A URL-bearing "new" converges onto the shared /navigate pipeline so it
		// gets the same browser routing, static-first phase, waitFor/banner/auto-
		// solve post-steps, and {tabId,url,title,route} response. A blank/about:blank
		// "new" stays on the lightweight create-only path.
		if req.URL != "" && req.URL != "about:blank" {
			h.navigateToURL(w, r, navigateRequest{URL: req.URL, NewTab: true})
			return
		}
		h.createBlankTab(w, r, strings.TrimSpace(req.BrowserContextID))

	case "focus":
		if req.TabID == "" {
			httpx.Error(w, 400, fmt.Errorf("tabId required"))
			return
		}
		if !h.ensureBrowserOrRespond(w, h.Config) {
			return
		}
		if err := h.Bridge.FocusTab(req.TabID); err != nil {
			WriteTabContextError(w, err, 404)
			return
		}

		h.setCurrentTabForRequest(r, req.TabID)
		h.recordActivity(r, activity.Update{Action: "tab.focus", TabID: req.TabID})

		httpx.JSON(w, 200, map[string]any{"focused": true, "tabId": req.TabID})

	default:
		httpx.Error(w, 400, fmt.Errorf("action must be 'new' or 'focus'; use /close to close a tab"))
	}
}

// createBlankTab creates a new blank tab without navigating — the no-URL /
// about:blank form of POST /tab {"action":"new"}. The URL form converges onto the
// shared navigate pipeline (navigateToURL); this path has no URL to validate,
// route, or navigate, so it reports {tabId,url,title} and a tab.new activity.
func (h *Handlers) createBlankTab(w http.ResponseWriter, r *http.Request, browserContextID string) {
	if !h.ensureBrowserOrRespond(w, h.Config) {
		return
	}

	var (
		newTabID string
		ctx      context.Context
		err      error
	)
	if browserContextID == "" {
		newTabID, ctx, _, err = h.Bridge.CreateTab("")
	} else {
		contextExists := false
		targets, listErr := h.Bridge.ListTargets()
		if listErr != nil {
			httpx.Error(w, 503, fmt.Errorf("list browser contexts: %w", listErr))
			return
		}
		for _, candidate := range targets {
			if candidate.BrowserContextID == browserContextID {
				contextExists = true
				break
			}
		}
		if !contextExists {
			httpx.Error(w, 400, fmt.Errorf("browserContextId is not owned by an attached page"))
			return
		}
		creator, ok := h.Bridge.(browserContextTabCreator)
		if !ok {
			httpx.Error(w, 501, fmt.Errorf("browser-context tab creation is unavailable"))
			return
		}
		newTabID, ctx, _, err = creator.CreateTabInBrowserContext("", browserContextID)
	}
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	curURL, _ := h.Bridge.CurrentURL(ctx)
	title, _ := h.Bridge.CurrentTitle(ctx)

	h.setCurrentTabForRequest(r, newTabID)
	h.recordActivity(r, activity.Update{Action: "tab.new", TabID: newTabID, URL: curURL})
	markCreatedTab(w, newTabID)
	response := map[string]any{"tabId": newTabID, "url": curURL, "title": title}
	if browserContextID != "" {
		response["browserContextId"] = browserContextID
	}
	httpx.JSON(w, 200, response)
}

// HandleTabClose closes the tab identified by the path. It is the tab-scoped
// equivalent of POST /close and exists so orchestrator dashboard commands can
// use the common /tabs/{id}/... proxy path.
func (h *Handlers) HandleTabClose(w http.ResponseWriter, r *http.Request) {
	tabID := strings.TrimSpace(r.PathValue("id"))
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	if r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0 {
		var req struct {
			TabID string `json:"tabId"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
		if req.TabID != "" && req.TabID != tabID {
			httpx.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	h.closeTab(w, r, tabID)
}

// HandleClose closes the tab identified by the JSON body, or the current/default
// tab when tabId is omitted. It is the shorthand form of POST /tabs/{id}/close.
func (h *Handlers) HandleClose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID string `json:"tabId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	h.closeTab(w, r, strings.TrimSpace(req.TabID))
}

func (h *Handlers) closeTab(w http.ResponseWriter, r *http.Request, tabID string) {
	if !h.ensureBrowserOrRespond(w, h.Config) {
		return
	}
	if tabID == "" {
		_, resolvedTabID, err := h.tabContext(r, "")
		if err != nil {
			WriteTabContextError(w, err, 404)
			return
		}
		tabID = resolvedTabID
	}

	if err := h.Bridge.CloseTab(tabID); err != nil {
		httpx.Error(w, 500, err)
		return
	}

	h.clearCurrentTabReferences(tabID)
	h.recordActivity(r, activity.Update{Action: "tab.close", TabID: tabID})
	w.Header().Set(activity.HeaderPTTabID, tabID)

	httpx.JSON(w, 200, map[string]any{"closed": true, "tabId": tabID})
}
