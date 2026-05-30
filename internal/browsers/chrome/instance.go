package chrome

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/cdptk"
)

// Instance implements browsers.RuntimeInstance for Google Chrome.
type Instance struct {
	browserCtx context.Context
	headless   bool
}

// NewInstance creates a Chrome RuntimeInstance backed by the given browser
// context. headless controls the screencast strategy (polling vs event-driven).
func NewInstance(browserCtx context.Context, headless bool) *Instance {
	return &Instance{browserCtx: browserCtx, headless: headless}
}

var _ browsers.RuntimeInstance = (*Instance)(nil)

// ---------------------------------------------------------------------------
// Visual capture
// ---------------------------------------------------------------------------

// CaptureScreenshot captures a screenshot of the current page.
func (i *Instance) CaptureScreenshot(ctx context.Context, format string, quality int, clip *cdptk.ScreenshotClip) ([]byte, error) {
	var buf []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		// Wake the target's renderer before capturing. Background / non-
		// foreground tabs throttle their compositor and stop painting, so
		// captureScreenshot blocks until the action deadline (~30s). A
		// best-effort BringToFront resumes painting for the target we are
		// about to capture; the error is ignored so providers whose CDP proxy
		// does not implement it still capture normally.
		_ = page.BringToFront().Do(c)

		var cdpFormat page.CaptureScreenshotFormat
		switch format {
		case "png":
			cdpFormat = page.CaptureScreenshotFormatPng
		default:
			cdpFormat = page.CaptureScreenshotFormatJpeg
		}

		// WithFromSurface(false) reads the renderer's current view directly
		// instead of waiting for a fresh compositor surface frame. On idle
		// pages in headed browsers (e.g. Cloak) the surface stops swapping
		// frames, so the default fromSurface=true blocks until the action
		// deadline (~30s) — stalling one-shot screenshots and polling
		// screencast alike. In headless Chrome the flag is a no-op.
		shot := page.CaptureScreenshot().WithFormat(cdpFormat).WithFromSurface(false)
		if clip != nil {
			shot = shot.WithClip(&page.Viewport{
				X:      clip.X,
				Y:      clip.Y,
				Width:  clip.Width,
				Height: clip.Height,
				Scale:  clip.Scale,
			})
		}
		if cdpFormat == page.CaptureScreenshotFormatJpeg && quality > 0 {
			shot = shot.WithQuality(int64(quality))
		}
		var err error
		buf, err = shot.Do(c)
		return err
	}))
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	return buf, nil
}

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

	stream := browsers.NewScreencastStream(frameCh, func() {
		_ = chromedp.Run(ctx,
			chromedp.ActionFunc(func(c context.Context) error {
				return page.StopScreencast().Do(c)
			}),
		)
	})

	// ACK goroutine
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
		stream.Close()
		return nil, fmt.Errorf("start screencast: %w", err)
	}

	stopRepaintLoop := cdptk.StartRepaintLoop(ctx)

	// Wrap the closer to also stop the repaint loop.
	origStream := stream
	stream = browsers.NewScreencastStream(frameCh, func() {
		stopRepaintLoop()
		origStream.Close()
	})

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

// ---------------------------------------------------------------------------
// JavaScript evaluation
// ---------------------------------------------------------------------------

// Evaluate evaluates a JavaScript expression in the page context.
func (i *Instance) Evaluate(ctx context.Context, expression string, result any, opts browsers.EvalOpts) error {
	var chromedpOpts []chromedp.EvaluateOption
	if opts.AwaitPromise {
		chromedpOpts = append(chromedpOpts, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		})
	}
	return chromedp.Run(ctx, chromedp.Evaluate(expression, result, chromedpOpts...))
}

// EvaluateInFrame evaluates a JavaScript expression in the given frame's
// execution context. If frameID is empty, behaves like Evaluate.
func (i *Instance) EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts browsers.EvalOpts) error {
	if frameID == "" {
		return i.Evaluate(ctx, expression, result, opts)
	}

	execID, err := frameExecutionContextID(ctx, frameID)
	if err != nil {
		return fmt.Errorf("resolve frame context: %w", err)
	}
	if execID == 0 {
		return i.Evaluate(ctx, expression, result, opts)
	}

	var raw json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    expression,
			"returnByValue": true,
			"contextId":     execID,
		}, &raw)
	}))
	if err != nil {
		return err
	}

	var parsed struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails,omitempty"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("evaluate in frame parse: %w", err)
	}
	if parsed.ExceptionDetails != nil && parsed.ExceptionDetails.Text != "" {
		return fmt.Errorf("%s", parsed.ExceptionDetails.Text)
	}
	if result == nil || len(parsed.Result.Value) == 0 {
		return nil
	}
	return json.Unmarshal(parsed.Result.Value, result)
}

// CallFunctionOnNode resolves a backend node ID to a Runtime object, then
// calls the given JavaScript function on it.
func (i *Instance) CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Step 1: Resolve backend node ID to a remote object.
		var resolveResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &resolveResult); err != nil {
			return fmt.Errorf("resolve node: %w", err)
		}

		var resolved struct {
			Object struct {
				ObjectID string `json:"objectId"`
			} `json:"object"`
		}
		if err := json.Unmarshal(resolveResult, &resolved); err != nil {
			return fmt.Errorf("parse resolved node: %w", err)
		}
		if resolved.Object.ObjectID == "" {
			return fmt.Errorf("element not found in DOM (backendNodeId=%d)", backendNodeID)
		}

		// Step 2: Call the function on the resolved object.
		params := map[string]any{
			"functionDeclaration": functionDecl,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}
		if len(args) > 0 {
			params["arguments"] = args
		}

		var callResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", params, &callResult); err != nil {
			return fmt.Errorf("call function on node: %w", err)
		}

		// Step 3: Parse the result.
		var callParsed struct {
			Result struct {
				Type  string          `json:"type"`
				Value json.RawMessage `json:"value"`
			} `json:"result"`
			ExceptionDetails *struct {
				Text string `json:"text"`
			} `json:"exceptionDetails,omitempty"`
		}
		if err := json.Unmarshal(callResult, &callParsed); err != nil {
			return fmt.Errorf("parse call result: %w", err)
		}
		if callParsed.ExceptionDetails != nil && callParsed.ExceptionDetails.Text != "" {
			return fmt.Errorf("call function on node: %s", callParsed.ExceptionDetails.Text)
		}

		if result == nil || len(callParsed.Result.Value) == 0 {
			return nil
		}
		return json.Unmarshal(callParsed.Result.Value, result)
	}))
}

// ---------------------------------------------------------------------------
// DOM
// ---------------------------------------------------------------------------

// DescribeNode returns DOM structural info for a backend node ID.
func (i *Instance) DescribeNode(ctx context.Context, backendNodeID int64) (*browsers.NodeInfo, error) {
	var info browsers.NodeInfo
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var result json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{
			"backendNodeId": backendNodeID,
		}, &result); err != nil {
			return fmt.Errorf("describe node: %w", err)
		}

		var parsed struct {
			Node struct {
				LocalName      string   `json:"localName"`
				NodeName       string   `json:"nodeName"`
				Attributes     []string `json:"attributes"`
				ChildNodeCount int      `json:"childNodeCount"`
			} `json:"node"`
		}
		if err := json.Unmarshal(result, &parsed); err != nil {
			return fmt.Errorf("parse describe node: %w", err)
		}

		info.LocalName = parsed.Node.LocalName
		if info.LocalName == "" {
			info.LocalName = parsed.Node.NodeName
		}
		info.Attributes = parsed.Node.Attributes
		info.ChildNodeCount = parsed.Node.ChildNodeCount
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// ResolveSelectorToNodeID finds a DOM node by a unified selector string and
// returns its NodeID.
func (i *Instance) ResolveSelectorToNodeID(ctx context.Context, selector string) (int64, error) {
	var nodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var expr string
		switch {
		case strings.HasPrefix(selector, "xpath:"):
			xpath := selector[len("xpath:"):]
			expr = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue})()`, xpath)
		case strings.HasPrefix(selector, "//") || strings.HasPrefix(selector, "(//"):
			expr = fmt.Sprintf(`(function(){var r=document.evaluate(%q,document,null,XPathResult.FIRST_ORDERED_NODE_TYPE,null);return r.singleNodeValue})()`, selector)
		case strings.HasPrefix(selector, "text:"):
			text := selector[len("text:"):]
			expr = fmt.Sprintf(`(function(){var w=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);while(w.nextNode()){if(w.currentNode.textContent.includes(%q))return w.currentNode.parentElement}return null})()`, text)
		case strings.HasPrefix(selector, "css:"):
			css := selector[len("css:"):]
			expr = fmt.Sprintf(`document.querySelector(%q)`, css)
		default:
			expr = fmt.Sprintf(`document.querySelector(%q)`, selector)
		}

		val, _, err := runtime.Evaluate(expr).Do(ctx)
		if err != nil {
			return fmt.Errorf("evaluate: %w", err)
		}
		if val.ObjectID == "" {
			return fmt.Errorf("no element matches selector")
		}
		node, err := dom.RequestNode(val.ObjectID).Do(ctx)
		if err != nil {
			return fmt.Errorf("request node: %w", err)
		}
		nodeID = int64(node)
		return nil
	}))
	return nodeID, err
}

// SetFileInputFiles sets files on a file input element identified by its node ID.
func (i *Instance) SetFileInputFiles(ctx context.Context, nodeID int64, paths []string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return dom.SetFileInputFiles(paths).WithNodeID(cdp.NodeID(nodeID)).Do(ctx)
	}))
}

// ---------------------------------------------------------------------------
// Cookies
// ---------------------------------------------------------------------------

// GetCookies retrieves cookies for the given URLs via CDP.
func (i *Instance) GetCookies(ctx context.Context, urls []string) ([]browsers.CookieData, error) {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = network.GetCookies().WithURLs(urls).Do(ctx)
		return err
	}))
	if err != nil {
		return nil, err
	}

	result := make([]browsers.CookieData, len(cookies))
	for idx, c := range cookies {
		result[idx] = browsers.CookieData{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  float64(c.Expires),
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: c.SameSite.String(),
		}
	}
	return result, nil
}

// SetCookie sets a single cookie via CDP.
func (i *Instance) SetCookie(ctx context.Context, params browsers.SetCookieParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		p := network.SetCookie(params.Name, params.Value).
			WithURL(params.URL).
			WithHTTPOnly(params.HTTPOnly).
			WithSecure(params.Secure)

		if params.Domain != "" {
			p = p.WithDomain(params.Domain)
		}
		if params.Path != "" {
			p = p.WithPath(params.Path)
		}
		if params.Expires > 0 {
			expires := cdp.TimeSinceEpoch(time.Unix(int64(params.Expires), 0))
			p = p.WithExpires(&expires)
		}
		if params.SameSite != "" {
			var sameSite network.CookieSameSite
			switch strings.ToLower(params.SameSite) {
			case "strict":
				sameSite = network.CookieSameSiteStrict
			case "lax":
				sameSite = network.CookieSameSiteLax
			case "none":
				sameSite = network.CookieSameSiteNone
			}
			if sameSite != "" {
				p = p.WithSameSite(sameSite)
			}
		}

		return p.Do(ctx)
	}))
}

// ---------------------------------------------------------------------------
// Emulation
// ---------------------------------------------------------------------------

// SetViewport overrides device metrics for the page.
func (i *Instance) SetViewport(ctx context.Context, params browsers.ViewportParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetDeviceMetricsOverride(params.Width, params.Height, params.DeviceScaleFactor, params.Mobile).
			WithScreenWidth(params.Width).WithScreenHeight(params.Height).Do(ctx)
	}))
}

// SetGeolocation overrides the browser geolocation.
func (i *Instance) SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetGeolocationOverride().WithLatitude(lat).WithLongitude(lng).WithAccuracy(accuracy).Do(ctx)
	}))
}

// SetEmulatedMedia sets an emulated media feature.
func (i *Instance) SetEmulatedMedia(ctx context.Context, feature, value string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: feature, Value: value}}).Do(ctx)
	}))
}

// ---------------------------------------------------------------------------
// Network state
// ---------------------------------------------------------------------------

// SetNetworkConditions emulates network conditions via CDP.
func (i *Instance) SetNetworkConditions(ctx context.Context, params browsers.NetworkConditions) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.OverrideNetworkState(params.Offline, params.Latency, params.DownloadThroughput, params.UploadThroughput).
			Do(ctx)
	}))
}

// SetExtraHTTPHeaders sets extra HTTP headers sent with every request.
func (i *Instance) SetExtraHTTPHeaders(ctx context.Context, headers map[string]string) error {
	hdrs := make(network.Headers, len(headers))
	for k, v := range headers {
		hdrs[k] = v
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetExtraHTTPHeaders(hdrs).Do(ctx)
	}))
}

// ---------------------------------------------------------------------------
// Navigation info
// ---------------------------------------------------------------------------

// CurrentURL returns the current page URL.
func (i *Instance) CurrentURL(ctx context.Context) (string, error) {
	var url string
	err := chromedp.Run(ctx, chromedp.Location(&url))
	return url, err
}

// CurrentTitle returns the current page title.
func (i *Instance) CurrentTitle(ctx context.Context) (string, error) {
	var title string
	err := chromedp.Run(ctx, chromedp.Title(&title))
	return title, err
}

// ---------------------------------------------------------------------------
// Download
// ---------------------------------------------------------------------------

// DownloadURL downloads a URL using the browser's session (cookies, stealth).
// It creates a temporary tab, enables fetch interception to validate requests,
// navigates to the URL, and returns the response body.
func (i *Instance) DownloadURL(ctx context.Context, dlURL string, opts browsers.DownloadOpts) (*browsers.DownloadResult, error) {
	// Create a temporary tab context for the download.
	tabCtx, tabCancel := chromedp.NewContext(i.browserCtx)
	defer tabCancel()

	tCtx, tCancel := context.WithCancel(ctx)
	defer tCancel()

	// Ensure tabCtx stays alive for the chromedp target.
	_ = tabCtx

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

// ---------------------------------------------------------------------------
// PDF
// ---------------------------------------------------------------------------

// PrintToPDF generates a PDF of the current page via CDP.
func (i *Instance) PrintToPDF(ctx context.Context, params browsers.PDFParams) ([]byte, error) {
	var buf []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		p := page.PrintToPDF().
			WithPrintBackground(params.PrintBackground).
			WithScale(params.Scale).
			WithLandscape(params.Landscape).
			WithPaperWidth(params.PaperWidth).
			WithPaperHeight(params.PaperHeight).
			WithMarginTop(params.MarginTop).
			WithMarginBottom(params.MarginBottom).
			WithMarginLeft(params.MarginLeft).
			WithMarginRight(params.MarginRight).
			WithPreferCSSPageSize(params.PreferCSSPageSize).
			WithDisplayHeaderFooter(params.DisplayHeaderFooter).
			WithGenerateTaggedPDF(params.GenerateTaggedPDF).
			WithGenerateDocumentOutline(params.GenerateDocumentOutline)

		if params.PageRanges != "" {
			p = p.WithPageRanges(params.PageRanges)
		}
		if params.HeaderTemplate != "" {
			p = p.WithHeaderTemplate(params.HeaderTemplate)
		}
		if params.FooterTemplate != "" {
			p = p.WithFooterTemplate(params.FooterTemplate)
		}

		var err error
		buf, _, err = p.Do(ctx)
		return err
	}))
	return buf, err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// frameExecutionContextID returns a Runtime.executionContextId that evaluates
// in the given frame's document. Returns (0, nil) when frameID is empty so
// callers can fall back to the default top-level context without branching.
func frameExecutionContextID(ctx context.Context, frameID string) (int64, error) {
	if frameID == "" {
		return 0, nil
	}

	var worldResult json.RawMessage
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Page.createIsolatedWorld", map[string]any{
			"frameId":   frameID,
			"worldName": "pinchtab-frame-scope",
		}, &worldResult)
	}))
	if err != nil {
		return 0, fmt.Errorf("create isolated world for frame %q: %w", frameID, err)
	}

	var resp struct {
		ExecutionContextID int64 `json:"executionContextId"`
	}
	if err := json.Unmarshal(worldResult, &resp); err != nil {
		return 0, err
	}
	if resp.ExecutionContextID == 0 {
		return 0, fmt.Errorf("frame %q has no execution context", frameID)
	}
	return resp.ExecutionContextID, nil
}

// isContextCanceled reports whether the error is a context cancellation.
func isContextCanceled(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), context.Canceled.Error()))
}
