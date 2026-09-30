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
	tabCtx, tabCancel := chromedp.NewContext(b.BrowserContext())
	defer tabCancel()

	// Bound all tab work by the download deadline AND the caller's
	// cancellation: tCtx derives from tabCtx so chromedp ops keep their
	// target, and AfterFunc bridges caller disconnect/deadline into tCancel.
	// The interceptor also calls tCancel to abort an in-flight transfer
	// (size cap, blocked remote IP).
	tCtx, tCancel := context.WithTimeout(tabCtx, downloadNavTimeout)
	defer tCancel()
	stop := context.AfterFunc(ctx, tCancel)
	defer stop()

	d := &downloadInterceptor{
		ctx:    tCtx,
		cancel: tCancel,
		opts:   opts,
		done:   make(chan struct{}, 1),
	}
	// The temp tab bypasses TabManager's tabSetup, so proxy auth handling
	// must be wired here.
	if b.Config != nil && bridgeruntime.ProxyAuthEnabled(b.Config.Proxy) {
		d.proxyAuth = true
		d.proxyUser = b.Config.Proxy.Username
		d.proxyPass = b.Config.Proxy.Password
	}

	if err := d.enableFetch(); err != nil {
		return nil, err
	}
	// Cleanup runs on tabCtx, not tCtx: it must still work after the
	// download deadline fired.
	defer func() {
		_ = chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			return fetch.Disable().Do(ctx)
		}))
	}()
	chromedp.ListenTarget(tCtx, d.handleEvent)

	if err := chromedp.Run(tCtx, network.Enable()); err != nil {
		return nil, fmt.Errorf("network enable: %w", err)
	}
	if err := d.navigateAndWait(dlURL); err != nil {
		return nil, err
	}
	return d.result()
}

// mu guards blockedErr and the capture fields, which are written from the
// event-dispatch goroutine and read by the caller; done only orders events
// sent before the read, not stragglers.
type downloadInterceptor struct {
	ctx       context.Context
	cancel    context.CancelFunc
	opts      DownloadOpts
	proxyAuth bool
	proxyUser string
	proxyPass string

	done          chan struct{}
	receivedBytes atomic.Int64

	mu             sync.Mutex
	blockedErr     error
	requestID      network.RequestID
	responseMIME   string
	responseStatus int
	mainFrameID    cdp.FrameID
}

func (d *downloadInterceptor) enableFetch() error {
	enable := fetch.Enable()
	if d.proxyAuth {
		enable = enable.WithHandleAuthRequests(true)
	}
	if err := chromedp.Run(d.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return enable.Do(ctx)
	})); err != nil {
		return fmt.Errorf("fetch enable: %w", err)
	}
	return nil
}

func (d *downloadInterceptor) executor() context.Context {
	return cdp.WithExecutor(d.ctx, chromedp.FromContext(d.ctx).Target)
}

func (d *downloadInterceptor) signalDone() {
	select {
	case d.done <- struct{}{}:
	default:
	}
}

func (d *downloadInterceptor) block(err error) {
	d.mu.Lock()
	if d.blockedErr == nil {
		d.blockedErr = err
	}
	d.mu.Unlock()
	d.signalDone()
}

func (d *downloadInterceptor) abort(err error) {
	d.block(err)
	d.cancel()
}

func (d *downloadInterceptor) blocked() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.blockedErr
}

func (d *downloadInterceptor) isMainRequest(id network.RequestID) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.requestID != "" && id == d.requestID
}

func (d *downloadInterceptor) handleEvent(ev any) {
	switch e := ev.(type) {
	case *fetch.EventAuthRequired:
		if d.proxyAuth {
			go d.answerAuth(e)
		}
	case *fetch.EventRequestPaused:
		go d.gateRequest(e)
	case *network.EventRequestWillBeSent:
		d.trackMainRequest(e)
	case *network.EventResponseReceived:
		d.checkResponse(e)
	case *network.EventDataReceived:
		d.countData(e)
	case *network.EventLoadingFinished:
		if d.isMainRequest(e.RequestID) {
			d.signalDone()
		}
	case *network.EventLoadingFailed:
		if d.isMainRequest(e.RequestID) {
			d.signalDone()
		}
	}
}

func (d *downloadInterceptor) answerAuth(e *fetch.EventAuthRequired) {
	resp := bridgeruntime.AuthChallengeResponse(e, d.proxyUser, d.proxyPass)
	_ = fetch.ContinueWithAuth(e.RequestID, resp).Do(d.executor())
}

func (d *downloadInterceptor) gateRequest(e *fetch.EventRequestPaused) {
	if d.opts.ValidateURL != nil {
		if err := d.opts.ValidateURL(e.Request.URL, e.RedirectedRequestID != ""); err != nil {
			d.block(err)
			_ = fetch.FailRequest(e.RequestID, network.ErrorReasonBlockedByClient).Do(d.executor())
			return
		}
	}
	_ = fetch.ContinueRequest(e.RequestID).Do(d.executor())
}

func (d *downloadInterceptor) trackMainRequest(e *network.EventRequestWillBeSent) {
	if e.Type != network.ResourceTypeDocument {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.mainFrameID == "" {
		d.mainFrameID = e.FrameID
	}
	if e.FrameID == d.mainFrameID {
		d.requestID = e.RequestID
	}
}

func (d *downloadInterceptor) checkResponse(e *network.EventResponseReceived) {
	d.mu.Lock()
	match := e.RequestID == d.requestID && d.requestID != ""
	if match {
		d.responseMIME = e.Response.MimeType
		d.responseStatus = int(e.Response.Status)
	}
	d.mu.Unlock()
	if !match {
		return
	}
	opts := d.opts
	if opts.IsDomainAllowed != nil && !opts.IsDomainAllowed(e.Response.URL) && opts.ValidateRemoteIP != nil {
		if err := opts.ValidateRemoteIP(e.Response.RemoteIPAddress); err != nil {
			d.abort(err)
			return
		}
	}
	if opts.MaxBytes > 0 && opts.ParseContentLength != nil {
		if contentLength, ok := opts.ParseContentLength(e.Response.Headers); ok && contentLength > int64(opts.MaxBytes) {
			d.abort(fmt.Errorf("%w: received %d bytes, max %d", ErrDownloadTooLarge, contentLength, opts.MaxBytes))
		}
	}
}

func (d *downloadInterceptor) countData(e *network.EventDataReceived) {
	if !d.isMainRequest(e.RequestID) {
		return
	}
	chunk := e.EncodedDataLength
	if chunk <= 0 {
		chunk = e.DataLength
	}
	maxBytes := int64(d.opts.MaxBytes)
	if maxBytes > 0 && chunk > 0 && d.receivedBytes.Add(chunk) > maxBytes {
		d.abort(fmt.Errorf("%w: received %d bytes, max %d", ErrDownloadTooLarge, d.receivedBytes.Load(), d.opts.MaxBytes))
	}
}

func (d *downloadInterceptor) navigateAndWait(dlURL string) error {
	if navErr := chromedp.Run(d.ctx, chromedp.Navigate(dlURL)); navErr != nil {
		if bErr := d.blocked(); bErr != nil {
			return bErr
		}
		if d.ctx.Err() != nil {
			return ErrDownloadTimeout
		}
		return navErr
	}

	select {
	case <-d.done:
	case <-d.ctx.Done():
		if bErr := d.blocked(); bErr != nil {
			return bErr
		}
		return ErrDownloadTimeout
	}
	return d.blocked()
}

func (d *downloadInterceptor) result() (*DownloadResult, error) {
	d.mu.Lock()
	reqID := d.requestID
	respMIME := d.responseMIME
	respStatus := d.responseStatus
	d.mu.Unlock()

	if respStatus >= 400 {
		return &DownloadResult{StatusCode: respStatus, MIMEType: respMIME}, fmt.Errorf("remote server returned HTTP %d", respStatus)
	}
	if reqID == "" {
		return nil, fmt.Errorf("download response was not captured")
	}

	var body []byte
	if err := chromedp.Run(d.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		body, err = network.GetResponseBody(reqID).Do(ctx)
		return err
	})); err != nil {
		return nil, fmt.Errorf("get response body: %w", err)
	}

	return &DownloadResult{
		Body:       body,
		MIMEType:   respMIME,
		StatusCode: respStatus,
	}, nil
}
