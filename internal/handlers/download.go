package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

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

	// Download allowlist: prefer the explicit per-feature list, but fall
	// back to the unified security.allowedDomains. This way operators only
	// need to configure trusted domains once.
	allowed := h.Config.DownloadAllowedDomains
	if len(allowed) == 0 {
		allowed = h.Config.AllowedDomains
	}
	validator := newDownloadURLGuard(allowed)
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
			body, mime, status, fetchErr := h.fetchDirectWithCookies(tCtx, browserCtx, dlURL, validator, maxDownloadBytes)
			if fetchErr != nil {
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
			h.recordActivity(r, activity.Update{Action: "download", URL: dlURL})
			h.writeDownloadResponse(w, body, responseMIME, dlURL, output, filePath, raw, maxDownloadBytes)
			return
		}
		httpx.Error(w, 502, fmt.Errorf("navigate to download URL: %w", err))
		return
	}

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
	h.recordActivity(r, activity.Update{Action: "download", URL: dlURL})
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
