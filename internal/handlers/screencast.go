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
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const screencastRepaintStartJS = `(function() {
  const key = "__pinchtabScreencastRepaint";
  const state = globalThis[key] || (globalThis[key] = { refs: 0 });
  state.refs += 1;
  if (state.refs > 1) {
    return state.refs;
  }

  const el = document.createElement("div");
  el.id = "__pinchtab_screencast_repaint";
  el.setAttribute("aria-hidden", "true");
  el.style.cssText = "position:fixed;left:0;top:0;width:1px;height:1px;pointer-events:none;opacity:0.999;background:rgba(0,0,0,0.001);transform:translateZ(0);will-change:opacity;z-index:2147483647;";
  (document.body || document.documentElement).appendChild(el);

  state.element = el;
  state.animation = el.animate(
    [{ opacity: 0.999 }, { opacity: 1 }],
    { duration: 1000, iterations: Infinity, direction: "alternate", easing: "linear" }
  );
  return state.refs;
})()`

const screencastRepaintStopJS = `(function() {
  const key = "__pinchtabScreencastRepaint";
  const state = globalThis[key];
  if (!state) {
    return 0;
  }

  state.refs = Math.max(0, (state.refs || 1) - 1);
  if (state.refs > 0) {
    return state.refs;
  }

  try {
    if (state.animation) {
      state.animation.cancel();
    }
  } catch (_) {}

  try {
    if (state.element) {
      state.element.remove();
    }
  } catch (_) {}

  delete globalThis[key];
  return 0;
})()`

// HandleScreencast upgrades to WebSocket and streams screencast frames for a tab.
// Query params: tabId (required), quality (1-100, default 40), maxWidth (default 800), fps (1-30, default 5)
func (h *Handlers) HandleScreencast(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowScreencast {
		httpx.ErrorCode(w, 403, "screencast_disabled", httpx.DisabledEndpointMessage("screencast", "security.allowScreencast"), false, map[string]any{
			"setting": "security.allowScreencast",
		})
		return
	}
	tabID := r.URL.Query().Get("tabId")
	if tabID == "" {
		targets, err := h.Bridge.ListTargets()
		if err == nil && len(targets) > 0 {
			tabID = string(targets[0].TargetID)
		}
	}

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		http.Error(w, "tab not found", 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
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

	stopRepaintLoop := func() {}
	if h.Config != nil && h.Config.Headless {
		stopRepaintLoop = startScreencastRepaintLoop(ctx)
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
		slog.Error("start screencast failed", "err", err, "tab", resolvedTabID)
		return
	}

	defer func() {
		once.Do(func() { close(done) })
		stopRepaintLoop()
		_ = chromedp.Run(ctx,
			chromedp.ActionFunc(func(c context.Context) error {
				return page.StopScreencast().Do(c)
			}),
		)
	}()

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

func startScreencastRepaintLoop(ctx context.Context) func() {
	if err := chromedp.Run(ctx, chromedp.Evaluate(screencastRepaintStartJS, nil)); err != nil {
		slog.Warn("enable screencast repaint loop failed", "err", err)
		return func() {}
	}

	return func() {
		if err := chromedp.Run(ctx, chromedp.Evaluate(screencastRepaintStopJS, nil)); err != nil {
			slog.Warn("disable screencast repaint loop failed", "err", err)
		}
	}
}

// HandleScreencastAll returns info for building a multi-tab screencast view.
func (h *Handlers) HandleScreencastAll(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowScreencast {
		httpx.ErrorCode(w, 403, "screencast_disabled", httpx.DisabledEndpointMessage("screencast", "security.allowScreencast"), false, map[string]any{
			"setting": "security.allowScreencast",
		})
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
			ID:    string(t.TargetID),
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
