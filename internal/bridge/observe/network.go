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
)

// DefaultNetworkBufferSize is the default number of entries kept per tab.
const DefaultNetworkBufferSize = 100

// NetworkEntry represents a single captured network request/response pair.
type NetworkEntry struct {
	RequestID       string            `json:"requestId"`
	URL             string            `json:"url"`
	Method          string            `json:"method"`
	Status          int               `json:"status,omitempty"`
	StatusText      string            `json:"statusText,omitempty"`
	ResourceType    string            `json:"resourceType"`
	RequestHeaders  map[string]string `json:"requestHeaders,omitempty"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
	PostData        string            `json:"postData,omitempty"`
	MimeType        string            `json:"mimeType,omitempty"`
	StartTime       time.Time         `json:"startTime"`
	EndTime         time.Time         `json:"endTime,omitempty"`
	Duration        float64           `json:"duration,omitempty"`
	Size            int64             `json:"size,omitempty"`
	Error           string            `json:"error,omitempty"`
	Finished        bool              `json:"finished"`
	Failed          bool              `json:"failed"`
}

// NetworkBuffer is a thread-safe ring buffer of network entries for a single tab.
type NetworkBuffer struct {
	mu      sync.RWMutex
	entries []NetworkEntry
	index   map[string]int
	maxSize int

	subMu       sync.Mutex
	subscribers map[int]chan NetworkEntry
	nextSubID   int
}

// NewNetworkBuffer creates a ring buffer with the given capacity.
func NewNetworkBuffer(size int) *NetworkBuffer {
	size = config.ClampNetworkBufferSize(size)
	if size <= 0 {
		size = DefaultNetworkBufferSize
	}
	return &NetworkBuffer{
		entries:     make([]NetworkEntry, 0, size),
		index:       make(map[string]int),
		maxSize:     size,
		subscribers: make(map[int]chan NetworkEntry),
	}
}

// Add inserts or updates a network entry.
func (nb *NetworkBuffer) Add(entry NetworkEntry) {
	nb.mu.Lock()

	isNew := false
	if idx, ok := nb.index[entry.RequestID]; ok {
		nb.entries[idx] = entry
	} else {
		isNew = true
		if len(nb.entries) >= nb.maxSize {
			oldest := nb.entries[0]
			delete(nb.index, oldest.RequestID)
			nb.entries = nb.entries[1:]
			for i, e := range nb.entries {
				nb.index[e.RequestID] = i
			}
		}
		nb.index[entry.RequestID] = len(nb.entries)
		nb.entries = append(nb.entries, entry)
	}
	nb.mu.Unlock()

	if isNew {
		nb.subMu.Lock()
		for _, ch := range nb.subscribers {
			select {
			case ch <- entry:
			default:
			}
		}
		nb.subMu.Unlock()
	}
}

// Subscribe returns a channel that receives new entries as they are added.
func (nb *NetworkBuffer) Subscribe() (int, <-chan NetworkEntry) {
	nb.subMu.Lock()
	defer nb.subMu.Unlock()
	id := nb.nextSubID
	nb.nextSubID++
	ch := make(chan NetworkEntry, 64)
	nb.subscribers[id] = ch
	return id, ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (nb *NetworkBuffer) Unsubscribe(id int) {
	nb.subMu.Lock()
	defer nb.subMu.Unlock()
	if ch, ok := nb.subscribers[id]; ok {
		close(ch)
		delete(nb.subscribers, id)
	}
}

// Get returns a specific entry by request ID.
func (nb *NetworkBuffer) Get(requestID string) (NetworkEntry, bool) {
	nb.mu.RLock()
	defer nb.mu.RUnlock()
	idx, ok := nb.index[requestID]
	if !ok {
		return NetworkEntry{}, false
	}
	return nb.entries[idx], true
}

// Update modifies an existing entry in place.
func (nb *NetworkBuffer) Update(requestID string, fn func(*NetworkEntry)) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	idx, ok := nb.index[requestID]
	if !ok {
		return
	}
	fn(&nb.entries[idx])
}

// List returns all entries, optionally filtered.
func (nb *NetworkBuffer) List(filter NetworkFilter) []NetworkEntry {
	nb.mu.RLock()
	defer nb.mu.RUnlock()

	result := make([]NetworkEntry, 0, len(nb.entries))
	for _, e := range nb.entries {
		if filter.Match(e) {
			result = append(result, e)
		}
	}
	return result
}

// Clear removes all entries.
func (nb *NetworkBuffer) Clear() {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	nb.entries = nb.entries[:0]
	nb.index = make(map[string]int)
}

// Len returns the number of entries.
func (nb *NetworkBuffer) Len() int {
	nb.mu.RLock()
	defer nb.mu.RUnlock()
	return len(nb.entries)
}

// MaxSizeForTest exposes the effective ring-buffer size for package-external tests.
func (nb *NetworkBuffer) MaxSizeForTest() int {
	nb.mu.RLock()
	defer nb.mu.RUnlock()
	return nb.maxSize
}

// NetworkFilter defines criteria for filtering network entries.
type NetworkFilter struct {
	URLPattern   string
	Method       string
	StatusRange  string
	ResourceType string
	Limit        int
}

// Match returns true if the entry matches the filter criteria.
func (f NetworkFilter) Match(e NetworkEntry) bool {
	if f.URLPattern != "" && !strings.Contains(strings.ToLower(e.URL), strings.ToLower(f.URLPattern)) {
		return false
	}
	if f.Method != "" && !strings.EqualFold(e.Method, f.Method) {
		return false
	}
	if f.ResourceType != "" && !strings.EqualFold(e.ResourceType, f.ResourceType) {
		return false
	}
	if f.StatusRange != "" && !MatchStatusRange(e.Status, f.StatusRange) {
		return false
	}
	return true
}

// MatchStatusRange checks whether a status code matches an exact or wildcard range.
func MatchStatusRange(status int, pattern string) bool {
	if pattern == "" {
		return true
	}
	if len(pattern) == 3 && pattern[1] != 'x' && pattern[2] != 'x' {
		var code int
		if _, err := fmt.Sscanf(pattern, "%d", &code); err == nil {
			return status == code
		}
	}
	if len(pattern) == 3 && (pattern[1] == 'x' || pattern[2] == 'x') {
		prefix := int(pattern[0] - '0')
		return status/100 == prefix
	}
	return true
}

// NetworkMonitor manages network capture for all tabs.
type NetworkMonitor struct {
	mu        sync.RWMutex
	buffers   map[string]*NetworkBuffer
	listeners map[string]context.CancelFunc
	bufSize   int
}

// NewNetworkMonitor creates a new monitor with the given per-tab buffer size.
func NewNetworkMonitor(bufferSize int) *NetworkMonitor {
	bufferSize = config.ClampNetworkBufferSize(bufferSize)
	if bufferSize <= 0 {
		bufferSize = DefaultNetworkBufferSize
	}
	return &NetworkMonitor{
		buffers:   make(map[string]*NetworkBuffer),
		listeners: make(map[string]context.CancelFunc),
		bufSize:   bufferSize,
	}
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

// GetOrCreateBufferForTest exposes getOrCreateBuffer for tests outside this package.
func (nm *NetworkMonitor) GetOrCreateBufferForTest(tabID string) *NetworkBuffer {
	return nm.getOrCreateBuffer(tabID)
}

// GetOrCreateBufferWithSizeForTest exposes buffer sizing for package-external tests.
func (nm *NetworkMonitor) GetOrCreateBufferWithSizeForTest(tabID string, size int) *NetworkBuffer {
	return nm.getOrCreateBufferWithSize(tabID, size)
}

// GetBuffer returns the buffer for a tab (nil if none).
func (nm *NetworkMonitor) GetBuffer(tabID string) *NetworkBuffer {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.buffers[tabID]
}

// BufferSizeForTest exposes the monitor default size for package-external tests.
func (nm *NetworkMonitor) BufferSizeForTest() int {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.bufSize
}

// StartCapture enables network monitoring on a tab's CDP session.
func (nm *NetworkMonitor) StartCapture(tabCtx context.Context, tabID string) error {
	return nm.StartCaptureWithSize(tabCtx, tabID, 0)
}

// StartCaptureWithSize enables network monitoring with a specific buffer size.
func (nm *NetworkMonitor) StartCaptureWithSize(tabCtx context.Context, tabID string, bufferSize int) error {
	buf := nm.getOrCreateBufferWithSize(tabID, bufferSize)

	if err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.Enable().Do(ctx)
	})); err != nil {
		return fmt.Errorf("network enable: %w", err)
	}

	chromedp.ListenTarget(tabCtx, func(ev interface{}) {
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
			})

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
		}
	})

	slog.Debug("network capture started", "tabId", tabID)
	return nil
}

// StopCapture removes the buffer and listener for a tab.
func (nm *NetworkMonitor) StopCapture(tabID string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	if cancel, ok := nm.listeners[tabID]; ok {
		cancel()
		delete(nm.listeners, tabID)
	}
	delete(nm.buffers, tabID)
}

// ClearTab clears the network buffer for a tab.
func (nm *NetworkMonitor) ClearTab(tabID string) {
	nm.mu.RLock()
	buf := nm.buffers[tabID]
	nm.mu.RUnlock()
	if buf != nil {
		buf.Clear()
	}
}

// ClearAll clears all network buffers.
func (nm *NetworkMonitor) ClearAll() {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	for _, buf := range nm.buffers {
		buf.Clear()
	}
}

// GetResponseBody fetches the response body for a specific request via CDP.
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

// GetResponseBodyDirect fetches the response body using a raw CDP executor context.
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
