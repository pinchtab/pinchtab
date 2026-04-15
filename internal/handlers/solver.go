package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/solver"

	// Register built-in solvers via init().
	_ "github.com/pinchtab/pinchtab/internal/bridge"
)

// HandleSolve attempts to solve a browser challenge on the current page.
//
// When "solver" is omitted from the request body, all registered solvers are
// tried in order via auto-detection.  When "solver" is set (e.g. "cloudflare"),
// only that solver is invoked.
//
// @Endpoint POST /solve
// @Description Auto-detect and solve browser challenges (Cloudflare, etc.)
//
// @Param tabId       string  body  Tab ID (optional — uses default tab)
// @Param solver      string  body  Solver name (optional — auto-detect)
// @Param maxAttempts int     body  Max solve attempts (optional, default: 3)
// @Param timeout     float64 body  Timeout in ms (optional, default: 30000)
//
// @Response 200 application/json Returns {tabId, solver, solved, challengeType, attempts, title, needsHumanHandoff, handoffReason}
// @Response 400 application/json Invalid request body or unknown solver
// @Response 423 application/json Tab is locked by another owner
// @Response 500 application/json Chrome/CDP error
//
// @Example curl:
//
//	curl -X POST http://localhost:9867/solve \
//	  -H "Content-Type: application/json" \
//	  -d '{"maxAttempts": 3, "timeout": 30000}'
//
// @Example curl (specific solver):
//
//	curl -X POST http://localhost:9867/solve \
//	  -H "Content-Type: application/json" \
//	  -d '{"solver": "cloudflare", "maxAttempts": 3}'
func (h *Handlers) HandleSolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID       string  `json:"tabId"`
		Solver      string  `json:"solver"`
		MaxAttempts int     `json:"maxAttempts"`
		Timeout     float64 `json:"timeout"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	// If a solver name is provided in the path, use it.
	if name := r.PathValue("name"); name != "" {
		req.Solver = name
	}

	// Validate solver name early.
	if req.Solver != "" {
		if _, ok := solver.Get(req.Solver); !ok {
			httpx.ErrorCode(w, 400, "unknown_solver",
				fmt.Sprintf("unknown solver %q (available: %v)", req.Solver, solver.Names()),
				false, nil)
			return
		}
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	owner := resolveOwner(r, "")
	if err := h.enforceTabLease(resolvedTabID, owner); err != nil {
		httpx.ErrorCode(w, 423, "tab_locked", err.Error(), false, nil)
		return
	}

	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	action := "solve"
	if req.Solver != "" {
		action = "solve:" + req.Solver
	}
	h.recordActivity(r, activity.Update{Action: action, TabID: resolvedTabID})

	timeout := 30 * time.Second
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Millisecond
	}

	tCtx, tCancel := context.WithTimeout(ctx, timeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	result, err := solver.Solve(tCtx, req.Solver, solver.Options{
		MaxAttempts: req.MaxAttempts,
	})
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("solve: %w", err))
		return
	}
	needsHumanHandoff, handoffReason := deriveHumanHandoff(result)

	// Re-check domain policy after solve — the page may have redirected
	// to a different domain once the challenge was resolved.
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	httpx.JSON(w, 200, map[string]any{
		"tabId":             resolvedTabID,
		"solver":            result.Solver,
		"solved":            result.Solved,
		"challengeType":     result.ChallengeType,
		"attempts":          result.Attempts,
		"title":             result.Title,
		"needsHumanHandoff": needsHumanHandoff,
		"handoffReason":     handoffReason,
	})
}

func deriveHumanHandoff(result *solver.Result) (bool, string) {
	if result == nil || result.Solved {
		return false, ""
	}

	challengeType := strings.ToLower(strings.TrimSpace(result.ChallengeType))
	title := strings.ToLower(strings.TrimSpace(result.Title))

	if strings.Contains(challengeType, "login") ||
		strings.Contains(challengeType, "auth") ||
		strings.Contains(challengeType, "credential") ||
		strings.Contains(challengeType, "password") ||
		strings.Contains(title, "sign in") ||
		strings.Contains(title, "log in") ||
		strings.Contains(title, "password") {
		return true, "credentials_required"
	}

	if strings.Contains(challengeType, "captcha") ||
		strings.Contains(challengeType, "turnstile") ||
		strings.Contains(challengeType, "recaptcha") ||
		strings.Contains(challengeType, "hcaptcha") ||
		strings.Contains(challengeType, "challenge") ||
		strings.Contains(title, "verify you are human") ||
		strings.Contains(title, "attention required") ||
		strings.Contains(title, "just a moment") {
		return true, "challenge_requires_manual_intervention"
	}

	return false, ""
}

// HandleTabSolve handles POST /tabs/{id}/solve and /tabs/{id}/solve/{name}.
//
// @Endpoint POST /tabs/{id}/solve
// @Description Solve a browser challenge on a specific tab
func (h *Handlers) HandleTabSolve(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	body := map[string]any{}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	body["tabId"] = tabID
	payload, err := json.Marshal(body)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	cloned := r.Clone(r.Context())
	cloned.Body = io.NopCloser(bytes.NewReader(payload))
	cloned.ContentLength = int64(len(payload))
	cloned.Header = r.Header.Clone()
	cloned.Header.Set("Content-Type", "application/json")
	h.HandleSolve(w, cloned)
}

// HandleListSolvers returns the list of registered solver names.
//
// @Endpoint GET /solvers
// @Description List available challenge solvers
// @Response 200 application/json Returns {solvers: ["cloudflare", ...]}
func (h *Handlers) HandleListSolvers(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, map[string]any{
		"solvers": solver.Names(),
	})
}
