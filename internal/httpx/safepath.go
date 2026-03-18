package httpx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SafePath resolves a user-provided path against a base directory and ensures
// the result stays within that directory for file creation. Existing symlinked
// components are rejected. Returns the cleaned absolute path.
//
// The implementation uses the filepath.Abs + strings.HasPrefix pattern
// recommended by CodeQL (go/path-injection) and OWASP path-traversal guidance.
func SafePath(base, userPath string) (string, error) {
	return SafeCreatePath(base, userPath)
}

// SafeCreatePath resolves a user-provided path against a base directory and
// rejects any existing symlinked component in the resolved path. It is intended
// for paths that may be created later.
func SafeCreatePath(base, userPath string) (string, error) {
	absBase, absPath, err := safeLexicalPath(base, userPath)
	if err != nil {
		return "", err
	}
	if err := ensurePathHasNoSymlinkComponents(absBase, absPath, false); err != nil {
		return "", err
	}
	return absPath, nil
}

// SafeExistingPath resolves a user-provided path against a base directory,
// requires the full path to exist, and rejects any symlinked component in the
// resolved path. It is intended for opening existing files.
func SafeExistingPath(base, userPath string) (string, error) {
	absBase, absPath, err := safeLexicalPath(base, userPath)
	if err != nil {
		return "", err
	}
	if err := ensurePathHasNoSymlinkComponents(absBase, absPath, true); err != nil {
		return "", err
	}
	return absPath, nil
}

func safeLexicalPath(base, userPath string) (string, string, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", "", fmt.Errorf("invalid base path: %w", err)
	}

	// Empty or "." means the base directory itself.
	if userPath == "" || userPath == "." {
		return absBase, absBase, nil
	}

	// Reject absolute paths outright — user input must be relative to base.
	if filepath.IsAbs(userPath) || strings.HasPrefix(userPath, "/") || strings.HasPrefix(userPath, string(filepath.Separator)) {
		return "", "", fmt.Errorf("absolute paths not allowed: %q", userPath)
	}

	// Go 1.20+: reject paths with "..", device names (NUL, CON on Windows),
	// and other non-local components.
	if !filepath.IsLocal(userPath) {
		return "", "", fmt.Errorf("path %q contains invalid components", userPath)
	}

	// Join, clean, resolve to absolute.
	joined := filepath.Join(absBase, userPath)
	absPath, err := filepath.Abs(joined)
	if err != nil {
		return "", "", fmt.Errorf("invalid resolved path: %w", err)
	}

	// Final containment check — the resolved path must be under absBase.
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return "", "", fmt.Errorf("path %q escapes base directory %q", userPath, absBase)
	}

	return absBase, absPath, nil
}

func ensurePathHasNoSymlinkComponents(absBase, absPath string, requireFullExistence bool) error {
	if info, err := os.Lstat(absBase); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("base path %q must not be a symlink", absBase)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect base path %q: %w", absBase, err)
	} else if requireFullExistence && absPath == absBase {
		return fmt.Errorf("path does not exist: %q", absBase)
	}

	relPath, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return fmt.Errorf("compute relative path: %w", err)
	}
	if relPath == "." {
		return nil
	}

	current := absBase
	for _, part := range strings.Split(relPath, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) && !requireFullExistence {
				return nil
			}
			if os.IsNotExist(err) {
				return fmt.Errorf("path does not exist: %q", current)
			}
			return fmt.Errorf("inspect path %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path %q traverses symlink %q", absPath, current)
		}
	}
	return nil
}
