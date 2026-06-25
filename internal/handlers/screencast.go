package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// HandleScreencast upgrades to WebSocket and streams screencast frames for a tab.
// Query params: tabId (required), quality (1-100, default 40), maxWidth (default 800), fps (1-30, default 5)
func (h *Handlers) HandleScreencast(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowScreencast {
		h.writeCapabilityDisabled(w, routes.CapScreencast)
		return
	}
	tabID := r.URL.Query().Get("tabId")
	if tabID == "" {
		targets, err := h.Bridge.ListTargets()
		if err == nil && len(targets) > 0 {
			tabID = targets[0].TargetID
		}
	}

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		httpx.Problem(w, http.StatusNotFound, "tab_not_found", "tab not found", false, nil)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	quality := queryParamInt(r, "quality", 30)
	maxWidth := queryParamInt(r, "maxWidth", 800)
	// everyNthFrame=1 sends every compositor frame; the requested fps still throttles
	// the rate. A higher skip (the old default of 4) caps the live tab well below the
	// requested fps and makes interactive control feel unresponsive (issue: dashboard
	// Live tab too laggy to type into).
	everyNth := queryParamInt(r, "everyNthFrame", 1)
	fps := queryParamInt(r, "fps", 1)
	if fps > 30 {
		fps = 30
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		slog.Error("ws upgrade failed", "err", err)
		return
	}
	defer func() { _ = conn.Close() }()

	if ctx == nil {
		return
	}

	var once sync.Once
	done := make(chan struct{})

	slog.Info("screencast started", "tab", resolvedTabID, "quality", quality, "maxWidth", maxWidth)

	go func() {
		for {
			_, _, err := wsutil.ReadClientData(conn)
			if err != nil {
				once.Do(func() { close(done) })
				return
			}
		}
	}()

	stream, err := h.Bridge.StartScreencast(ctx, bridge.ScreencastOpts{
		Quality:       quality,
		MaxWidth:      maxWidth,
		MaxHeight:     maxWidth * 3 / 4,
		EveryNthFrame: everyNth,
		FPS:           fps,
	})
	if err != nil {
		slog.Error("start screencast failed", "err", err, "tab", resolvedTabID)
		return
	}
	defer stream.Close()

	for {
		select {
		case frame, ok := <-stream.Frames:
			if !ok {
				return
			}
			if err := wsutil.WriteServerBinary(conn, frame); err != nil {
				return
			}
		case <-done:
			return
		case <-time.After(10 * time.Second):
			if err := wsutil.WriteServerMessage(conn, ws.OpPing, nil); err != nil {
				return
			}
		}
	}
}

// HandleScreencastAll returns info for building a multi-tab screencast view.
func (h *Handlers) HandleScreencastAll(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowScreencast {
		h.writeCapabilityDisabled(w, routes.CapScreencast)
		return
	}
	type tabInfo struct {
		ID    string `json:"id"`
		URL   string `json:"url,omitempty"`
		Title string `json:"title,omitempty"`
	}

	targets, err := h.Bridge.ListTargets()
	if err != nil {
		httpx.JSON(w, 200, []tabInfo{})
		return
	}

	tabs := make([]tabInfo, 0)
	for _, t := range targets {
		tabs = append(tabs, tabInfo{
			ID:    t.TargetID,
			URL:   t.URL,
			Title: t.Title,
		})
	}

	httpx.JSON(w, 200, tabs)
}

func queryParamInt(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n <= 0 {
		return def
	}
	return n
}
