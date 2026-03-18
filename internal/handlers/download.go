package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"sync/atomic"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/idpi"
)

var resolveDownloadHostIPs = func(ctx context.Context, network, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, network, host)
}

var errDownloadTooLarge = errors.New("download response too large")

var blockedDownloadPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), // Carrier-grade NAT/shared address space.
	netip.MustParsePrefix("198.18.0.0/15"), // Benchmark/testing networks.
}

type downloadURLGuard struct {
	allowedDomains []string
}

func newDownloadURLGuard(allowedDomains []string) *downloadURLGuard {
	return &downloadURLGuard{allowedDomains: append([]string(nil), allowedDomains...)}
}

func (g *downloadURLGuard) Validate(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("only http/https schemes are allowed")
	}

	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("internal or blocked host")
	}

	if ip := net.ParseIP(host); ip != nil {
		if err := validateDownloadIP(ip); err != nil {
			return err
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		ips, err := resolveDownloadHostIPs(ctx, "ip", host)
		if err != nil {
			return fmt.Errorf("could not resolve host")
		}
		for _, ip := range ips {
			if err := validateDownloadIP(ip); err != nil {
				return err
			}
		}
	}

	if result := idpi.CheckDomain(rawURL, config.IDPIConfig{
		Enabled:        len(g.allowedDomains) > 0,
		AllowedDomains: append([]string(nil), g.allowedDomains...),
		StrictMode:     true,
	}); result.Blocked {
		return fmt.Errorf("domain not allowed by security.downloadAllowedDomains")
	}
	return nil
}

func validateDownloadIP(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("private/internal IP blocked")
	}

	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return fmt.Errorf("private/internal IP blocked")
	}
	addr = addr.Unmap()
	if addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return fmt.Errorf("private/internal IP blocked")
	}
	for _, prefix := range blockedDownloadPrefixes {
		if prefix.Contains(addr) {
			return fmt.Errorf("private/internal IP blocked")
		}
	}
	return nil
}

func validateDownloadRemoteIPAddress(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		// Best-effort mitigation: some responses may not expose a remote IP
		// (for example, cached responses). Skip the post-connect check then.
		return nil
	}

	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")

	ip := net.ParseIP(raw)
	if ip == nil {
		return fmt.Errorf("download connected to an unparseable remote IP %q", raw)
	}
	if err := validateDownloadIP(ip); err != nil {
		return fmt.Errorf("download connected to blocked remote IP %s", raw)
	}
	return nil
}

// validateDownloadURL blocks file://, internal hosts, private IPs, and cloud metadata.
// Only public http/https URLs are allowed.
func validateDownloadURL(rawURL string) error {
	return newDownloadURLGuard(nil).Validate(rawURL)
}

func validateTabScopedDownloadURL(currentURL, requestedURL string) error {
	currentParsed, err := url.Parse(strings.TrimSpace(currentURL))
	if err != nil || currentParsed.Scheme == "" || currentParsed.Host == "" {
		return fmt.Errorf("tab-scoped downloads require an active http(s) page")
	}
	if currentParsed.Scheme != "http" && currentParsed.Scheme != "https" {
		return fmt.Errorf("tab-scoped downloads require an active http(s) page")
	}

	requestedParsed, err := url.Parse(strings.TrimSpace(requestedURL))
	if err != nil || requestedParsed.Scheme == "" || requestedParsed.Host == "" {
		return fmt.Errorf("invalid download URL")
	}

	if strings.EqualFold(currentParsed.Scheme, requestedParsed.Scheme) &&
		strings.EqualFold(currentParsed.Host, requestedParsed.Host) {
		return nil
	}

	return fmt.Errorf("tab-scoped downloads are limited to the current page origin")
}

type downloadRequestGuard struct {
	validator    *downloadURLGuard
	maxRedirects int
	redirects    atomic.Int32

	mu         sync.Mutex
	blockedErr error
}

func newDownloadRequestGuard(validator *downloadURLGuard, maxRedirects int) *downloadRequestGuard {
	return &downloadRequestGuard{
		validator:    validator,
		maxRedirects: maxRedirects,
	}
}

func (g *downloadRequestGuard) Validate(rawURL string, redirected bool) error {
	if err := g.validator.Validate(rawURL); err != nil {
		return fmt.Errorf("unsafe browser request: %w", err)
	}
	if redirected && g.maxRedirects >= 0 {
		count := int(g.redirects.Add(1))
		if count > g.maxRedirects {
			return fmt.Errorf("%w: got %d, max %d", bridge.ErrTooManyRedirects, count, g.maxRedirects)
		}
	}
	return nil
}

func (g *downloadRequestGuard) NoteBlocked(err error) {
	g.mu.Lock()
	if g.blockedErr == nil {
		g.blockedErr = err
	}
	g.mu.Unlock()
}

func (g *downloadRequestGuard) BlockedError() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.blockedErr
}

func downloadTooLargeError(size int64, maxBytes int) error {
	return fmt.Errorf("%w: received %d bytes, max %d", errDownloadTooLarge, size, maxBytes)
}

func parseContentLengthHeader(headers network.Headers) (int64, bool) {
	for key, raw := range headers {
		if !strings.EqualFold(strings.TrimSpace(key), "Content-Length") {
			continue
		}
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value == "" {
			return 0, false
		}
		size, err := strconv.ParseInt(value, 10, 64)
		if err != nil || size < 0 {
			return 0, false
		}
		return size, true
	}
	return 0, false
}

func writeDownloadGuardError(w http.ResponseWriter, err error, maxBytes int) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, bridge.ErrTooManyRedirects):
		httpx.Error(w, 422, fmt.Errorf("download: %w", err))
	case errors.Is(err, errDownloadTooLarge):
		httpx.ErrorCode(w, http.StatusRequestEntityTooLarge, "download_too_large", err.Error(), false, map[string]any{
			"maxBytes": maxBytes,
		})
	default:
		httpx.Error(w, 400, err)
	}
	return true
}

// HandleDownload fetches a URL using the browser's session (cookies, stealth)
// and returns the content. This preserves authentication and fingerprint.
//
// GET /download?url=<url>[&tabId=<id>][&output=file&path=/tmp/file][&raw=true]
func (h *Handlers) HandleDownload(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowDownload {
		httpx.ErrorCode(w, 403, "download_disabled", httpx.DisabledEndpointMessage("download", "security.allowDownload"), false, map[string]any{
			"setting": "security.allowDownload",
		})
		return
	}
	dlURL := r.URL.Query().Get("url")
	if dlURL == "" {
		httpx.Error(w, 400, fmt.Errorf("url parameter required"))
		return
	}
	maxDownloadBytes := h.Config.EffectiveDownloadMaxBytes()

	validator := newDownloadURLGuard(h.Config.DownloadAllowedDomains)
	if err := validator.Validate(dlURL); err != nil {
		httpx.Error(w, 400, fmt.Errorf("unsafe URL: %w", err))
		return
	}

	tabID := strings.TrimSpace(r.URL.Query().Get("tabId"))
	if tabID != "" {
		ctx, resolvedTabID, err := h.tabContext(r, tabID)
		if err != nil {
			httpx.Error(w, 404, err)
			return
		}
		owner := resolveOwner(r, "")
		if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
			httpx.ErrorCode(w, http.StatusLocked, "tab_locked", err.Error(), false, nil)
			return
		}
		currentURL, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID)
		if !ok {
			return
		}
		if currentURL == "" {
			if provider, ok := h.Bridge.(tabPolicyStateProvider); ok {
				if state, ok := provider.GetTabPolicyState(resolvedTabID); ok && state.CurrentURL != "" {
					currentURL = state.CurrentURL
				}
			}
		}
		if currentURL == "" {
			lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := chromedp.Run(lookupCtx, chromedp.Location(&currentURL)); err != nil {
				httpx.Error(w, 500, fmt.Errorf("resolve current tab url: %w", err))
				return
			}
		}
		if authn.CredentialsFromRequest(r).Method == authn.MethodCookie {
			if err := validateTabScopedDownloadURL(currentURL, dlURL); err != nil {
				httpx.ErrorCode(w, http.StatusForbidden, "download_scope_forbidden", err.Error(), false, map[string]any{
					"currentURL":   currentURL,
					"requestedURL": dlURL,
				})
				return
			}
		}
	}

	output := r.URL.Query().Get("output")
	filePath := r.URL.Query().Get("path")
	raw := r.URL.Query().Get("raw") == "true"

	// Create a temporary tab for the download — avoids navigating the user's tab away.
	browserCtx := h.Bridge.BrowserContext()
	tabCtx, tabCancel := chromedp.NewContext(browserCtx)
	defer tabCancel()

	tCtx, tCancel := context.WithTimeout(tabCtx, 30*time.Second)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	// Enable network tracking to capture response metadata.
	var requestID network.RequestID
	var responseMIME string
	var responseStatus int
	requestGuard := newDownloadRequestGuard(validator, h.Config.MaxRedirects)
	var mainFrameID cdp.FrameID
	done := make(chan struct{}, 1)
	var receivedBytes atomic.Int64

	// Intercept every browser-side request so redirects and follow-on navigations
	// cannot escape the public-only URL policy enforced for /download.
	if err := chromedp.Run(tCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().Do(ctx)
	})); err != nil {
		httpx.Error(w, 500, fmt.Errorf("fetch enable: %w", err))
		return
	}
	defer func() {
		_ = chromedp.Run(tCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			return fetch.Disable().Do(ctx)
		}))
	}()

	chromedp.ListenTarget(tCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventRequestPaused:
			// Handle in goroutine to avoid deadlocking the event dispatcher.
			go func() {
				reqID := e.RequestID
				if err := requestGuard.Validate(e.Request.URL, e.RedirectedRequestID != ""); err != nil {
					requestGuard.NoteBlocked(err)
					select {
					case done <- struct{}{}:
					default:
					}
					_ = fetch.FailRequest(reqID, network.ErrorReasonBlockedByClient).Do(cdp.WithExecutor(tCtx, chromedp.FromContext(tCtx).Target))
					return
				}
				_ = fetch.ContinueRequest(reqID).Do(cdp.WithExecutor(tCtx, chromedp.FromContext(tCtx).Target))
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
				if err := validateDownloadRemoteIPAddress(e.Response.RemoteIPAddress); err != nil {
					requestGuard.NoteBlocked(err)
					select {
					case done <- struct{}{}:
					default:
					}
					tCancel()
					return
				}
				if contentLength, ok := parseContentLengthHeader(e.Response.Headers); ok && contentLength > int64(maxDownloadBytes) {
					requestGuard.NoteBlocked(downloadTooLargeError(contentLength, maxDownloadBytes))
					select {
					case done <- struct{}{}:
					default:
					}
					tCancel()
					return
				}
			}
		case *network.EventDataReceived:
			if e.RequestID == requestID && requestID != "" {
				chunk := e.EncodedDataLength
				if chunk <= 0 {
					chunk = e.DataLength
				}
				if chunk > 0 && receivedBytes.Add(chunk) > int64(maxDownloadBytes) {
					requestGuard.NoteBlocked(downloadTooLargeError(receivedBytes.Load(), maxDownloadBytes))
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

	if err := chromedp.Run(tCtx, network.Enable()); err != nil {
		httpx.Error(w, 500, fmt.Errorf("network enable: %w", err))
		return
	}

	// Re-check scheme before navigation (validateDownloadURL already enforces this,
	// but inline check satisfies CodeQL SSRF analysis).
	if parsed, err := url.Parse(dlURL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		httpx.Error(w, 400, fmt.Errorf("invalid download URL scheme"))
		return
	}

	// Navigate the temp tab to the URL — uses browser's cookie jar and stealth.
	if err := chromedp.Run(tCtx, chromedp.Navigate(dlURL)); err != nil {
		if writeDownloadGuardError(w, requestGuard.BlockedError(), maxDownloadBytes) {
			return
		}
		httpx.Error(w, 502, fmt.Errorf("navigate to download URL: %w", err))
		return
	}

	// Wait for response.
	select {
	case <-done:
	case <-tCtx.Done():
		if writeDownloadGuardError(w, requestGuard.BlockedError(), maxDownloadBytes) {
			return
		}
		httpx.Error(w, 504, fmt.Errorf("download timed out"))
		return
	}

	if writeDownloadGuardError(w, requestGuard.BlockedError(), maxDownloadBytes) {
		return
	}

	if responseStatus >= 400 {
		httpx.Error(w, 502, fmt.Errorf("remote server returned HTTP %d", responseStatus))
		return
	}
	if requestID == "" {
		httpx.Error(w, 502, fmt.Errorf("download response was not captured"))
		return
	}

	// Get response body via CDP.
	var body []byte
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			b, err := network.GetResponseBody(requestID).Do(ctx)
			if err != nil {
				return err
			}
			body = b
			return nil
		}),
	); err != nil {
		httpx.Error(w, 500, fmt.Errorf("get response body: %w", err))
		return
	}
	if len(body) > maxDownloadBytes {
		httpx.ErrorCode(w, http.StatusRequestEntityTooLarge, "download_too_large",
			downloadTooLargeError(int64(len(body)), maxDownloadBytes).Error(), false, map[string]any{
				"maxBytes": maxDownloadBytes,
			})
		return
	}

	if responseMIME == "" {
		responseMIME = "application/octet-stream"
	}

	// Write to file.
	if output == "file" {
		if filePath == "" {
			httpx.Error(w, 400, fmt.Errorf("path required when output=file"))
			return
		}
		safe, pathErr := httpx.SafeCreatePath(h.Config.StateDir, filePath)
		if pathErr != nil {
			httpx.Error(w, 400, fmt.Errorf("invalid path: %w", pathErr))
			return
		}
		absBase, _ := filepath.Abs(h.Config.StateDir)
		absPath, pathErr := filepath.Abs(safe)
		if pathErr != nil || !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
			httpx.Error(w, 400, fmt.Errorf("invalid output path"))
			return
		}
		filePath = absPath
		if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
			httpx.Error(w, 500, fmt.Errorf("failed to create directory: %w", err))
			return
		}
		if err := os.WriteFile(filePath, body, 0600); err != nil {
			httpx.Error(w, 500, fmt.Errorf("failed to write file: %w", err))
			return
		}
		httpx.JSON(w, 200, map[string]any{
			"status":      "saved",
			"path":        filePath,
			"size":        len(body),
			"contentType": responseMIME,
		})
		return
	}

	// Raw bytes.
	if raw {
		w.Header().Set("Content-Type", responseMIME)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(200)
		_, _ = w.Write(body)
		return
	}

	// Default: base64 JSON response.
	httpx.JSON(w, 200, map[string]any{
		"data":        base64.StdEncoding.EncodeToString(body),
		"contentType": responseMIME,
		"size":        len(body),
		"url":         dlURL,
	})
}

// HandleTabDownload fetches a URL using the browser session for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/download
func (h *Handlers) HandleTabDownload(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}
	if _, _, err := h.Bridge.TabContext(tabID); err != nil {
		httpx.Error(w, 404, err)
		return
	}

	q := r.URL.Query()
	q.Set("tabId", tabID)

	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u

	h.HandleDownload(w, req)
}
