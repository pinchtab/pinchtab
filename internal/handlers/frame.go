package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/selector"
)

type frameScopeAPI interface {
	GetFrameScope(tabID string) (bridge.FrameScope, bool)
	SetFrameScope(tabID string, scope bridge.FrameScope)
	ClearFrameScope(tabID string)
}

type frameRequest struct {
	TabID  string `json:"tabId,omitempty"`
	Target string `json:"target,omitempty"`
}

func (h *Handlers) frameScopes() frameScopeAPI {
	scopes, _ := h.Bridge.(frameScopeAPI)
	return scopes
}

func (h *Handlers) currentFrameScope(tabID string) (bridge.FrameScope, bool) {
	scopes := h.frameScopes()
	if scopes == nil {
		return bridge.FrameScope{}, false
	}
	return scopes.GetFrameScope(tabID)
}

func (h *Handlers) selectorFrameID(tabID string) string {
	scope, ok := h.currentFrameScope(tabID)
	if !ok {
		return ""
	}
	return scope.FrameID
}

func (h *Handlers) scopeSnapshotNodesByFrame(nodes []bridge.RawAXNode, frameID string) []bridge.RawAXNode {
	if frameID == "" {
		return nodes
	}
	filtered := make([]bridge.RawAXNode, 0, len(nodes))
	for _, node := range nodes {
		if node.FrameID == frameID {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func (h *Handlers) resolveSelectorNodeID(ctx context.Context, tabID, raw string) (int64, error) {
	sel := selector.Parse(raw)
	cache := h.Bridge.GetRefCache(tabID)
	return bridge.ResolveUnifiedSelectorInFrame(ctx, sel, cache, h.selectorFrameID(tabID))
}

func ownerRefForFrame(cache *bridge.RefCache, frameID string) string {
	if cache == nil || frameID == "" {
		return ""
	}
	for _, node := range cache.Nodes {
		if node.ChildFrameID == frameID {
			return node.Ref
		}
	}
	for ref, target := range cache.Targets {
		if target.ChildFrameID == frameID {
			return ref
		}
	}
	return ""
}

func frameScopeForChildFrame(frameID, ownerRef, fallbackURL, fallbackName string, frames map[string]bridge.RawFrame) (bridge.FrameScope, bool) {
	if frameID == "" {
		return bridge.FrameScope{}, false
	}
	scope := bridge.FrameScope{
		FrameID:   frameID,
		FrameURL:  fallbackURL,
		FrameName: fallbackName,
		OwnerRef:  ownerRef,
	}
	if frame, ok := frames[frameID]; ok {
		if frame.URL != "" {
			scope.FrameURL = frame.URL
		}
		if frame.Name != "" {
			scope.FrameName = frame.Name
		}
	}
	return scope, true
}

func matchFrameByElementMeta(frames map[string]bridge.RawFrame, rootFrameID string, meta bridge.FrameElementMeta) (bridge.FrameScope, bool, error) {
	tag := strings.ToLower(strings.TrimSpace(meta.TagName))
	if tag != "iframe" && tag != "frame" {
		return bridge.FrameScope{}, false, nil
	}

	candidates := make([]bridge.FrameScope, 0, 1)
	addCandidate := func(frameID string, frame bridge.RawFrame) {
		candidates = append(candidates, bridge.FrameScope{
			FrameID:   frameID,
			FrameURL:  frame.URL,
			FrameName: frame.Name,
		})
	}

	if src := strings.TrimSpace(meta.Src); src != "" {
		for frameID, frame := range frames {
			if frameID == "" || frameID == rootFrameID {
				continue
			}
			if frame.URL == src {
				addCandidate(frameID, frame)
			}
		}
		if len(candidates) == 1 {
			return candidates[0], true, nil
		}
		if len(candidates) > 1 {
			return bridge.FrameScope{}, false, fmt.Errorf("frame selector matched multiple frames for src %q", src)
		}
	}

	label := strings.TrimSpace(meta.Name)
	if label == "" {
		label = strings.TrimSpace(meta.Title)
	}
	if label != "" {
		for frameID, frame := range frames {
			if frameID == "" || frameID == rootFrameID {
				continue
			}
			if frame.Name == label || strings.Contains(frame.URL, label) {
				addCandidate(frameID, frame)
			}
		}
		if len(candidates) == 1 {
			return candidates[0], true, nil
		}
		if len(candidates) > 1 {
			return bridge.FrameScope{}, false, fmt.Errorf("frame selector matched multiple frames for %q", label)
		}
	}

	var only bridge.FrameScope
	count := 0
	for frameID, frame := range frames {
		if frameID == "" || frameID == rootFrameID {
			continue
		}
		only = bridge.FrameScope{FrameID: frameID, FrameURL: frame.URL, FrameName: frame.Name}
		count++
	}
	if count == 1 {
		return only, true, nil
	}
	return bridge.FrameScope{}, false, nil
}

func frameScopeForOwnerNode(nodeID int64, cache *bridge.RefCache, frames map[string]bridge.RawFrame, ownerMap map[string]int64) (bridge.FrameScope, bool) {
	if nodeID == 0 {
		return bridge.FrameScope{}, false
	}
	for frameID, ownerNodeID := range ownerMap {
		if ownerNodeID != nodeID {
			continue
		}
		return frameScopeForChildFrame(frameID, ownerRefForFrame(cache, frameID), "", "", frames)
	}
	if cache == nil {
		return bridge.FrameScope{}, false
	}
	for ref, target := range cache.Targets {
		if target.BackendNodeID != nodeID || target.ChildFrameID == "" {
			continue
		}
		return frameScopeForChildFrame(target.ChildFrameID, ref, target.ChildFrameURL, target.ChildFrameName, frames)
	}
	for _, node := range cache.Nodes {
		if node.NodeID != nodeID || node.ChildFrameID == "" {
			continue
		}
		return frameScopeForChildFrame(node.ChildFrameID, node.Ref, node.ChildFrameURL, node.ChildFrameName, frames)
	}
	return bridge.FrameScope{}, false
}

func matchFrameByMeta(frames map[string]bridge.RawFrame, rootFrameID, target string) ([]bridge.FrameScope, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("missing frame target")
	}
	matches := make([]bridge.FrameScope, 0, 1)
	for frameID, frame := range frames {
		if frameID == "" || frameID == rootFrameID {
			continue
		}
		if frame.Name == target || frame.URL == target || strings.Contains(frame.URL, target) {
			matches = append(matches, bridge.FrameScope{
				FrameID:   frameID,
				FrameURL:  frame.URL,
				FrameName: frame.Name,
			})
		}
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("frame target %q matched multiple frames", target)
	}
	return matches, nil
}

func (h *Handlers) resolveFrameScope(ctx context.Context, tabID, target string) (bridge.FrameScope, bool, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return bridge.FrameScope{}, false, fmt.Errorf("missing frame target")
	}
	if strings.EqualFold(target, "main") {
		return bridge.FrameScope{}, true, nil
	}

	cache := h.Bridge.GetRefCache(tabID)
	sel := selector.Parse(target)
	var refScope bridge.FrameScope
	var hasRefScope bool
	if sel.Kind == selector.KindRef {
		if cache == nil {
			return bridge.FrameScope{}, false, fmt.Errorf("no snapshot cache available for ref %q", sel.Value)
		}
		refTarget, ok := cache.Lookup(sel.Value)
		if !ok {
			return bridge.FrameScope{}, false, fmt.Errorf("ref %q not found in snapshot cache", sel.Value)
		}
		if refTarget.ChildFrameID != "" {
			return bridge.FrameScope{
				FrameID:   refTarget.ChildFrameID,
				FrameURL:  refTarget.ChildFrameURL,
				FrameName: refTarget.ChildFrameName,
				OwnerRef:  sel.Value,
			}, false, nil
		}
		if refTarget.FrameID != "" {
			refScope = bridge.FrameScope{
				FrameID:   refTarget.FrameID,
				FrameURL:  refTarget.FrameURL,
				FrameName: refTarget.FrameName,
			}
			hasRefScope = true
		}
	}

	frameTree, err := bridge.FetchFrameTree(ctx)
	if err != nil {
		return bridge.FrameScope{}, false, fmt.Errorf("frame tree: %w", err)
	}
	rootFrameID := frameTree.Frame.ID
	frames := bridge.FrameMap(frameTree)
	ownerMap := bridge.FrameOwnerMap(ctx, frameTree)
	if hasRefScope {
		if refScope.FrameID == rootFrameID {
			return bridge.FrameScope{}, true, nil
		}
		return refScope, false, nil
	}

	if matches, err := matchFrameByMeta(frames, rootFrameID, target); err == nil && len(matches) == 1 {
		scope := matches[0]
		scope.OwnerRef = ownerRefForFrame(cache, scope.FrameID)
		return scope, false, nil
	} else if err != nil {
		return bridge.FrameScope{}, false, err
	}

	switch sel.Kind {
	case selector.KindCSS, selector.KindXPath, selector.KindText:
		nodeID, err := h.resolveSelectorNodeID(ctx, tabID, target)
		if err != nil {
			return bridge.FrameScope{}, false, err
		}
		if scope, ok := frameScopeForOwnerNode(nodeID, cache, frames, ownerMap); ok {
			return scope, false, nil
		}
		if sel.Kind == selector.KindCSS || sel.Kind == selector.KindXPath {
			meta, metaErr := bridge.ResolveFrameElementMetaInFrame(ctx, sel, h.selectorFrameID(tabID))
			if metaErr == nil {
				if scope, ok, matchErr := matchFrameByElementMeta(frames, rootFrameID, meta); matchErr != nil {
					return bridge.FrameScope{}, false, matchErr
				} else if ok {
					scope.OwnerRef = ownerRefForFrame(cache, scope.FrameID)
					return scope, false, nil
				}
			}
		}
	}

	return bridge.FrameScope{}, false, fmt.Errorf("frame target %q did not resolve to an iframe or frame", target)
}

func writeFrameScope(w http.ResponseWriter, tabID string, scope bridge.FrameScope, scoped bool) {
	resp := map[string]any{
		"tabId":   tabID,
		"scoped":  scoped,
		"target":  "main",
		"current": "main",
	}
	if scoped {
		resp["target"] = scope.FrameID
		resp["current"] = scope
		resp["frame"] = scope
	}
	httpx.JSON(w, 200, resp)
}

func (h *Handlers) HandleFrame(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	var req frameRequest
	if r.Method != http.MethodGet {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
			httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
		if req.TabID != "" {
			tabID = req.TabID
		}
	}

	ctx, resolvedTabID, err := h.tabContextWithHeader(w, r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}

	scopes := h.frameScopes()
	if scopes == nil {
		httpx.Error(w, 501, fmt.Errorf("frame scoping unavailable"))
		return
	}

	if r.Method == http.MethodGet {
		scope, ok := scopes.GetFrameScope(resolvedTabID)
		writeFrameScope(w, resolvedTabID, scope, ok)
		return
	}
	if strings.TrimSpace(req.Target) == "" {
		httpx.Error(w, 400, fmt.Errorf("missing required field 'target'"))
		return
	}

	tCtx, cancel := context.WithTimeout(ctx, h.Config.ActionTimeout)
	defer cancel()
	go httpx.CancelOnClientDone(r.Context(), cancel)

	scope, resetToMain, err := h.resolveFrameScope(tCtx, resolvedTabID, req.Target)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}
	if resetToMain || !scope.Active() {
		scopes.ClearFrameScope(resolvedTabID)
		writeFrameScope(w, resolvedTabID, bridge.FrameScope{}, false)
		return
	}

	if scope.OwnerRef == "" {
		scope.OwnerRef = ownerRefForFrame(h.Bridge.GetRefCache(resolvedTabID), scope.FrameID)
	}
	scopes.SetFrameScope(resolvedTabID, scope)
	writeFrameScope(w, resolvedTabID, scope, true)
}

func (h *Handlers) HandleTabFrame(w http.ResponseWriter, r *http.Request) {
	tabID := strings.TrimSpace(r.PathValue("id"))
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("missing tab id"))
		return
	}

	wrapped := r.Clone(r.Context())
	q := wrapped.URL.Query()
	q.Set("tabId", tabID)
	wrapped.URL.RawQuery = q.Encode()
	h.HandleFrame(w, wrapped)
}
