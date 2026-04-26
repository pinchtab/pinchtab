package handlers

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// fetchDirectWithCookies performs a Go HTTP fetch with browser cookies.
// Fallback for when Chrome navigation aborts (e.g. .gz files).
func (h *Handlers) fetchDirectWithCookies(ctx context.Context, browserCtx context.Context, dlURL string, validator *downloadURLGuard, maxBytes int) (body []byte, contentType string, statusCode int, err error) {
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

	// Inline re-validation: the caller already validated dlURL via the same
	// validator, and the guarded client re-checks IPs at dial time and on
	// redirects. Repeating it here is defence-in-depth and lets CodeQL see
	// the validation in the same function as the request sink.
	if err := validator.Validate(dlURL); err != nil {
		return nil, "", 0, err
	}
	parsed, err := url.Parse(dlURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, "", 0, fmt.Errorf("invalid download URL")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", parsed.String(), nil)
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

	client := newGuardedDownloadClient(validator, h.Config.MaxRedirects, 30)
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
