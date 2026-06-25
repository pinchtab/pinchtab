// Package runtimetypes holds the browser-runtime DTOs shared by internal/bridge
// and internal/browsers. It exists to give both packages one canonical set of
// structs without an import cycle (config → browsers → bridge → config): it is a
// leaf that imports only the standard library, so browsers and bridge can both
// alias these types instead of hand-mirroring them.
package runtimetypes

type EvalOpts struct {
	AwaitPromise bool
}

type NodeInfo struct {
	LocalName      string
	Attributes     []string
	ChildNodeCount int
}

type ScreencastOpts struct {
	Quality       int // 1-100, default 30
	MaxWidth      int // pixels, default 800
	MaxHeight     int // pixels, default 600
	EveryNthFrame int // frame skipping for event mode, default 4
	FPS           int // frames per second (caps at 30), default 1
}

// ScreencastStream carries decoded frames until Close releases resources.
// Callers must call Close when done. Close is safe to call concurrently with a
// producer that closes the same done channel: the select/default guards against
// a double close.
type ScreencastStream struct {
	Frames <-chan []byte
	done   chan struct{}
	closer func()
}

// NewScreencastStream creates a stream that owns its done channel; Close is the
// only closer of done.
func NewScreencastStream(frames <-chan []byte, closer func()) *ScreencastStream {
	return &ScreencastStream{
		Frames: frames,
		done:   make(chan struct{}),
		closer: closer,
	}
}

// NewScreencastStreamWithDone creates a stream over a caller-supplied done
// channel, for producers that close done themselves when the source ends (e.g.
// the bridge screencast loop). Close still guards against a double close.
func NewScreencastStreamWithDone(frames <-chan []byte, done chan struct{}, closer func()) *ScreencastStream {
	return &ScreencastStream{
		Frames: frames,
		done:   done,
		closer: closer,
	}
}

func (s *ScreencastStream) Close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	if s.closer != nil {
		s.closer()
	}
}

func (s *ScreencastStream) Done() <-chan struct{} {
	return s.done
}

type CookieData struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite"`
}

type SetCookieParams struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	URL      string  `json:"url"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite"`
	Expires  float64 `json:"expires"`
}

type ViewportParams struct {
	Width             int64
	Height            int64
	DeviceScaleFactor float64
	Mobile            bool
}

type NetworkConditions struct {
	Offline            bool
	Latency            float64
	DownloadThroughput float64
	UploadThroughput   float64
}

type DownloadOpts struct {
	// MaxBytes is the maximum response size in bytes. 0 means no limit.
	MaxBytes int
	// MaxRedirects limits browser-side redirects. -1 means unlimited.
	MaxRedirects int
	// ValidateURL is called for every request/redirect URL seen by the browser.
	// If it returns an error, the request is blocked.
	ValidateURL func(rawURL string, isRedirect bool) error
	// ValidateRemoteIP is called when the response remote IP is known.
	// If it returns an error, the download is aborted.
	ValidateRemoteIP func(remoteIP string) error
	// IsDomainAllowed reports whether a URL's domain is on the operator allowlist
	// (bypassing private-IP checks).
	IsDomainAllowed func(rawURL string) bool
	// ParseContentLength extracts Content-Length from response headers, if present.
	ParseContentLength func(headers map[string]interface{}) (int64, bool)
}

type DownloadResult struct {
	Body       []byte
	MIMEType   string
	StatusCode int
}

type PDFParams struct {
	Landscape               bool
	PrintBackground         bool
	Scale                   float64
	PaperWidth              float64
	PaperHeight             float64
	MarginTop               float64
	MarginBottom            float64
	MarginLeft              float64
	MarginRight             float64
	PageRanges              string
	PreferCSSPageSize       bool
	DisplayHeaderFooter     bool
	GenerateTaggedPDF       bool
	GenerateDocumentOutline bool
	HeaderTemplate          string
	FooterTemplate          string
}
