package httpx

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSafePath covers the original table-driven cases that ship with the project.
// These use hard-coded Unix-style paths which are resolved via filepath.Abs on every OS.
func TestSafePath(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		path    string
		wantErr bool
	}{
		{"valid relative", "/tmp/state", "pdfs/out.pdf", false},
		{"absolute inside rejected", "/tmp/state", "/tmp/state/pdfs/out.pdf", true},
		{"traversal dotdot", "/tmp/state", "../etc/passwd", true},
		{"traversal absolute", "/tmp/state", "/etc/passwd", true},
		{"traversal hidden", "/tmp/state", "pdfs/../../etc/passwd", true},
		{"absolute base rejected", "/tmp/state", "/tmp/state", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafePath(tt.base, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafePath(%q, %q) error = %v, wantErr %v", tt.base, tt.path, err, tt.wantErr)
			}
		})
	}
}

// TestSafePath_TempDir uses real temp directories so paths are valid on every OS
// and never rely on hard-coded drive letters or mount points.
func TestSafePath_TempDir(t *testing.T) {
	// Create a real base directory with a nested child.
	base := t.TempDir()
	child := filepath.Join(base, "sub", "dir")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("relative inside base", func(t *testing.T) {
		got, err := SafePath(base, filepath.Join("sub", "dir"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != child {
			t.Errorf("got %q, want %q", got, child)
		}
	})

	t.Run("absolute inside base rejected", func(t *testing.T) {
		_, err := SafePath(base, child)
		if err == nil {
			t.Error("expected error for absolute path, got nil")
		}
	})

	t.Run("absolute base itself rejected", func(t *testing.T) {
		_, err := SafePath(base, base)
		if err == nil {
			t.Error("expected error for absolute path, got nil")
		}
	})

	t.Run("dotdot escapes base", func(t *testing.T) {
		_, err := SafePath(base, filepath.Join("..", "escape"))
		if err == nil {
			t.Error("expected error for dot-dot traversal, got nil")
		}
	})

	t.Run("deep dotdot escapes base", func(t *testing.T) {
		_, err := SafePath(base, filepath.Join("sub", "..", "..", "escape"))
		if err == nil {
			t.Error("expected error for deep dot-dot traversal, got nil")
		}
	})

	// Create a sibling directory to test cross-directory traversal.
	sibling := filepath.Join(filepath.Dir(base), "sibling")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("setup sibling: %v", err)
	}
	defer func() { _ = os.RemoveAll(sibling) }()

	t.Run("absolute sibling rejected", func(t *testing.T) {
		_, err := SafePath(base, sibling)
		if err == nil {
			t.Errorf("expected error for sibling path %q, got nil", sibling)
		}
	})
}

// TestSafePath_RootRelative is the core regression test for the Windows bug.
// A leading separator without a drive letter (e.g. "/etc/passwd") must always
// be rejected because it resolves to a root-relative path on the current drive.
func TestSafePath_RootRelative(t *testing.T) {
	base := t.TempDir()

	attacks := []string{
		"/etc/passwd",
		"/Windows/System32/config/SAM",
	}
	for _, p := range attacks {
		t.Run(p, func(t *testing.T) {
			_, err := SafePath(base, p)
			if err == nil {
				t.Errorf("SafePath(%q, %q) should reject root-relative path", base, p)
			}
		})
	}
}

// TestSafePath_Windows exercises Windows-only vectors that are meaningless on Unix.
func TestSafePath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test cases")
	}

	base := t.TempDir() // e.g. C:\Users\...\TestSafePath_Windows...

	tests := []struct {
		name string
		path string
	}{
		{"backslash root", `\Windows\System32\config\SAM`},
		{"other drive letter", `D:\secret\file.txt`},
		{"UNC path", `\\server\share\secret.txt`},
		{"slash root", `/Windows/System32`},
		{"mixed separators escape", `..\..\..\Windows\System32`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafePath(base, tt.path)
			if err == nil {
				t.Errorf("SafePath(%q, %q) should reject path outside base", base, tt.path)
			}
		})
	}
}

// TestSafePath_EmptyAndDot verifies edge cases that should resolve to the base itself.
func TestSafePath_EmptyAndDot(t *testing.T) {
	base := t.TempDir()

	for _, p := range []string{".", ""} {
		t.Run("path="+p, func(t *testing.T) {
			got, err := SafePath(base, p)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", p, err)
			}
			if got != base {
				t.Errorf("got %q, want %q", got, base)
			}
		})
	}
}

// TestSafePath_PrefixBoundary ensures that a path whose prefix textually
// matches the base but crosses a directory boundary is rejected.
// e.g. base="/tmp/state" must not allow "/tmp/stateEVIL/secret".
func TestSafePath_PrefixBoundary(t *testing.T) {
	parent := t.TempDir()

	base := filepath.Join(parent, "state")
	evil := filepath.Join(parent, "stateEVIL", "secret")

	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("setup base: %v", err)
	}
	if err := os.MkdirAll(evil, 0o755); err != nil {
		t.Fatalf("setup evil: %v", err)
	}

	_, err := SafePath(base, evil)
	if err == nil {
		t.Errorf("SafePath(%q, %q) should reject prefix-boundary attack", base, evil)
	}
}

// TestSafePath_NullByte verifies that null bytes embedded in paths are not
// used to bypass the check (common technique: "safe\x00/../../../etc/passwd").
func TestSafePath_NullByte(t *testing.T) {
	base := t.TempDir()

	attacks := []string{
		"safe\x00/../../../etc/passwd",
		"\x00/etc/passwd",
		"sub/\x00",
	}
	for _, p := range attacks {
		t.Run("", func(t *testing.T) {
			result, err := SafePath(base, p)
			// Either an error OR the result must still be inside base.
			if err == nil && !strings.HasPrefix(result, base) {
				t.Errorf("SafePath(%q, %q) = %q — escaped base!", base, p, result)
			}
		})
	}
}

// TestSafePath_RelativeNormalizesInside verifies that a relative path with
// dot-dot segments that ultimately lands inside the base is allowed.
func TestSafePath_RelativeNormalizesInside(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// "a/b/../b" normalizes to "a/b" — still inside base.
	got, err := SafePath(base, filepath.Join("a", "b", "..", "b"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sub {
		t.Errorf("got %q, want %q", got, sub)
	}
}

// TestSafePath_ReturnValueIsAbsolute ensures the returned path is always absolute.
func TestSafePath_ReturnValueIsAbsolute(t *testing.T) {
	base := t.TempDir()

	for _, p := range []string{".", "", "sub/file.txt"} {
		t.Run("path="+p, func(t *testing.T) {
			got, err := SafePath(base, p)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", p, err)
			}
			if !filepath.IsAbs(got) {
				t.Errorf("SafePath(%q, %q) returned non-absolute path %q", base, p, got)
			}
		})
	}

	// Absolute user paths are rejected, so the return value test doesn't apply.
	t.Run("absolute user path rejected", func(t *testing.T) {
		_, err := SafePath(base, base)
		if err == nil {
			t.Error("expected error for absolute user path")
		}
	})
}

func TestSafeExistingPath(t *testing.T) {
	base := t.TempDir()
	goodDir := filepath.Join(base, "uploads")
	if err := os.MkdirAll(goodDir, 0o755); err != nil {
		t.Fatalf("setup good dir: %v", err)
	}
	goodFile := filepath.Join(goodDir, "file.txt")
	if err := os.WriteFile(goodFile, []byte("ok"), 0o600); err != nil {
		t.Fatalf("write good file: %v", err)
	}

	got, err := SafeExistingPath(base, filepath.Join("uploads", "file.txt"))
	if err != nil {
		t.Fatalf("SafeExistingPath() unexpected error: %v", err)
	}
	if got != goodFile {
		t.Fatalf("SafeExistingPath() = %q, want %q", got, goodFile)
	}

	if _, err := SafeExistingPath(base, filepath.Join("uploads", "missing.txt")); err == nil {
		t.Fatal("expected missing existing path to fail")
	}
}

func TestSafePathRejectsSymlinkComponents(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()

	linkDir := filepath.Join(base, "linked")
	makeSymlinkOrSkip(t, outside, linkDir)

	if _, err := SafeCreatePath(base, filepath.Join("linked", "new.txt")); err == nil {
		t.Fatal("expected SafeCreatePath to reject symlinked parent")
	}
	if _, err := SafeExistingPath(base, filepath.Join("linked", "existing.txt")); err == nil {
		t.Fatal("expected SafeExistingPath to reject symlinked parent")
	}
	if _, err := SafePath(base, filepath.Join("linked", "compat.txt")); err == nil {
		t.Fatal("expected SafePath compatibility wrapper to reject symlinked parent")
	}
}

func TestSafeCreatePathRejectsSymlinkLeaf(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	linkPath := filepath.Join(base, "leaf.txt")
	makeSymlinkOrSkip(t, target, linkPath)

	if _, err := SafeCreatePath(base, "leaf.txt"); err == nil {
		t.Fatal("expected SafeCreatePath to reject symlink leaf")
	}
}

func makeSymlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported in test environment: %v", err)
	}
}
