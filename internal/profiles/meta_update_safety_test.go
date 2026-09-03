package profiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// UpdateMeta is a partial update: it is handed one or two fields and must leave
// the rest of the record alone. It reads the sidecar to do that, and the reader
// it used answered "unreadable" and "absent" with the same zero value — so a
// profile.json truncated by a crash or half-written by an interrupted save turned
// the next description edit into permanent loss of the id and the useWhen text
// beside it, with no error to say so.
func TestUpdateMetaRefusesToRewriteAnUnreadableSidecar(t *testing.T) {
	base := t.TempDir()
	pm := NewProfileManager(base)
	if err := pm.Create("work"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	dir, err := pm.findProfileDirByName("work")
	if err != nil {
		t.Fatalf("findProfileDirByName: %v", err)
	}
	path := filepath.Join(dir, "profile.json")

	// A truncated write: valid JSON up to the point the process died.
	corrupt := []byte(`{"id":"work","name":"work","useWhen":"signed in as t`)
	if err := os.WriteFile(path, corrupt, 0644); err != nil {
		t.Fatalf("write corrupt sidecar: %v", err)
	}

	updateErr := pm.UpdateMeta("work", map[string]string{"description": "new"})
	if updateErr == nil {
		t.Fatal("UpdateMeta accepted an unreadable sidecar; it has just overwritten the fields it could not read")
	}
	if !strings.Contains(updateErr.Error(), "unreadable") {
		t.Errorf("error %q does not say the metadata could not be read, so an operator cannot tell this from a validation failure", updateErr)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	if string(after) != string(corrupt) {
		t.Errorf("the sidecar was rewritten to %q; a refused update must leave the file for recovery", after)
	}
}

// The absent case must stay a normal update rather than an error: a profile
// directory with no sidecar at all has nothing to lose, and refusing there would
// make the metadata uneditable for every profile created before it existed.
func TestUpdateMetaStillWritesWhenThereIsNoSidecarYet(t *testing.T) {
	base := t.TempDir()
	pm := NewProfileManager(base)
	if err := pm.Create("work"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	dir, err := pm.findProfileDirByName("work")
	if err != nil {
		t.Fatalf("findProfileDirByName: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "profile.json")); err != nil {
		t.Fatalf("remove sidecar: %v", err)
	}

	if err := pm.UpdateMeta("work", map[string]string{"description": "new"}); err != nil {
		t.Fatalf("UpdateMeta on a profile with no sidecar: %v", err)
	}
	if got := readProfileMeta(dir); got.Description != "new" || got.Name != "work" {
		t.Errorf("meta after update = %+v, want the description written and the name filled in", got)
	}
}

// A partial update must preserve the fields it was not asked to change.
func TestUpdateMetaPreservesTheFieldsItWasNotGiven(t *testing.T) {
	base := t.TempDir()
	pm := NewProfileManager(base)
	if err := pm.Create("work"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := pm.UpdateMeta("work", map[string]string{"useWhen": "signed in as test"}); err != nil {
		t.Fatalf("UpdateMeta useWhen: %v", err)
	}
	if err := pm.UpdateMeta("work", map[string]string{"description": "the work profile"}); err != nil {
		t.Fatalf("UpdateMeta description: %v", err)
	}

	dir, err := pm.findProfileDirByName("work")
	if err != nil {
		t.Fatalf("findProfileDirByName: %v", err)
	}
	got := readProfileMeta(dir)
	if got.UseWhen != "signed in as test" {
		t.Errorf("useWhen = %q after a description-only update, want it preserved", got.UseWhen)
	}
	if got.Description != "the work profile" {
		t.Errorf("description = %q, want the value just written", got.Description)
	}
	if got.ID == "" {
		t.Error("the profile lost its id across a metadata update")
	}
}

// The sidecar is replaced by rename, so a reader never sees a half-written file.
func TestWriteProfileMetaLeavesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	if err := writeProfileMeta(dir, ProfileMeta{ID: "a", Name: "a", UseWhen: "x"}); err != nil {
		t.Fatalf("writeProfileMeta: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("%s was left behind; the temp file must not survive a successful write", e.Name())
		}
	}
	if got := readProfileMeta(dir); got.UseWhen != "x" {
		t.Errorf("meta = %+v, want the written record", got)
	}
}
