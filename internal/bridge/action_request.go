package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

type ActionFunc func(ctx context.Context, req ActionRequest) (map[string]any, error)

// ActionRequest defines the parameters for a browser action.
//
// Element targeting uses a unified selector string that supports multiple
// strategies via prefix detection (see the selector package):
//
//	"e5"              → ref from snapshot
//	"css:#login"      → CSS selector (explicit)
//	"#login"          → CSS selector (auto-detected)
//	"xpath://div"     → XPath expression
//	"text:Submit"     → text content match
//	"find:login btn"  → semantic / natural-language query
//	"role:button Save" → role/name locator
//	"label:Email"     → form control by label
//	"testid:submit"   → test id locator
//	"last:button"     → positional selector wrapper
//
// For backward compatibility, the legacy Ref and Selector (CSS) fields
// are still accepted. Call NormalizeSelector() to merge them into the
// unified Selector field.
type ActionRequest struct {
	TabID    string `json:"tabId"`
	Kind     string `json:"kind"`
	Ref      string `json:"ref,omitempty"`
	Selector string `json:"selector,omitempty"`

	// Text/Value use omitempty, and HasText records that one of them was
	// SUPPLIED, for the same reason X/Y do below: an empty fill is a
	// legitimate request to clear the field, so absent and empty must not
	// share a representation. While they did, `fill` could not refuse a
	// request whose text never arrived — it wrote "" and answered
	// filled:true, len:0 — and its read-back verification, gated on a
	// non-empty Text, switched itself off in exactly that case. Re-marshaling
	// without omitempty would re-introduce "text":"" and make every forwarded
	// request look supplied.
	Text    string `json:"text,omitempty"`
	Value   string `json:"value,omitempty"`
	HasText bool   `json:"hasText,omitempty"`

	Key    string `json:"key"`
	NodeID int64  `json:"nodeId"`

	// X/Y use omitempty so that re-marshaling an ActionRequest without
	// explicit coordinates (e.g. when the tab-scoped handler forwards to
	// the generic one) doesn't spuriously re-introduce "x":0, "y":0. The
	// ActionRequest.UnmarshalJSON code infers HasXY from the presence of
	// these keys, so preserving omission is what makes "use current
	// pointer" work for mouse-down/up after a prior mouse-move. HasXY is
	// still marshaled (with omitempty) when it was explicitly set, which
	// preserves the explicit-click-at-(0,0) case through the round-trip.
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	HasXY  bool    `json:"hasXY,omitempty"`
	Button string  `json:"button,omitempty"`
	Mode   string  `json:"mode,omitempty"`

	// FrameW/FrameH are the pixel dimensions of the screencast frame the caller
	// mapped the coordinates against (the dashboard maps a click to frame-pixel
	// space). When set, X/Y are scaled from that frame space into the live CSS
	// viewport before dispatch, so a HiDPI frame (e.g. 2x the CSS viewport) does
	// not land clicks at twice their intended position. Zero means X/Y are
	// already CSS pixels.
	FrameW float64 `json:"frameW,omitempty"`
	FrameH float64 `json:"frameH,omitempty"`

	// Modifiers is the CDP key-modifier bitmask (Alt=1, Ctrl=2, Meta=4, Shift=8)
	// held during a press, enabling keyboard chords such as Ctrl+C or Shift+Arrow.
	Modifiers int `json:"modifiers,omitempty"`

	// ScrollX/ScrollY use omitempty for the same reason X/Y do: UnmarshalJSON
	// infers HasScroll from the presence of these keys, so a re-marshal that
	// re-introduced "scrollX":0 would turn every bare scroll into an explicit
	// zero and refuse it. The explicit zero survives the omission because
	// HasScroll is itself marshaled and the decoder ORs it back in — that is
	// what keeps an explicit zero from decaying into the default 120px step.
	ScrollX int `json:"scrollX,omitempty"`
	ScrollY int `json:"scrollY,omitempty"`

	HasScroll bool `json:"hasScroll,omitempty"`
	// DeltaX/DeltaY are the wheel spelling; ScrollX/ScrollY remain for compatibility.
	DeltaX   int  `json:"deltaX,omitempty"`
	DeltaY   int  `json:"deltaY,omitempty"`
	HasDelta bool `json:"hasDelta,omitempty"`
	DragX    int  `json:"dragX"`
	DragY    int  `json:"dragY"`

	// A drag's destination. ToSelector takes the same unified selector vocabulary as
	// Selector and the handler resolves it to ToNodeID; HasToXY separates an absent
	// destination from the legitimate one at (0,0).
	ToSelector string  `json:"toSelector,omitempty"`
	ToNodeID   int64   `json:"toNodeId,omitempty"`
	ToX        float64 `json:"toX,omitempty"`
	ToY        float64 `json:"toY,omitempty"`
	HasToXY    bool    `json:"hasToXY,omitempty"`

	WaitNav bool   `json:"waitNav"`
	Fast    bool   `json:"fast"`
	Owner   string `json:"owner"`
	// Submit is an explicit opt-in to a terminal action. Fill dispatches Enter
	// after filling; click uses one DOM click and lets the handler observe a
	// bounded post-submit state. Ordinary clicks retain their normal delivery.
	Submit bool `json:"submit,omitempty"`

	// DismissBanners, when true and combined with WaitNav, runs a best-effort
	// cookie/consent-banner dismissal pass after a click that triggered a
	// navigation. Bridge layer ignores this field; the handler reads it and
	// invokes the dismissal helper post-action. No effect on actions that
	// don't trigger navigation.
	DismissBanners bool `json:"dismissBanners,omitempty"`
	// DismissKnownInterstitials requests the conservative, catalog-backed
	// pre-action pass. Unlike DismissBanners it never removes generic overlays.
	DismissKnownInterstitials bool `json:"dismissKnownInterstitials,omitempty"`

	// Humanize, when set, overrides the per-instance `humanize` default for
	// this action only. nil = use the configured default. true forces the
	// bezier/jitter/pre-press-sleep code path; false forces the raw
	// straight-to-target dispatch.
	Humanize *bool `json:"humanize,omitempty"`

	// AutoSwitch, when explicitly set to false, disables the popup-aware
	// auto-switch behavior on click actions. nil or true = on (default). When
	// on, a click that opens a new tab adopts + focuses it and surfaces the
	// new tab ID on the response as "switchedToTab". Only click/dblclick
	// honor this field.
	AutoSwitch *bool `json:"autoSwitch,omitempty"`

	// DialogAction arms a one-shot dialog auto-handler before the action
	// executes. Used when clicking a button/link that opens a JS dialog
	// (alert/confirm/prompt). Values: "accept" or "dismiss". When set, the
	// dialog is handled automatically without a second HTTP call.
	DialogAction string `json:"dialogAction,omitempty"`
	// DialogText is the optional prompt text used when DialogAction is
	// "accept" on a prompt() dialog.
	DialogText string `json:"dialogText,omitempty"`

	// Browser specifies which browser to use for this request (e.g. "chrome",
	// "cloak", "ghost-chrome"). Validated against configured + registry
	// browsers. Recorded on route metadata but does not change actual
	// routing yet.
	Browser string `json:"browser,omitempty"`

	Vocab string `json:"vocab,omitempty"`
}

type actionRequestAlias ActionRequest

func (r *ActionRequest) UnmarshalJSON(data []byte) error {
	var alias actionRequestAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*r = ActionRequest(alias)
	r.Kind = CanonicalActionKind(r.Kind)
	r.HasXY = r.HasXY || hasJSONKey(raw, "x") || hasJSONKey(raw, "y")
	r.HasToXY = r.HasToXY || hasJSONKey(raw, "toX") || hasJSONKey(raw, "toY")
	r.HasScroll = r.HasScroll || hasJSONKey(raw, "scrollX") || hasJSONKey(raw, "scrollY")
	r.HasText = r.HasText || hasJSONKey(raw, "text") || hasJSONKey(raw, "value")
	if hasJSONKey(raw, "deltaX") {
		r.HasDelta = true
		if err := json.Unmarshal(raw["deltaX"], &r.DeltaX); err != nil {
			return err
		}
	}
	if hasJSONKey(raw, "deltaY") {
		r.HasDelta = true
		if err := json.Unmarshal(raw["deltaY"], &r.DeltaY); err != nil {
			return err
		}
	}
	return nil
}

// NormalizeSelector merges legacy Ref and Selector (CSS) fields into the
// unified Selector field. After calling this, only Selector needs to be
// inspected for element targeting. The method is idempotent.
//
// Priority: Ref > Selector (if both are set, Ref wins).
func (r *ActionRequest) NormalizeSelector() {
	if r.Ref != "" && r.Selector == "" {
		r.Selector = r.Ref
	}
}

func CanonicalActionKind(kind string) string {
	return kind
}

// IsSubmitClick reports whether an action requested the explicit once-only
// click-submit contract.
func IsSubmitClick(kind string, req ActionRequest) bool {
	return req.Submit && CanonicalActionKind(kind) == ActionClick
}

// ValidateSubmitAction rejects combinations that cannot preserve exactly-once
// submit dispatch. Fill's existing submit-via-Enter contract is unchanged.
func ValidateSubmitAction(kind string, req ActionRequest) error {
	if !req.Submit {
		return nil
	}
	kind = CanonicalActionKind(kind)
	switch kind {
	case ActionFill:
		return nil
	case ActionClick:
		if req.HasXY {
			return fmt.Errorf("click submit requires an element target; coordinates are not supported")
		}
		if req.WaitNav {
			return fmt.Errorf("click submit cannot be combined with waitNav")
		}
		if strings.TrimSpace(req.Mode) != "" {
			return fmt.Errorf("click submit cannot be combined with mode")
		}
		if req.Humanize != nil && *req.Humanize {
			return fmt.Errorf("click submit cannot be combined with humanize=true")
		}
		return nil
	default:
		return fmt.Errorf("submit is only supported for fill and click actions")
	}
}

// FillText is the one owner of what a fill writes: text, falling back to value,
// the same tolerance actionSelect has had in the other direction. Both are
// published request fields and no surface declared which one fill reads, so a
// caller sending the other spelling wrote nothing and was told it succeeded.
//
// The second return says whether anything was asked for at all, which is not the
// same as a non-empty answer: a supplied empty string clears the field.
func FillText(req ActionRequest) (string, bool) {
	if req.Text != "" {
		return req.Text, true
	}
	if req.Value != "" {
		return req.Value, true
	}
	return "", req.HasText
}

// SelectValue is FillText with the precedence reversed, and they stay two named functions
// because the precedence IS the difference — a shared helper taking a preference argument
// would hide it at both call sites.
//
// Supplied-ness reads HasText, which is inferred from key presence of text OR value, and
// those are exactly the two fields select reads. So an explicitly supplied empty string is
// a request to select the `<option value="">` placeholder, not an absent argument.
func SelectValue(req ActionRequest) (string, bool) {
	if req.Value != "" {
		return req.Value, true
	}
	if req.Text != "" {
		return req.Text, true
	}
	return "", req.HasText
}

// ValidateFillAction refuses a fill that carries no text under any spelling. That
// request used to write "" and answer filled:true with len:0 — indistinguishable
// from clearing a field on purpose, which is why it stayed invisible. Clearing
// stays available by sending the key explicitly.
func ValidateFillAction(kind string, req ActionRequest) error {
	if CanonicalActionKind(kind) != ActionFill {
		return nil
	}
	if _, supplied := FillText(req); !supplied {
		return fmt.Errorf(`fill requires "text" (or "value"); send "text": "" to clear the field`)
	}
	return nil
}

// ValidateButtonAction refuses a button name no pointer action can honour. It is not gated
// on the action kind, deliberately: the button is normalized inside cdpops for every action
// that dispatches a press, so a kind list here would have to be kept in step with that set
// and would leave any kind nobody thought of accepting a misspelling silently. An
// unrecognised name reached CDP as "left", so a mistyped right-click performed a left-click
// and reported success.
func ValidateButtonAction(kind string, req ActionRequest) error {
	return bridgecdpops.ValidateMouseButton(req.Button)
}

func hasJSONKey(raw map[string]json.RawMessage, key string) bool {
	_, ok := raw[key]
	return ok
}

func (b *Bridge) ExecuteAction(ctx context.Context, kind string, req ActionRequest) (map[string]any, error) {
	kind = CanonicalActionKind(kind)
	req.Kind = CanonicalActionKind(req.Kind)
	if req.Kind == "" {
		req.Kind = kind
	}
	if err := ValidateSubmitAction(kind, req); err != nil {
		return nil, err
	}
	if err := ValidateFillAction(kind, req); err != nil {
		return nil, err
	}
	if err := ValidateButtonAction(kind, req); err != nil {
		return nil, err
	}
	if kind == ActionClick && b.effectiveHumanize(req) && strings.TrimSpace(req.Mode) != "" {
		return nil, fmt.Errorf("mode and humanize are mutually exclusive")
	}
	fn, ok := b.Actions[kind]
	if !ok {
		return nil, fmt.Errorf("unknown action: %s", kind)
	}
	// Measured for every action, not for a selected few. This used to skip a
	// waitNav click and a submit click, from when the check RAISED an error and
	// refusing to error on a declared navigation was right. It only annotates the
	// result now, so both exclusions had stopped meaning anything except that the
	// two forms most likely to navigate were the two that never said where they
	// landed — a waiting caller got strictly less than a plain one, and refsStale
	// was missing from exactly the form whose next action depends on the new page.
	reportNavigation := b.Config == nil || b.Config.EnableActionGuards
	urlReader := b.URLReader
	if urlReader == nil {
		urlReader = defaultActionURLReader
		slog.Debug("URLReader is nil, using default fallback (guard checks may be no-ops without chromedp context)")
	}
	var beforeURL string
	if reportNavigation {
		if u, err := urlReader(ctx); err == nil {
			beforeURL = u
		}
	}

	res, err := fn(ctx, req)
	if err != nil {
		return nil, classifyActionError(err)
	}

	if reportNavigation && beforeURL != "" {
		if afterURL, uErr := urlReader(ctx); uErr == nil && navigationChanged(beforeURL, afterURL) {
			res = withNavigationOutcome(res, beforeURL, afterURL)
		}
	}

	return res, nil
}

// Navigated, LandedURL, PreviousURL and RefsStale are the keys a result grows
// when the action moved the page. They ride on the SUCCESSFUL result — the
// action ran, so the caller gets what it earned plus what it now needs to know:
// where it landed, and that every ref from its last snapshot is dead, because
// refs are minted per snapshot.
const (
	ResultNavigated   = "navigated"
	ResultLandedURL   = "url"
	ResultPreviousURL = "previousUrl"
	ResultRefsStale   = "refsStale"
)

// withNavigationOutcome never drops the action's own result: it is the value the
// API used to discard, and delivering it is the whole point.
func withNavigationOutcome(res map[string]any, before, after string) map[string]any {
	if res == nil {
		res = map[string]any{}
	}
	res[ResultNavigated] = true
	res[ResultLandedURL] = after
	res[ResultPreviousURL] = before
	res[ResultRefsStale] = true
	return res
}

func (b *Bridge) AvailableActions() []string {
	keys := make([]string, 0, len(b.Actions))
	for k := range b.Actions {
		keys = append(keys, k)
	}
	return keys
}
