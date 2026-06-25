package observe

import (
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// ExportEncoder streams network entries to an io.Writer in a specific format.
// Implementations are registered via RegisterFormat and created per-export.
// Not concurrent-safe; callers serialize Start → Encode* → Finish.
type ExportEncoder interface {
	// ContentType returns the MIME type for HTTP Content-Type header.
	ContentType() string

	// FileExtension returns the default extension including dot (e.g. ".har").
	FileExtension() string

	// Start writes any format preamble to w. Called once before the first Encode.
	Start(w io.Writer) error

	// Encode writes a single entry. Called zero or more times between Start and Finish.
	Encode(entry ExportEntry) error

	// Finish writes any format trailer. Called once after the last Encode.
	// Must produce valid output even if Encode was never called.
	Finish() error
}

// ExportEncoderFactory creates a new encoder for a single export session.
type ExportEncoderFactory func(creatorName, creatorVersion string) ExportEncoder

var (
	formatsMu sync.RWMutex
	formats   = map[string]ExportEncoderFactory{}
)

// RegisterFormat registers an export format. Called from init() in format files.
// Names are normalized to lowercase for case-insensitive lookup.
func RegisterFormat(name string, factory ExportEncoderFactory) {
	formatsMu.Lock()
	defer formatsMu.Unlock()
	formats[strings.ToLower(name)] = factory
}

// GetFormat returns the factory for a named format, or nil if not found.
func GetFormat(name string) ExportEncoderFactory {
	formatsMu.RLock()
	defer formatsMu.RUnlock()
	return formats[strings.ToLower(name)]
}

// ListFormats returns all registered format names, sorted.
func ListFormats() []string {
	formatsMu.RLock()
	defer formatsMu.RUnlock()
	names := make([]string, 0, len(formats))
	for name := range formats {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ExportEntry is the normalized representation of a network request/response.
// All export formats encode from this struct.
type ExportEntry struct {
	StartedDateTime string         `json:"startedDateTime"`
	Time            float64        `json:"time"`
	Request         ExportRequest  `json:"request"`
	Response        ExportResponse `json:"response"`
	Timings         ExportTimings  `json:"timings"`
}

// ExportRequest holds the request portion of an entry.
type ExportRequest struct {
	Method      string          `json:"method"`
	URL         string          `json:"url"`
	HTTPVersion string          `json:"httpVersion"`
	Headers     []NameValuePair `json:"headers"`
	QueryString []NameValuePair `json:"queryString"`
	PostData    *ExportPostData `json:"postData,omitempty"`
	HeadersSize int             `json:"headersSize"`
	BodySize    int             `json:"bodySize"`
}

// ExportResponse holds the response portion of an entry.
type ExportResponse struct {
	Status      int             `json:"status"`
	StatusText  string          `json:"statusText"`
	HTTPVersion string          `json:"httpVersion"`
	Headers     []NameValuePair `json:"headers"`
	Content     ExportContent   `json:"content"`
	HeadersSize int             `json:"headersSize"`
	BodySize    int64           `json:"bodySize"`
}

// ExportContent holds the response body metadata and optional text.
type ExportContent struct {
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

// ExportTimings holds timing breakdown. Values are in milliseconds.
// -1 means not applicable. send, wait, receive are always >= 0.
type ExportTimings struct {
	Send    float64 `json:"send"`
	Wait    float64 `json:"wait"`
	Receive float64 `json:"receive"`
}

// ExportPostData holds request body information.
type ExportPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

// NameValuePair is a generic name-value pair used for headers and query params.
type NameValuePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// NetworkEntryToExport converts a captured NetworkEntry to the canonical ExportEntry.
// body and base64Encoded are provided when response bodies are fetched on demand.
func NetworkEntryToExport(entry NetworkEntry, body string, base64Encoded bool) ExportEntry {
	e := ExportEntry{
		StartedDateTime: entry.StartTime.UTC().Format("2006-01-02T15:04:05.000Z"),
		Time:            entry.Duration,
		Request: ExportRequest{
			Method:      entry.Method,
			URL:         entry.URL,
			HTTPVersion: "HTTP/1.1",
			Headers:     mapToNameValuePairs(entry.RequestHeaders),
			QueryString: parseQueryString(entry.URL),
			HeadersSize: -1,
			BodySize:    len(entry.PostData),
		},
		Response: ExportResponse{
			Status:      entry.Status,
			StatusText:  entry.StatusText,
			HTTPVersion: "HTTP/1.1",
			Headers:     mapToNameValuePairs(entry.ResponseHeaders),
			Content: ExportContent{
				Size:     entry.Size,
				MimeType: entry.MimeType,
			},
			HeadersSize: -1,
			BodySize:    entry.Size,
		},
		Timings: computeTimings(entry),
	}

	if entry.PostData != "" {
		e.Request.PostData = &ExportPostData{
			MimeType: contentTypeFromHeaders(entry.RequestHeaders),
			Text:     entry.PostData,
		}
	}

	if body != "" {
		e.Response.Content.Text = body
		if base64Encoded {
			e.Response.Content.Encoding = "base64"
		}
	}

	return e
}

func mapToNameValuePairs(headers map[string]string) []NameValuePair {
	if len(headers) == 0 {
		return []NameValuePair{}
	}
	pairs := make([]NameValuePair, 0, len(headers))
	for k, v := range headers {
		pairs = append(pairs, NameValuePair{Name: k, Value: v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Name < pairs[j].Name })
	return pairs
}

func parseQueryString(rawURL string) []NameValuePair {
	u, err := url.Parse(rawURL)
	if err != nil || u.RawQuery == "" {
		return []NameValuePair{}
	}
	params := u.Query()
	pairs := make([]NameValuePair, 0, len(params))
	for k, vals := range params {
		for _, v := range vals {
			pairs = append(pairs, NameValuePair{Name: k, Value: v})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Name < pairs[j].Name })
	return pairs
}

func computeTimings(entry NetworkEntry) ExportTimings {
	total := entry.Duration
	if total <= 0 {
		return ExportTimings{Send: 0, Wait: 0, Receive: 0}
	}
	// Approximate breakdown: most time is wait (TTFB).
	// Without CDP-level timing detail, allocate 1ms send, 1ms receive, rest is wait.
	send := 1.0
	receive := 1.0
	if total < 3 {
		send = 0
		receive = 0
	}
	wait := total - send - receive
	if wait < 0 {
		wait = 0
	}
	return ExportTimings{Send: send, Wait: wait, Receive: receive}
}

// sensitiveHeaderNames are headers that contain credentials or session tokens.
// They are redacted by default in exports to prevent accidental data leakage.
var sensitiveHeaderNames = map[string]bool{
	"cookie":              true,
	"set-cookie":          true,
	"authorization":       true,
	"proxy-authorization": true,
	"x-api-key":           true,
	"x-csrf-token":        true,
}

// RedactSensitiveHeaders replaces the values of credential-bearing headers with [REDACTED].
func RedactSensitiveHeaders(pairs []NameValuePair) []NameValuePair {
	for i := range pairs {
		if sensitiveHeaderNames[strings.ToLower(pairs[i].Name)] {
			pairs[i].Value = "[REDACTED]"
		}
	}
	return pairs
}

func contentTypeFromHeaders(headers map[string]string) string {
	for k, v := range headers {
		if strings.EqualFold(k, "content-type") {
			return v
		}
	}
	return ""
}
