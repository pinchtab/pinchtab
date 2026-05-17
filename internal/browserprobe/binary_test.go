package browserprobe

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiscoverBinaryPrefersPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH executable stub requires unix shell permissions")
	}
	dir := t.TempDir()
	pathBinary := filepath.Join(dir, "fake-browser")
	if err := os.WriteFile(pathBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	fallback := filepath.Join(t.TempDir(), "fake-browser")
	if err := os.WriteFile(fallback, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	got := DiscoverBinary([]string{"fake-browser"}, []string{fallback})
	if got.Found != pathBinary {
		t.Fatalf("Found = %q, want %q", got.Found, pathBinary)
	}
}

func TestDiscoverBinaryReportsProbedPaths(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	fallback := filepath.Join(t.TempDir(), "missing-browser")

	got := DiscoverBinary([]string{"missing-browser"}, []string{fallback})
	if got.Found != "" {
		t.Fatalf("Found = %q, want empty", got.Found)
	}
	want := []string{"$PATH:missing-browser", fallback}
	if len(got.Probed) != len(want) {
		t.Fatalf("Probed = %v, want %v", got.Probed, want)
	}
	for i := range want {
		if got.Probed[i] != want[i] {
			t.Fatalf("Probed = %v, want %v", got.Probed, want)
		}
	}
}
