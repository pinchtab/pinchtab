package bridge

import (
	"context"
	"encoding/base64"
	"log/slog"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	// maxRouteDispatchWorkers caps concurrent CDP dispatch per tab so a broad
	// rule on an asset-heavy page cannot fan out into one goroutine per request.
	maxRouteDispatchWorkers = 8
	// routeDispatchQueueSize is the per-tab burst buffer before submit overflows
	// to a one-off goroutine.
	routeDispatchQueueSize = 256
)

type routeDispatchJob struct {
	e       *fetch.EventRequestPaused
	rule    RouteRule
	matched bool
}

type routeDispatchPool struct {
	queue chan routeDispatchJob
}

// newRouteDispatchPool starts `workers` goroutines that run `run` for each
// submitted job until ctx is cancelled. run is injected so the bounding
// mechanism can be tested without a CDP executor.
func newRouteDispatchPool(ctx context.Context, workers int, run func(routeDispatchJob)) *routeDispatchPool {
	p := &routeDispatchPool{queue: make(chan routeDispatchJob, routeDispatchQueueSize)}
	for range workers {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case job := <-p.queue:
					run(job)
				}
			}
		}()
	}
	return p
}

// submit enqueues without blocking and returns false when the queue is full, so
// the caller can fall back: a paused request must always be resolved, never
// dropped, and the listener callback must never block (blocking it would
// deadlock chromedp's event reader, which delivers the CDP responses dispatch
// waits on).
func (p *routeDispatchPool) submit(job routeDispatchJob) bool {
	select {
	case p.queue <- job:
		return true
	default:
		return false
	}
}

func (rm *RouteManager) registerListener(listenCtx context.Context, tabID string) {
	// Verify the context is target-bound. chromedp.ListenTarget on a
	// browser-level context (Target == nil) would broadcast to every tab —
	// we explicitly require the per-tab target so dispatch is session-scoped.
	if c := chromedp.FromContext(listenCtx); c == nil || c.Target == nil {
		slog.Warn("route listener not registered: chromedp context has no target", "tabId", tabID)
		return
	}
	pool := newRouteDispatchPool(listenCtx, maxRouteDispatchWorkers, func(job routeDispatchJob) {
		rm.dispatch(listenCtx, tabID, job.e, job.rule, job.matched)
	})
	chromedp.ListenTarget(listenCtx, func(ev interface{}) {
		if listenCtx.Err() != nil {
			return
		}
		e, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		eventResourceType := strings.ToLower(string(e.ResourceType))
		eventMethod := strings.ToUpper(strings.TrimSpace(e.Request.Method))
		rule, matched, hasRules := rm.match(tabID, e.Request.URL, eventResourceType, eventMethod)
		// Teardown in progress: rules drained but fetch.Disable not yet
		// landed. Skip dispatch — fetch.Disable will release pending requests.
		if !hasRules {
			return
		}
		job := routeDispatchJob{e: e, rule: rule, matched: matched}
		if !pool.submit(job) {
			// Queue saturated: fall back to a one-off goroutine (the prior
			// behavior) so the request is still resolved and the callback never
			// blocks. Bounded in the common case; no worse than before under flood.
			go rm.dispatch(listenCtx, tabID, e, rule, matched)
		}
	})
}

// dispatch issues the CDP response for a paused request. Errors are logged at
// Debug level — fetch operations frequently fail benignly when a tab navigates
// while a request is paused, and elevating those to Warn would be noisy.
func (rm *RouteManager) dispatch(listenCtx context.Context, tabID string, e *fetch.EventRequestPaused, rule RouteRule, matched bool) {
	if listenCtx.Err() != nil {
		return
	}
	executor := cdp.WithExecutor(listenCtx, chromedp.FromContext(listenCtx).Target)

	if !matched || rule.Action == RouteActionContinue {
		if err := fetch.ContinueRequest(e.RequestID).Do(executor); err != nil {
			slog.Debug("fetch.continueRequest failed", "tabId", tabID, "url", e.Request.URL, "err", err)
		}
		return
	}

	switch rule.Action {
	case RouteActionAbort:
		if err := fetch.FailRequest(e.RequestID, network.ErrorReasonBlockedByClient).Do(executor); err != nil {
			slog.Debug("fetch.failRequest failed", "tabId", tabID, "url", e.Request.URL, "err", err)
		}
	case RouteActionFulfill:
		if !rm.fulfillForgeryPermittedFor(e.Request.URL) {
			slog.Warn("route fulfill blocked: response forgery not permitted (allowlisted host or forbidden scheme)",
				"tabId", tabID,
				"url", e.Request.URL,
				"pattern", rule.Pattern,
			)
			if err := fetch.ContinueRequest(e.RequestID).Do(executor); err != nil {
				slog.Debug("fetch.continueRequest (fulfill fallthrough) failed", "tabId", tabID, "url", e.Request.URL, "err", err)
			}
			return
		}
		headers := []*fetch.HeaderEntry{{Name: "Content-Type", Value: rule.ContentType}}
		if err := fetch.FulfillRequest(e.RequestID, int64(rule.Status)).
			WithResponseHeaders(headers).
			WithBody(base64.StdEncoding.EncodeToString([]byte(rule.Body))).
			Do(executor); err != nil {
			slog.Debug("fetch.fulfillRequest failed", "tabId", tabID, "url", e.Request.URL, "err", err)
		}
	default:
		if err := fetch.ContinueRequest(e.RequestID).Do(executor); err != nil {
			slog.Debug("fetch.continueRequest (default) failed", "tabId", tabID, "url", e.Request.URL, "err", err)
		}
	}
}

// match looks for the first rule matching url + resourceType + method. The
// third return value (hasRules) is true iff the tab has any rules at all —
// callers use it to skip dispatch entirely during teardown windows.
//
// Method semantics:
//
//   - Rule.Method != ""   → strict method match (case-insensitive).
//   - Rule.Method == "" + non-OPTIONS event → match any method.
//   - Rule.Method == "" + OPTIONS event + Action == fulfill → SKIP. CORS
//     preflights deserve explicit opt-in: a fulfill that catches the
//     preflight without ACAO/ACAM/ACAH headers breaks the real request, and
//     a fulfill that fakes those headers bypasses CORS for the real call.
//     Operators who genuinely want to mock OPTIONS set Method:"OPTIONS".
//   - Rule.Method == "" + OPTIONS event + Action != fulfill → match.
//     Aborting/passing-through preflights is benign.
func (rm *RouteManager) match(tabID, url, resourceType, method string) (rule RouteRule, matched bool, hasRules bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	state := rm.perTab[tabID]
	if state == nil || len(state.rules) == 0 {
		return RouteRule{}, false, false
	}
	const optionsMethod = "OPTIONS"
	for _, r := range state.rules {
		if r.ResourceType != "" && !strings.EqualFold(r.ResourceType, resourceType) {
			continue
		}
		if r.Method != "" {
			if !strings.EqualFold(r.Method, method) {
				continue
			}
		} else if r.Action == RouteActionFulfill && method == optionsMethod {
			continue
		}
		if ruleMatchesURL(r, url) {
			return r, true, true
		}
	}
	return RouteRule{}, false, true
}
