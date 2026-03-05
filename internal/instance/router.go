package instance

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
)

// Router proxies HTTP requests to the bridge instance that owns the target tab.
// Uses the Locator for tab→instance resolution.
type Router struct {
	locator *Locator
	client  *http.Client
}

// NewRouter creates a Router backed by the given Locator.
func NewRouter(locator *Locator, client *http.Client) *Router {
	return &Router{locator: locator, client: client}
}

// ProxyTabRequest proxies an HTTP request to the bridge that owns the given tab.
// The original request path is preserved.
func (rt *Router) ProxyTabRequest(w http.ResponseWriter, r *http.Request, tabID string) {
	inst, err := rt.locator.FindInstanceByTabID(tabID)
	if err != nil {
		http.Error(w, fmt.Sprintf("tab not found: %s", err), http.StatusNotFound)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	rt.proxyTo(w, r, targetURL)
}

// ProxyToInstance proxies an HTTP request to a specific instance.
// The path is rewritten to strip the /instances/{id} prefix.
func (rt *Router) ProxyToInstance(w http.ResponseWriter, r *http.Request, inst *InstanceRef) {
	targetPath := r.URL.Path
	prefix := "/instances/" + inst.ID
	if len(targetPath) > len(prefix) {
		targetPath = targetPath[len(prefix):]
	} else {
		targetPath = ""
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort("localhost", inst.Port),
		Path:     targetPath,
		RawQuery: r.URL.RawQuery,
	}

	rt.proxyTo(w, r, targetURL)
}

// InstanceRef is a lightweight reference for proxying (avoids importing bridge.Instance).
type InstanceRef struct {
	ID   string
	Port string
}

func (rt *Router) proxyTo(w http.ResponseWriter, r *http.Request, targetURL *url.URL) {
	if targetURL.Hostname() != "localhost" {
		http.Error(w, "invalid proxy target: only localhost allowed", http.StatusBadRequest)
		return
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create proxy request: %s", err), http.StatusInternalServerError)
		return
	}

	// Copy headers, excluding hop-by-hop headers.
	for key, values := range r.Header {
		switch key {
		case "Host", "Connection", "Keep-Alive", "Proxy-Authenticate",
			"Proxy-Authorization", "Te", "Trailers", "Transfer-Encoding", "Upgrade":
		default:
			for _, v := range values {
				proxyReq.Header.Add(key, v)
			}
		}
	}

	resp, err := rt.client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy failed: %s", err), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
