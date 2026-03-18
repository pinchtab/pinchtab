package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
)

func (b *Bridge) actionFocus(ctx context.Context, req ActionRequest) (map[string]any, error) {
	if req.Selector != "" {
		return map[string]any{"focused": true}, chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery))
	}
	if req.NodeID > 0 {
		return map[string]any{"focused": true}, focusBackendNode(ctx, req.NodeID)
	}
	return map[string]any{"focused": true}, nil
}

func (b *Bridge) actionSelect(ctx context.Context, req ActionRequest) (map[string]any, error) {
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
}

func (b *Bridge) actionCheck(ctx context.Context, req ActionRequest) (map[string]any, error) {
	return checkUncheck(ctx, req, true)
}

func (b *Bridge) actionUncheck(ctx context.Context, req ActionRequest) (map[string]any, error) {
	return checkUncheck(ctx, req, false)
}

// checkUncheck implements both check and uncheck actions.
// If wantChecked is true, it ensures the element is checked; if false, unchecked.
// It only clicks if the current state differs from the desired state.
func checkUncheck(ctx context.Context, req ActionRequest, wantChecked bool) (map[string]any, error) {
	// Resolve the element to an objectId via backendNodeId or CSS selector.
	var resolveJS string
	if req.NodeID > 0 {
		// Resolve via backendNodeId using DOM.resolveNode (done below).
	} else if req.Selector != "" {
		resolveJS = req.Selector
	} else {
		return nil, fmt.Errorf("need selector, ref, or nodeId")
	}

	var objectID string
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if req.NodeID > 0 {
			var result json.RawMessage
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
				"backendNodeId": req.NodeID,
			}, &result); err != nil {
				return err
			}
			var resolved struct {
				Object struct {
					ObjectID string `json:"objectId"`
				} `json:"object"`
			}
			if err := json.Unmarshal(result, &resolved); err != nil {
				return err
			}
			objectID = resolved.Object.ObjectID
			return nil
		}
		// CSS selector path
		var evalResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    fmt.Sprintf(`document.querySelector(%q)`, resolveJS),
			"returnByValue": false,
		}, &evalResult); err != nil {
			return err
		}
		var er struct {
			Result struct {
				ObjectID string `json:"objectId"`
				Type     string `json:"type"`
				Subtype  string `json:"subtype"`
			} `json:"result"`
		}
		if err := json.Unmarshal(evalResult, &er); err != nil {
			return err
		}
		if er.Result.ObjectID == "" || er.Result.Subtype == "null" {
			return fmt.Errorf("element not found: %s", resolveJS)
		}
		objectID = er.Result.ObjectID
		return nil
	}))
	if err != nil {
		return nil, err
	}

	// Validate element is a checkbox or radio, and read current checked state.
	var isChecked bool
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		js := `function() {
			var tag = this.tagName ? this.tagName.toLowerCase() : "";
			var type = (this.type || "").toLowerCase();
			if (tag !== "input" || (type !== "checkbox" && type !== "radio")) {
				return {error: "element is not a checkbox or radio input (got " + tag + "[type=" + type + "])"};
			}
			return {checked: this.checked};
		}`
		var callResult json.RawMessage
		if err := chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": js,
			"objectId":            objectID,
			"returnByValue":       true,
		}, &callResult); err != nil {
			return err
		}
		var cr struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(callResult, &cr); err != nil {
			return err
		}
		var val struct {
			Error   string `json:"error"`
			Checked bool   `json:"checked"`
		}
		if err := json.Unmarshal(cr.Result.Value, &val); err != nil {
			return err
		}
		if val.Error != "" {
			return fmt.Errorf("%s", val.Error)
		}
		isChecked = val.Checked
		return nil
	}))
	if err != nil {
		return nil, err
	}

	// Click only if state needs to change.
	if isChecked != wantChecked {
		if req.NodeID > 0 {
			if err := ClickByNodeID(ctx, req.NodeID); err != nil {
				return nil, err
			}
		} else {
			if err := chromedp.Run(ctx, chromedp.Click(req.Selector, chromedp.ByQuery)); err != nil {
				return nil, err
			}
		}
	}

	return map[string]any{"checked": wantChecked}, nil
}
