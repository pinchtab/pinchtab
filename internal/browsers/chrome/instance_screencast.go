package chrome

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/cdptk"
)

// StartScreencast begins streaming screencast frames. The caller must call
// Close on the returned ScreencastStream when done. The instance picks
// event-driven (Page.startScreencast) or polling (CaptureScreenshot on a
// ticker) based on the headless flag.
func (i *Instance) StartScreencast(ctx context.Context, opts browsers.ScreencastOpts) (*browsers.ScreencastStream, error) {
	if i.headless {
		return i.startScreencastPolling(ctx, opts)
	}
	return i.startScreencastEventDriven(ctx, opts)
}

// startScreencastEventDriven uses CDP Page.startScreencast for headed browsers.
func (i *Instance) startScreencastEventDriven(ctx context.Context, opts browsers.ScreencastOpts) (*browsers.ScreencastStream, error) {
	fps := opts.FPS
	if fps <= 0 {
		fps = 1
	}
	minFrameInterval := time.Second / time.Duration(fps)

	frameCh := make(chan []byte, 3)
	ackCh := make(chan int64, 128)

	// stream is captured by the ack goroutine and the ListenTarget callback
	// below; it must never be reassigned (an unsynchronized write racing
	// their reads). The repaint-loop stop only exists after StartScreencast
	// succeeds, so the closer late-binds it through an atomic instead.
	var stopRepaint atomic.Value // holds func()
	stream := browsers.NewScreencastStream(frameCh, func() {
		if f, ok := stopRepaint.Load().(func()); ok {
			f()
		}
		_ = chromedp.Run(ctx,
			chromedp.ActionFunc(func(c context.Context) error {
				return page.StopScreencast().Do(c)
			}),
		)
	})

	go func() {
		for {
			select {
			case sessionID := <-ackCh:
				if err := chromedp.Run(ctx,
					chromedp.ActionFunc(func(c context.Context) error {
						return page.ScreencastFrameAck(sessionID).Do(c)
					}),
				); err != nil && !isContextCanceled(err) {
					slog.Debug("screencast ack failed", "err", err)
				}
			case <-stream.Done():
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
			case <-stream.Done():
				return
			default:
				go func(sessionID int64) {
					if err := chromedp.Run(ctx,
						chromedp.ActionFunc(func(c context.Context) error {
							return page.ScreencastFrameAck(sessionID).Do(c)
						}),
					); err != nil && !isContextCanceled(err) {
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
		stream.Close()
		return nil, fmt.Errorf("start screencast: %w", err)
	}

	stopRepaint.Store(cdptk.StartRepaintLoop(ctx))

	return stream, nil
}

// startScreencastPolling uses CaptureScreenshot on a ticker for headless browsers.
func (i *Instance) startScreencastPolling(ctx context.Context, opts browsers.ScreencastOpts) (*browsers.ScreencastStream, error) {
	fps := opts.FPS
	if fps <= 0 {
		fps = 1
	}
	frameInterval := time.Second / time.Duration(fps)
	if frameInterval <= 0 {
		frameInterval = time.Second
	}

	frameCh := make(chan []byte, 2)

	stream := browsers.NewScreencastStream(frameCh, nil)

	go func() {
		defer close(frameCh)

		t0 := time.Now()
		frame, err := i.CaptureScreenshot(ctx, "jpeg", opts.Quality, nil)
		if err != nil {
			slog.Warn("screencast polling: initial CaptureScreenshot failed",
				"err", err, "elapsed", time.Since(t0))
			return
		}
		slog.Debug("screencast polling: initial frame captured",
			"bytes", len(frame), "elapsed", time.Since(t0))

		select {
		case frameCh <- frame:
		case <-stream.Done():
			return
		}

		ticker := time.NewTicker(frameInterval)
		defer ticker.Stop()

		var frames int
		for {
			select {
			case <-ticker.C:
				t1 := time.Now()
				frame, err := i.CaptureScreenshot(ctx, "jpeg", opts.Quality, nil)
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
				case <-stream.Done():
					return
				}
			case <-stream.Done():
				return
			}
		}
	}()

	return stream, nil
}
