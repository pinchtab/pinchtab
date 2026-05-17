package handlers

import (
	"log/slog"

	"github.com/pinchtab/pinchtab/internal/bridge/tabstate"
)

// persistTabStateAfterNav snapshots the current list of open pages and writes
// it to <profileDir>/tabs.json. Called from the success path of HandleNavigate
// (both new-tab and existing-tab branches) so the on-disk state stays current
// after every URL change.
//
// This is the smilerite-hardening F3 hook (SMI-81 retro): if the daemon dies
// or the pod rolls, the persisted snapshot lets a caller reopen the same
// URLs against the surviving chromium profile via
// `POST /instances/{id}/tabs/restore`.
//
// Failures here are logged at warn and discarded — a tabstate write
// failure must not break the user-visible nav response. The next nav will
// try again.
func (h *Handlers) persistTabStateAfterNav() {
	if h == nil || h.Bridge == nil || h.Config == nil {
		return
	}
	profileDir := h.Config.ProfileDir
	if profileDir == "" {
		// Single-instance daemons launched without a per-instance profile
		// dir have nowhere safe to persist to. No-op.
		return
	}

	targets, err := h.Bridge.ListTargets()
	if err != nil {
		slog.Warn("tabstate: list targets failed; skipping persist",
			"err", err, "profileDir", profileDir)
		return
	}

	snap := tabstate.Snapshot{
		Tabs: make([]tabstate.PersistedTab, 0, len(targets)),
	}
	for _, t := range targets {
		if t == nil {
			continue
		}
		url := t.URL
		// Skip about:blank, chrome://, devtools:// — they're not useful
		// to restore. about:blank in particular is the placeholder used
		// while a real URL is loading; restoring it would create empty
		// tabs.
		if url == "" || url == "about:blank" {
			continue
		}
		if len(url) >= 9 && url[:9] == "chrome://" {
			continue
		}
		if len(url) >= 11 && url[:11] == "devtools://" {
			continue
		}
		snap.Tabs = append(snap.Tabs, tabstate.PersistedTab{
			ID:    string(t.TargetID),
			URL:   url,
			Title: t.Title,
		})
	}

	if err := tabstate.Persist(profileDir, snap); err != nil {
		slog.Warn("tabstate: persist failed; continuing",
			"err", err, "profileDir", profileDir, "tabs", len(snap.Tabs))
		return
	}
	slog.Debug("tabstate: persisted",
		"profileDir", profileDir, "tabs", len(snap.Tabs))
}
