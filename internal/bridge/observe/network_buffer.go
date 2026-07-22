package observe

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

const DefaultNetworkBufferSize = 100

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
	ResponseBody    string            `json:"responseBody,omitempty"`
	Base64Encoded   bool              `json:"base64Encoded,omitempty"`
	BodyRetained    bool              `json:"bodyRetained,omitempty"`
	BodyPending     bool              `json:"bodyPending,omitempty"`
	BodySkipped     bool              `json:"bodySkipped,omitempty"`
	BodySkipReason  string            `json:"bodySkipReason,omitempty"`
	BodyTruncated   bool              `json:"bodyTruncated,omitempty"`
	BodyError       string            `json:"bodyError,omitempty"`
}

type NetworkBuffer struct {
	mu            sync.RWMutex
	entries       []NetworkEntry // fixed-length ring (len == maxSize)
	index         map[string]int // RequestID → ring slot
	head          int            // slot of the oldest entry
	size          int            // current entry count (<= maxSize)
	maxSize       int
	retainedBytes int64

	// Inflight tracking is independent of the ring buffer so eviction
	// doesn't corrupt the count. inflightIDs holds request IDs currently
	// in flight; lastChange records the most recent in-flight transition
	// (request started or completed). Both are guarded by mu.
	inflightIDs map[string]struct{}
	lastChange  time.Time

	subMu       sync.Mutex
	subscribers map[int]chan NetworkEntry
	nextSubID   int

	// Completion notifications are a separate pub/sub from the entry broadcast
	// above: subscribers (e.g. live export) react to a request finishing/failing
	// instead of polling Get(). Kept distinct so entry subscribers (live network
	// stream) are not handed duplicate per-request events.
	completionSubMu     sync.Mutex
	completionSubs      map[int]chan string
	nextCompletionSubID int

	// bodyChangeCh broadcasts when async body retention resolves a request's body
	// state, so retained-body readers wait on it instead of spin-sleeping. It is
	// closed and replaced on each signal; waiters capture it before reading state.
	bodyChangeMu sync.Mutex
	bodyChangeCh chan struct{}
}

func NewNetworkBuffer(size int) *NetworkBuffer {
	size = config.ClampNetworkBufferSize(size)
	if size <= 0 {
		size = DefaultNetworkBufferSize
	}
	return &NetworkBuffer{
		entries:        make([]NetworkEntry, size),
		index:          make(map[string]int),
		maxSize:        size,
		inflightIDs:    make(map[string]struct{}),
		lastChange:     time.Now(),
		subscribers:    make(map[int]chan NetworkEntry),
		completionSubs: make(map[int]chan string),
		bodyChangeCh:   make(chan struct{}),
	}
}

// BodyChangeChan returns the current body-change broadcast channel. Capture it
// BEFORE reading body state so a signal between the read and the wait is not
// missed (the captured channel will already be closed).
func (nb *NetworkBuffer) BodyChangeChan() <-chan struct{} {
	nb.bodyChangeMu.Lock()
	defer nb.bodyChangeMu.Unlock()
	return nb.bodyChangeCh
}

// SignalBodyChange wakes all current body-change waiters by closing the channel
// and installing a fresh one for future waiters.
func (nb *NetworkBuffer) SignalBodyChange() {
	nb.bodyChangeMu.Lock()
	defer nb.bodyChangeMu.Unlock()
	close(nb.bodyChangeCh)
	nb.bodyChangeCh = make(chan struct{})
}

func (nb *NetworkBuffer) MarkRequestStart(requestID string) {
	if requestID == "" {
		return
	}
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if _, ok := nb.inflightIDs[requestID]; ok {
		return
	}
	nb.inflightIDs[requestID] = struct{}{}
	nb.lastChange = time.Now()
}

func (nb *NetworkBuffer) MarkRequestEnd(requestID string) {
	if requestID == "" {
		return
	}
	nb.mu.Lock()
	_, ended := nb.inflightIDs[requestID]
	if ended {
		delete(nb.inflightIDs, requestID)
		nb.lastChange = time.Now()
	}
	nb.mu.Unlock()

	// Notify completion subscribers outside nb.mu (publishCompletion takes the
	// separate completionSubMu). Only fires on the genuine in-flight→ended
	// transition, so each request yields exactly one completion event.
	if ended {
		nb.publishCompletion(requestID)
	}
}

// SubscribeCompletions returns a channel that receives the requestID of each
// request as it finishes or fails. Distinct from Subscribe (which broadcasts
// entries) so entry subscribers are not handed extra per-request events.
func (nb *NetworkBuffer) SubscribeCompletions() (int, <-chan string) {
	nb.completionSubMu.Lock()
	defer nb.completionSubMu.Unlock()
	id := nb.nextCompletionSubID
	nb.nextCompletionSubID++
	ch := make(chan string, 64)
	nb.completionSubs[id] = ch
	return id, ch
}

func (nb *NetworkBuffer) UnsubscribeCompletions(id int) {
	nb.completionSubMu.Lock()
	defer nb.completionSubMu.Unlock()
	if ch, ok := nb.completionSubs[id]; ok {
		close(ch)
		delete(nb.completionSubs, id)
	}
}

func (nb *NetworkBuffer) publishCompletion(requestID string) {
	nb.completionSubMu.Lock()
	defer nb.completionSubMu.Unlock()
	for _, ch := range nb.completionSubs {
		select {
		case ch <- requestID:
		default:
		}
	}
}

func (nb *NetworkBuffer) InflightStatus() (count int, lastChange time.Time) {
	nb.mu.RLock()
	defer nb.mu.RUnlock()
	return len(nb.inflightIDs), nb.lastChange
}

func (nb *NetworkBuffer) Add(entry NetworkEntry) {
	entry = normalizeNetworkEntry(entry)
	nb.mu.Lock()

	isNew := false
	if slot, ok := nb.index[entry.RequestID]; ok {
		nb.entries[slot] = entry
	} else {
		isNew = true
		var slot int
		if nb.size == nb.maxSize {
			slot = nb.head
			oldest := nb.entries[slot]
			if oldest.BodyRetained {
				nb.retainedBytes -= int64(len(oldest.ResponseBody))
				if nb.retainedBytes < 0 {
					nb.retainedBytes = 0
				}
			}
			delete(nb.index, oldest.RequestID)
			nb.head = (nb.head + 1) % nb.maxSize
		} else {
			slot = (nb.head + nb.size) % nb.maxSize
			nb.size++
		}
		nb.entries[slot] = entry
		nb.index[entry.RequestID] = slot
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

func (nb *NetworkBuffer) Subscribe() (int, <-chan NetworkEntry) {
	nb.subMu.Lock()
	defer nb.subMu.Unlock()
	id := nb.nextSubID
	nb.nextSubID++
	ch := make(chan NetworkEntry, 64)
	nb.subscribers[id] = ch
	return id, ch
}

func (nb *NetworkBuffer) Unsubscribe(id int) {
	nb.subMu.Lock()
	defer nb.subMu.Unlock()
	if ch, ok := nb.subscribers[id]; ok {
		close(ch)
		delete(nb.subscribers, id)
	}
}

func (nb *NetworkBuffer) Get(requestID string) (NetworkEntry, bool) {
	nb.mu.RLock()
	defer nb.mu.RUnlock()
	idx, ok := nb.index[requestID]
	if !ok {
		return NetworkEntry{}, false
	}
	return nb.entries[idx], true
}

func (nb *NetworkBuffer) Update(requestID string, fn func(*NetworkEntry)) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	idx, ok := nb.index[requestID]
	if !ok {
		return
	}
	before := nb.entries[idx]
	fn(&nb.entries[idx])
	nb.entries[idx] = normalizeNetworkEntry(nb.entries[idx])
	after := nb.entries[idx]
	if before.BodyRetained {
		nb.retainedBytes -= int64(len(before.ResponseBody))
	}
	if after.BodyRetained {
		nb.retainedBytes += int64(len(after.ResponseBody))
	}
	if nb.retainedBytes < 0 {
		nb.retainedBytes = 0
	}
}

func (nb *NetworkBuffer) RetainedBytes() int64 {
	nb.mu.RLock()
	defer nb.mu.RUnlock()
	return nb.retainedBytes
}

func (nb *NetworkBuffer) List(filter NetworkFilter) []NetworkEntry {
	nb.mu.RLock()
	defer nb.mu.RUnlock()

	result := make([]NetworkEntry, 0, nb.size)
	for i := 0; i < nb.size; i++ {
		e := nb.entries[(nb.head+i)%nb.maxSize]
		if filter.Match(e) {
			result = append(result, e)
		}
	}
	return result
}

// Clear removes all entries. Inflight tracking is preserved because
// active requests are not affected by a buffer clear.
func (nb *NetworkBuffer) Clear() {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	nb.entries = make([]NetworkEntry, nb.maxSize)
	nb.index = make(map[string]int)
	nb.head = 0
	nb.size = 0
	nb.retainedBytes = 0
}

func (nb *NetworkBuffer) Len() int {
	nb.mu.RLock()
	defer nb.mu.RUnlock()
	return nb.size
}

func (nb *NetworkBuffer) MaxSizeForTest() int {
	nb.mu.RLock()
	defer nb.mu.RUnlock()
	return nb.maxSize
}

type NetworkFilter struct {
	URLPattern   string
	Method       string
	StatusRange  string
	ResourceType string
	Limit        int
}

func (f NetworkFilter) Match(e NetworkEntry) bool {
	if f.URLPattern != "" && !strings.Contains(strings.ToLower(e.URL), strings.ToLower(f.URLPattern)) {
		return false
	}
	if f.Method != "" && !strings.EqualFold(e.Method, f.Method) {
		return false
	}
	if f.ResourceType != "" && !networkResourceTypeMatches(e.ResourceType, f.ResourceType) {
		return false
	}
	if f.StatusRange != "" && !MatchStatusRange(e.Status, f.StatusRange) {
		return false
	}
	return true
}

func networkResourceTypeMatches(entryType, filterType string) bool {
	if strings.EqualFold(entryType, filterType) {
		return true
	}
	// Keep fetch() traffic discoverable for older clients/tests that still
	// query type=XHR for script-initiated requests.
	if strings.EqualFold(filterType, "xhr") && strings.EqualFold(entryType, "fetch") {
		return true
	}
	if strings.EqualFold(filterType, "fetch") && strings.EqualFold(entryType, "xhr") {
		return true
	}
	return false
}

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
