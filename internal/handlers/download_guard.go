package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/network"
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

func (g *downloadURLGuard) isHostAllowed(host string) bool {
	if len(g.allowedDomains) == 0 {
		return false
	}
	host = netguard.NormalizeHost(host)
	if host == "" {
		return false
	}
	return g.isDomainAllowed("https://" + host)
}

// isDomainAllowed reports whether rawURL's domain is on the configured
// allowlist. Allowlisted domains bypass private-IP checks because they
// are explicitly trusted by the operator (e.g. internal docker hosts).
func (g *downloadURLGuard) isDomainAllowed(rawURL string) bool {
	if len(g.allowedDomains) == 0 {
		return false
	}
	result := idpi.CheckDomain(rawURL, config.IDPIConfig{
		Enabled:    true,
		StrictMode: true,
	}, g.allowedDomains)
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

	if len(g.allowedDomains) > 0 {
		result := idpi.CheckDomain(rawURL, config.IDPIConfig{
			Enabled:    true,
			StrictMode: true,
		}, g.allowedDomains)
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
