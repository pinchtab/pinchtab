package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// HandleEvaluate runs JavaScript in the current tab.
//
// @Endpoint POST /evaluate
func (h *Handlers) HandleEvaluate(w http.ResponseWriter, r *http.Request) {
	if !h.allows(routes.CapEvaluate) {
		h.writeCapabilityDisabled(w, routes.CapEvaluate)
		return
	}

	var req struct {
		TabID        string `json:"tabId"`
		Expression   string `json:"expression"`
		AwaitPromise bool   `json:"awaitPromise"`
	}
	if !decodeRequiredJSON(w, r, &req) {
		return
	}
	if req.Expression == "" {
		httpx.Error(w, 400, fmt.Errorf("expression required"))
		return
	}

	ctx, _, ok := h.guardedTabContext(w, r, req.TabID, guardDialogBlocked|guardDomainPolicy|guardHandoffPause)
	if !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	slog.Warn("evaluate",
		"tabId", req.TabID,
		"expressionLength", len(req.Expression),
		"remoteAddr", r.RemoteAddr,
	)

	var result any
	var subtype string
	opts := bridge.EvalOpts{AwaitPromise: req.AwaitPromise}
	// The subtype-aware path exists only on a live bridge; a runtime without it
	// falls back to the plain eval and simply gets no Promise hint. bridgeAs looks
	// through a decorator (e.g. the ghost-chrome adapter) to the underlying bridge.
	var err error
	if se, ok := bridgeAs[subtypeEvaluator](h.Bridge); ok {
		subtype, err = se.EvaluateSubtype(tCtx, req.Expression, &result, opts)
	} else {
		err = h.evalRuntime(tCtx, req.Expression, &result, opts)
	}
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "Cannot read properties of null") ||
			strings.Contains(errMsg, "Cannot read property") ||
			strings.Contains(errMsg, "null is not an object") {
			httpx.ErrorCode(w, 500, "evaluate_null_ref", fmt.Sprintf("evaluate: %s", errMsg), false, map[string]any{
				"hint": "querySelector returned null — the element doesn't exist. Use snapshot refs (e0, e1, ...) instead of raw selectors.",
			})
			return
		}
		httpx.Error(w, 500, fmt.Errorf("evaluate: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "evaluate"})

	resp := map[string]any{"result": result}
	// A Promise returned without awaitPromise decodes to {} with no other signal;
	// name the flag rather than let the empty object read as a successful result.
	if !req.AwaitPromise && subtype == evalPromiseSubtype {
		resp["hint"] = "the expression returned a Promise; re-run with awaitPromise:true (CLI --await-promise) to resolve its value"
	}
	httpx.JSON(w, 200, resp)
}

// evalPromiseSubtype is the RemoteObject subtype a Promise carries (CDP
// runtime.Subtype "promise").
const evalPromiseSubtype = "promise"

// subtypeEvaluator is the optional bridge capability that reports a result's
// RemoteObject subtype alongside its value, so HandleEvaluate can tell a
// Promise apart from a genuine empty object. *bridge.Bridge implements it.
type subtypeEvaluator interface {
	EvaluateSubtype(ctx context.Context, expression string, result any, opts bridge.EvalOpts) (string, error)
}

// HandleTabEvaluate runs JavaScript in a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/evaluate
func (h *Handlers) HandleTabEvaluate(w http.ResponseWriter, r *http.Request) {
	if !h.allows(routes.CapEvaluate) {
		h.writeCapabilityDisabled(w, routes.CapEvaluate)
		return
	}

	h.withPathTabIDBody(w, r, h.HandleEvaluate)
}
