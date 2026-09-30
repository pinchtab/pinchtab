package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"gopkg.in/yaml.v3"
)

func snapshotDiffFields(data map[string]any, prev, flat []bridge.A11yNode) map[string]any {
	added, changed, removed := bridge.DiffSnapshot(prev, flat)
	data["diff"] = true
	data["added"] = added
	data["changed"] = changed
	data["removed"] = removed
	data["counts"] = map[string]int{
		"added":   len(added),
		"changed": len(changed),
		"removed": len(removed),
		"total":   len(flat),
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

	data := res.scope.attach(map[string]any{
		"url":       res.url,
		"title":     res.title,
		"timestamp": now,
		"nodes":     res.flat,
		"count":     len(res.flat),
	})
	if q.doDiff && prevNodes != nil {
		snapshotDiffFields(data, prevNodes, res.flat)
	}
	if q.format == "yaml" {
		content, err := yaml.Marshal(data)
		if err != nil {
			return "", nil, fmt.Errorf("marshal yaml: %w", err)
		}
		return ".yaml", content, nil
	}
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	return ".json", content, nil
}

func (h *Handlers) writeSnapshotDiff(w http.ResponseWriter, q snapshotQuery, res snapshotResult, prevNodes []bridge.A11yNode, wrap bool) {
	if q.format == "compact" {
		added, changed, removed := bridge.DiffSnapshot(prevNodes, res.flat)
		header := fmt.Sprintf("%s | +%d ~%d -%d",
			snapshotCompactHeader(res.title, res.url, len(res.flat), res.scope), len(added), len(changed), len(removed))
		h.writeSnapshotPlain(w, q, res, header, "", bridge.FormatSnapshotCompactDiff(res.flat, added, changed, removed), wrap)
		return
	}

	data := res.scope.attach(map[string]any{
		"url":   res.url,
		"title": res.title,
		"route": res.route,
	})
	httpx.JSON(w, 200, attachIgnoredParams(snapshotDiffFields(data, prevNodes, res.flat), q.controls.Ignored))
}

func (h *Handlers) writeSnapshot(w http.ResponseWriter, q snapshotQuery, res snapshotResult, vocabularyToken string, idpiResult snapshotIDPIResult) {
	switch q.format {
	case "compact":
		h.writeSnapshotPlain(w, q, res, snapshotCompactHeader(res.title, res.url, len(res.flat), res.scope), res.hint,
			bridge.FormatSnapshotCompact(res.flat), idpiResult.WrapContent)
	case "text":
		h.writeSnapshotPlain(w, q, res, snapshotTextHeader(res.title, res.url, len(res.flat), res.scope), res.hint,
			bridge.FormatSnapshotText(res.flat), idpiResult.WrapContent)
	case "yaml":
		data := res.scope.attach(map[string]any{
			"url":   res.url,
			"title": res.title,
			"nodes": res.flat,
			"count": len(res.flat),
		})
		if res.hint != "" {
			data["hint"] = res.hint
		}
		attachIgnoredParams(data, q.controls.Ignored)
		content, err := yaml.Marshal(data)
		if err != nil {
			httpx.Error(w, 500, fmt.Errorf("marshal yaml: %w", err))
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.WriteHeader(200)
		_, _ = w.Write(content)
	default:
		resp := res.scope.attach(map[string]any{
			"url":             res.url,
			"title":           res.title,
			"route":           res.route,
			"nodes":           res.flat,
			"count":           len(res.flat),
			"vocabularyToken": vocabularyToken,
		})
		if res.truncated {
			resp["truncated"] = true
			resp["maxTokens"] = q.controls.MaxTokens
		}
		if res.hint != "" {
			resp["hint"] = res.hint
		}
		attachIgnoredParams(resp, q.controls.Ignored)
		if idpiResult.Threat {
			resp["idpiWarning"] = idpiResult.Reason
		}
		if idpiResult.WrapContent {
			resp["untrustedContent"] = true
			resp["idpiNotice"] = idpiNoticeText
		}
		httpx.JSON(w, 200, resp)
	}
}
