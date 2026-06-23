package cdptk

// MaxAnnotations bounds how many overlay rectangles we render per screenshot.
// Caps payload size and overlay-render cost on dense pages.
const MaxAnnotations = 200

// OverlayRootID is the stable DOM id for the injected annotation layer. We
// look for it before injection so a stale overlay from a previous failed
// capture is removed first.
const OverlayRootID = "__pinchtab_annotations__"

// CaptureMode selects how returned annotation boxes are projected.
type CaptureMode int

const (
	ModeViewport CaptureMode = iota
	ModeSelectorClip
	ModeBeyondViewport
)

type ScreenshotClip struct {
	X, Y, Width, Height, Scale float64
}

type AnnotationItem struct {
	Ref  string         `json:"ref"`
	Role string         `json:"role,omitempty"`
	Name string         `json:"name,omitempty"`
	Tag  string         `json:"tag,omitempty"`
	Box  AnnotationRect `json:"box"`
}

type AnnotationRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}
