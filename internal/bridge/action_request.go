package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// ActionFunc is the type for action handlers.
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
//
// For backward compatibility, the legacy Ref and Selector (CSS) fields
// are still accepted. Call NormalizeSelector() to merge them into the
// unified Selector field.
type ActionRequest struct {
	TabID    string `json:"tabId"`
	Kind     string `json:"kind"`
	Ref      string `json:"ref,omitempty"`
	Selector string `json:"selector,omitempty"`
	Text     string `json:"text"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	NodeID   int64  `json:"nodeId"`

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

	ScrollX int `json:"scrollX"`
	ScrollY int `json:"scrollY"`
	// DeltaX/DeltaY are explicit mouse-wheel deltas for low-level
	// mouse-wheel actions. ScrollX/ScrollY remain for backward compatibility.
	DeltaX int `json:"deltaX,omitempty"`
	DeltaY int `json:"deltaY,omitempty"`
	DragX  int `json:"dragX"`
	DragY  int `json:"dragY"`

	WaitNav bool   `json:"waitNav"`
	Fast    bool   `json:"fast"`
	Owner   string `json:"owner"`

	// DialogAction arms a one-shot dialog auto-handler before the action
	// executes. Used when clicking a button/link that opens a JS dialog
	// (alert/confirm/prompt). Values: "accept" or "dismiss". When set, the
	// dialog is handled automatically without a second HTTP call.
	DialogAction string `json:"dialogAction,omitempty"`
	// DialogText is the optional prompt text used when DialogAction is
	// "accept" on a prompt() dialog.
	DialogText string `json:"dialogText,omitempty"`
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
	if hasJSONKey(raw, "deltaX") {
		if err := json.Unmarshal(raw["deltaX"], &r.DeltaX); err != nil {
			return err
		}
	}
	if hasJSONKey(raw, "deltaY") {
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

func hasJSONKey(raw map[string]json.RawMessage, key string) bool {
	_, ok := raw[key]
	return ok
}

func (b *Bridge) ExecuteAction(ctx context.Context, kind string, req ActionRequest) (map[string]any, error) {
	kind = CanonicalActionKind(kind)
	req.Kind = CanonicalActionKind(req.Kind)
	fn, ok := b.Actions[kind]
	if !ok {
		return nil, fmt.Errorf("unknown action: %s", kind)
	}
	guardEnabled := b.Config == nil || b.Config.EnableActionGuards
	checkNav := guardEnabled && shouldCheckUnexpectedNavigation(req)
	urlReader := b.URLReader
	if urlReader == nil {
		urlReader = defaultActionURLReader
		slog.Debug("URLReader is nil, using default fallback (guard checks may be no-ops without chromedp context)")
	}
	var beforeURL string
	if checkNav {
		if u, err := urlReader(ctx); err == nil {
			beforeURL = u
		}
	}

	res, err := fn(ctx, req)
	if err != nil {
		return nil, classifyActionError(err)
	}

	if checkNav && beforeURL != "" {
		afterURL, uErr := urlReader(ctx)
		if uErr == nil {
			if navErr := checkUnexpectedNavigation(beforeURL, afterURL); navErr != nil {
				return nil, navErr
			}
		}
	}

	return res, nil
}

func (b *Bridge) AvailableActions() []string {
	keys := make([]string, 0, len(b.Actions))
	for k := range b.Actions {
		keys = append(keys, k)
	}
	return keys
}
