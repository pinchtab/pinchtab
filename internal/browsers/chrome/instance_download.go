package chrome

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browsers"
)

// DownloadURL downloads a URL using the browser's session (cookies, stealth).
// It creates a temporary tab, enables fetch interception to validate requests,
// navigates to the URL, and returns the response body.
func (i *Instance) DownloadURL(ctx context.Context, dlURL string, opts browsers.DownloadOpts) (*browsers.DownloadResult, error) {
	tabCtx, tabCancel := chromedp.NewContext(i.browserCtx)
	defer tabCancel()

	tCtx, tCancel := context.WithCancel(ctx)
	defer tCancel()

	var requestID network.RequestID
	var responseMIME string
	var responseStatus int
	var mainFrameID cdp.FrameID
	done := make(chan struct{}, 1)
	var receivedBytes atomic.Int64

	var mu sync.Mutex
	var blockedErr error

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

	if err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().Do(ctx)
	})); err != nil {
		return nil, fmt.Errorf("fetch enable: %w", err)
	}
	defer func() {
		_ = chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			return fetch.Disable().Do(ctx)
		}))
	}()

	chromedp.ListenTarget(tabCtx, func(ev interface{}) {
		switch e := ev.(type) {
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
						_ = fetch.FailRequest(reqID, network.ErrorReasonBlockedByClient).Do(cdp.WithExecutor(tabCtx, chromedp.FromContext(tabCtx).Target))
						return
					}
				}
				_ = fetch.ContinueRequest(reqID).Do(cdp.WithExecutor(tabCtx, chromedp.FromContext(tabCtx).Target))
			}()
		case *network.EventRequestWillBeSent:
			if e.Type != network.ResourceTypeDocument {
				return
			}
			if mainFrameID == "" {
				mainFrameID = e.FrameID
			}
			if e.FrameID == mainFrameID {
				requestID = e.RequestID
			}
		case *network.EventResponseReceived:
			if e.RequestID == requestID && requestID != "" {
				requestID = e.RequestID
				responseMIME = e.Response.MimeType
				responseStatus = int(e.Response.Status)
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
						noteBlocked(fmt.Errorf("download response too large: received %d bytes, max %d", contentLength, opts.MaxBytes))
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
			if e.RequestID == requestID && requestID != "" {
				chunk := e.EncodedDataLength
				if chunk <= 0 {
					chunk = e.DataLength
				}
				if opts.MaxBytes > 0 && chunk > 0 && receivedBytes.Add(chunk) > int64(opts.MaxBytes) {
					noteBlocked(fmt.Errorf("download response too large: received %d bytes, max %d", receivedBytes.Load(), opts.MaxBytes))
					select {
					case done <- struct{}{}:
					default:
					}
					tCancel()
					return
				}
			}
		case *network.EventLoadingFinished:
			if e.RequestID == requestID && requestID != "" {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		case *network.EventLoadingFailed:
			if e.RequestID == requestID && requestID != "" {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		}
	})

	if err := chromedp.Run(tabCtx, network.Enable()); err != nil {
		return nil, fmt.Errorf("network enable: %w", err)
	}

	navErr := chromedp.Run(tabCtx, chromedp.Navigate(dlURL))
	if navErr != nil {
		if bErr := getBlockedErr(); bErr != nil {
			return nil, bErr
		}
		return nil, navErr
	}

	select {
	case <-done:
	case <-tCtx.Done():
		if bErr := getBlockedErr(); bErr != nil {
			return nil, bErr
		}
		return nil, fmt.Errorf("download timed out")
	}

	if bErr := getBlockedErr(); bErr != nil {
		return nil, bErr
	}

	if responseStatus >= 400 {
		return &browsers.DownloadResult{StatusCode: responseStatus, MIMEType: responseMIME}, fmt.Errorf("remote server returned HTTP %d", responseStatus)
	}
	if requestID == "" {
		return nil, fmt.Errorf("download response was not captured")
	}

	var body []byte
	if err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		b, err := network.GetResponseBody(requestID).Do(ctx)
		if err != nil {
			return err
		}
		body = b
		return nil
	})); err != nil {
		return nil, fmt.Errorf("get response body: %w", err)
	}

	return &browsers.DownloadResult{
		Body:       body,
		MIMEType:   responseMIME,
		StatusCode: responseStatus,
	}, nil
}
