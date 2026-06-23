package bridge

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	bridgeruntime "github.com/pinchtab/pinchtab/internal/bridge/runtime"
)

const downloadNavTimeout = 30 * time.Second

// Texts match the historical messages so anything still string-matching keeps working.
var (
	ErrDownloadTooLarge = errors.New("download response too large")
	ErrDownloadTimeout  = errors.New("download timed out")
)

func (b *Bridge) DownloadURL(ctx context.Context, dlURL string, opts DownloadOpts) (*DownloadResult, error) {
	browserCtx := b.BrowserContext()
	tabCtx, tabCancel := chromedp.NewContext(browserCtx)
	defer tabCancel()

	// Bound all tab work by the download deadline AND the caller's
	// cancellation: tCtx derives from tabCtx so chromedp ops keep their
	// target, and AfterFunc bridges caller disconnect/deadline into tCancel.
	// The guard listeners below also call tCancel to abort an in-flight
	// transfer (size cap, blocked remote IP).
	tCtx, tCancel := context.WithTimeout(tabCtx, downloadNavTimeout)
	defer tCancel()
	stop := context.AfterFunc(ctx, tCancel)
	defer stop()

	done := make(chan struct{}, 1)
	var receivedBytes atomic.Int64

	// mu guards blockedErr and the capture fields, which are written from the
	// event-dispatch goroutine and read by this goroutine; the done channel
	// only orders events sent before the read, not stragglers.
	var mu sync.Mutex
	var blockedErr error
	var requestID network.RequestID
	var responseMIME string
	var responseStatus int
	var mainFrameID cdp.FrameID

	noteBlocked := func(err error) {
		mu.Lock()
		if blockedErr == nil {
			blockedErr = err
		}
		mu.Unlock()
	}

	getBlockedErr := func() error {
		mu.Lock()
		defer mu.Unlock()
		return blockedErr
	}

	isMainRequest := func(id network.RequestID) bool {
		mu.Lock()
		defer mu.Unlock()
		return requestID != "" && id == requestID
	}

	// The temp tab bypasses TabManager's tabSetup, so proxy auth handling
	// must be wired here; this listener's EventAuthRequired case answers
	// the challenges.
	proxyAuth := b.Config != nil && bridgeruntime.ProxyAuthEnabled(b.Config.Proxy)
	enableFetch := fetch.Enable()
	if proxyAuth {
		enableFetch = enableFetch.WithHandleAuthRequests(true)
	}
	if err := chromedp.Run(tCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return enableFetch.Do(ctx)
	})); err != nil {
		return nil, fmt.Errorf("fetch enable: %w", err)
	}
	// Cleanup runs on tabCtx, not tCtx: it must still work after the
	// download deadline fired.
	defer func() {
		_ = chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			return fetch.Disable().Do(ctx)
		}))
	}()

	chromedp.ListenTarget(tCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			if !proxyAuth {
				return
			}
			go func() {
				resp := bridgeruntime.AuthChallengeResponse(e, b.Config.Proxy.Username, b.Config.Proxy.Password)
				_ = fetch.ContinueWithAuth(e.RequestID, resp).Do(cdp.WithExecutor(tCtx, chromedp.FromContext(tCtx).Target))
			}()
		case *fetch.EventRequestPaused:
			go func() {
				reqID := e.RequestID
				isRedirect := e.RedirectedRequestID != ""
				if opts.ValidateURL != nil {
					if err := opts.ValidateURL(e.Request.URL, isRedirect); err != nil {
						noteBlocked(err)
						select {
						case done <- struct{}{}:
						default:
						}
						_ = fetch.FailRequest(reqID, network.ErrorReasonBlockedByClient).Do(cdp.WithExecutor(tCtx, chromedp.FromContext(tCtx).Target))
						return
					}
				}
				_ = fetch.ContinueRequest(reqID).Do(cdp.WithExecutor(tCtx, chromedp.FromContext(tCtx).Target))
			}()
		case *network.EventRequestWillBeSent:
			if e.Type != network.ResourceTypeDocument {
				return
			}
			mu.Lock()
			if mainFrameID == "" {
				mainFrameID = e.FrameID
			}
			if e.FrameID == mainFrameID {
				requestID = e.RequestID
			}
			mu.Unlock()
		case *network.EventResponseReceived:
			mu.Lock()
			match := e.RequestID == requestID && requestID != ""
			if match {
				responseMIME = e.Response.MimeType
				responseStatus = int(e.Response.Status)
			}
			mu.Unlock()
			if match {
				if opts.IsDomainAllowed != nil && !opts.IsDomainAllowed(e.Response.URL) {
					if opts.ValidateRemoteIP != nil {
						if err := opts.ValidateRemoteIP(e.Response.RemoteIPAddress); err != nil {
							noteBlocked(err)
							select {
							case done <- struct{}{}:
							default:
							}
							tCancel()
							return
						}
					}
				}
				if opts.MaxBytes > 0 && opts.ParseContentLength != nil {
					if contentLength, ok := opts.ParseContentLength(e.Response.Headers); ok && contentLength > int64(opts.MaxBytes) {
						noteBlocked(fmt.Errorf("%w: received %d bytes, max %d", ErrDownloadTooLarge, contentLength, opts.MaxBytes))
						select {
						case done <- struct{}{}:
						default:
						}
						tCancel()
						return
					}
				}
			}
		case *network.EventDataReceived:
			if isMainRequest(e.RequestID) {
				chunk := e.EncodedDataLength
				if chunk <= 0 {
					chunk = e.DataLength
				}
				if opts.MaxBytes > 0 && chunk > 0 && receivedBytes.Add(chunk) > int64(opts.MaxBytes) {
					noteBlocked(fmt.Errorf("%w: received %d bytes, max %d", ErrDownloadTooLarge, receivedBytes.Load(), opts.MaxBytes))
					select {
					case done <- struct{}{}:
					default:
					}
					tCancel()
					return
				}
			}
		case *network.EventLoadingFinished:
			if isMainRequest(e.RequestID) {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		case *network.EventLoadingFailed:
			if isMainRequest(e.RequestID) {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		}
	})

	if err := chromedp.Run(tCtx, network.Enable()); err != nil {
		return nil, fmt.Errorf("network enable: %w", err)
	}

	navErr := chromedp.Run(tCtx, chromedp.Navigate(dlURL))
	if navErr != nil {
		if bErr := getBlockedErr(); bErr != nil {
			return nil, bErr
		}
		if tCtx.Err() != nil {
			return nil, ErrDownloadTimeout
		}
		return nil, navErr
	}

	select {
	case <-done:
	case <-tCtx.Done():
		if bErr := getBlockedErr(); bErr != nil {
			return nil, bErr
		}
		return nil, ErrDownloadTimeout
	}

	if bErr := getBlockedErr(); bErr != nil {
		return nil, bErr
	}

	mu.Lock()
	reqID := requestID
	respMIME := responseMIME
	respStatus := responseStatus
	mu.Unlock()

	if respStatus >= 400 {
		return &DownloadResult{StatusCode: respStatus, MIMEType: respMIME}, fmt.Errorf("remote server returned HTTP %d", respStatus)
	}
	if reqID == "" {
		return nil, fmt.Errorf("download response was not captured")
	}

	var body []byte
	if err := chromedp.Run(tCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		b, err := network.GetResponseBody(reqID).Do(ctx)
		if err != nil {
			return err
		}
		body = b
		return nil
	})); err != nil {
		return nil, fmt.Errorf("get response body: %w", err)
	}

	return &DownloadResult{
		Body:       body,
		MIMEType:   respMIME,
		StatusCode: respStatus,
	}, nil
}
