package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/web"
)

// HandleAction performs a single action on a tab (click, type, fill, etc).
//
// @Endpoint POST /action
// @Description Interact with page elements: click, type text, fill inputs, press keys, hover, focus, scroll, select
//
// @Param tabId string body Tab ID (required)
// @Param kind string body Action type: "click", "type", "fill", "press", "hover", "focus", "scroll", "select" (required)
// @Param ref string body Element reference from snapshot (e.g., "e5") (required)
// @Param text string body Text to type or fill (for "type"/"fill" actions)
// @Param value string body Value for "select" action (e.g., option index)
// @Param key string body Key to press (for "press" action, e.g., "Enter", "Tab")
// @Param x int body X coordinate for "scroll" action (optional)
// @Param y int body Y coordinate for "scroll" action (optional)
//
// @Response 200 application/json Returns {success: true}
// @Response 400 application/json Invalid action or parameters
// @Response 404 application/json Tab or element not found
// @Response 500 application/json Chrome error
//
// @Example curl click:
//
//	curl -X POST http://localhost:9867/action \
//	  -H "Content-Type: application/json" \
//	  -d '{"tabId":"abc123","kind":"click","ref":"e5"}'
//
// @Example curl type:
//
//	curl -X POST http://localhost:9867/action \
//	  -H "Content-Type: application/json" \
//	  -d '{"tabId":"abc123","kind":"type","ref":"e3","text":"user@example.com"}'
//
// @Example curl fill form:
//
//	curl -X POST http://localhost:9867/action \
//	  -H "Content-Type: application/json" \
//	  -d '{"tabId":"abc123","kind":"fill","ref":"e3","text":"John Doe"}'
//
// @Example curl press key:
//
//	curl -X POST http://localhost:9867/action \
//	  -H "Content-Type: application/json" \
//	  -d '{"tabId":"abc123","kind":"press","ref":"e7","key":"Enter"}'
//
// @Example cli click:
//
//	pinchtab click e5
//
// @Example cli type:
//
//	pinchtab type e3 "user@example.com"
//
// @Example cli fill:
//
//	pinchtab fill e3 "John Doe"
func (h *Handlers) HandleAction(w http.ResponseWriter, r *http.Request) {
	// Convert single action to batch for consistency and reliability
	// This ensures we use the proven batch handler logic
	var singleReq bridge.ActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&singleReq); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	// Convert to batch request
	batchReq := actionsRequest{
		TabID:   singleReq.TabID,
		Actions: []bridge.ActionRequest{singleReq},
	}

	// Call the batch handler with the converted request
	// This reuses the proven batch implementation
	h.handleActionsBatch(w, r, batchReq)
}

type actionsRequest struct {
	TabID       string                 `json:"tabId"`
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
	// Ensure Chrome is initialized
	if err := h.ensureChrome(); err != nil {
		web.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	var req actionsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		web.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if len(req.Actions) == 0 {
		web.Error(w, 400, fmt.Errorf("actions array is empty"))
		return
	}

	h.handleActionsBatch(w, r, req)
}

// handleActionsBatch processes a batch of actions (used by both single and batch endpoints)
func (h *Handlers) handleActionsBatch(w http.ResponseWriter, r *http.Request, req actionsRequest) {

	ctx, resolvedTabID, err := h.Bridge.TabContext(req.TabID)
	if err != nil {
		web.Error(w, 404, err)
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

func countSuccessful(results []actionResult) int {
	count := 0
	for _, r := range results {
		if r.Success {
			count++
		}
	}
	return count
}
