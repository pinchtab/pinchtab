package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/audit"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/cdptk"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// runAxeAudit runs vendored axe-core in the isolated world, shapes the result
// into the audit envelope, ref-maps offending nodes back to snapshot refs, and
// writes the response. The isolated world is what makes the run tamper-proof:
// page script that overrides window.axe or a DOM prototype cannot reach it.
func (h *Handlers) runAxeAudit(w http.ResponseWriter, r *http.Request, tCtx context.Context, resolvedTabID string) {
	q := r.URL.Query()
	tags := splitCSVParam(q.Get("tags"))
	rules := splitCSVParam(q.Get("rules"))
	includeIncomplete := parseBoolQuery(q.Get("includeIncomplete"))
	selector := strings.TrimSpace(q.Get("selector"))

	config, err := audit.BuildAxeRunConfig(tags, rules)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("build axe config: %w", err))
		return
	}

	// Inject axe into every frame's isolated world first, so axe.run in the top
	// frame can coordinate its same-origin iframe audit with an axe instance
	// already listening in each child frame.
	injectAxeIntoAllFrames(tCtx)

	var raw audit.AxeRawResult
	if err := cdptk.EvaluateInIsolatedWorld(tCtx, "", axeRunSnippet(config, selector), &raw); err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("run axe: %w", err))
		return
	}

	report := audit.BuildAxeReport(raw, assets.AxeVersion, includeIncomplete)
	vocab := h.fillAxeRefs(tCtx, resolvedTabID, &report)
	publishVocab(w, resolvedTabID, vocab)

	httpx.JSON(w, 200, struct {
		TabID           string `json:"tabId"`
		VocabularyToken string `json:"vocabularyToken"`
		audit.AxeReport
	}{resolvedTabID, vocab, report})
}

// injectAxeIntoAllFrames loads the axe source into every reachable frame's
// isolated world (the top frame included), so a subsequent axe.run can audit
// same-origin iframes through the axe instance waiting in each. A cross-origin
// frame whose isolated world cannot be created is skipped, exactly as snapshot
// and text extraction skip it.
func injectAxeIntoAllFrames(tCtx context.Context) {
	var ids []string
	if tree, err := observe.FetchFrameTree(tCtx); err == nil {
		ids = observe.FrameIDs(tree)
	}
	if len(ids) == 0 {
		ids = []string{""} // top frame only
	}
	for _, id := range ids {
		_ = cdptk.EvaluateInIsolatedWorld(tCtx, id, assets.AxeJS, nil)
	}
}

// axeRunSnippet builds the in-page call: run axe over the document (or a
// selector scope), await it, and return the fields the report needs, reducing
// the large passes/inapplicable node lists to counts before they cross the wire.
func axeRunSnippet(config, selector string) string {
	target := "document"
	if selector != "" {
		encoded, _ := json.Marshal(selector)
		target = fmt.Sprintf("(document.querySelector(%s) || document)", encoded)
	}
	return fmt.Sprintf(`(function(){
  return axe.run(%s, %s).then(function(r){
    return {
      url: r.url || location.href,
      testEngine: r.testEngine,
      violations: r.violations,
      incomplete: r.incomplete,
      passes: (r.passes || []).length,
      inapplicable: (r.inapplicable || []).length
    };
  });
})()`, target, config)
}

// fillAxeRefs maps each offending node's deepest CSS target to a snapshot ref, so
// an agent can actuate a failing element directly. A node whose target does not
// resolve to a snapshot ref (a cross-origin iframe, a node not in the tree) is
// left without one rather than guessed.
// fillAxeRefs returns the vocabulary token of the ref cache the audit published,
// so the handler can set the X-PinchTab-Vocab header and body field as /snapshot
// and /capture do — the audit mints a fresh token whenever it re-epochs the tab
// (first read, or any read after a navigation), and a client that never learns it
// would echo a stale one and be refused 409 on the ref the audit just handed out.
func (h *Handlers) fillAxeRefs(tCtx context.Context, tabID string, report *audit.AxeReport) string {
	backendToRef, vocab := h.snapshotBackendRefs(tCtx, tabID)
	if len(backendToRef) == 0 {
		return vocab
	}
	fill := func(violations []audit.AxeViolation) {
		for i := range violations {
			for j := range violations[i].Nodes {
				node := &violations[i].Nodes[j]
				// Only a single-hop target names a top-frame element the top
				// document's querySelector can resolve. A multi-hop target is
				// inside an iframe; resolving its last selector against the top
				// document would map it to the wrong element, so leave it unref'd.
				if len(node.Target) != 1 {
					continue
				}
				backendID, err := bridge.BackendNodeIDForSelector(tCtx, "", node.Target[0])
				if err != nil || backendID == 0 {
					continue
				}
				if ref, ok := backendToRef[backendID]; ok {
					node.Ref = ref
				}
			}
		}
	}
	fill(report.Violations)
	fill(report.IncompleteViolations)
	return vocab
}

// snapshotBackendRefs builds a full-tree snapshot, stores it as the tab's ref
// cache (as /snapshot and /find do), and returns backend node id → ref from that
// cache. Storing the cache is what makes an axe node's ref actionable: /action
// resolves a ref against the tab's cache, so the ref would 404 if the audit
// computed it without publishing the snapshot it came from. A full tree (not the
// interactive filter) is used so non-interactive violation targets — images,
// low-contrast text — still map to a ref.
func (h *Handlers) snapshotBackendRefs(tCtx context.Context, tabID string) (map[int64]string, string) {
	rawNodes, err := bridge.FetchAXTree(tCtx)
	if err != nil {
		return nil, ""
	}
	flat, _ := bridge.BuildSnapshot(rawNodes, "", -1)
	cache := bridge.EpochRefs(h.Bridge.GetRefCache(tabID), flat)
	h.Bridge.SetRefCache(tabID, cache)

	refs := make(map[int64]string, len(cache.Nodes))
	for _, n := range cache.Nodes {
		if n.NodeID != 0 && n.Ref != "" {
			refs[n.NodeID] = n.Ref
		}
	}
	return refs, cache.DomEpoch
}

// splitCSVParam splits a comma list query value into trimmed, non-empty items.
func splitCSVParam(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
