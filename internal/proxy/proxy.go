// Package proxy provides a shared HTTP reverse-proxy helper used by
// strategies and the dashboard fallback routes. It consolidates the
// previously duplicated proxyHTTP / proxyRequest functions into one
// place with a shared http.Client and WebSocket upgrade support.
package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

var DefaultClient = &http.Client{Timeout: httpx.MaxNavigationHTTPDuration}

type Options struct {
	Client            *http.Client
	AllowedURL        func(*url.URL) bool
	RewriteRequest    func(*http.Request)
	OnResponseHeaders func(origReq *http.Request, resp *http.Response)
	// OnResponse is called with the upstream response body for non-streaming
	// responses (Content-Type application/json, body ≤ 64 KB). The original
	// request is passed so callers can enrich activity context.
	OnResponse func(origReq *http.Request, body []byte)
}

// strippedProxyRequestHeaders never reach the instance by being COPIED. Every member is
// dropped from the blind copy; x-request-id is then re-added deliberately by
// httpx.ForwardRequestID, which is what makes one proxied request traceable in both the
// outer and the instance log instead of only the outer one.
//
// It stays on this list rather than being deleted from it because the two are different
// permissions: pass-through would forward whatever arrived under that name from anywhere,
// while the re-add forwards the one value the outer chain resolved for this request.
// RequestIDMiddleware stamps that value onto the request, so what is forwarded is the id
// the outer server logs — including when the caller supplied it, which that middleware
// honours by design. The rest of this list protects genuinely different things and is
// untouched: cookie carries the session secret, and the forwarding trio plus x-real-ip
// carry client network identity the instance has no business learning.
var strippedProxyRequestHeaders = map[string]struct{}{
	"cookie":            {},
	"forwarded":         {},
	"x-forwarded-for":   {},
	"x-forwarded-host":  {},
	"x-forwarded-proto": {},
	"x-real-ip":         {},
	"x-request-id":      {},
}

func Forward(w http.ResponseWriter, r *http.Request, targetURL *url.URL, opts Options) {
	if targetURL == nil {
		httpx.Error(w, 502, fmt.Errorf("proxy error: missing target URL"))
		return
	}
	if opts.AllowedURL != nil && !opts.AllowedURL(targetURL) {
		httpx.Error(w, 400, fmt.Errorf("invalid proxy target"))
		return
	}

	// The hook gets its OWN url. Handing it targetURL made the two the same
	// object, so a hook that touched req.URL edited the value AllowedURL had
	// already approved — and the caller's, which it does not own. Re-gating an
	// aliased url is also unable to see the change: the orchestrator's gate asks
	// whether the url is same-origin with targetURL, and an alias always is.
	routedURL := *targetURL

	proxyReq := r.Clone(r.Context())
	proxyReq.URL = &routedURL
	proxyReq.Host = routedURL.Host
	proxyReq.Header = r.Header.Clone()
	activity.PropagateHeaders(r.Context(), proxyReq)
	hostBeforeRewrite := proxyReq.Host
	if opts.RewriteRequest != nil {
		opts.RewriteRequest(proxyReq)
	}

	// A rewrite that moved the target has to pass the same gate the original did,
	// or the hook is a way around it. Only re-asked when the target actually
	// changed, so the common path costs nothing.
	if opts.AllowedURL != nil && proxyReq.URL.String() != targetURL.String() && !opts.AllowedURL(proxyReq.URL) {
		httpx.Error(w, 400, fmt.Errorf("invalid proxy target"))
		return
	}

	if isWebSocketUpgrade(proxyReq) {
		ProxyWebSocket(w, proxyReq, proxyReq.URL.String())
		return
	}

	client := opts.Client
	if client == nil {
		client = DefaultClient
	}

	// Built from proxyReq, not from r: RewriteRequest is handed a whole
	// *http.Request and the WebSocket path below honours the whole of it, so
	// re-deriving the method, target and body from the original request made a
	// hook that rewrote any of them work over WebSocket and be silently ignored
	// over HTTP. Nothing in the module rewrites more than headers today, so this
	// changes no traffic — it makes the hook's own signature true before someone
	// takes it at its word.
	//
	// proxyReq cannot be sent as-is: it is a server request and carries
	// RequestURI, which a client request may not set.
	outReq, err := http.NewRequestWithContext(r.Context(), proxyReq.Method, proxyReq.URL.String(), proxyReq.Body)
	if err != nil {
		httpx.Error(w, 502, fmt.Errorf("proxy error: %w", err))
		return
	}
	// Only a rewrite propagates a Host. Left alone, the transport derives the
	// Host header from the URL as before, which spells a default port the way
	// the wire expects rather than the way targetURL.Host holds it.
	if proxyReq.Host != hostBeforeRewrite {
		outReq.Host = proxyReq.Host
	}
	copyRequestHeaders(outReq.Header, proxyReq.Header)
	httpx.ForwardRequestID(outReq.Header, proxyReq.Header)

	resp, err := client.Do(outReq)
	if err != nil {
		httpx.Error(w, 502, fmt.Errorf("instance unreachable: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	httpx.CopyProxiedResponseHeaders(w.Header(), resp.Header)
	recordProxiedFailureReason(w, resp)

	// Enrich activity from response headers (always available, regardless of body size).
	enrichActivityFromHeaders(r, resp.Header)
	if opts.OnResponseHeaders != nil {
		opts.OnResponseHeaders(r, resp)
	}

	// For small JSON responses, buffer to allow OnResponse to inspect the body.
	if opts.OnResponse != nil && isSmallJSON(resp) {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
		if readErr == nil {
			opts.OnResponse(r, body)
		}
		return
	}

	w.WriteHeader(resp.StatusCode)

	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}
}

func isSmallJSON(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return false
	}
	return resp.ContentLength >= 0 && resp.ContentLength <= 64<<10
}

// HTTP forwards an HTTP request to targetURL, streaming the response
// back to w. If the request is a WebSocket upgrade, it delegates to
// ProxyWebSocket instead.
func HTTP(w http.ResponseWriter, r *http.Request, targetURL string) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		httpx.Error(w, 502, fmt.Errorf("proxy error: %w", err))
		return
	}
	if parsed.RawQuery == "" {
		parsed.RawQuery = r.URL.RawQuery
	}
	Forward(w, r, parsed, Options{})
}

// recordProxiedFailureReason carries the reason across the hop: the instance's error
// producer stamped these headers on the response it serialised, so reading them here
// keeps the reason coming from the producer — never from re-parsing the body.
// The status is deliberately NOT consulted. A multi-step run answers 200 with its
// failures in the body and publishes the reason beside it, so a status gate here
// dropped exactly that case and left the front door's counter, failures.recent, log
// level and activity record unmoved for a batch in which every step failed. The
// header being present is the whole condition, and it is a stronger one: only a
// producer that called RecordFailureReason stamps it, so ordinary 200 traffic
// records nothing and cannot be counted as a failure.
func recordProxiedFailureReason(w http.ResponseWriter, resp *http.Response) {
	code := strings.TrimSpace(resp.Header.Get(httpx.FailureCodeHeader))
	if code == "" {
		return
	}
	httpx.RecordFailureReason(w, code, resp.Header.Get(httpx.FailureMessageHeader))
}

// enrichActivityFromHeaders extracts the tab id from upstream response headers and
// enriches the activity event. It works for all response sizes, unlike body-based
// enrichment, which is limited to small JSON responses.
func enrichActivityFromHeaders(origReq *http.Request, respHeaders http.Header) {
	tabID := strings.TrimSpace(respHeaders.Get(activity.HeaderPTTabID))
	if tabID != "" {
		activity.EnrichRequest(origReq, activity.Update{TabID: tabID})
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	for _, v := range r.Header["Upgrade"] {
		if strings.EqualFold(v, "websocket") {
			return true
		}
	}
	return false
}

func copyRequestHeaders(dst, src http.Header) {
	for k, vv := range src {
		if httpx.IsHopByHopHeader(k) {
			continue
		}
		if _, skip := strippedProxyRequestHeaders[strings.ToLower(k)]; skip {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
