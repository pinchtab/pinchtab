package tabstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPersist_RoundTrip writes a snapshot, reads it back, and confirms the
// fields make the trip. SavedAt is overwritten on Persist so we don't try to
// pin its value; only that it's set to ~now.
func TestPersist_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := Snapshot{
		InstanceID: "inst_42344f86",
		Tabs: []PersistedTab{
			{ID: "tab_a", URL: "https://secure.sarsefiling.co.za/TaxRefund/SOA", Title: "SOA"},
			{ID: "tab_b", URL: "https://enterprisests.standardbank.co.za/login"},
		},
	}

	before := time.Now().UTC().Add(-time.Second)
	if err := Persist(dir, in); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if got.InstanceID != in.InstanceID {
		t.Errorf("InstanceID: got %q want %q", got.InstanceID, in.InstanceID)
	}
	if len(got.Tabs) != len(in.Tabs) {
		t.Fatalf("Tabs: got %d entries, want %d", len(got.Tabs), len(in.Tabs))
	}
	for i, tab := range got.Tabs {
		if tab.URL != in.Tabs[i].URL {
			t.Errorf("Tabs[%d].URL: got %q want %q", i, tab.URL, in.Tabs[i].URL)
		}
		if tab.ID != in.Tabs[i].ID {
			t.Errorf("Tabs[%d].ID: got %q want %q", i, tab.ID, in.Tabs[i].ID)
		}
	}
	if got.SavedAt.Before(before) || got.SavedAt.After(after) {
		t.Errorf("SavedAt: got %v, want between %v and %v", got.SavedAt, before, after)
	}
}

// TestPersist_FilePermissions checks that the persisted file is 0600
// (per-instance secret-bearing, no need for group/world access).
func TestPersist_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	if err := Persist(dir, Snapshot{InstanceID: "x"}); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	info, err := os.Stat(Path(dir))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file perm: got %#o want 0600", perm)
	}
}

// TestPersist_NoTmpLeak verifies that no .tmp file is left behind after a
// successful Persist. (Rename is atomic; the tmp should be gone.)
func TestPersist_NoTmpLeak(t *testing.T) {
	dir := t.TempDir()
	if err := Persist(dir, Snapshot{InstanceID: "x"}); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName+".tmp")); !os.IsNotExist(err) {
		t.Errorf("tabs.json.tmp still present after rename: %v", err)
	}
}

// TestPersist_NilTabsBecomesEmptyArray ensures the JSON has an empty array,
// not a null, when there are no tabs. Easier for jq/dashboard to consume.
func TestPersist_NilTabsBecomesEmptyArray(t *testing.T) {
	dir := t.TempDir()
	if err := Persist(dir, Snapshot{InstanceID: "x", Tabs: nil}); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	body, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(body), `"tabs": []`) {
		t.Errorf("expected \"tabs\": [] in payload, got:\n%s", body)
	}
}

// TestPersist_EmptyProfileDir is an error.
func TestPersist_EmptyProfileDir(t *testing.T) {
	if err := Persist("", Snapshot{}); err == nil {
		t.Error("Persist(\"\"): expected error, got nil")
	}
}

// TestRead_MissingFile is not an error — it's just "nothing to restore".
func TestRead_MissingFile(t *testing.T) {
	dir := t.TempDir()
	snap, err := Read(dir)
	if err != nil {
		t.Fatalf("Read on empty dir: %v (want nil)", err)
	}
	if len(snap.Tabs) != 0 {
		t.Errorf("Read on empty dir: got %d tabs, want 0", len(snap.Tabs))
	}
}

// TestRead_MalformedFile is tolerated — empty snapshot, no error.
// Why: an interrupted Persist (eg SIGKILL between WriteFile and Rename)
// could leave us with a partially-written file in theory. We don't want
// every subsequent Read to fail-loud.
func TestRead_MalformedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(Path(dir), []byte("{this is not json"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	snap, err := Read(dir)
	if err != nil {
		t.Errorf("Read on malformed: %v (want nil)", err)
	}
	if len(snap.Tabs) != 0 {
		t.Errorf("Read on malformed: got %d tabs, want 0", len(snap.Tabs))
	}
}

// TestRead_NullTabsField — JSON null normalised to empty slice so callers
// can iterate without nil-guarding.
func TestRead_NullTabsField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(Path(dir), []byte(`{"instanceId":"x","tabs":null,"savedAt":"2026-01-01T00:00:00Z"}`), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	snap, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if snap.Tabs == nil {
		t.Error("expected non-nil tabs slice after Read of explicit null")
	}
}

// TestPersist_OverwritesPrevious confirms latest-wins. We don't keep history.
func TestPersist_OverwritesPrevious(t *testing.T) {
	dir := t.TempDir()
	if err := Persist(dir, Snapshot{Tabs: []PersistedTab{{ID: "a", URL: "https://a/"}}}); err != nil {
		t.Fatalf("first Persist: %v", err)
	}
	if err := Persist(dir, Snapshot{Tabs: []PersistedTab{{ID: "b", URL: "https://b/"}}}); err != nil {
		t.Fatalf("second Persist: %v", err)
	}
	snap, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(snap.Tabs) != 1 || snap.Tabs[0].URL != "https://b/" {
		t.Errorf("got %+v, want the second snapshot to win", snap.Tabs)
	}
}

// TestPath is trivial but it pins the on-disk filename so we don't
// accidentally change it without changing it intentionally.
func TestPath(t *testing.T) {
	got := Path("/profiles/foo")
	want := "/profiles/foo/tabs.json"
	if got != want {
		t.Errorf("Path: got %q want %q", got, want)
	}
}

// TestPersist_RoundTrip_JSONShape verifies the wire shape so dashboards
// and external tools can rely on these field names.
func TestPersist_RoundTrip_JSONShape(t *testing.T) {
	dir := t.TempDir()
	if err := Persist(dir, Snapshot{
		InstanceID: "inst_x",
		Tabs:       []PersistedTab{{ID: "t1", URL: "https://example/", Title: "T"}},
	}); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	body, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"instanceId", "savedAt", "tabs"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing top-level field %q in JSON", k)
		}
	}
	tabs, _ := got["tabs"].([]any)
	if len(tabs) != 1 {
		t.Fatalf("tabs len: got %d want 1", len(tabs))
	}
	tab, _ := tabs[0].(map[string]any)
	for _, k := range []string{"id", "url", "title"} {
		if _, ok := tab[k]; !ok {
			t.Errorf("missing tab field %q", k)
		}
	}
}
