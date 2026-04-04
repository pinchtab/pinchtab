package handlers

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
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
	"github.com/pinchtab/pinchtab/internal/netguard"
)

var errDownloadTooLarge = errors.New("download response too large")

type downloadURLGuard struct {
	allowedDomains []string
}

func newDownloadURLGuard(allowedDomains []string) *downloadURLGuard {
	return &downloadURLGuard{allowedDomains: append([]string(nil), allowedDomains...)}
}

// isDomainAllowed reports whether rawURL's domain is on the configured
// allowlist. Allowlisted domains bypass private-IP checks because they
// are explicitly trusted by the operator (e.g. internal docker hosts).
func (g *downloadURLGuard) isDomainAllowed(rawURL string) bool {
	if len(g.allowedDomains) == 0 {
		return false
	}
	result := idpi.CheckDomain(rawURL, config.IDPIConfig{
		Enabled:        true,
		AllowedDomains: append([]string(nil), g.allowedDomains...),
		StrictMode:     true,
	})
	return !result.Blocked
}

func (g *downloadURLGuard) Validate(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("only http/https schemes are allowed")
	}

	host := netguard.NormalizeHost(parsed.Hostname())
	if host == "" || netguard.IsLocalHost(host) {
		return fmt.Errorf("internal or blocked host")
	}

	// Allowlisted domains bypass IP validation (e.g. internal docker hosts).
	if len(g.allowedDomains) > 0 {
		result := idpi.CheckDomain(rawURL, config.IDPIConfig{
			Enabled:        true,
			AllowedDomains: append([]string(nil), g.allowedDomains...),
			StrictMode:     true,
		})
		if result.Blocked {
			return fmt.Errorf("domain not allowed by security.downloadAllowedDomains")
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	if _, err := netguard.ResolveAndValidatePublicIPs(ctx, host); err != nil {
		if errors.Is(err, netguard.ErrResolveHost) {
			return fmt.Errorf("could not resolve host")
		}
		if errors.Is(err, netguard.ErrPrivateInternalIP) {
			return fmt.Errorf("private/internal IP blocked")
		}
		return fmt.Errorf("could not resolve host")
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

	normalized := netguard.NormalizeRemoteIP(raw)
	if err := netguard.ValidateRemoteIPAddress(raw); err != nil {
		if errors.Is(err, netguard.ErrUnparseableRemoteIP) {
			return fmt.Errorf("download connected to an unparseable remote IP %q", normalized)
		}
		if errors.Is(err, netguard.ErrPrivateInternalIP) {
			return fmt.Errorf("download connected to blocked remote IP %s", normalized)
		}
		return fmt.Errorf("download connected to an unparseable remote IP %q", raw)
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
	// Skip validation for Chrome internal URLs (about:blank, chrome-error://, etc.)
	// that fire during tab creation before the actual navigation begins.
	if rawURL == "about:blank" || strings.HasPrefix(rawURL, "chrome") {
		return nil
	}
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
				if !validator.isDomainAllowed(e.Response.URL) {
					if err := validateDownloadRemoteIPAddress(e.Response.RemoteIPAddress); err != nil {
						requestGuard.NoteBlocked(err)
						select {
						case done <- struct{}{}:
						default:
						}
						tCancel()
						return
					}
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
		// Chrome aborts navigation for binary downloads (.gz, etc.).
		// Fall back to a direct Go HTTP fetch using the browser's cookies.
		if isNavigationAborted(err) {
			slog.Info("download: Chrome navigation aborted, falling back to direct fetch", "url", dlURL)
			body, mime, status, fetchErr := h.fetchDirectWithCookies(tCtx, browserCtx, dlURL, maxDownloadBytes)
			if fetchErr != nil {
				// Return 400 for redirect-blocked errors (SSRF protection)
				errMsg := fetchErr.Error()
				if strings.Contains(errMsg, "blocked") || strings.Contains(errMsg, "private") {
					httpx.Error(w, 400, fmt.Errorf("unsafe browser request: %w", fetchErr))
					return
				}
				httpx.Error(w, 502, fmt.Errorf("download fallback: %w", fetchErr))
				return
			}
			if status >= 400 {
				httpx.Error(w, 502, fmt.Errorf("remote server returned HTTP %d", status))
				return
			}
			responseMIME = mime
			h.writeDownloadResponse(w, body, responseMIME, dlURL, output, filePath, raw, maxDownloadBytes)
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
	h.writeDownloadResponse(w, body, responseMIME, dlURL, output, filePath, raw, maxDownloadBytes)
}

func (h *Handlers) writeDownloadResponse(w http.ResponseWriter, body []byte, mime, dlURL, output, filePath string, raw bool, maxBytes int) {
	if len(body) > maxBytes {
		httpx.ErrorCode(w, http.StatusRequestEntityTooLarge, "download_too_large",
			downloadTooLargeError(int64(len(body)), maxBytes).Error(), false, map[string]any{
				"maxBytes": maxBytes,
			})
		return
	}

	if mime == "" {
		mime = "application/octet-stream"
	}

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
			"contentType": mime,
		})
		return
	}

	if raw {
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(200)
		_, _ = w.Write(body)
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"data":        base64.StdEncoding.EncodeToString(body),
		"contentType": mime,
		"size":        len(body),
		"url":         dlURL,
	})
}

// fetchDirectWithCookies performs a Go HTTP fetch with browser cookies.
// Fallback for when Chrome navigation aborts (e.g. .gz files).
func (h *Handlers) fetchDirectWithCookies(ctx context.Context, browserCtx context.Context, dlURL string, maxBytes int) (body []byte, contentType string, statusCode int, err error) {
	var browserCookies []*network.Cookie
	if fetchErr := chromedp.Run(browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			browserCookies, err = network.GetCookies().WithURLs([]string{dlURL}).Do(ctx)
			return err
		}),
	); fetchErr != nil {
		slog.Debug("download fallback: failed to get browser cookies", "err", fetchErr)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", dlURL, nil)
	if err != nil {
		return nil, "", 0, err
	}
	if h.Config.UserAgent != "" {
		req.Header.Set("User-Agent", h.Config.UserAgent)
	}
	req.Header.Set("Accept", "*/*")

	for _, c := range browserCookies {
		req.AddCookie(&http.Cookie{Name: c.Name, Value: c.Value})
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			host := req.URL.Hostname()
			if netguard.IsLocalHost(host) {
				return fmt.Errorf("redirect to local network blocked: %s", host)
			}
			if _, err := netguard.ResolveAndValidatePublicIPs(req.Context(), host); err != nil {
				return fmt.Errorf("redirect to private network blocked: %s", host)
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if contentLength, parseErr := strconv.ParseInt(cl, 10, 64); parseErr == nil {
			if contentLength > int64(maxBytes) {
				return nil, "", 0, errDownloadTooLarge
			}
		}
	}

	reader := io.Reader(resp.Body)
	isGzip := isGzipContent(resp.Header.Get("Content-Type"), dlURL) &&
		!strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip")

	if isGzip {
		gz, gzErr := gzip.NewReader(resp.Body)
		if gzErr != nil {
			return nil, "", 0, fmt.Errorf("gzip decompress: %w", gzErr)
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}

	// LimitReader on decompressed stream protects against gzip bombs.
	data, err := io.ReadAll(io.LimitReader(reader, int64(maxBytes)+1))
	if err != nil {
		return nil, "", 0, err
	}
	if len(data) > maxBytes {
		return nil, "", 0, errDownloadTooLarge
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	// If we decompressed gzip, report the inner content type
	if isGzip {
		ct = inferDecompressedContentType(dlURL, ct)
	}

	return data, ct, resp.StatusCode, nil
}

func isGzipContent(contentType, rawURL string) bool {
	if strings.Contains(strings.ToLower(contentType), "gzip") {
		return true
	}
	// Use path.Ext for precise extension matching (.gz, not .pgz or .ngz)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(path.Ext(parsed.Path), ".gz")
}

func inferDecompressedContentType(rawURL, fallback string) string {
	lower := strings.ToLower(rawURL)
	// Strip .gz to infer inner type
	if strings.HasSuffix(lower, ".xml.gz") {
		return "application/xml"
	}
	if strings.HasSuffix(lower, ".json.gz") {
		return "application/json"
	}
	if strings.HasSuffix(lower, ".txt.gz") || strings.HasSuffix(lower, ".csv.gz") {
		return "text/plain"
	}
	return "application/octet-stream"
}

func isNavigationAborted(err error) bool {
	return err != nil && strings.Contains(err.Error(), "net::ERR_ABORTED")
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
