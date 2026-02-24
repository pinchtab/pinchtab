package web

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafePath validates that the resolved path stays within the allowed base directory.
// Returns the cleaned absolute path or an error if traversal is detected.
func SafePath(base, userPath string) (string, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("invalid base path: %w", err)
	}

	var resolved string
	if filepath.IsAbs(userPath) {
		resolved = filepath.Clean(userPath)
	} else {
		resolved = filepath.Clean(filepath.Join(absBase, userPath))
	}

	// Ensure resolved path is within or equal to base
	if !strings.HasPrefix(resolved, absBase+string(filepath.Separator)) && resolved != absBase {
		return "", fmt.Errorf("path %q escapes base directory %q", userPath, absBase)
	}

	return resolved, nil
}
