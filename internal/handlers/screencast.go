package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/pinchtab/pinchtab/internal/web"
)

// HandleScreencast upgrades to WebSocket and streams screencast frames for a tab.
// Query params: tabId (required), quality (1-100, default 40), maxWidth (default 800), fps (1-30, default 5)
func (h *Handlers) HandleScreencast(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	if tabID == "" {
		targets, err := h.Bridge.ListTargets()
		if err == nil && len(targets) > 0 {
			tabID = string(targets[0].TargetID)
		}
	}

	ctx, _, err := h.Bridge.TabContext(tabID)
	if err != nil {
		http.Error(w, "tab not found", 404)
		return
	}

	quality := queryParamInt(r, "quality", 30)
	maxWidth := queryParamInt(r, "maxWidth", 800)
	everyNth := queryParamInt(r, "everyNthFrame", 4)
	fps := queryParamInt(r, "fps", 1)
	if fps > 30 {
		fps = 30
	}
	minFrameInterval := time.Second / time.Duration(fps)

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		slog.Error("ws upgrade failed", "err", err)
		return
	}
	defer func() { _ = conn.Close() }()

	if ctx == nil {
		return
	}

	frameCh := make(chan []byte, 3)
	var once sync.Once
	done := make(chan struct{})

	// Listen for screencast frames with rate limiting
	var lastFrame time.Time
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *page.EventScreencastFrame:
			go func() {
				_ = chromedp.Run(ctx,
					chromedp.ActionFunc(func(c context.Context) error {
						return page.ScreencastFrameAck(e.SessionID).Do(c)
					}),
				)
			}()

			now := time.Now()
			if now.Sub(lastFrame) < minFrameInterval {
				return
			}
			lastFrame = now

			data, err := base64.StdEncoding.DecodeString(e.Data)
			if err != nil {
				return
			}

			select {
			case frameCh <- data:
			default:
			}
		}
	})

	err = chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			return page.StartScreencast().
				WithFormat(page.ScreencastFormatJpeg).
				WithQuality(int64(quality)).
				WithMaxWidth(int64(maxWidth)).
				WithMaxHeight(int64(maxWidth * 3 / 4)).
				WithEveryNthFrame(int64(everyNth)).
				Do(c)
		}),
	)
	if err != nil {
		slog.Error("start screencast failed", "err", err, "tab", tabID)
		return
	}

	defer func() {
		once.Do(func() { close(done) })
		_ = chromedp.Run(ctx,
			chromedp.ActionFunc(func(c context.Context) error {
				return page.StopScreencast().Do(c)
			}),
		)
	}()

	slog.Info("screencast started", "tab", tabID, "quality", quality, "maxWidth", maxWidth)

	go func() {
		for {
			_, _, err := wsutil.ReadClientData(conn)
			if err != nil {
				once.Do(func() { close(done) })
				return
			}
		}
	}()

	for {
		select {
		case frame := <-frameCh:
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
	type tabInfo struct {
		ID    string `json:"id"`
		URL   string `json:"url,omitempty"`
		Title string `json:"title,omitempty"`
	}

	targets, err := h.Bridge.ListTargets()
	if err != nil {
		web.JSON(w, 200, []tabInfo{})
		return
	}

	tabs := make([]tabInfo, 0)
	for _, t := range targets {
		tabs = append(tabs, tabInfo{
			ID:    string(t.TargetID),
			URL:   t.URL,
			Title: t.Title,
		})
	}

	web.JSON(w, 200, tabs)
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
