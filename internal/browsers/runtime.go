package browsers

import (
	"context"

	"github.com/pinchtab/pinchtab/internal/cdptk"
)

// ---------------------------------------------------------------------------
// Runtime parameter / result types
//
// These mirror the identically-named types in internal/bridge so that the
// RuntimeInstance interface can be defined without creating an import cycle
// (config → browsers → bridge → config). Bridge implementations convert
// between the two sets at the delegation boundary.
// ---------------------------------------------------------------------------

// EvalOpts configures JavaScript evaluation behavior.
type EvalOpts struct {
	AwaitPromise bool
}

// NodeInfo holds DOM structural info returned by DescribeNode.
type NodeInfo struct {
	LocalName      string
	Attributes     []string
	ChildNodeCount int
}

// ScreencastOpts configures a screencast stream.
type ScreencastOpts struct {
	Quality       int // 1-100, default 30
	MaxWidth      int // pixels, default 800
	MaxHeight     int // pixels, default 600
	EveryNthFrame int // frame skipping for event mode, default 4
	FPS           int // frames per second (caps at 30), default 1
}

// ScreencastStream delivers decoded binary JPEG frames over a channel.
// Callers must call Close when done to release resources.
type ScreencastStream struct {
	Frames <-chan []byte
	done   chan struct{}
	closer func()
}

// NewScreencastStream creates a ScreencastStream with the given frame
// channel and cleanup function.
func NewScreencastStream(frames <-chan []byte, closer func()) *ScreencastStream {
	return &ScreencastStream{
		Frames: frames,
		done:   make(chan struct{}),
		closer: closer,
	}
}

// Close stops the screencast and releases resources.
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

// Done returns a channel that is closed when the stream is stopped.
func (s *ScreencastStream) Done() <-chan struct{} {
	return s.done
}

// CookieData is a browser-level representation of a cookie.
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

// SetCookieParams holds parameters for setting a single cookie.
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

// ViewportParams holds parameters for viewport emulation.
type ViewportParams struct {
	Width             int64
	Height            int64
	DeviceScaleFactor float64
	Mobile            bool
}

// NetworkConditions holds parameters for network emulation.
type NetworkConditions struct {
	Offline            bool
	Latency            float64
	DownloadThroughput float64
	UploadThroughput   float64
}

// DownloadOpts configures a browser-based download.
type DownloadOpts struct {
	MaxBytes           int
	MaxRedirects       int
	ValidateURL        func(rawURL string, isRedirect bool) error
	ValidateRemoteIP   func(remoteIP string) error
	IsDomainAllowed    func(rawURL string) bool
	ParseContentLength func(headers map[string]interface{}) (int64, bool)
}

// DownloadResult holds the result of a browser-based download.
type DownloadResult struct {
	Body       []byte
	MIMEType   string
	StatusCode int
}

// PDFParams holds parameters for PDF generation.
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

// ---------------------------------------------------------------------------
// RuntimeInstance interface
// ---------------------------------------------------------------------------

// RuntimeInstance defines post-launch browser operations. Each browser
// provider (chrome, cloak, ghost-chrome) implements this interface to
// own its runtime behavior. The Bridge delegates operations to the
// active RuntimeInstance.
//
// Future implementations:
//
//	var _ RuntimeInstance = (*chrome.Instance)(nil)
//	var _ RuntimeInstance = (*cloak.Instance)(nil)
type RuntimeInstance interface {
	// Visual capture
	CaptureScreenshot(ctx context.Context, format string, quality int, clip *cdptk.ScreenshotClip) ([]byte, error)
	StartScreencast(ctx context.Context, opts ScreencastOpts) (*ScreencastStream, error)

	// JavaScript evaluation
	Evaluate(ctx context.Context, expression string, result any, opts EvalOpts) error
	EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts EvalOpts) error
	CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error

	// DOM
	DescribeNode(ctx context.Context, backendNodeID int64) (*NodeInfo, error)
	ResolveSelectorToNodeID(ctx context.Context, selector string) (int64, error)
	SetFileInputFiles(ctx context.Context, nodeID int64, paths []string) error

	// Cookies
	GetCookies(ctx context.Context, urls []string) ([]CookieData, error)
	SetCookie(ctx context.Context, params SetCookieParams) error

	// Emulation
	SetViewport(ctx context.Context, params ViewportParams) error
	SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error
	SetEmulatedMedia(ctx context.Context, feature, value string) error

	// Network state
	SetNetworkConditions(ctx context.Context, params NetworkConditions) error
	SetExtraHTTPHeaders(ctx context.Context, headers map[string]string) error

	// Navigation info
	CurrentURL(ctx context.Context) (string, error)
	CurrentTitle(ctx context.Context) (string, error)

	// Download
	DownloadURL(ctx context.Context, dlURL string, opts DownloadOpts) (*DownloadResult, error)

	// PDF
	PrintToPDF(ctx context.Context, params PDFParams) ([]byte, error)
}
