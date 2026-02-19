package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
)

type actionRequest struct {
	TabID    string `json:"tabId"`
	Kind     string `json:"kind"`
	Ref      string `json:"ref"`
	Selector string `json:"selector"`
	Text     string `json:"text"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	NodeID   int64  `json:"nodeId"`
	ScrollX  int    `json:"scrollX"`
	ScrollY  int    `json:"scrollY"`
	WaitNav  bool   `json:"waitNav"`
	Fast     bool   `json:"fast"`
}

type ActionFunc func(ctx context.Context, req actionRequest) (map[string]any, error)

func (b *Bridge) initActionRegistry() {
	b.actions = map[string]ActionFunc{
		actionClick: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			var err error
			if req.Selector != "" {
				err = chromedp.Run(ctx, chromedp.Click(req.Selector, chromedp.ByQuery))
			} else if req.NodeID > 0 {
				err = clickByNodeID(ctx, req.NodeID)
			} else {
				return nil, fmt.Errorf("need selector, ref, or nodeId")
			}
			if err != nil {
				return nil, err
			}
			if req.WaitNav {
				_ = chromedp.Run(ctx, chromedp.Sleep(cfg.WaitNavDelay))
			}
			return map[string]any{"clicked": true}, nil
		},
		actionType: func(ctx context.Context, req actionRequest) (map[string]any, error) {
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
				return map[string]any{"typed": req.Text}, typeByNodeID(ctx, req.NodeID, req.Text)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		actionFill: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.Selector != "" {
				return map[string]any{"filled": req.Text}, chromedp.Run(ctx, chromedp.SetValue(req.Selector, req.Text, chromedp.ByQuery))
			}
			return map[string]any{"filled": req.Text}, nil
		},
		actionPress: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.Key == "" {
				return nil, fmt.Errorf("key required for press")
			}
			return map[string]any{"pressed": req.Key}, chromedp.Run(ctx, chromedp.KeyEvent(req.Key))
		},
		actionFocus: func(ctx context.Context, req actionRequest) (map[string]any, error) {
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
		actionHover: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.NodeID > 0 {
				return map[string]any{"hovered": true}, hoverByNodeID(ctx, req.NodeID)
			}
			if req.Selector != "" {
				return map[string]any{"hovered": true}, chromedp.Run(ctx,
					chromedp.Evaluate(fmt.Sprintf(`document.querySelector(%q)?.dispatchEvent(new MouseEvent('mouseover', {bubbles:true}))`, req.Selector), nil),
				)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		actionSelect: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			val := req.Value
			if val == "" {
				val = req.Text
			}
			if val == "" {
				return nil, fmt.Errorf("value required for select")
			}
			if req.NodeID > 0 {
				return map[string]any{"selected": val}, selectByNodeID(ctx, req.NodeID, val)
			}
			if req.Selector != "" {
				return map[string]any{"selected": val}, chromedp.Run(ctx,
					chromedp.SetValue(req.Selector, val, chromedp.ByQuery),
				)
			}
			return nil, fmt.Errorf("need selector or ref")
		},
		actionScroll: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.NodeID > 0 {
				return map[string]any{"scrolled": true}, scrollByNodeID(ctx, req.NodeID)
			}
			if req.ScrollX != 0 || req.ScrollY != 0 {
				js := fmt.Sprintf("window.scrollBy(%d, %d)", req.ScrollX, req.ScrollY)
				return map[string]any{"scrolled": true, "x": req.ScrollX, "y": req.ScrollY},
					chromedp.Run(ctx, chromedp.Evaluate(js, nil))
			}
			return map[string]any{"scrolled": true, "y": 800},
				chromedp.Run(ctx, chromedp.Evaluate("window.scrollBy(0, 800)", nil))
		},
		actionHumanClick: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.NodeID > 0 {
				if err := humanClickElement(ctx, cdp.NodeID(req.NodeID)); err != nil {
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
				if err := humanClickElement(ctx, nodes[0].NodeID); err != nil {
					return nil, err
				}
				return map[string]any{"clicked": true, "human": true}, nil
			}
			return nil, fmt.Errorf("need selector, ref, or nodeId")
		},
		actionHumanType: func(ctx context.Context, req actionRequest) (map[string]any, error) {
			if req.Text == "" {
				return nil, fmt.Errorf("text required for humanType")
			}

			if req.Selector != "" {
				if err := chromedp.Run(ctx, chromedp.Focus(req.Selector, chromedp.ByQuery)); err != nil {
					return nil, err
				}
			} else if req.NodeID > 0 {
				if err := chromedp.Run(ctx,
					chromedp.ActionFunc(func(ctx context.Context) error {
						return dom.Focus().WithNodeID(cdp.NodeID(req.NodeID)).Do(ctx)
					}),
				); err != nil {
					return nil, err
				}
			} else {
				return nil, fmt.Errorf("need selector, ref, or nodeId")
			}

			actions := humanType(req.Text, req.Fast)
			if err := chromedp.Run(ctx, actions...); err != nil {
				return nil, err
			}

			return map[string]any{"typed": req.Text, "human": true}, nil
		},
	}
}

func (b *Bridge) handleAction(w http.ResponseWriter, r *http.Request) {
	var req actionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	ctx, resolvedTabID, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, cfg.ActionTimeout)
	defer tCancel()
	go cancelOnClientDone(r.Context(), tCancel)

	if req.Ref != "" && req.NodeID == 0 && req.Selector == "" {
		cache := b.GetRefCache(resolvedTabID)
		if cache != nil {
			if nid, ok := cache.refs[req.Ref]; ok {
				req.NodeID = nid
			}
		}
		if req.NodeID == 0 {
			jsonResp(w, 400, map[string]string{
				"error": fmt.Sprintf("ref %s not found - take a /snapshot first", req.Ref),
			})
			return
		}
	}

	registry := b.actions
	if req.Kind == "" {
		kinds := make([]string, 0, len(registry))
		for k := range registry {
			kinds = append(kinds, k)
		}
		jsonResp(w, 400, map[string]string{
			"error": fmt.Sprintf("missing required field 'kind' - valid values: %s", strings.Join(kinds, ", ")),
		})
		return
	}
	fn, ok := registry[req.Kind]
	if !ok {
		kinds := make([]string, 0, len(registry))
		for k := range registry {
			kinds = append(kinds, k)
		}
		jsonResp(w, 400, map[string]string{
			"error": fmt.Sprintf("unknown action: %s - valid values: %s", req.Kind, strings.Join(kinds, ", ")),
		})
		return
	}

	result, err := fn(tCtx, req)
	if err != nil {
		jsonErr(w, 500, fmt.Errorf("action %s: %w", req.Kind, err))
		return
	}

	jsonResp(w, 200, result)
}

type actionsRequest struct {
	TabID       string          `json:"tabId"`
	Actions     []actionRequest `json:"actions"`
	StopOnError bool            `json:"stopOnError"`
}

type actionResult struct {
	Index   int            `json:"index"`
	Success bool           `json:"success"`
	Result  map[string]any `json:"result,omitempty"`
	Error   string         `json:"error,omitempty"`
}

func (b *Bridge) handleActions(w http.ResponseWriter, r *http.Request) {
	var req actionsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		jsonErr(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if len(req.Actions) == 0 {
		jsonErr(w, 400, fmt.Errorf("actions array is empty"))
		return
	}

	ctx, resolvedTabID, err := b.TabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	results := make([]actionResult, 0, len(req.Actions))
	registry := b.actions

	for i, action := range req.Actions {
		if action.TabID == "" {
			action.TabID = resolvedTabID
		} else if action.TabID != resolvedTabID {
			ctx, resolvedTabID, err = b.TabContext(action.TabID)
			if err != nil {
				results = append(results, actionResult{
					Index: i, Success: false,
					Error: fmt.Sprintf("tab not found: %v", err),
				})
				if req.StopOnError {
					break
				}
				continue
			}
		}

		tCtx, tCancel := context.WithTimeout(ctx, cfg.ActionTimeout)

		if action.Ref != "" && action.NodeID == 0 && action.Selector == "" {
			cache := b.GetRefCache(resolvedTabID)
			if cache != nil {
				if nid, ok := cache.refs[action.Ref]; ok {
					action.NodeID = nid
				}
			}
			if action.NodeID == 0 {
				tCancel()
				results = append(results, actionResult{
					Index: i, Success: false,
					Error: fmt.Sprintf("ref %s not found - take a /snapshot first", action.Ref),
				})
				if req.StopOnError {
					break
				}
				continue
			}
		}

		if action.Kind == "" {
			tCancel()
			results = append(results, actionResult{
				Index: i, Success: false, Error: "missing required field 'kind'",
			})
			if req.StopOnError {
				break
			}
			continue
		}

		fn, ok := registry[action.Kind]
		if !ok {
			tCancel()
			kinds := make([]string, 0, len(registry))
			for k := range registry {
				kinds = append(kinds, k)
			}
			results = append(results, actionResult{
				Index: i, Success: false,
				Error: fmt.Sprintf("unknown action: %s - valid values: %s", action.Kind, strings.Join(kinds, ", ")),
			})
			if req.StopOnError {
				break
			}
			continue
		}

		actionRes, err := fn(tCtx, action)
		tCancel()

		if err != nil {
			results = append(results, actionResult{
				Index: i, Success: false,
				Error: fmt.Sprintf("action %s: %v", action.Kind, err),
			})
			if req.StopOnError {
				break
			}
		} else {
			results = append(results, actionResult{
				Index: i, Success: true, Result: actionRes,
			})
		}

		if i < len(req.Actions)-1 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	jsonResp(w, 200, map[string]any{
		"results":    results,
		"total":      len(req.Actions),
		"successful": countSuccessful(results),
		"failed":     len(req.Actions) - countSuccessful(results),
	})
}

func countSuccessful(results []actionResult) int {
	count := 0
	for _, r := range results {
		if r.Success {
			count++
		}
	}
	return count
}
