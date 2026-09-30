package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureChromiumSkeletonSeedsPreferences(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureChromiumSkeleton(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "Default", "Preferences"))
	if err != nil || string(data) != "{}" {
		t.Fatalf("Preferences = %q (err=%v), want {}", data, err)
	}
}

func TestEnsureChromiumSkeletonPreservesExistingPreferences(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Default"), 0755); err != nil {
		t.Fatal(err)
	}
	prefs := filepath.Join(dir, "Default", "Preferences")
	if err := os.WriteFile(prefs, []byte(`{"real":"prefs"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureChromiumSkeleton(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(prefs)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"real":"prefs"}` {
		t.Fatalf("existing Preferences was overwritten: %s", data)
	}
}
