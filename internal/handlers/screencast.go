package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const screencastRepaintWorldName = "__pinchtab_screencast"

var getScreencastFrameTree = func(ctx context.Context) (*page.FrameTree, error) {
	return page.GetFrameTree().Do(ctx)
}

var createScreencastIsolatedWorld = func(ctx context.Context, params *page.CreateIsolatedWorldParams) (runtime.ExecutionContextID, error) {
	return params.Do(ctx)
}

var evaluateScreencastInWorld = func(ctx context.Context, params *runtime.EvaluateParams) (*runtime.RemoteObject, *runtime.ExceptionDetails, error) {
	return params.Do(ctx)
}

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

	if h.Config != nil && h.Config.Headless {
		h.streamHeadlessScreencast(ctx, conn, quality, minFrameInterval, done)
		return
	}

	stopRepaintLoop := func() {}
	frameCh := make(chan []byte, 3)
	ackCh := make(chan int64, 128)

	go func() {
		for {
			select {
			case sessionID := <-ackCh:
				if err := chromedp.Run(ctx,
					chromedp.ActionFunc(func(c context.Context) error {
						return page.ScreencastFrameAck(sessionID).Do(c)
					}),
				); err != nil && !errors.Is(err, context.Canceled) {
					slog.Debug("screencast ack failed", "err", err, "tab", resolvedTabID)
				}
			case <-done:
				return
			}
		}
	}()

	// Listen for screencast frames with rate limiting
	var lastFrame time.Time
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *page.EventScreencastFrame:
			select {
			case ackCh <- e.SessionID:
			case <-done:
				return
			default:
				go func(sessionID int64) {
					if err := chromedp.Run(ctx,
						chromedp.ActionFunc(func(c context.Context) error {
							return page.ScreencastFrameAck(sessionID).Do(c)
						}),
					); err != nil && !errors.Is(err, context.Canceled) {
						slog.Debug("screencast ack fallback failed", "err", err, "tab", resolvedTabID)
					}
				}(e.SessionID)
			}

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

	stopRepaintLoop = startScreencastRepaintLoop(ctx)

	defer func() {
		once.Do(func() { close(done) })
		stopRepaintLoop()
		_ = chromedp.Run(ctx,
			chromedp.ActionFunc(func(c context.Context) error {
				return page.StopScreencast().Do(c)
			}),
		)
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

func (h *Handlers) streamHeadlessScreencast(ctx context.Context, conn net.Conn, quality int, frameInterval time.Duration, done <-chan struct{}) {
	if frameInterval <= 0 {
		frameInterval = time.Second
	}

	sendFrame := func() error {
		frame, err := captureScreencastJPEG(ctx, quality)
		if err != nil {
			return err
		}
		return wsutil.WriteServerBinary(conn, frame)
	}

	if err := sendFrame(); err != nil {
		return
	}

	frameTicker := time.NewTicker(frameInterval)
	defer frameTicker.Stop()

	pingTicker := time.NewTicker(10 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-frameTicker.C:
			if err := sendFrame(); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := wsutil.WriteServerMessage(conn, ws.OpPing, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

func captureScreencastJPEG(ctx context.Context, quality int) ([]byte, error) {
	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			var err error
			buf, err = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(int64(quality)).
				Do(c)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func startScreencastRepaintLoop(ctx context.Context) func() {
	execCtxID, err := createScreencastExecutionContext(ctx)
	if err != nil {
		slog.Warn("enable screencast repaint loop failed", "err", err)
		return func() {}
	}

	if err := evaluateScreencastJS(ctx, execCtxID, assets.ScreencastRepaintStartJS); err != nil {
		slog.Warn("enable screencast repaint loop failed", "err", err)
		return func() {}
	}

	return func() {
		if err := evaluateScreencastJS(ctx, execCtxID, assets.ScreencastRepaintStopJS); err != nil {
			slog.Warn("disable screencast repaint loop failed", "err", err)
		}
	}
}

func createScreencastExecutionContext(ctx context.Context) (runtime.ExecutionContextID, error) {
	frameTree, err := getScreencastFrameTree(ctx)
	if err != nil {
		return 0, fmt.Errorf("get frame tree: %w", err)
	}
	if frameTree == nil || frameTree.Frame == nil {
		return 0, errors.New("missing top frame")
	}

	execCtxID, err := createScreencastIsolatedWorld(ctx, newScreencastIsolatedWorldParams(frameTree.Frame.ID))
	if err != nil {
		return 0, fmt.Errorf("create isolated world: %w", err)
	}
	return execCtxID, nil
}

func evaluateScreencastJS(ctx context.Context, execCtxID runtime.ExecutionContextID, expression string) error {
	_, exceptionDetails, err := evaluateScreencastInWorld(ctx, newScreencastEvaluateParams(execCtxID, expression))
	if err != nil {
		return fmt.Errorf("evaluate in isolated world: %w", err)
	}
	if exceptionDetails != nil {
		return fmt.Errorf("evaluate in isolated world: %w", exceptionDetails)
	}
	return nil
}

func newScreencastIsolatedWorldParams(frameID cdp.FrameID) *page.CreateIsolatedWorldParams {
	return page.CreateIsolatedWorld(frameID).WithWorldName(screencastRepaintWorldName)
}

func newScreencastEvaluateParams(execCtxID runtime.ExecutionContextID, expression string) *runtime.EvaluateParams {
	return runtime.Evaluate(expression).WithContextID(execCtxID)
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
