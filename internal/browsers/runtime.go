package browsers

import (
	"context"

	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/runtimetypes"
)

// The runtime DTOs are defined once in internal/runtimetypes and aliased here so
// the RuntimeInstance interface and internal/bridge share one set of structs
// without an import cycle (config → browsers → bridge → config).

type (
	EvalOpts          = runtimetypes.EvalOpts
	NodeInfo          = runtimetypes.NodeInfo
	ScreencastOpts    = runtimetypes.ScreencastOpts
	ScreencastStream  = runtimetypes.ScreencastStream
	CookieData        = runtimetypes.CookieData
	SetCookieParams   = runtimetypes.SetCookieParams
	ViewportParams    = runtimetypes.ViewportParams
	NetworkConditions = runtimetypes.NetworkConditions
	DownloadOpts      = runtimetypes.DownloadOpts
	DownloadResult    = runtimetypes.DownloadResult
	PDFParams         = runtimetypes.PDFParams
)

// NewScreencastStream wraps runtimetypes.NewScreencastStream so provider runtimes
// keep constructing streams through the browsers package.
func NewScreencastStream(frames <-chan []byte, closer func()) *ScreencastStream {
	return runtimetypes.NewScreencastStream(frames, closer)
}

// RuntimeInstance defines post-launch browser operations. Each browser
// provider (chrome, cloak, ghost-chrome) implements this interface to
// own its runtime behavior. The Bridge delegates operations to the
// active RuntimeInstance. Implementations live in the provider
// sub-packages (chrome.Instance, cloak.Instance, ghostchrome.Instance);
// compile-time assertions can't live here without inverting the imports.
type RuntimeInstance interface {
	CaptureScreenshot(ctx context.Context, format string, quality int, clip *cdptk.ScreenshotClip) ([]byte, error)
	StartScreencast(ctx context.Context, opts ScreencastOpts) (*ScreencastStream, error)

	Evaluate(ctx context.Context, expression string, result any, opts EvalOpts) error
	EvaluateInFrame(ctx context.Context, frameID string, expression string, result any, opts EvalOpts) error
	CallFunctionOnNode(ctx context.Context, backendNodeID int64, functionDecl string, args []map[string]any, result any) error

	DescribeNode(ctx context.Context, backendNodeID int64) (*NodeInfo, error)
	ResolveSelectorToNodeID(ctx context.Context, selector string) (int64, error)
	SetFileInputFiles(ctx context.Context, nodeID int64, paths []string) error

	GetCookies(ctx context.Context, urls []string) ([]CookieData, error)
	SetCookie(ctx context.Context, params SetCookieParams) error

	SetViewport(ctx context.Context, params ViewportParams) error
	SetGeolocation(ctx context.Context, lat, lng, accuracy float64) error
	SetEmulatedMedia(ctx context.Context, feature, value string) error

	SetNetworkConditions(ctx context.Context, params NetworkConditions) error
	SetExtraHTTPHeaders(ctx context.Context, headers map[string]string) error

	CurrentURL(ctx context.Context) (string, error)
	CurrentTitle(ctx context.Context) (string, error)

	DownloadURL(ctx context.Context, dlURL string, opts DownloadOpts) (*DownloadResult, error)

	PrintToPDF(ctx context.Context, params PDFParams) ([]byte, error)
}
