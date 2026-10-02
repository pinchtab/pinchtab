package observe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/sanitize"
)

const defaultRetainBodyMaxBytesPerTab = 4 << 20
const defaultRetainBodyConcurrency = 4

const (
	maxNetworkURLBytes          = 8 * 1024
	maxNetworkMethodBytes       = 32
	maxNetworkResourceTypeBytes = 64
	maxNetworkStatusTextBytes   = 512
	maxNetworkMimeTypeBytes     = 512
	maxNetworkErrorBytes        = 2 * 1024
	maxNetworkPostDataBytes     = 64 * 1024
	maxNetworkHeaderKeyBytes    = 256
	maxNetworkHeaderValueBytes  = 4 * 1024
	maxNetworkHeaderTotalBytes  = 32 * 1024
)

type NetworkMonitor struct {
	mu                  sync.RWMutex
	buffers             map[string]*NetworkBuffer
	listeners           map[string]context.CancelFunc
	bufSize             int
	retainBodies        bool
	retainBodyMaxBytes  int
	retainBodyMaxPerTab int64
	retainBodySemaphore chan struct{}
}

func NewNetworkMonitor(bufferSize int) *NetworkMonitor {
	bufferSize = config.ClampNetworkBufferSize(bufferSize)
	if bufferSize <= 0 {
		bufferSize = DefaultNetworkBufferSize
	}
	return &NetworkMonitor{
		buffers:             make(map[string]*NetworkBuffer),
		listeners:           make(map[string]context.CancelFunc),
		bufSize:             bufferSize,
		retainBodies:        false,
		retainBodyMaxBytes:  0,
		retainBodyMaxPerTab: defaultRetainBodyMaxBytesPerTab,
		retainBodySemaphore: make(chan struct{}, defaultRetainBodyConcurrency),
	}
}

func (nm *NetworkMonitor) ConfigureBodyRetention(enabled bool, maxBytes int) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.retainBodies = enabled
	if maxBytes < 0 {
		maxBytes = 0
	}
	nm.retainBodyMaxBytes = maxBytes
}

func (nm *NetworkMonitor) getOrCreateBuffer(tabID string) *NetworkBuffer {
	return nm.getOrCreateBufferWithSize(tabID, 0)
}

func (nm *NetworkMonitor) getOrCreateBufferWithSize(tabID string, size int) *NetworkBuffer {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	buf, ok := nm.buffers[tabID]
	if !ok {
		if size <= 0 {
			size = nm.bufSize
		}
		buf = NewNetworkBuffer(size)
		nm.buffers[tabID] = buf
	}
	return buf
}

func (nm *NetworkMonitor) GetOrCreateBufferForTest(tabID string) *NetworkBuffer {
	return nm.getOrCreateBuffer(tabID)
}

func (nm *NetworkMonitor) GetOrCreateBufferWithSizeForTest(tabID string, size int) *NetworkBuffer {
	return nm.getOrCreateBufferWithSize(tabID, size)
}

func (nm *NetworkMonitor) GetBuffer(tabID string) *NetworkBuffer {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.buffers[tabID]
}

func (nm *NetworkMonitor) BufferSizeForTest() int {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.bufSize
}

func (nm *NetworkMonitor) StartCapture(tabCtx context.Context, tabID string) error {
	return nm.StartCaptureWithSize(tabCtx, tabID, 0)
}

func (nm *NetworkMonitor) StartCaptureWithSize(tabCtx context.Context, tabID string, bufferSize int) error {
	buf := nm.getOrCreateBufferWithSize(tabID, bufferSize)

	listenerCtx, _, alreadyActive := nm.reserveCaptureListener(tabID, tabCtx)
	if alreadyActive {
		// Capture is already running for this tab (the buffer exists above). Do
		// NOT stack another ListenTarget callback — that would double-record events.
		return nil
	}

	if err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.Enable().Do(ctx)
	})); err != nil {
		nm.releaseCaptureListener(tabID)
		return fmt.Errorf("network enable: %w", err)
	}

	chromedp.ListenTarget(listenerCtx, func(ev any) {
		nm.recordEvent(tabCtx, buf, ev)
	})

	// Self-heal the listeners map when the listener ends (StopCapture cancel or
	// tab close via tabCtx), so a later capture for a reused tabID re-registers.
	go func() {
		<-listenerCtx.Done()
		nm.mu.Lock()
		delete(nm.listeners, tabID)
		nm.mu.Unlock()
	}()

	slog.Debug("network capture started", "tabId", tabID)
	return nil
}

func (nm *NetworkMonitor) recordEvent(tabCtx context.Context, buf *NetworkBuffer, ev any) {
	switch e := ev.(type) {
	case *network.EventRequestWillBeSent:
		buf.Add(requestEntryFromEvent(e))
		buf.MarkRequestStart(string(e.RequestID), string(e.LoaderID))

	case *network.EventResponseReceived:
		buf.Update(string(e.RequestID), func(entry *NetworkEntry) {
			entry.Status = int(e.Response.Status)
			entry.StatusText = e.Response.StatusText
			entry.MimeType = e.Response.MimeType
			if e.Response.Headers != nil {
				respHeaders := make(map[string]string)
				for k, v := range e.Response.Headers {
					if s, ok := v.(string); ok {
						respHeaders[k] = s
					}
				}
				entry.ResponseHeaders = respHeaders
			}
			if e.Response.EncodedDataLength > 0 {
				entry.Size = int64(e.Response.EncodedDataLength)
			}
		})

	case *network.EventLoadingFinished:
		buf.Update(string(e.RequestID), func(entry *NetworkEntry) {
			markEntryEnded(entry)
			if e.EncodedDataLength > 0 {
				entry.Size = int64(e.EncodedDataLength)
			}
			if nm.bodyRetentionEnabled() {
				entry.BodyPending = true
				entry.BodySkipped = false
				entry.BodySkipReason = ""
				entry.BodyError = ""
			}
		})
		buf.MarkRequestEnd(string(e.RequestID))
		go nm.maybeRetainBody(tabCtx, buf, string(e.RequestID))

	case *network.EventLoadingFailed:
		buf.Update(string(e.RequestID), func(entry *NetworkEntry) {
			markEntryFailed(entry, e.ErrorText)
		})
		buf.MarkRequestEnd(string(e.RequestID))

	case *page.EventFrameNavigated:
		if e.Frame != nil && e.Frame.ParentID == "" {
			buf.AbandonRequestsOutsideDocument(string(e.Frame.LoaderID))
		}
	}
}

// reserveCaptureListener reserves the per-tab listener slot. If capture is
// already active for tabID it returns alreadyActive=true (with a nil cancel);
// otherwise it stores a fresh cancel derived from tabCtx and returns it.
func (nm *NetworkMonitor) reserveCaptureListener(tabID string, tabCtx context.Context) (context.Context, context.CancelFunc, bool) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	if _, exists := nm.listeners[tabID]; exists {
		return nil, nil, true
	}
	listenerCtx, cancel := context.WithCancel(tabCtx)
	nm.listeners[tabID] = cancel
	return listenerCtx, cancel, false
}

// releaseCaptureListener cancels and removes the per-tab capture listener, if any.
func (nm *NetworkMonitor) releaseCaptureListener(tabID string) {
	nm.mu.Lock()
	cancel, ok := nm.listeners[tabID]
	delete(nm.listeners, tabID)
	nm.mu.Unlock()
	if ok {
		cancel()
	}
}

func (nm *NetworkMonitor) StopCapture(tabID string) {
	nm.releaseCaptureListener(tabID)
	nm.mu.Lock()
	delete(nm.buffers, tabID)
	nm.mu.Unlock()
}

func (nm *NetworkMonitor) ClearTab(tabID string) {
	nm.mu.RLock()
	buf := nm.buffers[tabID]
	nm.mu.RUnlock()
	if buf != nil {
		buf.Clear()
	}
}

func (nm *NetworkMonitor) ClearAll() {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	for _, buf := range nm.buffers {
		buf.Clear()
	}
}

func (nm *NetworkMonitor) bodyRetentionEnabled() bool {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.retainBodies
}

// fetchResponseBody reads the body from the browser. Indirected so retention
// policy can be exercised without a live CDP target.
var fetchResponseBody = GetResponseBody

func skipRetainedBody(buf *NetworkBuffer, requestID, reason string) {
	buf.Update(requestID, func(entry *NetworkEntry) {
		entry.BodyPending = false
		entry.BodySkipped = true
		entry.BodySkipReason = reason
		entry.BodyError = ""
	})
}

// Operator-facing names of the two budgets a retained body is cut against, used
// to build the skip reason so a dropped body says which one dropped it.
const (
	retentionLimitScope  = "retention limit"
	retentionBudgetScope = "retention budget"
	postDataLimitScope   = "request body limit"
)

// clampRetainedBody applies a byte budget to a retained payload — a response body,
// or the request body in PostData. One rule: a retained payload is a byte-exact
// prefix of what crossed the wire, or there is no retained payload. Text is cut on a rune boundary with the suffix-free helper — the display
// variant appends "..." inside the budget, and a machine-read body must not carry
// characters the payload never contained when the truncated flag already says it
// was cut. A base64 body cannot be cut at all (the encoding's length and padding make
// a fragment undecodable in whole, not only at the tail), and a budget smaller
// than the body's first character leaves no whole rune to keep; both are dropped
// with a reason rather than returned corrupt or returned empty-but-retained.
func clampRetainedBody(body string, base64Encoded bool, limit int, scope string) (clamped string, truncated bool, dropReason string) {
	if limit <= 0 || len(body) <= limit {
		return body, false, ""
	}
	if base64Encoded {
		return "", false, "base64 body exceeds " + scope
	}
	prefix := sanitize.TruncateUTF8BytesExact(body, limit)
	if prefix == "" {
		return "", false, scope + " is smaller than the body's first character"
	}
	return prefix, true, ""
}

func (nm *NetworkMonitor) maybeRetainBody(tabCtx context.Context, buf *NetworkBuffer, requestID string) {
	// Every return path below resolves BodyPending via buf.Update; signal once on
	// exit so retained-body readers wake instead of polling.
	defer buf.SignalBodyChange()

	nm.mu.RLock()
	enabled := nm.retainBodies
	maxBytes := nm.retainBodyMaxBytes
	nm.mu.RUnlock()
	if !enabled {
		skipRetainedBody(buf, requestID, "retention disabled")
		return
	}
	if buf.RetainedBytes() >= nm.retainBodyMaxPerTab {
		skipRetainedBody(buf, requestID, "retention budget exceeded")
		return
	}
	select {
	case nm.retainBodySemaphore <- struct{}{}:
		defer func() { <-nm.retainBodySemaphore }()
	default:
		skipRetainedBody(buf, requestID, "retention concurrency limit reached")
		return
	}
	body, base64Encoded, err := fetchResponseBody(tabCtx, requestID)
	if err != nil {
		buf.Update(requestID, func(entry *NetworkEntry) {
			entry.BodyPending = false
			entry.BodySkipped = false
			entry.BodySkipReason = ""
			entry.BodyError = err.Error()
		})
		return
	}
	body, truncated, dropReason := clampRetainedBody(body, base64Encoded, maxBytes, retentionLimitScope)
	if dropReason != "" {
		skipRetainedBody(buf, requestID, dropReason)
		return
	}
	remainingBudget := int(nm.retainBodyMaxPerTab - buf.RetainedBytes())
	if remainingBudget <= 0 {
		skipRetainedBody(buf, requestID, "retention budget exceeded")
		return
	}
	body, cutForBudget, dropReason := clampRetainedBody(body, base64Encoded, remainingBudget, retentionBudgetScope)
	if dropReason != "" {
		skipRetainedBody(buf, requestID, dropReason)
		return
	}
	truncated = truncated || cutForBudget
	buf.Update(requestID, func(entry *NetworkEntry) {
		entry.ResponseBody = body
		entry.Base64Encoded = base64Encoded
		entry.BodyRetained = true
		entry.BodyPending = false
		entry.BodySkipped = false
		entry.BodySkipReason = ""
		entry.BodyTruncated = truncated
		entry.BodyError = ""
	})
}

func (nm *NetworkMonitor) IsTabIdle(tabID string) (bool, bool) {
	nm.mu.RLock()
	buf, ok := nm.buffers[tabID]
	nm.mu.RUnlock()
	if !ok || buf == nil {
		return false, false
	}
	count, _ := buf.InflightStatus()
	return count == 0, true
}

// GetResponseBody is the only response-body fetcher. It reads Network.getResponseBody
// as raw JSON so the body and the base64Encoded flag describing it come from the same
// call and travel together.
//
// It deliberately does NOT use cdproto's typed constructor plus Do: that helper
// base64-decodes the payload inside the dependency and returns []byte, so by the time
// it returns the flag is gone and the string holds raw bytes. A second fetcher built
// that way reported base64Encoded=false for every response, which meant a binary body
// was retained as a string of U+FFFD once JSON-encoded — and made clampRetainedBody's
// drop-and-mark branch unreachable in production, since nothing ever set the flag it
// tests.
func GetResponseBody(ctx context.Context, requestID string) (string, bool, error) {
	var body string
	var base64Encoded bool

	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		executor := chromedp.FromContext(ctx).Target
		if executor == nil {
			return fmt.Errorf("no CDP executor available")
		}
		var result json.RawMessage
		if err := executor.Execute(ctx, "Network.getResponseBody", map[string]any{
			"requestId": requestID,
		}, &result); err != nil {
			return err
		}
		var resp struct {
			Body          string `json:"body"`
			Base64Encoded bool   `json:"base64Encoded"`
		}
		if err := json.Unmarshal(result, &resp); err != nil {
			return err
		}
		body = resp.Body
		base64Encoded = resp.Base64Encoded
		return nil
	}))

	return body, base64Encoded, err
}

// decodePostData joins a request's body entries into the bytes the page sent. CDP declares
// PostDataEntry.Bytes as a protocol binary field, so each entry arrives base64-encoded, and
// joining the entries as strings is wrong twice over: the published value is an encoded blob
// in a field named after the request body, and base64(a)+base64(b) is not base64(a+b) once an
// entry's length is not a multiple of three, so any multi-entry body arrives corrupt.
//
// Anything that cannot be published as the text it claims to be publishes nothing, and says
// why: an entry that is not base64, or a body that is not valid UTF-8 once decoded, such as a
// file part in a multipart POST. The alternative is mojibake, or a blob in a field with no
// encoding signal, or — worse — an empty string that reads as a request sent with no body.
// The second return is that reason, empty when there is nothing to explain.
func decodePostData(entries []*network.PostDataEntry) (string, string) {
	var decoded []byte
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		chunk, err := base64.StdEncoding.DecodeString(entry.Bytes)
		if err != nil {
			return "", "request body entry is not base64"
		}
		decoded = append(decoded, chunk...)
	}
	if !utf8.Valid(decoded) {
		return "", "request body is not valid UTF-8"
	}
	return string(decoded), ""
}

// requestEntryFromEvent builds the entry a started request is recorded as. It is the one
// place a refused request body turns into a stated reason on the entry, so an absent
// postData is never mistaken for a request sent without one.
func requestEntryFromEvent(e *network.EventRequestWillBeSent) NetworkEntry {
	headers := make(map[string]string)
	if e.Request.Headers != nil {
		for k, v := range e.Request.Headers {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}
	postData, postDataSkipReason := decodePostData(e.Request.PostDataEntries)
	return NetworkEntry{
		RequestID:          string(e.RequestID),
		URL:                e.Request.URL,
		Method:             e.Request.Method,
		ResourceType:       e.Type.String(),
		RequestHeaders:     headers,
		PostData:           postData,
		PostDataSkipped:    postDataSkipReason != "",
		PostDataSkipReason: postDataSkipReason,
		StartTime:          time.Now(),
	}
}

func normalizeNetworkEntry(entry NetworkEntry) NetworkEntry {
	entry.URL = sanitize.TruncateUTF8BytesWithEllipsis(entry.URL, maxNetworkURLBytes)
	entry.Method = sanitize.TruncateUTF8BytesWithEllipsis(entry.Method, maxNetworkMethodBytes)
	entry.ResourceType = sanitize.TruncateUTF8BytesWithEllipsis(entry.ResourceType, maxNetworkResourceTypeBytes)
	entry.StatusText = sanitize.TruncateUTF8BytesWithEllipsis(entry.StatusText, maxNetworkStatusTextBytes)
	entry.MimeType = sanitize.TruncateUTF8BytesWithEllipsis(entry.MimeType, maxNetworkMimeTypeBytes)
	entry.Error = sanitize.TruncateUTF8BytesWithEllipsis(entry.Error, maxNetworkErrorBytes)
	// The request body is clamped by the one owner of that rule, so it inherits the
	// prefix-or-nothing policy the response body already states. The budget measures the
	// decoded body, which is what PostData holds, so the constant describes the request
	// content rather than its encoded length. Flags are only ever set here, never cleared:
	// an entry is re-normalised on every update, and a second pass sees a value already
	// within budget.
	clampedPostData, postDataTruncated, postDataDropReason := clampRetainedBody(entry.PostData, false, maxNetworkPostDataBytes, postDataLimitScope)
	entry.PostData = clampedPostData
	if postDataTruncated {
		entry.PostDataTruncated = true
	}
	if postDataDropReason != "" {
		entry.PostDataSkipped = true
		entry.PostDataSkipReason = postDataDropReason
	}
	entry.RequestHeaders = normalizeNetworkHeaders(entry.RequestHeaders)
	entry.ResponseHeaders = normalizeNetworkHeaders(entry.ResponseHeaders)
	return entry
}

func normalizeNetworkHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	remaining := maxNetworkHeaderTotalBytes
	normalized := make(map[string]string, len(headers))
	for key, value := range headers {
		if remaining <= 0 {
			break
		}

		key = sanitize.TruncateUTF8BytesWithEllipsis(key, maxNetworkHeaderKeyBytes)
		if key == "" {
			continue
		}

		valueLimit := maxNetworkHeaderValueBytes
		if max := remaining - len(key); max < valueLimit {
			valueLimit = max
		}
		if valueLimit <= 0 {
			break
		}

		value = sanitize.TruncateUTF8BytesWithEllipsis(value, valueLimit)
		entryBytes := len(key) + len(value)
		if entryBytes <= 0 {
			continue
		}

		normalized[key] = value
		remaining -= entryBytes
	}

	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

// BrokenAsset is a subresource that failed to load: an HTTP error response
// (status >= 400) or a request that failed outright (network error, abort).
type BrokenAsset struct {
	URL          string `json:"url"`
	ResourceType string `json:"resourceType"`
	StatusCode   int    `json:"statusCode"`
	Error        string `json:"error,omitempty"`
}

// IsBrokenAsset reports whether entry represents a failed load: a response
// with status >= 400, or a failed request. In-flight requests are not broken.
func IsBrokenAsset(entry NetworkEntry) bool {
	return entry.Status >= 400 || entry.Failed
}

// BrokenAssets classifies the broken loads in entries. Resource types are
// the CDP categories lowercased (image, script, stylesheet, font, xhr,
// fetch, document, ...).
func BrokenAssets(entries []NetworkEntry) []BrokenAsset {
	broken := []BrokenAsset{}
	for _, entry := range entries {
		if !IsBrokenAsset(entry) {
			continue
		}
		broken = append(broken, BrokenAsset{
			URL:          entry.URL,
			ResourceType: strings.ToLower(entry.ResourceType),
			StatusCode:   entry.Status,
			Error:        entry.Error,
		})
	}
	return broken
}
