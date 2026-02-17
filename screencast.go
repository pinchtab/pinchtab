package main

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
)

// ---------------------------------------------------------------------------
// Screencast — streams live JPEG frames from Chrome tabs over WebSocket
// ---------------------------------------------------------------------------

// handleScreencast upgrades to WebSocket and streams screencast frames for a tab.
// Query params: tabId (required), quality (1-100, default 40), maxWidth (default 800), fps (1-30, default 5)
func (b *Bridge) handleScreencast(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	if tabID == "" {
		// Use first available tab
		b.mu.RLock()
		for id := range b.tabs {
			tabID = id
			break
		}
		b.mu.RUnlock()
	}

	b.mu.RLock()
	tab, ok := b.tabs[tabID]
	b.mu.RUnlock()
	if !ok {
		http.Error(w, "tab not found", 404)
		return
	}

	quality := queryParamInt(r, "quality", 40)
	maxWidth := queryParamInt(r, "maxWidth", 800)
	everyNth := queryParamInt(r, "everyNthFrame", 2)

	// Upgrade to WebSocket
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		slog.Error("ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	ctx := tab.ctx
	if ctx == nil {
		return
	}

	// Channel for frames
	frameCh := make(chan []byte, 3)
	var once sync.Once
	done := make(chan struct{})

	// Listen for screencast frames
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *page.EventScreencastFrame:
			// Decode base64 frame data
			data, err := base64.StdEncoding.DecodeString(e.Data)
			if err != nil {
				return
			}

			// Ack the frame so Chrome sends the next one
			go func() {
				_ = chromedp.Run(ctx,
					chromedp.ActionFunc(func(c context.Context) error {
						return page.ScreencastFrameAck(e.SessionID).Do(c)
					}),
				)
			}()

			// Non-blocking send
			select {
			case frameCh <- data:
			default:
				// Drop frame if consumer is slow
			}
		}
	})

	// Start screencast
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

	// Read pump — detect client disconnect
	go func() {
		for {
			_, _, err := wsutil.ReadClientData(conn)
			if err != nil {
				once.Do(func() { close(done) })
				return
			}
		}
	}()

	// Write pump — send frames to WebSocket client
	for {
		select {
		case frame := <-frameCh:
			if err := wsutil.WriteServerBinary(conn, frame); err != nil {
				return
			}
		case <-done:
			return
		case <-time.After(10 * time.Second):
			// Keepalive ping if no frames
			if err := wsutil.WriteServerMessage(conn, ws.OpPing, nil); err != nil {
				return
			}
		}
	}
}

// handleScreencastAll returns info for building a multi-tab screencast view.
func (b *Bridge) handleScreencastAll(w http.ResponseWriter, r *http.Request) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	type tabInfo struct {
		ID  string `json:"id"`
		URL string `json:"url,omitempty"`
	}

	tabs := make([]tabInfo, 0, len(b.tabs))
	for id, tab := range b.tabs {
		url := ""
		if tab.ctx != nil {
			var loc string
			_ = chromedp.Run(tab.ctx, chromedp.Location(&loc))
			url = loc
		}
		tabs = append(tabs, tabInfo{ID: id, URL: url})
	}

	jsonResp(w, 200, tabs)
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
