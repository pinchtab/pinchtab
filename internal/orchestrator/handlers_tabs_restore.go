package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge/tabstate"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// handleInstanceTabsRestore is the smilerite-hardening F3 endpoint: it
// reopens fresh chromium tabs for every URL persisted in the instance's
// <profileDir>/tabs.json snapshot. Smiles calls this after she sees a
// `tab-not-found` error from a previous tabId (which happens when the
// daemon process or pod has been restarted since she last looked).
//
// Semantics:
//
//   - Does NOT dedupe against currently-open tabs. The instance may
//     already have some tabs from chromium's session-restore; this
//     endpoint adds tabs from the persistence file on top. Callers who
//     want a clean slate should close existing tabs first via GET
//     /instances/{id}/tabs + POST /tabs/{id}/close.
//
//   - If the persistence file is missing or empty, returns
//     200 {restored:[], skipped:[]}. Not an error — a fresh profile
//     just hasn't navigated anywhere yet.
//
//   - If a per-tab open fails, that URL goes into `skipped` with the
//     reason. Other tabs still proceed. The endpoint returns 200 unless
//     EVERY tab failed (or the instance itself is unreachable).
//
//   - Restore is sequential, not parallel. Pinchtab serializes per-tab
//     CDP work anyway, and bursting N concurrent /tab calls just queues
//     them behind each other inside the bridge.
//
// Route: POST /instances/{id}/tabs/restore
func (o *Orchestrator) handleInstanceTabsRestore(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	o.mu.RLock()
	inst, ok := o.instances[id]
	o.mu.RUnlock()
	if !ok {
		httpx.Error(w, 404, fmt.Errorf("instance %q not found", id))
		return
	}
	if inst.Status != "running" {
		httpx.Error(w, 503, fmt.Errorf("instance %q is not running (status: %s)", id, inst.Status))
		return
	}

	profilePath := o.profilePathFor(inst.ProfileName)
	if profilePath == "" {
		httpx.Error(w, 500, fmt.Errorf("instance %q has no resolvable profile path", id))
		return
	}

	snap, err := tabstate.Read(profilePath)
	if err != nil {
		// Read only returns errors for genuine I/O problems (missing file
		// is treated as empty). Surface as 500 — the caller can't recover.
		httpx.Error(w, 500, fmt.Errorf("read tabstate: %w", err))
		return
	}

	type restoredEntry struct {
		NewTabID    string    `json:"newTabId"`
		URL         string    `json:"url"`
		FromSavedAt time.Time `json:"fromSavedAt"`
	}
	type skippedEntry struct {
		URL    string `json:"url"`
		Reason string `json:"reason"`
	}

	restored := make([]restoredEntry, 0, len(snap.Tabs))
	skipped := make([]skippedEntry, 0)

	for _, tab := range snap.Tabs {
		newTabID, openErr := o.openTabOnInstance(r, inst, tab.URL)
		if openErr != nil {
			skipped = append(skipped, skippedEntry{URL: tab.URL, Reason: openErr.Error()})
			continue
		}
		restored = append(restored, restoredEntry{
			NewTabID:    newTabID,
			URL:         tab.URL,
			FromSavedAt: snap.SavedAt,
		})
	}

	// Edge case: persisted state had tabs but ALL of them failed to
	// reopen. Still return 200 with both arrays — the caller can inspect
	// `skipped` and decide what to do (Smiles will fail the run with a
	// clear message per her preflight rule).
	httpx.JSON(w, 200, map[string]any{
		"ok":         true,
		"restored":   restored,
		"skipped":    skipped,
		"instanceId": inst.ID,
		"profile":    inst.ProfileName,
		"savedAt":    snap.SavedAt,
	})
}

// profilePathFor mirrors the resolution used at instance-launch time
// (orchestrator.go: `profilePath := filepath.Join(o.baseDir, name); …
// profiles.ProfilePath(name)`). Returning the empty string means no path
// could be resolved.
func (o *Orchestrator) profilePathFor(profileName string) string {
	if profileName == "" {
		return ""
	}
	path := filepath.Join(o.baseDir, profileName)
	if o.profiles != nil {
		if resolved, err := o.profiles.ProfilePath(profileName); err == nil {
			path = resolved
		}
	}
	return path
}

// openTabOnInstance POSTs `{action:"new", url:url}` to the instance's
// bridge /tab endpoint and returns the new tabId from the response.
// This is the same shape handleInstanceTabOpen uses; we just do it
// from the orchestrator side because we need to drive N opens in a
// single request and stitch the responses together.
func (o *Orchestrator) openTabOnInstance(r *http.Request, inst *InstanceInternal, url string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"action": "new",
		"url":    url,
	})
	if err != nil {
		return "", fmt.Errorf("marshal open request: %w", err)
	}

	targetURL, err := o.instancePathURL(inst, "/tab", "")
	if err != nil {
		return "", fmt.Errorf("build target url: %w", err)
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL.String(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	o.applyInstanceAuth(req, inst)

	client := o.client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("instance unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Keep the error short; the upstream body may be verbose.
		preview := string(rb)
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		return "", fmt.Errorf("instance returned %d: %s", resp.StatusCode, preview)
	}

	var parsed struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(rb, &parsed); err != nil {
		return "", fmt.Errorf("parse instance response: %w", err)
	}
	if parsed.TabID == "" {
		return "", fmt.Errorf("instance response missing tabId")
	}
	return parsed.TabID, nil
}
