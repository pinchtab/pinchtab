package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/web"
)

func resolveOwner(r *http.Request, fallback string) string {
	if o := strings.TrimSpace(r.Header.Get("X-Owner")); o != "" {
		return o
	}
	if o := strings.TrimSpace(r.URL.Query().Get("owner")); o != "" {
		return o
	}
	return strings.TrimSpace(fallback)
}

func (h *Handlers) enforceTabLease(tabID, owner string) error {
	if tabID == "" {
		return nil
	}
	lock := h.Bridge.TabLockInfo(tabID)
	if lock == nil {
		return nil
	}
	if owner == "" {
		return fmt.Errorf("tab %s is locked by %s; owner required", tabID, lock.Owner)
	}
	if owner != lock.Owner {
		return fmt.Errorf("tab %s is locked by %s", tabID, lock.Owner)
	}
	return nil
}

func (h *Handlers) HandleAction(w http.ResponseWriter, r *http.Request) {
	var req bridge.ActionRequest
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		req.Kind = q.Get("kind")
		req.TabID = q.Get("tabId")
		req.Owner = q.Get("owner")
		req.Ref = q.Get("ref")
		req.Selector = q.Get("selector")
		req.Text = q.Get("text")
		req.Value = q.Get("value")
		req.Key = q.Get("key")
		if v := q.Get("nodeId"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				req.NodeID = n
			}
		}
	} else {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
			web.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
	}

	ctx, resolvedTabID, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}
	owner := resolveOwner(r, req.Owner)
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		web.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
		return
	}

	actionTimeout := h.Config.ActionTimeout
	if r.Method == http.MethodGet {
		if v := r.URL.Query().Get("timeout"); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				if n > 0 && n <= 60 {
					actionTimeout = time.Duration(n * float64(time.Second))
				}
			}
		}
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()
	go web.CancelOnClientDone(r.Context(), tCancel)

	if req.Ref != "" && req.NodeID == 0 && req.Selector == "" {
		cache := h.Bridge.GetRefCache(resolvedTabID)
		if cache != nil {
			if nid, ok := cache.Refs[req.Ref]; ok {
				req.NodeID = nid
			}
		}
		if req.NodeID == 0 {
			web.JSON(w, 400, map[string]string{
				"error": fmt.Sprintf("ref %s not found - take a /snapshot first", req.Ref),
			})
			return
		}
	}

	if req.Kind == "" {
		kinds := h.Bridge.AvailableActions()
		web.JSON(w, 400, map[string]string{
			"error": fmt.Sprintf("missing required field 'kind' - valid values: %s", strings.Join(kinds, ", ")),
		})
		return
	}

	result, err := h.Bridge.ExecuteAction(tCtx, req.Kind, req)
	if err != nil && req.Ref != "" && shouldRetryStaleRef(err) {
		recordStaleRefRetry()
		h.refreshRefCache(tCtx, resolvedTabID)
		if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
			if nid, ok := cache.Refs[req.Ref]; ok {
				req.NodeID = nid
				result, err = h.Bridge.ExecuteAction(tCtx, req.Kind, req)
			}
		}
	}
	if err != nil {
		if strings.HasPrefix(err.Error(), "unknown action") {
			kinds := h.Bridge.AvailableActions()
			web.JSON(w, 400, map[string]string{
				"error": fmt.Sprintf("%s - valid values: %s", err.Error(), strings.Join(kinds, ", ")),
			})
			return
		}
		web.ErrorCode(w, 500, "action_failed", fmt.Sprintf("action %s: %v", req.Kind, err), true, nil)
		return
	}

	web.JSON(w, 200, result)
}

type actionsRequest struct {
	TabID       string                 `json:"tabId"`
	Owner       string                 `json:"owner"`
	Actions     []bridge.ActionRequest `json:"actions"`
	StopOnError bool                   `json:"stopOnError"`
}

type actionResult struct {
	Index   int            `json:"index"`
	Success bool           `json:"success"`
	Result  map[string]any `json:"result,omitempty"`
	Error   string         `json:"error,omitempty"`
}

func (h *Handlers) HandleActions(w http.ResponseWriter, r *http.Request) {
	var req actionsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if len(req.Actions) == 0 {
		web.Error(w, 400, fmt.Errorf("actions array is empty"))
		return
	}

	ctx, resolvedTabID, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}
	owner := resolveOwner(r, req.Owner)
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		web.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
		return
	}

	results := make([]actionResult, 0, len(req.Actions))

	for i, action := range req.Actions {
		if action.TabID == "" {
			action.TabID = resolvedTabID
		} else if action.TabID != resolvedTabID {
			ctx, resolvedTabID, err = h.Bridge.TabContext(action.TabID)
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
			if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
				results = append(results, actionResult{Index: i, Success: false, Error: err.Error()})
				if req.StopOnError {
					break
				}
				continue
			}
		}

		tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)

		if action.Ref != "" && action.NodeID == 0 && action.Selector == "" {
			cache := h.Bridge.GetRefCache(resolvedTabID)
			if cache != nil {
				if nid, ok := cache.Refs[action.Ref]; ok {
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

		actionRes, err := h.Bridge.ExecuteAction(tCtx, action.Kind, action)
		if err != nil && action.Ref != "" && shouldRetryStaleRef(err) {
			recordStaleRefRetry()
			h.refreshRefCache(tCtx, resolvedTabID)
			if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
				if nid, ok := cache.Refs[action.Ref]; ok {
					action.NodeID = nid
					actionRes, err = h.Bridge.ExecuteAction(tCtx, action.Kind, action)
				}
			}
		}
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

	web.JSON(w, 200, map[string]any{
		"results":    results,
		"total":      len(req.Actions),
		"successful": countSuccessful(results),
		"failed":     len(req.Actions) - countSuccessful(results),
	})
}

func (h *Handlers) HandleMacro(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID       string                 `json:"tabId"`
		Owner       string                 `json:"owner"`
		Steps       []bridge.ActionRequest `json:"steps"`
		StopOnError bool                   `json:"stopOnError"`
		StepTimeout float64                `json:"stepTimeout"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.ErrorCode(w, 400, "bad_request", fmt.Sprintf("decode: %v", err), false, nil)
		return
	}
	if len(req.Steps) == 0 {
		web.ErrorCode(w, 400, "bad_request", "steps array is empty", false, nil)
		return
	}
	owner := resolveOwner(r, req.Owner)
	stepTimeout := h.Config.ActionTimeout
	if req.StepTimeout > 0 && req.StepTimeout <= 60 {
		stepTimeout = time.Duration(req.StepTimeout * float64(time.Second))
	}

	ctx, resolvedTabID, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		web.Error(w, 404, err)
		return
	}
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		web.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
		return
	}

	results := make([]actionResult, 0, len(req.Steps))
	for i, step := range req.Steps {
		if step.TabID == "" {
			step.TabID = resolvedTabID
		}
		tCtx, cancel := context.WithTimeout(ctx, stepTimeout)
		res, err := h.Bridge.ExecuteAction(tCtx, step.Kind, step)
		if err != nil && step.Ref != "" && shouldRetryStaleRef(err) {
			recordStaleRefRetry()
			h.refreshRefCache(tCtx, resolvedTabID)
			if cache := h.Bridge.GetRefCache(resolvedTabID); cache != nil {
				if nid, ok := cache.Refs[step.Ref]; ok {
					step.NodeID = nid
					res, err = h.Bridge.ExecuteAction(tCtx, step.Kind, step)
				}
			}
		}
		cancel()
		if err != nil {
			results = append(results, actionResult{Index: i, Success: false, Error: err.Error()})
			if req.StopOnError {
				break
			}
			continue
		}
		results = append(results, actionResult{Index: i, Success: true, Result: res})
	}

	web.JSON(w, 200, map[string]any{
		"kind":       "macro",
		"results":    results,
		"total":      len(req.Steps),
		"successful": countSuccessful(results),
		"failed":     len(req.Steps) - countSuccessful(results),
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

func shouldRetryStaleRef(err error) bool {
	if err == nil {
		return false
	}
	e := strings.ToLower(err.Error())
	return strings.Contains(e, "could not find node") || strings.Contains(e, "node with given id") || strings.Contains(e, "no node")
}

func (h *Handlers) refreshRefCache(ctx context.Context, tabID string) {
	var rawResult json.RawMessage
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(c context.Context) error {
			return chromedp.FromContext(c).Target.Execute(c, "Accessibility.getFullAXTree", nil, &rawResult)
		}),
	); err != nil {
		return
	}
	var treeResp struct {
		Nodes []bridge.RawAXNode `json:"nodes"`
	}
	if err := json.Unmarshal(rawResult, &treeResp); err != nil {
		return
	}
	flat, refs := bridge.BuildSnapshot(treeResp.Nodes, bridge.FilterInteractive, -1)
	h.Bridge.SetRefCache(tabID, &bridge.RefCache{Refs: refs, Nodes: flat})
}
