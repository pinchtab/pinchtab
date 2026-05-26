package cloak

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProbeFontsViaFilesystemMatchesWindowsFontTokens(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Arial.ttf"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "segoeui.ttf"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	got := probeFontsViaFilesystem([]string{dir})
	want := []string{"arial", "segoe ui"}
	if len(got) != len(want) {
		t.Fatalf("matched = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("matched = %v, want %v", got, want)
		}
	}
}
