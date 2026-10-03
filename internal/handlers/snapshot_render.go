package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func (res snapshotResult) wireNodes(nodes []bridge.A11yNode) []bridge.A11yNode {
	if res.rootFrameID == "" {
		return nodes
	}
	wire := slices.Clone(nodes)
	for i := range wire {
		if wire[i].FrameID == res.rootFrameID {
			wire[i].FrameID, wire[i].FrameURL, wire[i].FrameName = "", "", ""
		}
	}
	return wire
}

func (res snapshotResult) attachRootFrame(payload map[string]any) map[string]any {
	if res.rootFrameID != "" {
		payload["frameId"] = res.rootFrameID
	}
	return payload
}

func snapshotDiffFields(data map[string]any, prev []bridge.A11yNode, res snapshotResult) map[string]any {
	added, changed, removed := bridge.DiffSnapshot(prev, res.flat)
	data["diff"] = true
	data["added"] = res.wireNodes(added)
	data["changed"] = res.wireNodes(changed)
	data["removed"] = res.wireNodes(removed)
	data["counts"] = map[string]int{
		"added":   len(added),
		"changed": len(changed),
		"removed": len(removed),
		"total":   len(res.flat),
	}
	return data
}

func (h *Handlers) writeSnapshotPlain(w http.ResponseWriter, q snapshotQuery, res snapshotResult, header, hint, content string, wrap bool) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(snapshotFraming(q.format, header, hint, q.controls.Ignored, res.truncated, q.controls.MaxTokens)))
	if wrap {
		content = h.IDPIGuard.WrapContent(content, res.url)
	}
	_, _ = w.Write([]byte(content))
}

func (h *Handlers) writeSnapshotFile(w http.ResponseWriter, q snapshotQuery, res snapshotResult, prevNodes []bridge.A11yNode) {
	snapshotDir := filepath.Join(h.Config.StateDir, "snapshots")
	if err := os.MkdirAll(snapshotDir, 0750); err != nil {
		httpx.Error(w, 500, fmt.Errorf("create snapshot dir: %w", err))
		return
	}

	timestamp := exportTimestamp()
	ext, content, err := marshalSnapshotFile(q, res, prevNodes)
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}

	var filePath string
	if q.outputPath != "" {
		p, status, err := saveToCallerPath(h.Config.StateDir, q.outputPath, content)
		if err != nil {
			httpx.Error(w, status, fmt.Errorf("snapshot: %w", err))
			return
		}
		filePath = p
	} else {
		filePath, err = writeUniqueFile(snapshotDir, "snapshot-"+timestamp, ext, content)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("write snapshot: %w", err))
			return
		}
	}

	httpx.JSON(w, 200, attachIgnoredParams(map[string]any{
		"path":      filePath,
		"size":      len(content),
		"format":    q.format,
		"timestamp": timestamp,
	}, q.controls.Ignored))
}

func marshalSnapshotFile(q snapshotQuery, res snapshotResult, prevNodes []bridge.A11yNode) (string, []byte, error) {
	now := time.Now().Format(time.RFC3339)
	if q.format == "text" {
		content := fmt.Sprintf("%s\n# %s\n\n%s",
			snapshotTextHeader(res.title, res.url, len(res.flat), res.scope), now,
			bridge.FormatSnapshotText(res.flat))
		return ".txt", []byte(content), nil
	}

	data := res.attachRootFrame(res.scope.attach(map[string]any{
		"url":       res.url,
		"title":     res.title,
		"timestamp": now,
		"nodes":     res.wireNodes(res.flat),
		"count":     len(res.flat),
	}))
	if q.doDiff && prevNodes != nil {
		snapshotDiffFields(data, prevNodes, res)
	}
	if q.format == "yaml" {
		content, err := observe.MarshalYAMLInJSONShape(data)
		if err != nil {
			return "", nil, err
		}
		return ".yaml", content, nil
	}
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	return ".json", content, nil
}

func (h *Handlers) writeSnapshotPlainDiff(w http.ResponseWriter, q snapshotQuery, res snapshotResult, prevNodes []bridge.A11yNode, wrap bool) {
	added, changed, removed := bridge.DiffSnapshot(prevNodes, res.flat)
	counts := fmt.Sprintf(" | +%d ~%d -%d", len(added), len(changed), len(removed))
	if q.format == "text" {
		h.writeSnapshotPlain(w, q, res, snapshotTextHeader(res.title, res.url, len(res.flat), res.scope)+counts, "",
			bridge.FormatSnapshotTextDiff(res.flat, added, changed, removed), wrap)
		return
	}
	h.writeSnapshotPlain(w, q, res, snapshotCompactHeader(res.title, res.url, len(res.flat), res.scope)+counts, "",
		bridge.FormatSnapshotCompactDiff(res.flat, added, changed, removed), wrap)
}

func (h *Handlers) writeSnapshotDiff(w http.ResponseWriter, q snapshotQuery, res snapshotResult, prevNodes []bridge.A11yNode, wrap bool) {
	switch q.format {
	case "compact", "text":
		h.writeSnapshotPlainDiff(w, q, res, prevNodes, wrap)
	default:
		data := snapshotDiffFields(res.attachRootFrame(res.scope.attach(withRoute(map[string]any{
			"url":   res.url,
			"title": res.title,
		}, res.route))), prevNodes, res)
		if q.format == "yaml" {
			writeSnapshotYAML(w, data, q.controls.Ignored)
			return
		}
		httpx.JSON(w, 200, attachIgnoredParams(data, q.controls.Ignored))
	}
}

func writeSnapshotYAML(w http.ResponseWriter, data map[string]any, ignored []string) {
	content, err := observe.MarshalYAMLInJSONShape(attachIgnoredParams(data, ignored))
	if err != nil {
		httpx.Error(w, 500, err)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write(content)
}

func (h *Handlers) writeSnapshot(w http.ResponseWriter, q snapshotQuery, res snapshotResult, vocabularyToken string, idpiResult snapshotIDPIResult) {
	switch q.format {
	case "compact":
		h.writeSnapshotPlain(w, q, res, snapshotCompactHeader(res.title, res.url, len(res.flat), res.scope), res.hint,
			bridge.FormatSnapshotCompact(res.flat), idpiResult.WrapContent)
	case "text":
		h.writeSnapshotPlain(w, q, res, snapshotTextHeader(res.title, res.url, len(res.flat), res.scope), res.hint,
			bridge.FormatSnapshotText(res.flat), idpiResult.WrapContent)
	default:
		resp := res.attachRootFrame(res.scope.attach(withRoute(map[string]any{
			"url":             res.url,
			"title":           res.title,
			"nodes":           res.wireNodes(res.flat),
			"count":           len(res.flat),
			"vocabularyToken": vocabularyToken,
		}, res.route)))
		if res.truncated {
			resp["truncated"] = true
			resp["maxTokens"] = q.controls.MaxTokens
		}
		if res.hint != "" {
			resp["hint"] = res.hint
		}
		if idpiResult.Threat {
			resp["idpiWarning"] = idpiResult.Reason
		}
		if idpiResult.WrapContent {
			resp["untrustedContent"] = true
			resp["idpiNotice"] = idpiNoticeText
		}
		if q.format == "yaml" {
			writeSnapshotYAML(w, resp, q.controls.Ignored)
			return
		}
		httpx.JSON(w, 200, attachIgnoredParams(resp, q.controls.Ignored))
	}
}
