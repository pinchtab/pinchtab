package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

type inspectResponse struct {
	TabID     string         `json:"tabId,omitempty"`
	FrameID   string         `json:"frameId,omitempty"`
	URL       string         `json:"url,omitempty"`
	Title     string         `json:"title,omitempty"`
	HTML      string         `json:"html,omitempty"`
	Styles    map[string]any `json:"styles,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
}

type inspectPayload struct {
	Title  string         `json:"title"`
	URL    string         `json:"url"`
	HTML   string         `json:"html,omitempty"`
	Styles map[string]any `json:"styles,omitempty"`
}

type inspectKind string

const (
	inspectKindTitle  inspectKind = "title"
	inspectKindURL    inspectKind = "url"
	inspectKindHTML   inspectKind = "html"
	inspectKindStyles inspectKind = "styles"
)

func (h *Handlers) HandleTitle(w http.ResponseWriter, r *http.Request) {
	h.handleInspect(w, r, inspectKindTitle)
}

func (h *Handlers) HandleTabTitle(w http.ResponseWriter, r *http.Request) {
	h.forwardInspectTabRoute(w, r, h.HandleTitle)
}

func (h *Handlers) HandleURL(w http.ResponseWriter, r *http.Request) {
	h.handleInspect(w, r, inspectKindURL)
}

func (h *Handlers) HandleTabURL(w http.ResponseWriter, r *http.Request) {
	h.forwardInspectTabRoute(w, r, h.HandleURL)
}

func (h *Handlers) HandleHTML(w http.ResponseWriter, r *http.Request) {
	h.handleInspect(w, r, inspectKindHTML)
}

func (h *Handlers) HandleTabHTML(w http.ResponseWriter, r *http.Request) {
	h.forwardInspectTabRoute(w, r, h.HandleHTML)
}

func (h *Handlers) HandleStyles(w http.ResponseWriter, r *http.Request) {
	h.handleInspect(w, r, inspectKindStyles)
}

func (h *Handlers) HandleTabStyles(w http.ResponseWriter, r *http.Request) {
	h.forwardInspectTabRoute(w, r, h.HandleStyles)
}

func (h *Handlers) handleInspect(w http.ResponseWriter, r *http.Request, kind inspectKind) {
	tabID := r.URL.Query().Get("tabId")
	h.recordReadRequest(r, string(kind), tabID)

	if err := h.ensureChrome(); err != nil {
		if h.writeBridgeUnavailable(w, err) {
			return
		}
		httpx.Error(w, 500, fmt.Errorf("chrome initialization: %w", err))
		return
	}

	ctx, resolvedTabID, err := h.tabContextWithHeader(w, r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}
	defer h.armAutoCloseIfEnabled(resolvedTabID)

	tCtx, tCancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer tCancel()
	go httpx.CancelOnClientDone(r.Context(), tCancel)

	targetFrameID := r.URL.Query().Get("frameId")
	if targetFrameID == "" {
		if scope, ok := h.currentFrameScope(resolvedTabID); ok {
			targetFrameID = scope.FrameID
		}
	}

	payload, err := h.inspectPayload(tCtx, resolvedTabID, targetFrameID, r.URL.Query().Get("selector"), r.URL.Query().Get("ref"), kind)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	resp := inspectResponse{
		TabID:   resolvedTabID,
		FrameID: targetFrameID,
		URL:     payload.URL,
		Title:   payload.Title,
	}
	if kind == inspectKindHTML {
		resp.HTML = payload.HTML
		if maxChars := parsePositiveInt(r.URL.Query().Get("maxChars")); maxChars > 0 && len(resp.HTML) > maxChars {
			resp.HTML = resp.HTML[:maxChars]
			resp.Truncated = true
		}
	}
	if kind == inspectKindStyles {
		styles := payload.Styles
		if prop := strings.TrimSpace(r.URL.Query().Get("prop")); prop != "" {
			resp.Styles = map[string]any{prop: styles[prop]}
		} else {
			resp.Styles = sortCSSMap(styles)
		}
	}

	h.recordResolvedURL(r, resp.URL)
	httpx.JSON(w, 200, resp)
}

func (h *Handlers) inspectPayload(ctx context.Context, tabID, frameID, rawSelector, ref string, kind inspectKind) (inspectPayload, error) {
	if ref != "" {
		return h.inspectByRef(ctx, tabID, ref, kind)
	}
	if rawSelector != "" {
		return h.inspectBySelector(ctx, tabID, rawSelector, frameID, kind)
	}
	return h.inspectDocument(ctx, frameID, kind)
}

func (h *Handlers) inspectDocument(ctx context.Context, frameID string, kind inspectKind) (inspectPayload, error) {
	metaExpr := inspectDocumentExpression(kind)
	var payload inspectPayload
	if err := h.evalInspectExpression(ctx, frameID, metaExpr, &payload); err != nil {
		return inspectPayload{}, fmt.Errorf("inspect %s: %w", kind, err)
	}
	return payload, nil
}

func inspectDocumentExpression(kind inspectKind) string {
	switch kind {
	case inspectKindStyles:
		return `(() => {
			const doc = document;
			const win = doc.defaultView || window;
			const root = doc.documentElement || doc.body;
			const style = root ? win.getComputedStyle(root) : null;
			const styles = {};
			if (style) {
				for (const name of style) styles[name] = style.getPropertyValue(name);
			}
			return {
				title: doc.title || "",
				url: String(doc.location ? doc.location.href : win.location.href),
				html: doc.documentElement ? doc.documentElement.outerHTML : "",
				styles
			};
		})()`
	default:
		return `(() => {
			const doc = document;
			const win = doc.defaultView || window;
			return {
				title: doc.title || "",
				url: String(doc.location ? doc.location.href : win.location.href),
				html: doc.documentElement ? doc.documentElement.outerHTML : "",
				styles: {}
			};
		})()`
	}
}

func (h *Handlers) inspectByRef(ctx context.Context, tabID, ref string, kind inspectKind) (inspectPayload, error) {
	cache := h.Bridge.GetRefCache(tabID)
	if cache == nil {
		return inspectPayload{}, fmt.Errorf("ref not found: %s (no snapshot cache)", ref)
	}
	target, ok := cache.Lookup(ref)
	if !ok {
		return inspectPayload{}, fmt.Errorf("ref not found: %s", ref)
	}
	return inspectByBackendNodeID(ctx, target.BackendNodeID, kind)
}

func (h *Handlers) inspectBySelector(ctx context.Context, tabID, rawSelector, frameID string, kind inspectKind) (inspectPayload, error) {
	nodeID, err := h.resolveSelectorNodeIDInFrame(ctx, tabID, rawSelector, frameID)
	if err != nil {
		return inspectPayload{}, frameScopedSelectorError("selector", err)
	}
	return inspectByBackendNodeID(ctx, nodeID, kind)
}

func inspectByBackendNodeID(ctx context.Context, nodeID int64, kind inspectKind) (inspectPayload, error) {
	var payload inspectPayload
	if nodeID == 0 {
		return payload, fmt.Errorf("element not found in DOM (backendNodeId=%d)", nodeID)
	}

	var resolveResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", map[string]any{
			"backendNodeId": nodeID,
		}, &resolveResult)
	})); err != nil {
		return payload, fmt.Errorf("resolve node: %w", err)
	}

	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(resolveResult, &resolved); err != nil {
		return payload, fmt.Errorf("parse resolved node: %w", err)
	}
	if resolved.Object.ObjectID == "" {
		return payload, fmt.Errorf("element not found in DOM (backendNodeId=%d)", nodeID)
	}

	functionDeclaration := inspectFunctionDeclaration(kind)
	var callResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
			"functionDeclaration": functionDeclaration,
			"objectId":            resolved.Object.ObjectID,
			"returnByValue":       true,
		}, &callResult)
	})); err != nil {
		return payload, fmt.Errorf("inspect %s: %w", kind, err)
	}

	var result struct {
		Result struct {
			Value inspectPayload `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails,omitempty"`
	}
	if err := json.Unmarshal(callResult, &result); err != nil {
		return payload, fmt.Errorf("inspect %s parse: %w", kind, err)
	}
	if result.ExceptionDetails != nil && result.ExceptionDetails.Text != "" {
		return payload, fmt.Errorf("inspect %s: %s", kind, result.ExceptionDetails.Text)
	}
	return result.Result.Value, nil
}

func inspectFunctionDeclaration(kind inspectKind) string {
	switch kind {
	case inspectKindStyles:
		return `function() {
			const el = this;
			const doc = el.ownerDocument || document;
			const win = doc.defaultView || window;
			const style = win.getComputedStyle(el);
			const styles = {};
			for (const name of style) styles[name] = style.getPropertyValue(name);
			return {
				title: doc.title || '',
				url: String(doc.location ? doc.location.href : win.location.href),
				styles,
			};
		}`
	default:
		return `function() {
			const el = this;
			const doc = el.ownerDocument || document;
			const win = doc.defaultView || window;
			return {
				title: doc.title || '',
				url: String(doc.location ? doc.location.href : win.location.href),
				html: el.outerHTML || '',
			};
		}`
	}
}

func (h *Handlers) evalInspectExpression(ctx context.Context, frameID, expr string, out any) error {
	if frameID == "" {
		return h.evalRuntime(ctx, expr, out)
	}
	execID, err := bridge.FrameExecutionContextID(ctx, frameID)
	if err != nil {
		return fmt.Errorf("resolve frame context: %w", err)
	}
	var raw json.RawMessage
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.evaluate", map[string]any{
			"expression":    expr,
			"returnByValue": true,
			"contextId":     execID,
		}, &raw)
	}))
	if err != nil {
		return err
	}
	var result struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails,omitempty"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("inspect parse: %w", err)
	}
	if result.ExceptionDetails != nil && result.ExceptionDetails.Text != "" {
		return fmt.Errorf("%s", result.ExceptionDetails.Text)
	}
	if len(result.Result.Value) == 0 {
		return nil
	}
	return json.Unmarshal(result.Result.Value, out)
}

func parsePositiveInt(raw string) int {
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func sortCSSMap(css map[string]any) map[string]any {
	if css == nil {
		return map[string]any{}
	}
	keys := make([]string, 0, len(css))
	for k := range css {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(css))
	for _, k := range keys {
		out[k] = css[k]
	}
	return out
}

func (h *Handlers) forwardInspectTabRoute(w http.ResponseWriter, r *http.Request, next func(http.ResponseWriter, *http.Request)) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}
	q := r.URL.Query()
	q.Set("tabId", tabID)
	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u
	next(w, req)
}
