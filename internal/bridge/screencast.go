package bridge

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/config"
)

// StartScreencast begins streaming screencast frames. The caller must call
// Close on the returned ScreencastStream when done. The bridge picks
// event-driven (Page.startScreencast) or polling (CaptureScreenshot on a
// ticker) based on the browser's CapEventScreencast capability and headless
// mode. Polling is used for headless browsers and providers whose CDP proxy
// does not support Page.startScreencast (e.g. Cloak).
func (b *Bridge) StartScreencast(ctx context.Context, opts ScreencastOpts) (*ScreencastStream, error) {
	if b.shouldUsePollingScreencast() {
		return b.startScreencastPolling(ctx, opts)
	}
	return b.startScreencastEventDriven(ctx, opts)
}

func (b *Bridge) shouldUsePollingScreencast() bool {
	if b.Config != nil && b.Config.Headless {
		return true
	}
	if b.Config != nil {
		browserID := config.NormalizeBrowser(b.Config.DefaultBrowser)
		if browser, ok := browsers.Get(browserID); ok {
			return !browser.Capabilities().Has(browsers.CapEventScreencast)
		}
	}
	return false
}

// startScreencastEventDriven uses CDP Page.startScreencast for headed browsers.
func (b *Bridge) startScreencastEventDriven(ctx context.Context, opts ScreencastOpts) (*ScreencastStream, error) {
	fps := opts.FPS
	if fps <= 0 {
		fps = 1
	}
	minFrameInterval := time.Second / time.Duration(fps)

	frameCh := make(chan []byte, 3)
	done := make(chan struct{})
	ackCh := make(chan int64, 128)

	// ACK goroutine
	go func() {
		for {
			select {
			case sessionID := <-ackCh:
				if err := chromedp.Run(ctx,
					chromedp.ActionFunc(func(c context.Context) error {
						return page.ScreencastFrameAck(sessionID).Do(c)
					}),
				); err != nil && !errors.Is(err, context.Canceled) {
					slog.Debug("screencast ack failed", "err", err)
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
						slog.Debug("screencast ack fallback failed", "err", err)
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
				slog.Warn("screencast frame base64 decode failed", "error", err)
				return
			}

			select {
			case frameCh <- data:
			default:
			}
		}
	})

	// Start the CDP screencast
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			return page.StartScreencast().
				WithFormat(page.ScreencastFormatJpeg).
				WithQuality(int64(opts.Quality)).
				WithMaxWidth(int64(opts.MaxWidth)).
				WithMaxHeight(int64(opts.MaxHeight)).
				WithEveryNthFrame(int64(opts.EveryNthFrame)).
				Do(c)
		}),
	)
	if err != nil {
		close(done)
		return nil, fmt.Errorf("start screencast: %w", err)
	}

	stopRepaintLoop := cdptk.StartRepaintLoop(ctx)

	stream := &ScreencastStream{
		Frames: frameCh,
		done:   done,
		closer: func() {
			stopRepaintLoop()
			_ = chromedp.Run(ctx,
				chromedp.ActionFunc(func(c context.Context) error {
					return page.StopScreencast().Do(c)
				}),
			)
		},
	}
	return stream, nil
}

// startScreencastPolling uses CaptureScreenshot on a ticker for headless browsers.
func (b *Bridge) startScreencastPolling(ctx context.Context, opts ScreencastOpts) (*ScreencastStream, error) {
	fps := opts.FPS
	if fps <= 0 {
		fps = 1
	}
	frameInterval := time.Second / time.Duration(fps)
	if frameInterval <= 0 {
		frameInterval = time.Second
	}

	frameCh := make(chan []byte, 2)
	done := make(chan struct{})

	capture := func(ctx context.Context) ([]byte, error) {
		if b.captureFunc != nil {
			return b.captureFunc(ctx, "jpeg", opts.Quality)
		}
		return b.CaptureScreenshot(ctx, "jpeg", opts.Quality, nil)
	}

	go func() {
		defer close(frameCh)

		t0 := time.Now()
		frame, err := capture(ctx)
		if err != nil {
			slog.Warn("screencast polling: initial CaptureScreenshot failed",
				"err", err, "elapsed", time.Since(t0))
			return
		}
		slog.Debug("screencast polling: initial frame captured",
			"bytes", len(frame), "elapsed", time.Since(t0))

		select {
		case frameCh <- frame:
		case <-done:
			return
		}

		ticker := time.NewTicker(frameInterval)
		defer ticker.Stop()

		var frames int
		for {
			select {
			case <-ticker.C:
				t1 := time.Now()
				frame, err := capture(ctx)
				if err != nil {
					slog.Warn("screencast polling: CaptureScreenshot failed",
						"err", err, "frame", frames, "elapsed", time.Since(t1))
					return
				}
				frames++
				if frames <= 3 || frames%30 == 0 {
					slog.Debug("screencast polling: frame captured",
						"frame", frames, "bytes", len(frame), "elapsed", time.Since(t1))
				}
				select {
				case frameCh <- frame:
				case <-done:
					return
				}
			case <-done:
				return
			}
		}
	}()

	stream := &ScreencastStream{
		Frames: frameCh,
		done:   done,
	}
	return stream, nil
}
