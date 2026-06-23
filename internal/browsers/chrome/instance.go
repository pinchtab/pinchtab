package chrome

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/cdptk"
)

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
	return cdpops.CallFunctionOnNode(ctx, backendNodeID, functionDecl, args, result)
}

func (i *Instance) DescribeNode(ctx context.Context, backendNodeID int64) (*browsers.NodeInfo, error) {
	return cdpops.DescribeNode(ctx, backendNodeID)
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

func (i *Instance) SetFileInputFiles(ctx context.Context, nodeID int64, paths []string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return dom.SetFileInputFiles(paths).WithNodeID(cdp.NodeID(nodeID)).Do(ctx)
	}))
}

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

func (i *Instance) SetViewport(ctx context.Context, params browsers.ViewportParams) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetDeviceMetricsOverride(params.Width, params.Height, params.DeviceScaleFactor, params.Mobile).
			WithScreenWidth(params.Width).WithScreenHeight(params.Height).Do(ctx)
	}))
}

func (i *Instance) SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetGeolocationOverride().WithLatitude(lat).WithLongitude(lng).WithAccuracy(accuracy).Do(ctx)
	}))
}

func (i *Instance) SetEmulatedMedia(ctx context.Context, feature, value string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: feature, Value: value}}).Do(ctx)
	}))
}

func (i *Instance) SetNetworkConditions(ctx context.Context, params browsers.NetworkConditions) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.OverrideNetworkState(params.Offline, params.Latency, params.DownloadThroughput, params.UploadThroughput).
			Do(ctx)
	}))
}

func (i *Instance) SetExtraHTTPHeaders(ctx context.Context, headers map[string]string) error {
	hdrs := make(network.Headers, len(headers))
	for k, v := range headers {
		hdrs[k] = v
	}
	return chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetExtraHTTPHeaders(hdrs).Do(ctx)
	}))
}

func (i *Instance) CurrentURL(ctx context.Context) (string, error) {
	var url string
	err := chromedp.Run(ctx, chromedp.Location(&url))
	return url, err
}

func (i *Instance) CurrentTitle(ctx context.Context) (string, error) {
	var title string
	err := chromedp.Run(ctx, chromedp.Title(&title))
	return title, err
}

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

// frameExecutionContextID returns a Runtime.executionContextId that evaluates
// in the given frame's document. Returns (0, nil) when frameID is empty so
// callers can fall back to the default top-level context without branching.
func frameExecutionContextID(ctx context.Context, frameID string) (int64, error) {
	return cdpops.FrameExecutionContextID(ctx, frameID)
}

func isContextCanceled(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), context.Canceled.Error()))
}
