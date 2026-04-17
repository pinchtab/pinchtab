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
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// DefaultClient is the shared HTTP client for proxy requests.
// A 60-second timeout accommodates lazy Chrome initialization (8-20s)
// and tab navigation (up to 60s for NavigateTimeout in bridge config).
var DefaultClient = &http.Client{Timeout: 60 * time.Second}

type Options struct {
	Client         *http.Client
	AllowedURL     func(*url.URL) bool
	RewriteRequest func(*http.Request)
	// OnResponse is called with the upstream response body for non-streaming
	// responses (Content-Type application/json, body ≤ 64 KB). The original
	// request is passed so callers can enrich activity context.
	OnResponse func(origReq *http.Request, body []byte)
}

var hopByHopHeaders = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailers":            {},
	"transfer-encoding":   {},
	"upgrade":             {},
	"host":                {},
}

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

	proxyReq := r.Clone(r.Context())
	proxyReq.URL = targetURL
	proxyReq.Host = targetURL.Host
	proxyReq.Header = r.Header.Clone()
	activity.PropagateHeaders(r.Context(), proxyReq)
	if opts.RewriteRequest != nil {
		opts.RewriteRequest(proxyReq)
	}

	if isWebSocketUpgrade(proxyReq) {
		handlers.ProxyWebSocket(w, proxyReq, targetURL.String())
		return
	}

	client := opts.Client
	if client == nil {
		client = DefaultClient
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		httpx.Error(w, 502, fmt.Errorf("proxy error: %w", err))
		return
	}
	copyRequestHeaders(outReq.Header, proxyReq.Header)

	resp, err := client.Do(outReq)
	if err != nil {
		httpx.Error(w, 502, fmt.Errorf("instance unreachable: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	copyHeaders(w.Header(), resp.Header)

	// Enrich activity from response headers (always available, regardless of body size).
	enrichActivityFromHeaders(r, resp.Header)

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
// handlers.ProxyWebSocket instead.
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

// enrichActivityFromHeaders extracts tab ID from upstream response headers
// and enriches the activity event. This works for all response sizes,
// unlike body-based enrichment which is limited to small JSON responses.
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

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		if _, skip := hopByHopHeaders[strings.ToLower(k)]; skip {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func copyRequestHeaders(dst, src http.Header) {
	for k, vv := range src {
		lower := strings.ToLower(k)
		if _, skip := hopByHopHeaders[lower]; skip {
			continue
		}
		if _, skip := strippedProxyRequestHeaders[lower]; skip {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
