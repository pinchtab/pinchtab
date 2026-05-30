package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/bridge"
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
			WriteTabContextError(w, err, 404)
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
			var lookupErr error
			currentURL, lookupErr = h.Bridge.CurrentURL(lookupCtx)
			if lookupErr != nil {
				httpx.Error(w, 500, fmt.Errorf("resolve current tab url: %w", lookupErr))
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

	// Re-check scheme before navigation (validateDownloadURL already enforces this,
	// but inline check satisfies CodeQL SSRF analysis).
	if parsed, err := url.Parse(dlURL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		httpx.Error(w, 400, fmt.Errorf("invalid download URL scheme"))
		return
	}

	tCtx, tCancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	requestGuard := newDownloadRequestGuard(validator, h.Config.MaxRedirects)
	browserCtx := h.Bridge.BrowserContext()

	result, err := h.Bridge.DownloadURL(tCtx, dlURL, bridge.DownloadOpts{
		MaxBytes:     maxDownloadBytes,
		MaxRedirects: h.Config.MaxRedirects,
		ValidateURL: func(rawURL string, isRedirect bool) error {
			return requestGuard.Validate(rawURL, isRedirect)
		},
		ValidateRemoteIP: func(remoteIP string) error {
			return validateDownloadRemoteIPAddress(remoteIP)
		},
		IsDomainAllowed: func(rawURL string) bool {
			return validator.isDomainAllowed(rawURL)
		},
		ParseContentLength: func(headers map[string]interface{}) (int64, bool) {
			return parseContentLengthHeaderGeneric(headers)
		},
	})
	if err != nil {
		if bErr := requestGuard.BlockedError(); bErr != nil {
			if writeDownloadGuardError(w, bErr, maxDownloadBytes) {
				return
			}
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
			h.recordActivity(r, activity.Update{Action: "download", URL: dlURL})
			h.writeDownloadResponse(w, body, mime, dlURL, output, filePath, raw, maxDownloadBytes)
			return
		}
		if errors.Is(err, bridge.ErrTooManyRedirects) {
			httpx.Error(w, 422, fmt.Errorf("download: %w", err))
			return
		}
		if strings.Contains(err.Error(), "download response too large") {
			httpx.ErrorCode(w, http.StatusRequestEntityTooLarge, "download_too_large", err.Error(), false, map[string]any{
				"maxBytes": maxDownloadBytes,
			})
			return
		}
		if strings.Contains(err.Error(), "timed out") {
			httpx.Error(w, 504, fmt.Errorf("download timed out"))
			return
		}
		if strings.Contains(err.Error(), "HTTP") && result != nil && result.StatusCode >= 400 {
			httpx.Error(w, 502, err)
			return
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "blocked") || strings.Contains(errMsg, "private") || strings.Contains(errMsg, "unsafe") {
			httpx.Error(w, 400, fmt.Errorf("unsafe browser request: %w", err))
			return
		}
		httpx.Error(w, 502, fmt.Errorf("navigate to download URL: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "download", URL: dlURL})
	h.writeDownloadResponse(w, result.Body, result.MIMEType, dlURL, output, filePath, raw, maxDownloadBytes)
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
	if _, _, err := h.tabContext(r, tabID); err != nil {
		WriteTabContextError(w, err, 404)
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
