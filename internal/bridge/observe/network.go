package observe

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
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

	chromedp.ListenTarget(listenerCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			headers := make(map[string]string)
			if e.Request.Headers != nil {
				for k, v := range e.Request.Headers {
					if s, ok := v.(string); ok {
						headers[k] = s
					}
				}
			}
			var postData string
			if e.Request.HasPostData && len(e.Request.PostDataEntries) > 0 {
				for _, entry := range e.Request.PostDataEntries {
					postData += entry.Bytes
				}
			}
			entry := NetworkEntry{
				RequestID:      string(e.RequestID),
				URL:            e.Request.URL,
				Method:         e.Request.Method,
				ResourceType:   e.Type.String(),
				RequestHeaders: headers,
				PostData:       postData,
				StartTime:      time.Now(),
			}
			buf.Add(entry)
			buf.MarkRequestStart(string(e.RequestID))

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
				entry.Finished = true
				entry.EndTime = time.Now()
				if !entry.StartTime.IsZero() {
					entry.Duration = float64(entry.EndTime.Sub(entry.StartTime).Milliseconds())
				}
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
				entry.Failed = true
				entry.Finished = true
				entry.EndTime = time.Now()
				if !entry.StartTime.IsZero() {
					entry.Duration = float64(entry.EndTime.Sub(entry.StartTime).Milliseconds())
				}
				entry.Error = e.ErrorText
			})
			buf.MarkRequestEnd(string(e.RequestID))
		}
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

func (nm *NetworkMonitor) maybeRetainBody(tabCtx context.Context, buf *NetworkBuffer, requestID string) {
	// Every return path below resolves BodyPending via buf.Update; signal once on
	// exit so retained-body readers wake instead of polling.
	defer buf.SignalBodyChange()

	nm.mu.RLock()
	enabled := nm.retainBodies
	maxBytes := nm.retainBodyMaxBytes
	nm.mu.RUnlock()
	if !enabled {
		buf.Update(requestID, func(entry *NetworkEntry) {
			entry.BodyPending = false
			entry.BodySkipped = true
			entry.BodySkipReason = "retention disabled"
			entry.BodyError = ""
		})
		return
	}
	if buf.RetainedBytes() >= nm.retainBodyMaxPerTab {
		buf.Update(requestID, func(entry *NetworkEntry) {
			entry.BodyPending = false
			entry.BodySkipped = true
			entry.BodySkipReason = "retention budget exceeded"
			entry.BodyError = ""
		})
		return
	}
	select {
	case nm.retainBodySemaphore <- struct{}{}:
		defer func() { <-nm.retainBodySemaphore }()
	default:
		buf.Update(requestID, func(entry *NetworkEntry) {
			entry.BodyPending = false
			entry.BodySkipped = true
			entry.BodySkipReason = "retention concurrency limit reached"
			entry.BodyError = ""
		})
		return
	}
	body, base64Encoded, err := GetResponseBodyDirect(tabCtx, requestID)
	if err != nil {
		buf.Update(requestID, func(entry *NetworkEntry) {
			entry.BodyPending = false
			entry.BodySkipped = false
			entry.BodySkipReason = ""
			entry.BodyError = err.Error()
		})
		return
	}
	truncated := false
	if maxBytes > 0 && len(body) > maxBytes {
		body = body[:maxBytes]
		truncated = true
	}
	remainingBudget := int(nm.retainBodyMaxPerTab - buf.RetainedBytes())
	if remainingBudget <= 0 {
		buf.Update(requestID, func(entry *NetworkEntry) {
			entry.BodyPending = false
			entry.BodySkipped = true
			entry.BodySkipReason = "retention budget exceeded"
			entry.BodyError = ""
		})
		return
	}
	if len(body) > remainingBudget {
		body = body[:remainingBudget]
		truncated = true
	}
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

func (nm *NetworkMonitor) GetResponseBody(tabCtx context.Context, requestID string) (string, bool, error) {
	var body string
	var base64Encoded bool

	err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var result json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Network.getResponseBody", map[string]any{
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

func GetResponseBodyDirect(ctx context.Context, requestID string) (string, bool, error) {
	var body string
	var base64Encoded bool

	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		executor := chromedp.FromContext(ctx).Target
		if executor == nil {
			return fmt.Errorf("no CDP executor available")
		}
		params := network.GetResponseBody(network.RequestID(requestID))
		resp, err := params.Do(cdp.WithExecutor(ctx, executor))
		if err != nil {
			return err
		}
		body = string(resp)
		base64Encoded = false
		return nil
	}))

	return body, base64Encoded, err
}

func normalizeNetworkEntry(entry NetworkEntry) NetworkEntry {
	entry.URL = sanitize.TruncateUTF8Bytes(entry.URL, maxNetworkURLBytes)
	entry.Method = sanitize.TruncateUTF8Bytes(entry.Method, maxNetworkMethodBytes)
	entry.ResourceType = sanitize.TruncateUTF8Bytes(entry.ResourceType, maxNetworkResourceTypeBytes)
	entry.StatusText = sanitize.TruncateUTF8Bytes(entry.StatusText, maxNetworkStatusTextBytes)
	entry.MimeType = sanitize.TruncateUTF8Bytes(entry.MimeType, maxNetworkMimeTypeBytes)
	entry.Error = sanitize.TruncateUTF8Bytes(entry.Error, maxNetworkErrorBytes)
	entry.PostData = sanitize.TruncateUTF8Bytes(entry.PostData, maxNetworkPostDataBytes)
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

		key = sanitize.TruncateUTF8Bytes(key, maxNetworkHeaderKeyBytes)
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

		value = sanitize.TruncateUTF8Bytes(value, valueLimit)
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
