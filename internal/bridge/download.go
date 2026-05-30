package bridge

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// DownloadOpts configures a browser-based download.
type DownloadOpts struct {
	// MaxBytes is the maximum response size in bytes. 0 means no limit.
	MaxBytes int
	// MaxRedirects limits browser-side redirects. -1 means unlimited.
	MaxRedirects int
	// ValidateURL is called for every request/redirect URL seen by the browser.
	// If it returns an error, the request is blocked.
	ValidateURL func(rawURL string, isRedirect bool) error
	// ValidateRemoteIP is called when the response remote IP is known.
	// If it returns an error, the download is aborted.
	ValidateRemoteIP func(remoteIP string) error
	// IsDomainAllowed reports whether a URL's domain is on the operator allowlist
	// (bypassing private-IP checks).
	IsDomainAllowed func(rawURL string) bool
	// ParseContentLength extracts Content-Length from response headers, if present.
	ParseContentLength func(headers map[string]interface{}) (int64, bool)
}

// DownloadResult holds the result of a browser-based download.
type DownloadResult struct {
	Body       []byte
	MIMEType   string
	StatusCode int
}

// DownloadURL downloads a URL using the browser's session (cookies, stealth).
// It creates a temporary tab, enables fetch interception to validate requests,
// navigates to the URL, and returns the response body.
func (b *Bridge) DownloadURL(ctx context.Context, dlURL string, opts DownloadOpts) (*DownloadResult, error) {
	// Create a temporary tab context for the download.
	browserCtx := b.BrowserContext()
	tabCtx, tabCancel := chromedp.NewContext(browserCtx)
	defer tabCancel()

	tCtx, tCancel := context.WithCancel(ctx)
	defer tCancel()

	// Replace the tab context with one that inherits the caller's deadline.
	_ = tabCtx // ensure tabCtx stays alive for the chromedp target

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

	// Enable fetch interception.
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

	// Navigate the temp tab to the URL.
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
		return &DownloadResult{StatusCode: responseStatus, MIMEType: responseMIME}, fmt.Errorf("remote server returned HTTP %d", responseStatus)
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

	return &DownloadResult{
		Body:       body,
		MIMEType:   responseMIME,
		StatusCode: responseStatus,
	}, nil
}
