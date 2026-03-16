package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/human"
	"github.com/pinchtab/pinchtab/internal/selector"
)

const (
	ActionClick       = "click"
	ActionDoubleClick = "dblclick"
	ActionType        = "type"
	ActionFill        = "fill"
	ActionPress       = "press"
	ActionFocus       = "focus"
	ActionHover       = "hover"
	ActionSelect      = "select"
	ActionScroll      = "scroll"
	ActionDrag        = "drag"
	ActionHumanClick  = "humanClick"
	ActionHumanType   = "humanType"
)

func (b *Bridge) InitActionRegistry() {
	b.Actions = map[string]ActionFunc{
		ActionClick: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			var err error
			if req.Selector != "" {
				err = chromedp.Run(ctx, chromedp.Click(req.Selector, chromedp.ByQuery))
			} else if req.NodeID > 0 {
				err = ClickByNodeID(ctx, req.NodeID)
			} else if req.HasXY {
				err = ClickByCoordinate(ctx, req.X, req.Y)
			} else {
				return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
			}
			if err != nil {
				return nil, err
			}
			if req.WaitNav {
				_ = chromedp.Run(ctx, chromedp.Sleep(b.Config.WaitNavDelay))
			}
			return map[string]any{"clicked": true}, nil
		},
		ActionDoubleClick: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			var err error
			if req.Selector != "" {
				err = chromedp.Run(ctx, chromedp.DoubleClick(req.Selector, chromedp.ByQuery))
			} else if req.NodeID > 0 {
				err = DoubleClickByNodeID(ctx, req.NodeID)
			} else if req.HasXY {
				err = DoubleClickByCoordinate(ctx, req.X, req.Y)
			} else {
				return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
			}
			if err != nil {
				return nil, err
			}
			return map[string]any{"doubleclicked": true}, nil
		},
		ActionType: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.Text == "" {
				return nil, fmt.Errorf("text required for type")
			}
			if req.Selector != "" {
				return map[string]any{"typed": req.Text}, chromedp.Run(ctx,
					chromedp.Click(req.Selector, chromedp.ByQuery),
					chromedp.SendKeys(req.Selector, req.Text, chromedp.ByQuery),
				)
			}
			if req.NodeID > 0 {
				return map[string]any{"typed": req.Text}, TypeByNodeID(ctx, req.NodeID, req.Text)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		ActionFill: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.Selector != "" {
				return map[string]any{"filled": req.Text}, chromedp.Run(ctx, chromedp.SetValue(req.Selector, req.Text, chromedp.ByQuery))
			}
			if req.NodeID > 0 {
				return map[string]any{"filled": req.Text}, FillByNodeID(ctx, req.NodeID, req.Text)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		ActionPress: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.Key == "" {
				return nil, fmt.Errorf("key required for press")
			}
			return map[string]any{"pressed": req.Key}, DispatchNamedKey(ctx, req.Key)
		},
		ActionFocus: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.Selector != "" {
				return map[string]any{"focused": true}, chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery))
			}
			if req.NodeID > 0 {
				return map[string]any{"focused": true}, chromedp.Run(ctx,
					chromedp.ActionFunc(func(ctx context.Context) error {
						p := map[string]any{"backendNodeId": req.NodeID}
						return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", p, nil)
					}),
				)
			}
			return map[string]any{"focused": true}, nil
		},
		ActionHover: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.NodeID > 0 {
				return map[string]any{"hovered": true}, HoverByNodeID(ctx, req.NodeID)
			}
			if req.Selector != "" {
				return map[string]any{"hovered": true}, chromedp.Run(ctx,
					chromedp.Evaluate(fmt.Sprintf(`document.querySelector(%q)?.dispatchEvent(new MouseEvent('mouseover', {bubbles:true}))`, req.Selector), nil),
				)
			}
			if req.HasXY {
				return map[string]any{"hovered": true}, HoverByCoordinate(ctx, req.X, req.Y)
			}
			return nil, fmt.Errorf("need selector, ref, nodeId, or x/y coordinates")
		},
		ActionSelect: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			val := req.Value
			if val == "" {
				val = req.Text
			}
			if val == "" {
				return nil, fmt.Errorf("value required for select")
			}
			if req.NodeID > 0 {
				return map[string]any{"selected": val}, SelectByNodeID(ctx, req.NodeID, val)
			}
			if req.Selector != "" {
				return map[string]any{"selected": val}, chromedp.Run(ctx,
					chromedp.SetValue(req.Selector, val, chromedp.ByQuery),
				)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		ActionScroll: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.NodeID > 0 {
				return map[string]any{"scrolled": true}, ScrollByNodeID(ctx, req.NodeID)
			}
			if req.ScrollX != 0 || req.ScrollY != 0 {
				js := fmt.Sprintf("window.scrollBy(%d, %d)", req.ScrollX, req.ScrollY)
				return map[string]any{"scrolled": true, "x": req.ScrollX, "y": req.ScrollY},
					chromedp.Run(ctx, chromedp.Evaluate(js, nil))
			}
			return map[string]any{"scrolled": true, "y": 800},
				chromedp.Run(ctx, chromedp.Evaluate("window.scrollBy(0, 800)", nil))
		},
		ActionDrag: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.DragX == 0 && req.DragY == 0 {
				return nil, fmt.Errorf("dragX or dragY required for drag")
			}
			if req.NodeID > 0 {
				err := DragByNodeID(ctx, req.NodeID, req.DragX, req.DragY)
				if err != nil {
					return nil, err
				}
				return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY}, nil
			}
			if req.Selector != "" {
				var nodes []*cdp.Node
				if err := chromedp.Run(ctx,
					chromedp.Nodes(req.Selector, &nodes, chromedp.ByQuery),
				); err != nil {
					return nil, err
				}
				if len(nodes) == 0 {
					return nil, fmt.Errorf("element not found: %s", req.Selector)
				}
				err := DragByNodeID(ctx, int64(nodes[0].BackendNodeID), req.DragX, req.DragY)
				if err != nil {
					return nil, err
				}
				return map[string]any{"dragged": true, "dragX": req.DragX, "dragY": req.DragY}, nil
			}
			return nil, fmt.Errorf("need selector, ref, or nodeId")
		},
		ActionHumanClick: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.NodeID > 0 {
				// req.NodeID is a backendDOMNodeId from the accessibility tree
				if err := human.ClickElement(ctx, cdp.BackendNodeID(req.NodeID)); err != nil {
					return nil, err
				}
				return map[string]any{"clicked": true, "human": true}, nil
			}
			if req.Selector != "" {
				var nodes []*cdp.Node
				if err := chromedp.Run(ctx,
					chromedp.Nodes(req.Selector, &nodes, chromedp.ByQuery),
				); err != nil {
					return nil, err
				}
				if len(nodes) == 0 {
					return nil, fmt.Errorf("element not found: %s", req.Selector)
				}
				// Use BackendNodeID from the DOM node
				if err := human.ClickElement(ctx, nodes[0].BackendNodeID); err != nil {
					return nil, err
				}
				return map[string]any{"clicked": true, "human": true}, nil
			}
			return nil, fmt.Errorf("need selector, ref, or nodeId")
		},
		ActionHumanType: func(ctx context.Context, req ActionRequest) (map[string]any, error) {
			if req.Text == "" {
				return nil, fmt.Errorf("text required for humanType")
			}

			if req.Selector != "" {
				if err := chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery)); err != nil {
					return nil, err
				}
			} else if req.NodeID > 0 {
				// req.NodeID is a BackendNodeID from the accessibility tree (same as humanClick).
				// Must use DOM.focus with backendNodeId, not dom.Focus().WithNodeID() which
				// expects a DOM NodeID — a different ID space. Using the wrong type causes
				// "Could not find node with given id (-32000)". See issue #226.
				if err := chromedp.Run(ctx,
					chromedp.ActionFunc(func(ctx context.Context) error {
						return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", map[string]any{"backendNodeId": req.NodeID}, nil)
					}),
				); err != nil {
					return nil, err
				}
			} else {
				return nil, fmt.Errorf("need selector, ref, or nodeId")
			}

			actions := human.Type(req.Text, req.Fast)
			if err := chromedp.Run(ctx, actions...); err != nil {
				return nil, err
			}

			return map[string]any{"typed": req.Text, "human": true}, nil
		},
	}
}

// ResolveXPathToNodeID resolves an XPath expression to a backend node ID
// using CDP's DOM.performSearch + DOM.getSearchResults.
func ResolveXPathToNodeID(ctx context.Context, xpath string) (int64, error) {
	var backendNodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Use DOM.getDocument first to ensure the DOM is available.
		var docResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getDocument", map[string]any{"depth": 0}, &docResult); err != nil {
			return fmt.Errorf("get document: %w", err)
		}

		// Perform XPath search.
		var searchResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.performSearch", map[string]any{
			"query": xpath,
		}, &searchResult); err != nil {
			return fmt.Errorf("xpath search: %w", err)
		}

		var sr struct {
			SearchID    string `json:"searchId"`
			ResultCount int    `json:"resultCount"`
		}
		if err := json.Unmarshal(searchResult, &sr); err != nil {
			return err
		}
		if sr.ResultCount == 0 {
			return fmt.Errorf("xpath %q: no elements found", xpath)
		}

		// Get the first result.
		var getResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getSearchResults", map[string]any{
			"searchId":  sr.SearchID,
			"fromIndex": 0,
			"toIndex":   1,
		}, &getResult); err != nil {
			return fmt.Errorf("get search results: %w", err)
		}

		var gr struct {
			NodeIDs []int64 `json:"nodeIds"`
		}
		if err := json.Unmarshal(getResult, &gr); err != nil {
			return err
		}
		if len(gr.NodeIDs) == 0 {
			return fmt.Errorf("xpath %q: no node IDs returned", xpath)
		}

		// Convert DOM NodeID → BackendNodeID.
		var descResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{
			"nodeId": gr.NodeIDs[0],
		}, &descResult); err != nil {
			return fmt.Errorf("describe node: %w", err)
		}

		var desc struct {
			Node struct {
				BackendNodeID int64 `json:"backendNodeId"`
			} `json:"node"`
		}
		if err := json.Unmarshal(descResult, &desc); err != nil {
			return err
		}
		backendNodeID = desc.Node.BackendNodeID

		// Clean up the search.
		_ = chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.discardSearchResults", map[string]any{
			"searchId": sr.SearchID,
		}, nil)

		return nil
	}))
	return backendNodeID, err
}

// ResolveTextToNodeID finds the first element whose visible text content
// contains the given string and returns its backend node ID.
func ResolveTextToNodeID(ctx context.Context, text string) (int64, error) {
	// Use XPath with contains(text(), ...) for text matching.
	xpath := fmt.Sprintf(`//*[contains(text(), %s)]`, xpathString(text))
	return ResolveXPathToNodeID(ctx, xpath)
}

// ResolveCSSToNodeID resolves a CSS selector to a backend node ID.
func ResolveCSSToNodeID(ctx context.Context, css string) (int64, error) {
	var backendNodeID int64
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var docResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.getDocument", map[string]any{"depth": 0}, &docResult); err != nil {
			return fmt.Errorf("get document: %w", err)
		}
		var doc struct {
			Root struct {
				NodeID int64 `json:"nodeId"`
			} `json:"root"`
		}
		if err := json.Unmarshal(docResult, &doc); err != nil {
			return err
		}

		var qResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.querySelector", map[string]any{
			"nodeId":   doc.Root.NodeID,
			"selector": css,
		}, &qResult); err != nil {
			return fmt.Errorf("querySelector: %w", err)
		}
		var qr struct {
			NodeID int64 `json:"nodeId"`
		}
		if err := json.Unmarshal(qResult, &qr); err != nil {
			return err
		}
		if qr.NodeID == 0 {
			return fmt.Errorf("css %q: no element found", css)
		}

		var descResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.describeNode", map[string]any{
			"nodeId": qr.NodeID,
		}, &descResult); err != nil {
			return fmt.Errorf("describe node: %w", err)
		}
		var desc struct {
			Node struct {
				BackendNodeID int64 `json:"backendNodeId"`
			} `json:"node"`
		}
		if err := json.Unmarshal(descResult, &desc); err != nil {
			return err
		}
		backendNodeID = desc.Node.BackendNodeID
		return nil
	}))
	return backendNodeID, err
}

// ResolveUnifiedSelector resolves a parsed selector to a backend node ID.
// For ref selectors, the refCache is consulted. For CSS, XPath, and text
// selectors, CDP is used directly. Semantic selectors are not resolved here
// (they require the semantic matcher at a higher layer).
func ResolveUnifiedSelector(ctx context.Context, sel selector.Selector, refCache *RefCache) (int64, error) {
	switch sel.Kind {
	case selector.KindRef:
		if refCache != nil {
			if nid, ok := refCache.Refs[sel.Value]; ok {
				return nid, nil
			}
		}
		return 0, fmt.Errorf("ref %s not found in snapshot cache", sel.Value)

	case selector.KindCSS:
		return ResolveCSSToNodeID(ctx, sel.Value)

	case selector.KindXPath:
		return ResolveXPathToNodeID(ctx, sel.Value)

	case selector.KindText:
		return ResolveTextToNodeID(ctx, sel.Value)

	case selector.KindSemantic:
		return 0, fmt.Errorf("semantic selectors must be resolved at the handler layer via /find")

	default:
		return 0, fmt.Errorf("unknown selector kind: %q", sel.Kind)
	}
}

// xpathString returns an XPath-safe string literal using concat().
// This safely handles all combinations of quotes and special characters.
func xpathString(s string) string {
	// Always use concat() for safety — handles any quote combination.
	// Split on single quotes and rejoin with quoted single quote segments.
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	// Use concat to handle strings with single quotes.
	// e.g., "it's" becomes concat('it', "'", 's')
	parts := strings.Split(s, "'")
	var result strings.Builder
	result.WriteString("concat(")
	for i, part := range parts {
		if i > 0 {
			result.WriteString(`, "'", `)
		}
		result.WriteString("'")
		result.WriteString(part)
		result.WriteString("'")
	}
	result.WriteString(")")
	return result.String()
}
