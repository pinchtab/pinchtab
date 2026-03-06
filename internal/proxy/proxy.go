// Package proxy provides a shared HTTP reverse-proxy helper used by
// strategies and the dashboard fallback routes. It consolidates the
// previously duplicated proxyHTTP / proxyRequest functions into one
// place with a shared http.Client and WebSocket upgrade support.
package proxy

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/web"
)

// DefaultClient is the shared HTTP client for proxy requests.
// A 60-second timeout accommodates lazy Chrome initialization (8-20s)
// and tab navigation (up to 60s for NavigateTimeout in bridge config).
var DefaultClient = &http.Client{Timeout: 60 * time.Second}

// HTTP forwards an HTTP request to targetURL, streaming the response
// back to w. If the request is a WebSocket upgrade, it delegates to
// handlers.ProxyWebSocket instead.
func HTTP(w http.ResponseWriter, r *http.Request, targetURL string) {
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	if isWebSocketUpgrade(r) {
		handlers.ProxyWebSocket(w, r, targetURL)
		return
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		web.Error(w, 502, fmt.Errorf("proxy error: %w", err))
		return
	}
	for k, vv := range r.Header {
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}

	resp, err := DefaultClient.Do(proxyReq)
	if err != nil {
		web.Error(w, 502, fmt.Errorf("instance unreachable: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
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

func isWebSocketUpgrade(r *http.Request) bool {
	for _, v := range r.Header["Upgrade"] {
		if strings.EqualFold(v, "websocket") {
			return true
		}
	}
	return false
}
