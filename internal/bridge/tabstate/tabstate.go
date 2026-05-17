// Package tabstate persists a pinchtab instance's open-tab list to disk so
// the set of URLs can be reopened after a daemon (or pod) restart. This is
// the smilerite-hardening F3 patch (SMI-81 retro, 2026-05-17): pod
// rollovers in Kubernetes destroy in-memory chromedp tab handles, but the
// chromium profile (cookies/storage) survives on a PersistentVolume.
// Pairing the surviving profile with a persisted URL list lets a caller
// reopen "where they were" after a restart without re-auth.
//
// Design notes:
//
//   - One file per pinchtab instance, lives at <profileDir>/tabs.json.
//     The profile dir is the chromium --user-data-dir for the instance,
//     so the persistence file rides on the same PV that already carries
//     the session.
//
//   - Writes are atomic via tabs.json.tmp + rename. A torn write would
//     either give us the old snapshot back or nothing; either is safe.
//
//   - Reads are tolerant: a missing file or malformed JSON returns an
//     empty Snapshot with no error. Restore must not break the caller
//     just because there was nothing to restore.
//
//   - No retention/versioning. tabs.json is overwritten on every save.
//     If we ever need history we can rotate; today's design is "latest
//     wins" because the caller (Smiles) only ever wants "what was open
//     last".
package tabstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// FileName is the on-disk name of the persistence file inside a profile dir.
// Exported so handlers/tests can refer to it without duplicating the literal.
const FileName = "tabs.json"

// PersistedTab is the minimal projection of a chromium tab that we save.
// Title is best-effort — if the tab hasn't loaded enough for chromium to
// have a title yet, it'll be empty; not an error.
type PersistedTab struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// Snapshot is what we serialize. InstanceID is informational — restore
// matches by profile dir, not by InstanceID, because the daemon picks a
// fresh InstanceID on every launch.
type Snapshot struct {
	InstanceID string         `json:"instanceId,omitempty"`
	SavedAt    time.Time      `json:"savedAt"`
	Tabs       []PersistedTab `json:"tabs"`
}

// Path returns the absolute path of the tabstate file for a given profile dir.
// Exported because handlers + tests both need to know where the file lives.
func Path(profileDir string) string {
	return filepath.Join(profileDir, FileName)
}

// Persist writes snap to <profileDir>/tabs.json atomically. The profile
// directory must already exist (it does — chromium would have failed to
// launch otherwise). Atomic via tmp+rename within the same dir so the
// rename is guaranteed by POSIX semantics to be atomic.
//
// SavedAt is overwritten with the current wall-clock time. Callers can
// leave it zero.
func Persist(profileDir string, snap Snapshot) error {
	if profileDir == "" {
		return errors.New("tabstate: empty profile dir")
	}

	snap.SavedAt = time.Now().UTC()
	if snap.Tabs == nil {
		// Always serialize a present-but-empty array, not a JSON null.
		// Easier for downstream tooling (jq, dashboard) to read.
		snap.Tabs = []PersistedTab{}
	}

	body, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("tabstate: marshal: %w", err)
	}

	dst := Path(profileDir)
	tmp := dst + ".tmp"

	// 0600 — the profile dir is per-instance and only the daemon should
	// read it. No reason for it to be group/world readable.
	if err := os.WriteFile(tmp, body, 0600); err != nil {
		return fmt.Errorf("tabstate: write tmp: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		// Best-effort cleanup of the orphan tmp file.
		_ = os.Remove(tmp)
		return fmt.Errorf("tabstate: rename: %w", err)
	}
	return nil
}

// Read returns the persisted snapshot for a profile dir. A missing file or
// a malformed file is treated as "nothing persisted yet" — the returned
// Snapshot will have an empty Tabs slice and no error.
//
// All other errors (permission denied, I/O failure on a present file) are
// surfaced. Callers should distinguish between "no state" (snap.Tabs
// length == 0) and "error reading state" by checking err.
func Read(profileDir string) (Snapshot, error) {
	if profileDir == "" {
		return Snapshot{Tabs: []PersistedTab{}}, errors.New("tabstate: empty profile dir")
	}

	body, err := os.ReadFile(Path(profileDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Snapshot{Tabs: []PersistedTab{}}, nil
		}
		return Snapshot{Tabs: []PersistedTab{}}, fmt.Errorf("tabstate: read: %w", err)
	}

	var snap Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		// Treat malformed-on-disk as "no state". This handles a partial
		// write from a process killed mid-Persist (vanishingly rare given
		// the atomic-rename strategy, but the renames-not-atomic edge
		// case exists on some filesystems and we'd rather not crash the
		// caller).
		return Snapshot{Tabs: []PersistedTab{}}, nil
	}
	if snap.Tabs == nil {
		snap.Tabs = []PersistedTab{}
	}
	return snap, nil
}
